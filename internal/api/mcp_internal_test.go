package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

func TestInProcessTransport(t *testing.T) {
	var gotReqID, gotURI, gotBody string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReqID = r.Header.Get(echo.HeaderXRequestID)
		gotURI = r.RequestURI
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	ctx := logging.WithRequestID(context.Background(), "rid-123")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inProcessBaseURL+"/api/tags?x=1", strings.NewReader(`{"name":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := inProcessTransport{h: h}.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotReqID != "rid-123" {
		t.Errorf("handler X-Request-Id = %q; want rid-123", gotReqID)
	}
	if gotURI != "/api/tags?x=1" {
		t.Errorf("handler RequestURI = %q; want /api/tags?x=1", gotURI)
	}
	if gotBody != `{"name":"a"}` {
		t.Errorf("handler body = %q", gotBody)
	}
	// A RoundTripper must not modify the caller's request.
	if req.Header.Get(echo.HeaderXRequestID) != "" || req.RequestURI != "" {
		t.Errorf("caller request mutated: header=%q uri=%q", req.Header.Get(echo.HeaderXRequestID), req.RequestURI)
	}
	if resp.StatusCode != http.StatusCreated || resp.Request != req {
		t.Errorf("response status=%d request-is-caller=%v", resp.StatusCode, resp.Request == req)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != `{"ok":true}` {
		t.Errorf("response body = %q", b)
	}

	// A cancelled ctx never reaches the handler.
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	gotBody = "untouched"
	req2, _ := http.NewRequestWithContext(cctx, http.MethodGet, inProcessBaseURL+"/api/tags", nil)
	if _, err := (inProcessTransport{h: h}).RoundTrip(req2); err == nil {
		t.Error("RoundTrip with cancelled ctx: want error")
	}
	if gotBody != "untouched" {
		t.Error("handler ran for a cancelled request")
	}
}
