package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		maxLen   int
		wantName string
		wantErr  bool
		wantCode int
		wantMsg  string
	}{
		{"trims surrounding whitespace", "  Backlog  ", 100, "Backlog", false, 0, ""},
		{"empty rejected", "", 100, "", true, http.StatusBadRequest, "name is required"},
		{"whitespace-only rejected", "   ", 100, "", true, http.StatusBadRequest, "name is required"},
		{"at limit allowed", strings.Repeat("a", 100), 100, strings.Repeat("a", 100), false, 0, ""},
		{"over limit rejected", strings.Repeat("a", 101), 100, "", true, http.StatusBadRequest, "name must be 100 characters or less"},
		{"length measured after trim", "  " + strings.Repeat("a", 100) + "  ", 100, strings.Repeat("a", 100), false, 0, ""},
		{"zero maxLen disables length check", strings.Repeat("a", 500), 0, strings.Repeat("a", 500), false, 0, ""},
		{"zero maxLen still rejects empty", "  ", 0, "", true, http.StatusBadRequest, "name is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateName(c.input, c.maxLen)
			if c.wantErr {
				if err == nil {
					t.Fatalf("validateName(%q, %d) = (%q, nil), want error", c.input, c.maxLen, got)
				}
				he, ok := err.(*echo.HTTPError)
				if !ok {
					t.Fatalf("error type = %T, want *echo.HTTPError", err)
				}
				if he.Code != c.wantCode {
					t.Errorf("code = %d, want %d", he.Code, c.wantCode)
				}
				if he.Message != c.wantMsg {
					t.Errorf("message = %q, want %q", he.Message, c.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateName(%q, %d) returned unexpected error: %v", c.input, c.maxLen, err)
			}
			if got != c.wantName {
				t.Errorf("name = %q, want %q", got, c.wantName)
			}
		})
	}
}
