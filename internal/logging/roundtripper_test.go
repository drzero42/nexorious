package logging

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoundTripper_LogsAndStripsQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	client := &http.Client{Transport: NewRoundTripper(http.DefaultTransport)}
	resp, err := client.Get(srv.URL + "/ISteamUser/GetPlayerSummaries/v2/?key=SECRET&steamids=1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	m := decode(t, &buf)
	if m[KeyStatus] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want 418", m[KeyStatus])
	}
	if got, ok := m[KeyEndpoint].(string); !ok || got != "/ISteamUser/GetPlayerSummaries/v2/" {
		t.Errorf("endpoint = %v, want path without query", m[KeyEndpoint])
	}
	if bytes.Contains(buf.Bytes(), []byte("SECRET")) {
		t.Errorf("log leaked query secret: %s", buf.String())
	}
	if _, ok := m[KeyDurationMS]; !ok {
		t.Errorf("missing duration_ms")
	}
}

func TestRoundTripper_LogsTransportError(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(NewContextHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(prev)

	client := &http.Client{Transport: NewRoundTripper(http.DefaultTransport)}
	// .invalid is a reserved TLD that never resolves.
	_, err := client.Get("http://nonexistent.invalid/x")
	if err == nil {
		t.Fatal("expected transport error")
	}
	m := decode(t, &buf)
	if m[KeyStatus] != float64(0) {
		t.Errorf("status = %v, want 0 on transport error", m[KeyStatus])
	}
	if _, ok := m[KeyErr]; !ok {
		t.Errorf("expected err attr on transport failure")
	}
}
