package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJobRetry(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/job-42/retry-failed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":       true,
			"message":       "retrying",
			"retried_count": 3,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "retry", "job-42"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job retry: %v\n%s", err, out.String())
	}
	if !called {
		t.Fatal("retry-failed endpoint not called")
	}
	s := out.String()
	if !strings.Contains(s, "retrying") {
		t.Fatalf("output missing message: %s", s)
	}
	if !strings.Contains(s, "3") {
		t.Fatalf("output missing count: %s", s)
	}
}

func TestJobCancelWithYesFlag(t *testing.T) {
	const id = "job-99"
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/"+id+"/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "cancelled"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "cancel", id, "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job cancel -y: %v\n%s", err, out.String())
	}
	if !called {
		t.Fatal("cancel endpoint not called")
	}
	if !strings.Contains(out.String(), "cancelled job "+id) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestJobCancelAbortOffTTY(t *testing.T) {
	const id = "job-77"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/"+id+"/cancel", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cancel endpoint must not be called when aborted")
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"job", "cancel", id})
	if err := root.Execute(); err != nil {
		t.Fatalf("job cancel (abort) returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Aborted.") {
		t.Fatalf("output = %s", out.String())
	}
}
