package main

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
)

// seedResetUser inserts a user with a known bcrypt hash and returns its id.
func seedResetUser(t *testing.T, username, password string, isAdmin bool) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), auth.BcryptCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id := "u-" + username
	_, err = testDB.ExecContext(context.Background(),
		"INSERT INTO users (id, username, password_hash, is_active, is_admin) VALUES (?, ?, ?, ?, ?)",
		id, username, string(hash), true, isAdmin,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedResetSession inserts one session row for userID.
func seedResetSession(t *testing.T, userID string) {
	t.Helper()
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID: %v", err)
	}
	_, err = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at)
		 VALUES (gen_random_uuid()::text, ?, ?, now() + interval '30 days')`,
		userID, auth.HashToken(sessionID),
	)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestResetUserPassword_HappyPath(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	id := seedResetUser(t, "alice", "oldpassword", true)
	seedResetSession(t, id)
	seedResetSession(t, id)

	if err := resetUserPassword(ctx, testDB, "alice", "newpassword"); err != nil {
		t.Fatalf("resetUserPassword: %v", err)
	}

	var hash string
	if err := testDB.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil {
		t.Fatalf("select hash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpassword")) != nil {
		t.Error("new password does not verify against stored hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("oldpassword")) == nil {
		t.Error("old password still verifies; hash was not changed")
	}

	var sessions int
	if err := testDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_sessions WHERE user_id = ?", id).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("sessions = %d, want 0 (all should be wiped)", sessions)
	}
}

func TestResetUserPassword_UnknownUser(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	err := resetUserPassword(ctx, testDB, "ghost", "newpassword")
	if !errors.Is(err, errUserNotFound) {
		t.Fatalf("err = %v, want errUserNotFound", err)
	}
}

func TestResetUserPassword_TooShort(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	id := seedResetUser(t, "bob", "oldpassword", false)

	err := resetUserPassword(ctx, testDB, "bob", "short77") // 7 chars
	if !errors.Is(err, errPasswordTooShort) {
		t.Fatalf("err = %v, want errPasswordTooShort", err)
	}

	// Hash must be unchanged.
	var hash string
	if err := testDB.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE id = ?", id).Scan(&hash); err != nil {
		t.Fatalf("select hash: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("oldpassword")) != nil {
		t.Error("hash changed despite too-short password")
	}
}
