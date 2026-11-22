package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// tiedInsertOrder is the order the tie-order tests insert rows 1..4 in. It is
// neither ascending nor descending, so a list whose ties fall back to heap
// (insertion) order matches neither sort direction the tests assert.
var tiedInsertOrder = []int{2, 4, 1, 3}

// listIDs GETs path (query included) and returns the "id" of every element of
// the response's listKey array, in order.
func listIDs(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path, token, listKey string) []string {
	t.Helper()
	rec := getAuth(t, handler, path, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", path, rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("GET %s: unmarshal: %v", path, err)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp[listKey], &rows); err != nil {
		t.Fatalf("GET %s: unmarshal %s: %v", path, listKey, err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// pagedIDs reads pages 1..n of path with per_page=1 and returns every id seen.
func pagedIDs(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path, token, listKey string, n int) []string {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var ids []string
	for page := 1; page <= n; page++ {
		ids = append(ids, listIDs(t, handler, fmt.Sprintf("%s%sper_page=1&page=%d", path, sep, page), token, listKey)...)
	}
	return ids
}
