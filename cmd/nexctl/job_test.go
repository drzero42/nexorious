package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJobListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{{
				"id": "job-1", "job_type": "sync", "source": "steam", "status": "completed",
				"priority": "normal", "total_items": 42, "created_at": "2026-06-18T00:00:00Z",
			}},
			"total": 1, "page": 1, "per_page": 20, "total_pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job list: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "job-1") || !strings.Contains(out.String(), "steam") {
		t.Fatalf("table = %s", out.String())
	}
}

func TestJobListQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "completed" {
			t.Errorf("status = %q, want completed", q.Get("status"))
		}
		if q.Get("job_type") != "sync" {
			t.Errorf("job_type = %q, want sync", q.Get("job_type"))
		}
		if q.Get("source") != "steam" {
			t.Errorf("source = %q, want steam", q.Get("source"))
		}
		if q.Get("sort_by") != "status" {
			t.Errorf("sort_by = %q, want status", q.Get("sort_by"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]any{}, "total": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "list", "--status", "completed", "--type", "sync", "--source", "steam", "--sort", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job list filtered: %v", err)
	}
}

func TestJobListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs":  []map[string]any{{"id": "job-1"}, {"id": "job-2"}},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-q", "job", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job list -q: %v", err)
	}
	if strings.TrimSpace(out.String()) != "job-1\njob-2" {
		t.Fatalf("quiet = %q", out.String())
	}
}

func TestJobShowProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/job-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "job-1", "job_type": "import", "source": "vglist", "status": "processing",
			"priority": "normal", "total_items": 10, "created_at": "2026-06-18T00:00:00Z",
			"progress": map[string]any{
				"pending": 3, "processing": 1, "completed": 4, "pending_review": 2,
				"skipped": 0, "failed": 0, "total": 10, "percent": 40,
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "show", "job-1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job show: %v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "pending_review: 2") || !strings.Contains(s, "percent:        40") {
		t.Fatalf("progress not rendered: %s", s)
	}
}

func TestJobShowNilFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/job-2", func(w http.ResponseWriter, _ *http.Request) {
		// started_at / completed_at / duration_seconds / file_path / error_message all absent.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "job-2", "job_type": "sync", "source": "steam", "status": "pending",
			"priority": "normal", "total_items": 0, "created_at": "2026-06-18T00:00:00Z",
			"is_terminal": false, "auto_retry_done": false,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"job", "show", "job-2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("job show: %v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "started:    -") || !strings.Contains(s, "duration:   -") {
		t.Fatalf("nil fields not rendered as '-': %s", s)
	}
	if !strings.Contains(s, "terminal:   no") || !strings.Contains(s, "error:      none") {
		t.Fatalf("scalar fields not rendered: %s", s)
	}
}
