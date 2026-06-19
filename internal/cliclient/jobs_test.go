package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestListJobs(t *testing.T) {
	var gotQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{
				{
					"id":       "j-1",
					"job_type": "sync",
					"source":   "steam",
					"status":   "completed",
					"priority": "high",
					"progress": map[string]any{
						"pending": 0, "processing": 0, "completed": 3,
						"pending_review": 0, "skipped": 0, "failed": 0,
						"total": 3, "percent": 100,
					},
				},
			},
			"total": 1, "page": 1, "per_page": 20, "total_pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	params := url.Values{"status": {"completed"}}
	page, err := c.ListJobs("k", params)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(page.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(page.Jobs))
	}
	if page.Jobs[0].ID != "j-1" {
		t.Errorf("job id = %q, want j-1", page.Jobs[0].ID)
	}
	if page.Total != 1 || page.TotalPages != 1 {
		t.Errorf("pagination: total=%d total_pages=%d", page.Total, page.TotalPages)
	}
	if page.Jobs[0].Progress.Completed != 3 || page.Jobs[0].Progress.Percent != 100 {
		t.Errorf("progress = %+v", page.Jobs[0].Progress)
	}
	if gotQuery.Get("status") != "completed" {
		t.Errorf("status query param not forwarded: got %q", gotQuery.Get("status"))
	}
}

func TestListJobsEmptyParams(t *testing.T) {
	var gotRawQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jobs": []map[string]any{}, "total": 0, "page": 1, "per_page": 20, "total_pages": 0,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if _, err := c.ListJobs("k", url.Values{}); err != nil {
		t.Fatalf("ListJobs empty params: %v", err)
	}
	if gotRawQuery != "" {
		t.Errorf("expected no query string, got %q", gotRawQuery)
	}
}

func TestGetJob(t *testing.T) {
	dur := 12.5
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/j-42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":               "j-42",
			"job_type":         "import",
			"source":           "file",
			"status":           "completed",
			"priority":         "low",
			"total_items":      5,
			"auto_retry_done":  true,
			"is_terminal":      true,
			"duration_seconds": dur,
			"created_at":       "2026-06-01T10:00:00Z",
			"started_at":       "2026-06-01T10:00:01Z",
			"completed_at":     "2026-06-01T10:00:13Z",
			"progress": map[string]any{
				"pending": 0, "processing": 0, "completed": 5,
				"pending_review": 0, "skipped": 0, "failed": 0,
				"total": 5, "percent": 100,
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	job, err := c.GetJob("k", "j-42")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if job.ID != "j-42" {
		t.Errorf("id = %q, want j-42", job.ID)
	}
	if job.DurationSeconds == nil || *job.DurationSeconds != 12.5 {
		t.Errorf("duration_seconds = %v, want 12.5", job.DurationSeconds)
	}
	if !job.IsTerminal {
		t.Error("expected is_terminal true")
	}
	if !job.AutoRetryDone {
		t.Error("expected auto_retry_done true")
	}
	if job.Progress.Total != 5 || job.Progress.Percent != 100 {
		t.Errorf("progress = %+v", job.Progress)
	}
	if job.StartedAt == nil || *job.StartedAt != "2026-06-01T10:00:01Z" {
		t.Errorf("started_at = %v", job.StartedAt)
	}
	if job.CompletedAt == nil || *job.CompletedAt != "2026-06-01T10:00:13Z" {
		t.Errorf("completed_at = %v", job.CompletedAt)
	}
}

func TestGetJobNullableFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/j-9", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "j-9", "job_type": "sync", "source": "steam",
			"status": "pending", "priority": "high",
			"file_path": nil, "error_message": nil,
			"started_at": nil, "completed_at": nil, "duration_seconds": nil,
			"progress": map[string]any{
				"pending": 2, "processing": 0, "completed": 0,
				"pending_review": 0, "skipped": 0, "failed": 0,
				"total": 2, "percent": 0,
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	job, err := c.GetJob("k", "j-9")
	if err != nil {
		t.Fatalf("GetJob nullable: %v", err)
	}
	if job.FilePath != nil {
		t.Errorf("expected nil file_path, got %v", job.FilePath)
	}
	if job.DurationSeconds != nil {
		t.Errorf("expected nil duration_seconds, got %v", job.DurationSeconds)
	}
	if job.ErrorMessage != nil {
		t.Errorf("expected nil error_message, got %v", job.ErrorMessage)
	}
}

func TestGetJobItems(t *testing.T) {
	var gotQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/j-1/items", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":           "ji-1",
					"job_id":       "j-1",
					"item_key":     "ext-123",
					"source_title": "Half-Life",
					"status":       "completed",
					"created_at":   "2026-06-01T10:00:00Z",
				},
			},
			"total": 1, "page": 1, "per_page": 50, "total_pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	params := url.Values{"status": {"completed"}}
	page, err := c.GetJobItems("k", "j-1", params)
	if err != nil {
		t.Fatalf("GetJobItems: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].ID != "ji-1" {
		t.Errorf("item id = %q, want ji-1", page.Items[0].ID)
	}
	if page.Items[0].SourceTitle != "Half-Life" {
		t.Errorf("source_title = %q, want Half-Life", page.Items[0].SourceTitle)
	}
	if page.Total != 1 || page.TotalPages != 1 {
		t.Errorf("pagination: total=%d total_pages=%d", page.Total, page.TotalPages)
	}
	if gotQuery.Get("status") != "completed" {
		t.Errorf("status query param not forwarded: got %q", gotQuery.Get("status"))
	}
}

func TestRetryFailedJob(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/j-5/retry-failed", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":       true,
			"message":       "3 items re-enqueued",
			"retried_count": 3,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	result, err := c.RetryFailedJob("k", "j-5")
	if err != nil {
		t.Fatalf("RetryFailedJob: %v", err)
	}
	if !result.Success {
		t.Error("expected success true")
	}
	if result.RetriedCount != 3 {
		t.Errorf("retried_count = %d, want 3", result.RetriedCount)
	}
	if result.Message != "3 items re-enqueued" {
		t.Errorf("message = %q", result.Message)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/jobs/j-5/retry-failed" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestCancelJob(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs/j-7/cancel", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "cancelled"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.CancelJob("k", "j-7"); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/jobs/j-7/cancel" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestGetJobPathEscaping(t *testing.T) {
	var gotRequestURI string
	mux := http.NewServeMux()
	// r.RequestURI is the raw, un-decoded request target line as sent by the client.
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "id with spaces", "job_type": "sync", "source": "steam",
			"status": "pending", "priority": "high",
			"progress": map[string]any{
				"pending": 0, "processing": 0, "completed": 0,
				"pending_review": 0, "skipped": 0, "failed": 0,
				"total": 0, "percent": 0,
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if _, err := c.GetJob("k", "id with spaces"); err != nil {
		t.Fatalf("GetJob path escape: %v", err)
	}
	if gotRequestURI != "/api/jobs/id%20with%20spaces" {
		t.Errorf("request URI = %q, want /api/jobs/id%%20with%%20spaces", gotRequestURI)
	}
}
