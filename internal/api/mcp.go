package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/mcptools"
	"github.com/drzero42/nexorious/internal/observability"
)

// mcpPath is the MCP endpoint (Streamable HTTP, stateless, JSON responses).
const mcpPath = "/api/mcp"

// inProcessBaseURL is the base URL of the cliclient the MCP tools use. Its
// requests never touch the network (see inProcessTransport); the host only
// fills req.Host.
const inProcessBaseURL = "http://nexorious.in-process"

// maxLoggedToolName caps the client-chosen tool name on the log line.
const maxLoggedToolName = 64

// registerMCPRoutes mounts the MCP endpoint, or an explicit 404 when MCP is
// disabled. The explicit 404 matters: with no route, the SPA catch-all GET /*
// matches the path and Echo answers a POST with 405, and nexctl's stdio bridge
// relies on 404 to report "MCP is disabled".
func registerMCPRoutes(e *echo.Echo, db *bun.DB, enabled bool, corsOrigins []string, version string) {
	if !enabled {
		disabled := func(c *echo.Context) error {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "MCP is disabled on this server (MCP_ENABLED=false)"})
		}
		e.POST(mcpPath, disabled)
		e.GET(mcpPath, disabled)
		e.DELETE(mcpPath, disabled)
		return
	}

	// One server per key scope, chosen per request: the endpoint is stateless,
	// so tools/list for a read key must come from a server that never
	// registered the write tools.
	opts := &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// Only takes effect for clients on protocol >= 2026-07-28; older ones
		// keep running the tool after they hang up.
		PropagateRequestCancellation: true,
		// The SDK's loopback check 403s any request that reaches a loopback
		// listener with a non-loopback Host — i.e. every request through a
		// reverse proxy on the same host. DNS rebinding is instead stopped by
		// APIKeyMiddleware (a rebound page can't know the API key), with
		// mcpOriginCheck refusing cross-origin browser requests.
		DisableLocalhostProtection: true,
	}
	sdkFor := func(scope mcptools.Scope) http.Handler {
		srv := newMCPServer(e, version, scope)
		return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, opts)
	}
	sdkRead, sdkWrite := sdkFor(mcptools.ScopeRead), sdkFor(mcptools.ScopeWrite)

	h := func(c *echo.Context) error {
		req := c.Request()
		// APIKeyMiddleware has already validated the key; tools reuse it on
		// every REST sub-call so each one is re-authenticated and re-scoped.
		key := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		sdk := sdkRead
		if auth.ScopeFromContext(c) == auth.ScopeWrite {
			sdk = sdkWrite
		}
		sdk.ServeHTTP(c.Response(), req.WithContext(withMCPKey(req.Context(), key)))
		return nil
	}
	mws := []echo.MiddlewareFunc{mcpOriginCheck(corsOrigins), auth.APIKeyMiddleware(db)}
	// GET/DELETE reach the SDK so it answers 405, as the spec requires of a
	// server that offers no SSE stream; unrouted they would 404 via the SPA.
	e.POST(mcpPath, h, mws...)
	e.GET(mcpPath, h, mws...)
	e.DELETE(mcpPath, h, mws...)
}

// newMCPServer builds the MCP server for one key scope, whose tools call the
// REST API in-process through e, with the caller's key, and observes every
// tool call.
func newMCPServer(e http.Handler, version string, scope mcptools.Scope) *mcp.Server {
	c := cliclient.NewWithTransport(inProcessBaseURL, inProcessTransport{h: e})
	srv := mcptools.NewServer(c, mcpKeyFromContext, version, scope)
	srv.AddReceivingMiddleware(observeMCPToolCalls)
	return srv
}

type mcpKeyCtxKey struct{}

func withMCPKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, mcpKeyCtxKey{}, key)
}

// mcpKeyFromContext is the mcptools.KeyFunc for the server: the raw Bearer key
// of the /api/mcp request the tool call belongs to.
func mcpKeyFromContext(ctx context.Context) string {
	s, _ := ctx.Value(mcpKeyCtxKey{}).(string)
	return s
}

// mcpOriginCheck refuses cross-origin browser requests: no Origin → allow;
// Origin host equal to the request Host, or listed in CORS_ORIGINS → allow;
// otherwise 403. It compares hosts, not schemes, so it works behind a
// TLS-terminating proxy (server sees http, browser sends https://…).
func mcpOriginCheck(trusted []string) echo.MiddlewareFunc {
	cop := http.NewCrossOriginProtection()
	for _, o := range trusted {
		if err := cop.AddTrustedOrigin(o); err != nil {
			slog.Warn("mcp: ignoring CORS_ORIGINS entry for /api/mcp origin check", "origin", o, logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := cop.Check(c.Request()); err != nil {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "cross-origin request refused"})
			}
			return next(c)
		}
	}
}

// inProcessTransport is an http.RoundTripper that serves each request with the
// server's own handler instead of over TCP, so MCP sub-calls run the full
// middleware chain (state gates, auth, scope, access log).
type inProcessTransport struct{ h http.Handler }

func (t inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	// Clone: a RoundTripper must not modify the caller's request.
	r := req.Clone(req.Context())
	r.RequestURI = r.URL.RequestURI() // server-side field; the access log reads it
	// RequestIDMiddleware replaces the ctx request id with X-Request-Id (or a
	// fresh one), so forward it to keep sub-call log lines correlated.
	if id := logging.RequestID(r.Context()); id != "" {
		r.Header.Set(echo.HeaderXRequestID, id)
	}
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, r)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

// observeMCPToolCalls logs one line and records metrics per tools/call. A tool
// error reaches here as a result with IsError set (the SDK converts handler
// errors), so both that and a returned error count as outcome "error".
func observeMCPToolCalls(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		tool := ""
		if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && p != nil {
			tool = p.Name
		}
		start := time.Now()
		res, err := next(ctx, method, req)
		d := time.Since(start)

		outcome := "ok"
		// The metric label must stay bounded, but the name is client-chosen. A
		// call reaches a handler (err == nil) only for a registered tool; an
		// unknown tool or malformed params come back as a protocol error, so
		// those share one label.
		metricTool := tool
		if err != nil {
			outcome = "error"
			metricTool = "invalid"
		} else if r, ok := res.(*mcp.CallToolResult); ok && r != nil && r.IsError {
			outcome = "error"
		}
		if len(tool) > maxLoggedToolName {
			tool = tool[:maxLoggedToolName]
		}
		attrs := []any{logging.KeyTool, tool, logging.KeyOutcome, outcome, logging.KeyDurationMS, d.Milliseconds()}
		if err != nil {
			attrs = append(attrs, logging.KeyErr, err)
		}
		slog.InfoContext(ctx, "mcp tool call", attrs...)
		observability.RecordMCPToolCall(ctx, metricTool, outcome, d)
		return res, err
	}
}
