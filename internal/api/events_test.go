package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/notify"
)

func TestEventCursor_RoundTrip(t *testing.T) {
	occurred := time.Date(2026, 6, 2, 10, 30, 0, 123456789, time.UTC)
	id := "evt-abc"

	token := api.EncodeEventCursor(occurred, id)
	if token == "" {
		t.Fatal("expected non-empty cursor")
	}

	gotTime, gotID, err := api.DecodeEventCursor(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotTime.Equal(occurred) {
		t.Errorf("time: got %v want %v", gotTime, occurred)
	}
	if gotID != id {
		t.Errorf("id: got %q want %q", gotID, id)
	}
}

func TestEventCursor_Malformed(t *testing.T) {
	for _, tok := range []string{"not-base64!!", "", "bm90LWEtY3Vyc29y"} {
		if _, _, err := api.DecodeEventCursor(tok); err == nil {
			t.Errorf("expected error decoding %q, got nil", tok)
		}
	}
}

// insertEvent inserts one row into the events table. actorUserID may be nil
// (system/admin events). The referenced user must already exist when non-nil.
func insertEvent(t *testing.T, db *bun.DB, id, eventType, scope string, actorUserID *string, payload string, occurredAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, type, scope, actor_user_id, payload, occurred_at)
		 VALUES (?, ?, ?, ?, ?::jsonb, ?)`,
		id, eventType, scope, actorUserID, payload, occurredAt,
	)
	if err != nil {
		t.Fatalf("insertEvent: %v", err)
	}
}

type eventsListResp struct {
	Events []struct {
		ID            string          `json:"id"`
		Type          string          `json:"type"`
		Category      string          `json:"category"`
		Scope         string          `json:"scope"`
		OccurredAt    time.Time       `json:"occurred_at"`
		ActorUserID   *string         `json:"actor_user_id"`
		ActorUsername *string         `json:"actor_username"`
		Title         string          `json:"title"`
		Body          string          `json:"body"`
		Payload       json.RawMessage `json:"payload"`
	} `json:"events"`
	NextCursor *string `json:"next_cursor"`
}

func TestAdminEvents_KeysetPaging(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "events-paging")

	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		insertEvent(t, testDB,
			"evt-"+string(rune('a'+i)),
			notify.TypeSyncCompleted, notify.ScopeUser, &adminID,
			`{}`, base.Add(time.Duration(i)*time.Minute),
		)
	}

	// Page 1: newest 2.
	rec := getAuth(t, e, "/api/admin/events?limit=2", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page1 eventsListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &page1); err != nil {
		t.Fatalf("unmarshal page1: %v", err)
	}
	if len(page1.Events) != 2 {
		t.Fatalf("page1: expected 2 events, got %d", len(page1.Events))
	}
	if page1.Events[0].ID != "evt-e" || page1.Events[1].ID != "evt-d" {
		t.Errorf("page1 order wrong: %s, %s", page1.Events[0].ID, page1.Events[1].ID)
	}
	if page1.NextCursor == nil {
		t.Fatal("page1: expected non-nil next_cursor")
	}

	// Page 2: strictly older, no dupes.
	rec = getAuth(t, e, "/api/admin/events?limit=2&before="+*page1.NextCursor, adminTok)
	var page2 eventsListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &page2); err != nil {
		t.Fatalf("unmarshal page2: %v", err)
	}
	if len(page2.Events) != 2 || page2.Events[0].ID != "evt-c" || page2.Events[1].ID != "evt-b" {
		t.Fatalf("page2 wrong: %+v", page2.Events)
	}

	// Page 3: last row, next_cursor null.
	rec = getAuth(t, e, "/api/admin/events?limit=2&before="+*page2.NextCursor, adminTok)
	var page3 eventsListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &page3); err != nil {
		t.Fatalf("unmarshal page3: %v", err)
	}
	if len(page3.Events) != 1 || page3.Events[0].ID != "evt-a" {
		t.Fatalf("page3 wrong: %+v", page3.Events)
	}
	if page3.NextCursor != nil {
		t.Errorf("page3: expected nil next_cursor, got %v", *page3.NextCursor)
	}
}

func TestAdminEvents_NonAdminForbidden(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, regTok := setupRegularUser(t, testDB, e, "events-forbid")

	rec := getAuth(t, e, "/api/admin/events", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminEvents_Unauthenticated(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/events", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 unauthenticated, got %d", rec.Code)
	}
}

func TestAdminEvents_SystemEventNullActor(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "events-null")

	insertEvent(t, testDB, "evt-sys", notify.TypeAdminMaintCompleted, notify.ScopeAdmin, nil,
		`{}`, time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC))

	rec := getAuth(t, e, "/api/admin/events", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp eventsListResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Events))
	}
	if resp.Events[0].ActorUserID != nil || resp.Events[0].ActorUsername != nil {
		t.Errorf("expected null actor id and username, got id=%v name=%v",
			resp.Events[0].ActorUserID, resp.Events[0].ActorUsername)
	}
}

func TestAdminEvents_MalformedCursor(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	_, adminTok := setupAdminUser(t, testDB, e, "events-badcur")

	rec := getAuth(t, e, "/api/admin/events?before=not-a-cursor!!", adminTok)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed cursor, got %d", rec.Code)
	}
}

func TestAdminEvents_Filters(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	adminID, adminTok := setupAdminUser(t, testDB, e, "events-filters")
	insertAuthTestUser(t, testDB, "u-bob-filt", "bobfilt", "password123", true, false)
	bobID := "u-bob-filt"

	base := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	insertEvent(t, testDB, "evt-sync", notify.TypeSyncCompleted, notify.ScopeUser, &adminID, `{}`, base)
	insertEvent(t, testDB, "evt-imp", notify.TypeImportCompleted, notify.ScopeUser, &bobID, `{}`, base.Add(time.Minute))
	insertEvent(t, testDB, "evt-bkp", notify.TypeAdminBackupCompleted, notify.ScopeAdmin, nil, `{}`, base.Add(2*time.Minute))

	get := func(query string) eventsListResp {
		t.Helper()
		rec := getAuth(t, e, "/api/admin/events"+query, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("query %q: expected 200, got %d: %s", query, rec.Code, rec.Body.String())
		}
		var resp eventsListResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp
	}

	if r := get("?scope=admin"); len(r.Events) != 1 || r.Events[0].ID != "evt-bkp" {
		t.Errorf("scope filter wrong: %+v", r.Events)
	}
	if r := get("?type=" + notify.TypeImportCompleted); len(r.Events) != 1 || r.Events[0].ID != "evt-imp" {
		t.Errorf("type filter wrong: %+v", r.Events)
	}
	if r := get("?category=Backups"); len(r.Events) != 1 || r.Events[0].ID != "evt-bkp" {
		t.Errorf("category filter wrong: %+v", r.Events)
	}
	if r := get("?user=bobfilt"); len(r.Events) != 1 || r.Events[0].ID != "evt-imp" {
		t.Errorf("user-by-name filter wrong: %+v", r.Events)
	}
	if r := get("?user=" + adminID); len(r.Events) != 1 || r.Events[0].ID != "evt-sync" {
		t.Errorf("user-by-id filter wrong: %+v", r.Events)
	}
	since := base.Add(30 * time.Second).Format(time.RFC3339)
	until := base.Add(90 * time.Second).Format(time.RFC3339)
	if r := get("?since=" + since + "&until=" + until); len(r.Events) != 1 || r.Events[0].ID != "evt-imp" {
		t.Errorf("since/until filter wrong: %+v", r.Events)
	}
}
