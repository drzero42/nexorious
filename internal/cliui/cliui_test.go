package cliui

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "x", "y"); got != "x" {
		t.Fatalf("FirstNonEmpty = %q, want x", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Fatalf("FirstNonEmpty empty = %q, want empty", got)
	}
}

func TestPromptTrimsLine(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("  hello \n"))
	var out bytes.Buffer
	got, err := Prompt(in, &out, "Name: ")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "hello" {
		t.Fatalf("Prompt = %q, want hello", got)
	}
	if !strings.Contains(out.String(), "Name: ") {
		t.Fatalf("label not written: %q", out.String())
	}
}

func TestConfirm(t *testing.T) {
	yes, err := Confirm(bufio.NewReader(strings.NewReader("y\n")), &bytes.Buffer{}, "ok?", false)
	if err != nil || !yes {
		t.Fatalf("Confirm y = (%v,%v), want (true,nil)", yes, err)
	}
	no, err := Confirm(bufio.NewReader(strings.NewReader("\n")), &bytes.Buffer{}, "ok?", false)
	if err != nil || no {
		t.Fatalf("Confirm default = (%v,%v), want (false,nil)", no, err)
	}
	skip, err := Confirm(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, "ok?", true)
	if err != nil || !skip {
		t.Fatalf("Confirm assumeYes = (%v,%v), want (true,nil)", skip, err)
	}
}

func TestEncodeJSON(t *testing.T) {
	var out bytes.Buffer
	if err := EncodeJSON(&out, map[string]int{"a": 1}); err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if got := out.String(); got != "{\n  \"a\": 1\n}\n" {
		t.Fatalf("EncodeJSON = %q", got)
	}
}
