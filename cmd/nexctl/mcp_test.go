package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

type mcpStanza struct {
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"mcpServers"`
}

func runMCPConfig(t *testing.T, args ...string) (mcpStanza, string) {
	t.Helper()
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"mcp", "config"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp config: %v\n%s", err, errOut.String())
	}
	var cfg mcpStanza
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil {
		t.Fatalf("stdout is not pure JSON: %v\n%s", err, out.String())
	}
	return cfg, errOut.String()
}

func TestMCPConfigStanzas(t *testing.T) {
	seedProfile(t, "https://example.test/")
	cfg, stderr := runMCPConfig(t)

	s, ok := cfg.MCPServers["nexorious"]
	if !ok || s.Command != "nexctl" || !slices.Equal(s.Args, []string{"mcp", "serve"}) {
		t.Fatalf("stdio stanza = %+v", cfg)
	}
	h, ok := cfg.MCPServers["nexorious-http"]
	if !ok || h.Type != "http" || h.URL != "https://example.test/api/mcp" || h.Headers["Authorization"] != "Bearer k" {
		t.Fatalf("http stanza = %+v", h)
	}
	if !strings.Contains(stderr, "API key") {
		t.Fatalf("missing secret warning on stderr: %q", stderr)
	}
}

func TestMCPConfigStanzaProfile(t *testing.T) {
	seedProfile(t, "https://example.test")
	cfg, _ := runMCPConfig(t, "--profile", "default")
	want := []string{"mcp", "serve", "--profile", "default"}
	if s := cfg.MCPServers["nexorious"]; !slices.Equal(s.Args, want) {
		t.Fatalf("stanza args = %v, want %v", s.Args, want)
	}
}

// runPump feeds input through an mcpPump pointed at srvURL and returns stdout
// and stderr.
func runPump(t *testing.T, srvURL, input string) (string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	p := mcpPump{client: cliclient.New(srvURL), key: "k", profile: "default", out: &out, errOut: &errOut}
	if err := p.run(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("pump: %v", err)
	}
	return out.String(), errOut.String()
}

func stubMCP(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mcp" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json, text/event-stream" {
			t.Errorf("Accept = %q", got)
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

type rpcError struct {
	ID    json.RawMessage `json:"id"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeLines(t *testing.T, out string) []rpcError {
	t.Helper()
	var res []rpcError
	for line := range strings.Lines(out) {
		var e rpcError
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line is not JSON: %v: %q", err, line)
		}
		res = append(res, e)
	}
	return res
}

func TestMCPPumpRelaysInOrder(t *testing.T) {
	url := stubMCP(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m struct {
			ID int `json:"id"`
		}
		_ = json.Unmarshal(body, &m)
		w.Header().Set("Content-Type", "application/json")
		// Multi-line body: the pump must emit it as one line.
		_, _ = io.WriteString(w, "{\n  \"jsonrpc\": \"2.0\",\n  \"id\": "+strconv.Itoa(m.ID)+",\n  \"result\": {}\n}\n")
	})
	out, _ := runPump(t, url, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n\n"+`{"jsonrpc":"2.0","id":2,"method":"ping"}`+"\n")
	want := `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"result":{}}` + "\n"
	if out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestMCPPumpNotificationAccepted(t *testing.T) {
	url := stubMCP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	out, errOut := runPump(t, url, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	if out != "" || errOut != "" {
		t.Fatalf("stdout = %q, stderr = %q; want both empty", out, errOut)
	}
}

func TestMCPPumpUnwrapsSSE(t *testing.T) {
	url := stubMCP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"n\"}\n\n"+
			"id: 2\ndata: {\"jsonrpc\":\"2.0\",\ndata: \"id\":7,\"result\":{}}\n\n")
	})
	out, _ := runPump(t, url, `{"jsonrpc":"2.0","id":7,"method":"tools/list"}`+"\n")
	want := `{"jsonrpc":"2.0","method":"n"}` + "\n" + `{"jsonrpc":"2.0","id":7,"result":{}}` + "\n"
	if out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}

func TestMCPPumpHTTPFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"invalid api key"}`, `invalid or expired API key for profile "default"`},
		{"forbidden", http.StatusForbidden, `{"error":"cross-origin request refused"}`, "cross-origin request refused"},
		{"no endpoint 404", http.StatusNotFound, `{"message":"not found"}`, "no MCP endpoint"},
		{"no endpoint 405", http.StatusMethodNotAllowed, `{"message":"Method Not Allowed"}`, "no MCP endpoint"},
		{"server error", http.StatusBadGateway, ``, "502"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := stubMCP(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			out, _ := runPump(t, url, `{"jsonrpc":"2.0","id":"abc","method":"tools/list"}`+"\n")
			lines := decodeLines(t, out)
			if len(lines) != 1 {
				t.Fatalf("got %d lines: %q", len(lines), out)
			}
			e := lines[0]
			if string(e.ID) != `"abc"` || e.Error.Code != jsonrpcServerError || !strings.Contains(e.Error.Message, tc.want) {
				t.Fatalf("reply = %s %+v, want id \"abc\" and message containing %q", e.ID, e.Error, tc.want)
			}
		})
	}
}

func TestMCPPumpUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	out, _ := runPump(t, url, `{"jsonrpc":"2.0","id":3,"method":"ping"}`+"\n")
	lines := decodeLines(t, out)
	if len(lines) != 1 || string(lines[0].ID) != "3" || !strings.Contains(lines[0].Error.Message, "cannot reach") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestMCPPumpFailedNotificationGoesToStderr(t *testing.T) {
	url := stubMCP(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	out, errOut := runPump(t, url, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	if !strings.Contains(errOut, "invalid or expired API key") {
		t.Fatalf("stderr = %q", errOut)
	}
}

// TestNexctlDoesNotLinkMCPSDK guards #1262: the MCP SDK is a server-only
// dependency; nexctl's mcp serve is a protocol-agnostic pump.
func TestNexctlDoesNotLinkMCPSDK(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	out, err := exec.Command(gobin, "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for line := range strings.Lines(string(out)) {
		if strings.Contains(line, "modelcontextprotocol") || strings.Contains(line, "internal/mcptools") {
			t.Fatalf("nexctl links %s", strings.TrimSpace(line))
		}
	}
}
