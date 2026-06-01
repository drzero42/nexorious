package notify

import (
	"context"
	"testing"
)

func TestRecorderSenderRecords(t *testing.T) {
	r := NewRecorderSender()
	if err := r.Send(context.Background(), "noop://", "Title", "Body"); err != nil {
		t.Fatalf("recorder Send returned error: %v", err)
	}
	sent := r.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded send, got %d", len(sent))
	}
	if sent[0].URL != "noop://" || sent[0].Title != "Title" || sent[0].Body != "Body" {
		t.Errorf("unexpected recorded send: %+v", sent[0])
	}
}

func TestShoutrrrSenderInvalidURL(t *testing.T) {
	s := NewShoutrrrSender()
	if err := s.Send(context.Background(), "this-is-not-a-valid-shoutrrr-url", "T", "B"); err == nil {
		t.Error("expected error for invalid shoutrrr URL, got nil")
	}
}
