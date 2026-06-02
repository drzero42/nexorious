package api_test

import (
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/api"
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
