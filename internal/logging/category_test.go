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

func TestCat_EmitsPanicCategory(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)

	log.ErrorContext(context.Background(), "boom", Cat(CategoryPanic))

	m := decode(t, &buf)
	if m[KeyCategory] != "panic" {
		t.Errorf("category = %v, want panic", m[KeyCategory])
	}
}
