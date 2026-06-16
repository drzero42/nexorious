package csvmap

import (
	"strings"
	"testing"
)

func TestValidate_RejectsBadColumnFormat(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		Status:  StatusConfig{Column: &StatusColumn{Column: "shelves", Format: "bogus"}},
	}
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("want a format error, got %v", err)
	}
}

func TestValidate_PlayLogRequiresFields(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		PlayLog: &PlayLogConfig{Column: "dates"}, // missing SecondsField/CompletionField
	}
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "PlayLog") {
		t.Fatalf("want a PlayLog error, got %v", err)
	}
}

func TestValidate_PlayLogAndDurationExclusive(t *testing.T) {
	cfg := Config{
		Columns:  ColumnMap{Title: "name"},
		Duration: &DurationConfig{Format: "decimal"},
		PlayLog:  &PlayLogConfig{Column: "dates", SecondsField: "s", CompletionField: "c"},
	}
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want a mutual-exclusion error, got %v", err)
	}
}

func TestValidate_AcceptsGrouvee(t *testing.T) {
	if err := validate(Grouvee()); err != nil {
		t.Fatalf("Grouvee() must validate, got %v", err)
	}
}
