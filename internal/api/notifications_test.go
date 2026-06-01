package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/notify"
)

// newNotifTestEncrypter builds a dedicated Encrypter for notification tests.
func newNotifTestEncrypter(t *testing.T) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter("test-key-test-key-test-key-test-")
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}
	return enc
}

// notifCtx builds an echo Context for a direct handler call with the given
// authenticated user, admin flag, JSON body, and :id param.
func notifCtx(t *testing.T, method, path string, body any, userID string, isAdmin bool, paramID string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	if userID != "" {
		c.Set("user_id", userID)
		c.Set("is_admin", isAdmin)
	}
	if paramID != "" {
		c.SetPathValues(echo.PathValues{{Name: "id", Value: paramID}})
	}
	return c, rec
}

// notifSubsList returns the current subscription event_types for userID by
// calling HandleListSubscriptions directly.
func notifSubsList(t *testing.T, h *api.NotificationsHandler, userID string) []string {
	t.Helper()
	c, rec := notifCtx(t, http.MethodGet, "/api/notifications/subscriptions", nil, userID, false, "")
	if err := h.HandleListSubscriptions(c); err != nil {
		t.Fatalf("HandleListSubscriptions: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("list subscriptions: expected 200, got %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		EventTypes []string `json:"event_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal subscriptions: %v", err)
	}
	return body.EventTypes
}

// ─── TestCreateChannelEncryptsURL ─────────────────────────────────────────────

func TestCreateChannelEncryptsURL(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	h := api.NewNotificationsHandler(testDB, enc, notify.NewRecorderSender())

	userID := "u-notif-enc"
	insertAuthTestUser(t, testDB, userID, "notif-enc", "pass123", true, false)

	const plainURL = "discord://token@channelid"
	c, rec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "My Discord", "url": plainURL}, userID, false, "")
	if err := h.HandleCreateChannel(c); err != nil {
		t.Fatalf("HandleCreateChannel: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body)
	}

	// Response must not expose the URL (encrypted or otherwise).
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["url"]; ok {
		t.Fatal("response must not contain a url field")
	}
	if _, ok := resp["encrypted_url"]; ok {
		t.Fatal("response must not contain an encrypted_url field")
	}
	if resp["name"] != "My Discord" {
		t.Fatalf("expected name=My Discord, got %v", resp["name"])
	}

	// The stored value must be encrypted, not plaintext.
	var stored string
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT encrypted_url FROM notification_channels WHERE user_id = ?`, userID,
	).Scan(&stored); err != nil {
		t.Fatalf("query encrypted_url: %v", err)
	}
	if stored == plainURL {
		t.Fatal("stored url must be encrypted, not plaintext")
	}
	if len(stored) < 7 || stored[:7] != "enc:v1:" {
		t.Fatalf("expected encrypted_url to start with enc:v1:, got %q", stored)
	}
	// And it must round-trip back to the plaintext.
	dec, err := enc.Decrypt(stored)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(dec) != plainURL {
		t.Fatalf("decrypt mismatch: got %q want %q", string(dec), plainURL)
	}
}

// ─── TestListChannelsHidesURL ─────────────────────────────────────────────────

func TestListChannelsHidesURL(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	h := api.NewNotificationsHandler(testDB, enc, notify.NewRecorderSender())

	userID := "u-notif-list"
	insertAuthTestUser(t, testDB, userID, "notif-list", "pass123", true, false)

	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "Chan A", "url": "slack://token"}, userID, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	if ccRec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", ccRec.Code, ccRec.Body)
	}

	c, rec := notifCtx(t, http.MethodGet, "/api/notifications/channels", nil, userID, false, "")
	if err := h.HandleListChannels(c); err != nil {
		t.Fatalf("list: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(items))
	}
	item := items[0]
	for _, k := range []string{"id", "name", "created_at"} {
		if _, ok := item[k]; !ok {
			t.Fatalf("expected field %q in list item, got %v", k, item)
		}
	}
	if _, ok := item["url"]; ok {
		t.Fatal("list item must not contain url")
	}
	if _, ok := item["encrypted_url"]; ok {
		t.Fatal("list item must not contain encrypted_url")
	}
}

// ─── TestCreateChannelRejectsBlankName ────────────────────────────────────────

func TestCreateChannelRejectsBlankName(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-blankname"
	insertAuthTestUser(t, testDB, userID, "notif-blankname", "pass123", true, false)

	c, _ := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "   ", "url": "slack://token"}, userID, false, "")
	err := h.HandleCreateChannel(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

// ─── TestCreateChannelRejectsBlankURL ─────────────────────────────────────────

func TestCreateChannelRejectsBlankURL(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-blankurl"
	insertAuthTestUser(t, testDB, userID, "notif-blankurl", "pass123", true, false)

	c, _ := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "Has Name", "url": ""}, userID, false, "")
	err := h.HandleCreateChannel(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

// ─── TestPutSubscriptionsRejectsAdminForNonAdmin ──────────────────────────────

func TestPutSubscriptionsRejectsAdminForNonAdmin(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-nonadmin"
	insertAuthTestUser(t, testDB, userID, "notif-nonadmin", "pass123", true, false)

	c, _ := notifCtx(t, http.MethodPut, "/api/notifications/subscriptions",
		map[string]any{"event_types": []string{notify.TypeAdminBackupFailed}}, userID, false, "")
	err := h.HandlePutSubscriptions(c)
	assertHTTPError(t, err, http.StatusBadRequest)

	// Nothing should have been persisted.
	if got := notifSubsList(t, h, userID); len(got) != 0 {
		t.Fatalf("expected no subscriptions after rejected PUT, got %v", got)
	}
}

// ─── TestPutSubscriptionsReplacesSet ──────────────────────────────────────────

func TestPutSubscriptionsReplacesSet(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-replace"
	insertAuthTestUser(t, testDB, userID, "notif-replace", "pass123", true, false)

	// First set.
	c1, rec1 := notifCtx(t, http.MethodPut, "/api/notifications/subscriptions",
		map[string]any{"event_types": []string{notify.TypeSyncFailed, notify.TypeImportFailed}}, userID, false, "")
	if err := h.HandlePutSubscriptions(c1); err != nil {
		t.Fatalf("first PUT: %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Fatalf("first PUT: expected 200, got %d: %s", rec1.Code, rec1.Body)
	}

	// Replace with a different set.
	c2, rec2 := notifCtx(t, http.MethodPut, "/api/notifications/subscriptions",
		map[string]any{"event_types": []string{notify.TypeImportFailed, notify.TypeExportFailed}}, userID, false, "")
	if err := h.HandlePutSubscriptions(c2); err != nil {
		t.Fatalf("second PUT: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("second PUT: expected 200, got %d: %s", rec2.Code, rec2.Body)
	}

	got := notifSubsList(t, h, userID)
	want := []string{notify.TypeExportFailed, notify.TypeImportFailed} // sorted by event_type
	assertStringSetEqual(t, got, want)
}

// ─── TestResetSubscriptionsRestoresDefaults ───────────────────────────────────

func TestResetSubscriptionsRestoresDefaults(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-reset"
	insertAuthTestUser(t, testDB, userID, "notif-reset", "pass123", true, false)

	// Subscribe to a custom (non-default) set.
	c1, _ := notifCtx(t, http.MethodPut, "/api/notifications/subscriptions",
		map[string]any{"event_types": []string{notify.TypeSyncCompleted, notify.TypeImportCompleted}}, userID, false, "")
	if err := h.HandlePutSubscriptions(c1); err != nil {
		t.Fatalf("PUT: %v", err)
	}

	// Reset.
	c2, rec2 := notifCtx(t, http.MethodPost, "/api/notifications/subscriptions/reset", nil, userID, false, "")
	if err := h.HandleResetSubscriptions(c2); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("reset: expected 200, got %d: %s", rec2.Code, rec2.Body)
	}

	got := notifSubsList(t, h, userID)
	// The reset context user is a non-admin, so admin-scoped defaults are skipped.
	var want []string
	for _, eventType := range notify.DefaultSubscriptions() {
		if !notify.IsAdminType(eventType) {
			want = append(want, eventType)
		}
	}
	assertStringSetEqual(t, got, want)
}

// ─── TestEventTypesHidesAdminFromNonAdmin ─────────────────────────────────────

func TestEventTypesHidesAdminFromNonAdmin(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-evt"
	insertAuthTestUser(t, testDB, userID, "notif-evt", "pass123", true, false)

	// Non-admin: no admin-scoped entries.
	cNon, recNon := notifCtx(t, http.MethodGet, "/api/notifications/event-types", nil, userID, false, "")
	if err := h.HandleListEventTypes(cNon); err != nil {
		t.Fatalf("event-types (non-admin): %v", err)
	}
	if recNon.Code != http.StatusOK {
		t.Fatalf("event-types (non-admin): expected 200, got %d", recNon.Code)
	}
	var nonAdmin []notify.EventTypeMeta
	if err := json.Unmarshal(recNon.Body.Bytes(), &nonAdmin); err != nil {
		t.Fatalf("unmarshal non-admin: %v", err)
	}
	for _, m := range nonAdmin {
		if m.Scope == notify.ScopeAdmin {
			t.Fatalf("non-admin response must not include admin-scoped type %q", m.Type)
		}
	}

	// Admin: must include at least one admin-scoped entry.
	cAdm, recAdm := notifCtx(t, http.MethodGet, "/api/notifications/event-types", nil, userID, true, "")
	if err := h.HandleListEventTypes(cAdm); err != nil {
		t.Fatalf("event-types (admin): %v", err)
	}
	var adminList []notify.EventTypeMeta
	if err := json.Unmarshal(recAdm.Body.Bytes(), &adminList); err != nil {
		t.Fatalf("unmarshal admin: %v", err)
	}
	if len(adminList) <= len(nonAdmin) {
		t.Fatalf("admin list (%d) must be larger than non-admin list (%d)", len(adminList), len(nonAdmin))
	}
	hasAdmin := false
	for _, m := range adminList {
		if m.Scope == notify.ScopeAdmin {
			hasAdmin = true
			break
		}
	}
	if !hasAdmin {
		t.Fatal("admin response must include at least one admin-scoped type")
	}
}

// ─── TestChannelOwnershipEnforced ─────────────────────────────────────────────

func TestChannelOwnershipEnforced(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	h := api.NewNotificationsHandler(testDB, enc, notify.NewRecorderSender())

	userA := "u-notif-owner-a"
	userB := "u-notif-owner-b"
	insertAuthTestUser(t, testDB, userA, "notif-owner-a", "pass123", true, false)
	insertAuthTestUser(t, testDB, userB, "notif-owner-b", "pass123", true, false)

	// User A creates a channel.
	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "A's channel", "url": "slack://atoken"}, userA, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(ccRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatal("expected non-empty channel id")
	}

	// User B tries to delete it -> 404.
	cDel, _ := notifCtx(t, http.MethodDelete, "/api/notifications/channels/"+channelID, nil, userB, false, channelID)
	err := h.HandleDeleteChannel(cDel)
	assertHTTPError(t, err, http.StatusNotFound)

	// The channel must still exist.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM notification_channels WHERE id = ?`, channelID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("channel must still exist after cross-user delete, count=%d", count)
	}
}

// ─── TestTestChannelSendsViaRecorder ──────────────────────────────────────────

func TestTestChannelSendsViaRecorder(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	rec := notify.NewRecorderSender()
	h := api.NewNotificationsHandler(testDB, enc, rec)

	userID := "u-notif-test"
	insertAuthTestUser(t, testDB, userID, "notif-test", "pass123", true, false)

	const plainURL = "telegram://token@telegram?chats=123"
	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "TestChan", "url": plainURL}, userID, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(ccRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatal("expected non-empty channel id")
	}

	cTest, testRec := notifCtx(t, http.MethodPost, "/api/notifications/channels/"+channelID+"/test", nil, userID, false, channelID)
	if err := h.HandleTestChannel(cTest); err != nil {
		t.Fatalf("test channel: %v", err)
	}
	if testRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", testRec.Code, testRec.Body)
	}

	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded send, got %d", len(sent))
	}
	if sent[0].URL != plainURL {
		t.Fatalf("recorder URL mismatch: got %q want %q", sent[0].URL, plainURL)
	}
}

// ─── TestTestURLSendsViaRecorder ──────────────────────────────────────────────

func TestTestURLSendsViaRecorder(t *testing.T) {
	rec := notify.NewRecorderSender()
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), rec)

	userID := "u-notif-testurl"
	// No DB setup needed — HandleTestURL doesn't touch the DB.
	c, testRec := notifCtx(t, http.MethodPost, "/api/notifications/test",
		map[string]any{"url": "noop://x"}, userID, false, "")
	if err := h.HandleTestURL(c); err != nil {
		t.Fatalf("HandleTestURL: %v", err)
	}
	if testRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", testRec.Code, testRec.Body)
	}
	sent := rec.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded send, got %d", len(sent))
	}
	if sent[0].URL != "noop://x" {
		t.Fatalf("recorder URL mismatch: got %q want %q", sent[0].URL, "noop://x")
	}
}

// ─── TestTestURLRejectsBlankURL ───────────────────────────────────────────────

func TestTestURLRejectsBlankURL(t *testing.T) {
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-testurl-blank"
	c, _ := notifCtx(t, http.MethodPost, "/api/notifications/test",
		map[string]any{"url": ""}, userID, false, "")
	err := h.HandleTestURL(c)
	assertHTTPError(t, err, http.StatusBadRequest)
}

// ─── TestTestURLReturns502OnSendError ─────────────────────────────────────────

func TestTestURLReturns502OnSendError(t *testing.T) {
	failRec := notify.NewRecorderSender()
	failRec.Err = errors.New("smtp down")
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), failRec)

	userID := "u-notif-testurl-502"
	c, _ := notifCtx(t, http.MethodPost, "/api/notifications/test",
		map[string]any{"url": "noop://x"}, userID, false, "")
	err := h.HandleTestURL(c)
	assertHTTPError(t, err, http.StatusBadGateway)
}

// ─── shared assert helpers ────────────────────────────────────────────────────

// assertHTTPError asserts err is an *echo.HTTPError with the expected status.
func assertHTTPError(t *testing.T, err error, wantCode int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected *echo.HTTPError with code %d, got nil", wantCode)
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != wantCode {
		t.Fatalf("expected code %d, got %d (%v)", wantCode, he.Code, he.Message)
	}
}

// assertStringSetEqual asserts two string slices contain the same elements
// (order-independent).
func assertStringSetEqual(t *testing.T, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if len(g) != len(w) {
		t.Fatalf("set size mismatch: got %v want %v", got, want)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("set mismatch: got %v want %v", got, want)
		}
	}
}

// ─── TestUpdateChannelOwnershipEnforced ───────────────────────────────────────

func TestUpdateChannelOwnershipEnforced(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	h := api.NewNotificationsHandler(testDB, enc, notify.NewRecorderSender())

	userA := "u-notif-upd-owner-a"
	userB := "u-notif-upd-owner-b"
	insertAuthTestUser(t, testDB, userA, "notif-upd-owner-a", "pass123", true, false)
	insertAuthTestUser(t, testDB, userB, "notif-upd-owner-b", "pass123", true, false)

	// User A creates a channel.
	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "A original name", "url": "slack://atoken"}, userA, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(ccRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatal("expected non-empty channel id")
	}

	// User B tries to rename A's channel → 404.
	newName := "B renamed it"
	cUpd, _ := notifCtx(t, http.MethodPatch, "/api/notifications/channels/"+channelID,
		map[string]any{"name": newName}, userB, false, channelID)
	err := h.HandleUpdateChannel(cUpd)
	assertHTTPError(t, err, http.StatusNotFound)

	// A's channel name must be unchanged in the DB.
	var storedName string
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT name FROM notification_channels WHERE id = ?`, channelID,
	).Scan(&storedName); err != nil {
		t.Fatalf("query name: %v", err)
	}
	if storedName != "A original name" {
		t.Fatalf("channel name must be unchanged, got %q", storedName)
	}
}

// ─── TestTestChannelOwnershipEnforced ─────────────────────────────────────────

func TestTestChannelOwnershipEnforced(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	rec := notify.NewRecorderSender()
	h := api.NewNotificationsHandler(testDB, enc, rec)

	userA := "u-notif-tst-owner-a"
	userB := "u-notif-tst-owner-b"
	insertAuthTestUser(t, testDB, userA, "notif-tst-owner-a", "pass123", true, false)
	insertAuthTestUser(t, testDB, userB, "notif-tst-owner-b", "pass123", true, false)

	// User A creates a channel.
	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "A's chan", "url": "slack://atoken"}, userA, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(ccRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatal("expected non-empty channel id")
	}

	// User B tries to test A's channel → 404.
	cTest, _ := notifCtx(t, http.MethodPost, "/api/notifications/channels/"+channelID+"/test",
		nil, userB, false, channelID)
	err := h.HandleTestChannel(cTest)
	assertHTTPError(t, err, http.StatusNotFound)

	// No sends must have been recorded.
	if got := len(rec.Sent()); got != 0 {
		t.Fatalf("expected 0 sends, got %d", got)
	}
}

// ─── TestPutSubscriptionsRejectsUnknownType ───────────────────────────────────

func TestPutSubscriptionsRejectsUnknownType(t *testing.T) {
	truncateAllTables(t)
	h := api.NewNotificationsHandler(testDB, newNotifTestEncrypter(t), notify.NewRecorderSender())

	userID := "u-notif-unk-type"
	insertAuthTestUser(t, testDB, userID, "notif-unk-type", "pass123", true, false)

	c, _ := notifCtx(t, http.MethodPut, "/api/notifications/subscriptions",
		map[string]any{"event_types": []string{"totally.bogus.type"}}, userID, false, "")
	err := h.HandlePutSubscriptions(c)
	assertHTTPError(t, err, http.StatusBadRequest)

	// No subscription rows must have been written for this user.
	var count int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM notification_subscriptions WHERE user_id = ?`, userID,
	).Scan(&count); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 subscription rows, got %d", count)
	}
}

// ─── TestTestChannelReturns502OnSendError ─────────────────────────────────────

func TestTestChannelReturns502OnSendError(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	failRec := notify.NewRecorderSender()
	failRec.Err = errors.New("smtp down")
	h := api.NewNotificationsHandler(testDB, enc, failRec)

	userID := "u-notif-502"
	insertAuthTestUser(t, testDB, userID, "notif-502", "pass123", true, false)

	// Create a channel for the user.
	cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels",
		map[string]any{"name": "Failing chan", "url": "slack://failtoken"}, userID, false, "")
	if err := h.HandleCreateChannel(cc); err != nil {
		t.Fatalf("create: %v", err)
	}
	var created map[string]any
	if err := json.Unmarshal(ccRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" {
		t.Fatal("expected non-empty channel id")
	}

	// Test the channel → sender fails → expect 502.
	cTest, _ := notifCtx(t, http.MethodPost, "/api/notifications/channels/"+channelID+"/test",
		nil, userID, false, channelID)
	err := h.HandleTestChannel(cTest)
	assertHTTPError(t, err, http.StatusBadGateway)
}

// ─── TestListChannelsIsolatedPerUser ──────────────────────────────────────────

func TestListChannelsIsolatedPerUser(t *testing.T) {
	truncateAllTables(t)
	enc := newNotifTestEncrypter(t)
	h := api.NewNotificationsHandler(testDB, enc, notify.NewRecorderSender())

	userA := "u-notif-iso-a"
	userB := "u-notif-iso-b"
	insertAuthTestUser(t, testDB, userA, "notif-iso-a", "pass123", true, false)
	insertAuthTestUser(t, testDB, userB, "notif-iso-b", "pass123", true, false)

	// User A creates 2 channels.
	for _, ch := range []map[string]any{
		{"name": "Chan One", "url": "slack://token1"},
		{"name": "Chan Two", "url": "slack://token2"},
	} {
		cc, ccRec := notifCtx(t, http.MethodPost, "/api/notifications/channels", ch, userA, false, "")
		if err := h.HandleCreateChannel(cc); err != nil {
			t.Fatalf("create channel %q: %v", ch["name"], err)
		}
		if ccRec.Code != http.StatusCreated {
			t.Fatalf("create channel %q: expected 201, got %d: %s", ch["name"], ccRec.Code, ccRec.Body)
		}
	}

	// User B (no channels) GETs /channels → empty list.
	cB, recB := notifCtx(t, http.MethodGet, "/api/notifications/channels", nil, userB, false, "")
	if err := h.HandleListChannels(cB); err != nil {
		t.Fatalf("list (user B): %v", err)
	}
	if recB.Code != http.StatusOK {
		t.Fatalf("list (user B): expected 200, got %d: %s", recB.Code, recB.Body)
	}
	var itemsB []map[string]any
	if err := json.Unmarshal(recB.Body.Bytes(), &itemsB); err != nil {
		t.Fatalf("unmarshal (user B): %v", err)
	}
	if len(itemsB) != 0 {
		t.Fatalf("user B must see 0 channels, got %d", len(itemsB))
	}

	// User A GETs /channels → 2 items.
	cA, recA := notifCtx(t, http.MethodGet, "/api/notifications/channels", nil, userA, false, "")
	if err := h.HandleListChannels(cA); err != nil {
		t.Fatalf("list (user A): %v", err)
	}
	if recA.Code != http.StatusOK {
		t.Fatalf("list (user A): expected 200, got %d: %s", recA.Code, recA.Body)
	}
	var itemsA []map[string]any
	if err := json.Unmarshal(recA.Body.Bytes(), &itemsA); err != nil {
		t.Fatalf("unmarshal (user A): %v", err)
	}
	if len(itemsA) != 2 {
		t.Fatalf("user A must see 2 channels, got %d", len(itemsA))
	}
}
