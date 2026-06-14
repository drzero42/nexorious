package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListImportSources(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)
	_, token := setupTagUser(t, testDB, e, "import-sources")

	req := httptest.NewRequest(http.MethodGet, "/api/import/sources", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, s := range got {
		if s["slug"] == "darkadia" {
			found = true
			if s["display_name"] != "Darkadia" {
				t.Errorf("display_name = %v, want Darkadia", s["display_name"])
			}
			if acc, ok := s["accept"].([]any); !ok || len(acc) == 0 {
				t.Errorf("accept = %v, want non-empty list", s["accept"])
			}
		}
	}
	if !found {
		t.Error("darkadia missing from /import/sources")
	}
}
