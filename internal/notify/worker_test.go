package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/crypto"
)

func insertWorkerUser(t *testing.T, username string, admin bool) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES (?, ?, 'x', ?)`,
		id, username, admin,
	).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertWorkerChannel(t *testing.T, userID, name, url string, enc *crypto.Encrypter) {
	t.Helper()
	ct, err := enc.Encrypt([]byte(url))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO notification_channels (id, user_id, name, encrypted_url, created_at) VALUES (?, ?, ?, ?, now())`,
		uuid.NewString(), userID, name, ct,
	).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func subscribeWorker(t *testing.T, userID, eventType string) {
	t.Helper()
	if _, err := testDB.NewRaw(
		`INSERT INTO notification_subscriptions (user_id, event_type, created_at) VALUES (?, ?, now())`,
		userID, eventType,
	).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func insertWorkerEvent(t *testing.T, typ, scope string, actor *string, payload string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO events (id, type, scope, actor_user_id, payload, occurred_at) VALUES (?, ?, ?, ?, ?::jsonb, ?)`,
		id, typ, scope, actor, payload, time.Now().UTC(),
	).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	return id
}

func workerEncrypter(t *testing.T) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter("test-key-test-key-test-key-test-key")
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestNotifyWorkerUserScopeDelivers(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := workerEncrypter(t)
	rec := NewRecorderSender()

	uid := insertWorkerUser(t, "alice", false)
	insertWorkerChannel(t, uid, "phone", "noop://alice", enc)
	subscribeWorker(t, uid, TypeSyncFailed)
	eventID := insertWorkerEvent(t, TypeSyncFailed, ScopeUser, &uid, `{"storefront":"steam"}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(rec.Sent()))
	}
	if got := rec.Sent()[0].URL; got != "noop://alice" {
		t.Errorf("expected delivery to noop://alice, got %q", got)
	}
}

func TestNotifyWorkerUserNotSubscribedNoDelivery(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := workerEncrypter(t)
	rec := NewRecorderSender()

	uid := insertWorkerUser(t, "bob", false)
	insertWorkerChannel(t, uid, "phone", "noop://bob", enc)
	eventID := insertWorkerEvent(t, TypeSyncFailed, ScopeUser, &uid, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 0 {
		t.Fatalf("expected 0 deliveries for unsubscribed user, got %d", len(rec.Sent()))
	}
}

func TestNotifyWorkerAdminScopeFansOut(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := workerEncrypter(t)
	rec := NewRecorderSender()

	a1 := insertWorkerUser(t, "admin1", true)
	a2 := insertWorkerUser(t, "admin2", true)
	nonAdmin := insertWorkerUser(t, "user1", false)
	insertWorkerChannel(t, a1, "c", "noop://a1", enc)
	insertWorkerChannel(t, a2, "c", "noop://a2", enc)
	insertWorkerChannel(t, nonAdmin, "c", "noop://u1", enc)
	subscribeWorker(t, a1, TypeAdminBackupFailed)
	subscribeWorker(t, a2, TypeAdminBackupFailed)
	subscribeWorker(t, nonAdmin, TypeAdminBackupFailed) // must be ignored: non-admin

	eventID := insertWorkerEvent(t, TypeAdminBackupFailed, ScopeAdmin, nil, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Sent()) != 2 {
		t.Fatalf("expected 2 admin deliveries (non-admin excluded), got %d", len(rec.Sent()))
	}
}

func TestNotifyWorkerSendFailureSwallowed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	enc := workerEncrypter(t)
	rec := NewRecorderSender()
	rec.Err = errors.New("boom")

	uid := insertWorkerUser(t, "carol", false)
	insertWorkerChannel(t, uid, "phone", "noop://carol", enc)
	subscribeWorker(t, uid, TypeSyncFailed)
	eventID := insertWorkerEvent(t, TypeSyncFailed, ScopeUser, &uid, `{}`)

	w := &NotifyWorker{DB: testDB, Encrypter: enc, Sender: rec}
	if err := w.Work(ctx, &river.Job[NotifyArgs]{Args: NotifyArgs{EventID: eventID}}); err != nil {
		t.Fatalf("send failure should be swallowed, got err: %v", err)
	}
	if len(rec.Sent()) != 1 {
		t.Fatalf("expected exactly 1 send attempt, got %d", len(rec.Sent()))
	}
}
