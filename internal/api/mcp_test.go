package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/logging"
)

const mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

// insertMCPTestKey inserts an API key with the given scope for userID and
// returns the raw key.
func insertMCPTestKey(t *testing.T, db *bun.DB, userID, scope string) string {
	t.Helper()
	raw, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO api_keys (id, user_id, name, key_hash, scopes) VALUES (gen_random_uuid()::text, ?, 'mcp-test', ?, ?)`,
		userID, auth.HashToken(raw), scope,
	); err != nil {
		t.Fatalf("insert api key: %v", err)
	}
	return raw
}

func mcpTestCfg(enabled bool, corsOrigins ...string) *config.Config {
	cfg := testCfg()
	cfg.MCPEnabled = enabled
	cfg.CORSOrigins = corsOrigins
	return cfg
}

// headerTransport adds fixed headers to every request.
type headerTransport struct {
	headers map[string]string
}

func (h headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connectMCP(t *testing.T, url string, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   url + "/api/mcp",
		HTTPClient: &http.Client{Transport: headerTransport{headers: headers}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// rawMCPPost sends one initialize request and returns the status code.
func rawMCPPost(t *testing.T, handler http.Handler, headers map[string]string) int {
	t.Helper()
	return rawMCPPostReq(t, handler, httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(mcpInitializeBody)), headers)
}

func rawMCPPostReq(t *testing.T, handler http.Handler, req *http.Request, headers map[string]string) int {
	t.Helper()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMCP_WriteKeyRoundTrip(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "write")
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO tags (id, user_id, name) VALUES (gen_random_uuid()::text, 'u-mcp', 'Seeded')`); err != nil {
		t.Fatalf("seed tag: %v", err)
	}

	logs := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(logs, nil))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv := httptest.NewServer(newTestEcho(t, testDB, mcpTestCfg(true)))
	t.Cleanup(srv.Close)
	const reqID = "mcp-test-request-id"
	session := connectMCP(t, srv.URL, map[string]string{
		"Authorization": "Bearer " + key,
		"X-Request-Id":  reqID,
	})
	ctx := context.Background()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"tag_list", "tag_create", "game_list"} {
		if !names[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}

	// Read tool: sees the seeded row through an in-process REST sub-call.
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "tag_list"})
	if err != nil || res.IsError {
		t.Fatalf("tag_list: err=%v result=%+v", err, res)
	}
	out, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(out), `"Seeded"`) {
		t.Errorf("tag_list output missing seeded tag: %s", out)
	}

	// Write tool: the row lands in the DB under the key's user.
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "tag_create", Arguments: map[string]any{"name": "FromMCP"}})
	if err != nil || res.IsError {
		t.Fatalf("tag_create: err=%v result=%+v", err, res)
	}
	var n int
	if err := testDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tags WHERE user_id = 'u-mcp' AND name = 'FromMCP'`).Scan(&n); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if n != 1 {
		t.Errorf("tag rows = %d; want 1", n)
	}

	// Correlation: the sub-call's access-log line carries the MCP request's
	// request_id, and the tool call itself is logged once with its outcome.
	var subCallLogged, toolLogged bool
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["msg"] == "request" && rec["route"] == "/api/tags" && rec["request_id"] == reqID {
			subCallLogged = true
		}
		if rec["msg"] == "mcp tool call" && rec["tool"] == "tag_create" && rec["outcome"] == "ok" && rec["request_id"] == reqID {
			toolLogged = true
		}
	}
	if !subCallLogged {
		t.Errorf("no /api/tags sub-call log line with request_id %q:\n%s", reqID, logs.String())
	}
	if !toolLogged {
		t.Errorf("no 'mcp tool call' line for tag_create with request_id %q:\n%s", reqID, logs.String())
	}
}

// A read-scoped key reaches /api/mcp (every MCP message is a POST), lists only
// the read tools, runs them, and is refused a write tool without the DB
// changing.
func TestMCP_ReadKeyScope(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "read")
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO tags (id, user_id, name) VALUES (gen_random_uuid()::text, 'u-mcp', 'Seeded')`); err != nil {
		t.Fatalf("seed tag: %v", err)
	}

	srv := httptest.NewServer(newTestEcho(t, testDB, mcpTestCfg(true)))
	t.Cleanup(srv.Close)
	session := connectMCP(t, srv.URL, map[string]string{"Authorization": "Bearer " + key})
	ctx := context.Background()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"tag_list", "game_list", "pool_list", "sync_status", "library_smells_list"} {
		if !names[want] {
			t.Errorf("read key tools/list missing read tool %q", want)
		}
	}
	for _, unwanted := range []string{"tag_create", "tag_rm", "game_add", "game_edit", "game_rm", "pool_create", "sync_run", "sync_reset", "library_smells_apply"} {
		if names[unwanted] {
			t.Errorf("read key tools/list includes write tool %q", unwanted)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "tag_list"})
	if err != nil || res.IsError {
		t.Fatalf("tag_list with read key: err=%v result=%+v", err, res)
	}
	out, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(out), `"Seeded"`) {
		t.Errorf("tag_list output missing seeded tag: %s", out)
	}

	for _, call := range []*mcp.CallToolParams{
		{Name: "tag_create", Arguments: map[string]any{"name": "FromReadKey"}},
		{Name: "tag_rm", Arguments: map[string]any{"ref": "Seeded"}},
	} {
		res, err := session.CallTool(ctx, call)
		if err != nil {
			t.Fatalf("%s: %v", call.Name, err)
		}
		if !res.IsError {
			t.Errorf("%s with read key: want tool error, got %+v", call.Name, res)
			continue
		}
		if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "write-scoped key") {
			t.Errorf("%s error = %q; want a hint to mint a write-scoped key", call.Name, text)
		}
	}
	var tagNames []string
	if err := testDB.NewSelect().Table("tags").Column("name").Where("user_id = 'u-mcp'").Scan(ctx, &tagNames); err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tagNames) != 1 || tagNames[0] != "Seeded" {
		t.Errorf("tags after refused writes = %v; want [Seeded]", tagNames)
	}
}

// A write key is served the full tool set, including every write tool.
func TestMCP_WriteKeyListsWriteTools(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "write")

	srv := httptest.NewServer(newTestEcho(t, testDB, mcpTestCfg(true)))
	t.Cleanup(srv.Close)
	session := connectMCP(t, srv.URL, map[string]string{"Authorization": "Bearer " + key})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"tag_list", "tag_create", "game_add", "game_rm", "pool_reorder", "sync_disconnect", "library_smells_apply"} {
		if !names[want] {
			t.Errorf("write key tools/list missing %q", want)
		}
	}
}

func TestMCP_SessionCookieWithoutKeyIs401(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	sessionID := insertAuthTestSession(t, testDB, "u-mcp")
	e := newTestEcho(t, testDB, mcpTestCfg(true))

	if got := rawMCPPost(t, e, map[string]string{"Cookie": "session=" + sessionID}); got != http.StatusUnauthorized {
		t.Errorf("cookie-only status = %d; want 401", got)
	}
}

func TestMCP_OriginCheck(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "write")
	e := newTestEcho(t, testDB, mcpTestCfg(true, "https://trusted.example"))
	bearer := "Bearer " + key

	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{"no origin", "", http.StatusOK},
		// httptest.NewRequest uses Host example.com; the scheme is ignored.
		{"same host", "https://example.com", http.StatusOK},
		{"listed in CORS_ORIGINS", "https://trusted.example", http.StatusOK},
		{"foreign origin", "https://evil.example", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := map[string]string{"Authorization": bearer}
			if tc.origin != "" {
				h["Origin"] = tc.origin
			}
			if got := rawMCPPost(t, e, h); got != tc.want {
				t.Errorf("status = %d; want %d", got, tc.want)
			}
		})
	}
}

// A browser MCP client on a CORS_ORIGINS origin sends Mcp-Protocol-Version,
// so its preflight must allow that header or the POST is never sent (#1278).
func TestMCP_CORSPreflightAllowsProtocolVersionHeader(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	e := newTestEcho(t, testDB, mcpTestCfg(true, "https://trusted.example"))

	req := httptest.NewRequest(http.MethodOptions, "/api/mcp", nil)
	req.Header.Set("Origin", "https://trusted.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type,mcp-protocol-version")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d; want 204 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://trusted.example" {
		t.Errorf("Access-Control-Allow-Origin = %q; want the trusted origin", got)
	}
	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowed, "mcp-protocol-version") {
		t.Errorf("Access-Control-Allow-Headers = %q; want it to include Mcp-Protocol-Version", allowed)
	}
}

// A reverse proxy on the same host connects to the loopback listener but
// forwards the public Host. The SDK's default DNS-rebinding check 403s exactly
// that; the endpoint must accept it.
func TestMCP_SameHostReverseProxyAllowed(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "write")
	e := newTestEcho(t, testDB, mcpTestCfg(true))

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(mcpInitializeBody))
	req.Host = "games.example.com"
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey,
		&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8000}))
	if got := rawMCPPostReq(t, e, req, map[string]string{"Authorization": "Bearer " + key}); got != http.StatusOK {
		t.Errorf("proxied request status = %d; want 200", got)
	}
}

// With MCP disabled the route is not registered: the SPA catch-all GET /*
// matches the path, so a POST gets Echo's generic 405 with nothing
// MCP-specific in the body (nexctl's bridge maps 404/405 to "no endpoint").
func TestMCP_DisabledIsUnregistered(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, mcpTestCfg(false))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(mcpInitializeBody))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/mcp with MCP disabled = %d; want 405", rec.Code)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "mcp") {
		t.Errorf("disabled response mentions MCP: %s", rec.Body.String())
	}
}

func TestMCP_GetIs405(t *testing.T) {
	truncateAllTables(t)
	insertAuthTestUser(t, testDB, "u-mcp", "mcpuser", "password123", true, false)
	key := insertMCPTestKey(t, testDB, "u-mcp", "write")
	e := newTestEcho(t, testDB, mcpTestCfg(true))

	req := httptest.NewRequest(http.MethodGet, "/api/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		body, _ := io.ReadAll(rec.Body)
		t.Errorf("GET /api/mcp = %d; want 405 (body %s)", rec.Code, body)
	}
}
