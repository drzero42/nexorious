package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// validateName trims a user-supplied name and enforces the common rules shared
// by the pool, tag, and notification-channel endpoints: the trimmed value must
// be non-empty, and (when maxLen > 0) at most maxLen characters. It returns the
// trimmed name, or an *echo.HTTPError (400) with the canonical message. A
// maxLen of 0 disables the length check.
func validateName(name string, maxLen int) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if maxLen > 0 && len(name) > maxLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("name must be %d characters or less", maxLen))
	}
	return name, nil
}
