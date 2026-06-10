package logging

import (
	"bytes"
	"context"
	"testing"
)

func TestCat_EmitsCategory(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "db down", Cat(CategoryDB))

	m := decode(t, &buf)
	if m[KeyCategory] != "db" {
		t.Errorf("category = %v, want db", m[KeyCategory])
	}
}
