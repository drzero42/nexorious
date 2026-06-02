package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// EncodeEventCursor packs (occurred_at, id) into an opaque base64 token used as
// the `before` keyset cursor for GET /api/admin/events.
func EncodeEventCursor(occurredAt time.Time, id string) string {
	raw := occurredAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeEventCursor reverses EncodeEventCursor. Returns an error on any
// malformed token so the handler can reply 400.
func DecodeEventCursor(token string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor time: %w", err)
	}
	return ts, parts[1], nil
}
