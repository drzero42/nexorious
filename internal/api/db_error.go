package api

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/ui"
)

// DBErrorHandler serves the database-unavailable error page.
type DBErrorHandler struct {
	migrator    *migrate.Migrator
	redactedDSN string
	tmpl        *template.Template
}

// NewDBErrorHandler creates a DBErrorHandler that renders the db-error page.
// resolvedDatabaseURL is the raw DATABASE_URL — the password is redacted before display.
func NewDBErrorHandler(resolvedDatabaseURL string, migrator *migrate.Migrator) *DBErrorHandler {
	redacted := redactDSN(resolvedDatabaseURL)
	tmpl := template.Must(template.ParseFS(ui.DBErrorBox, "db-error/index.html"))
	return &DBErrorHandler{migrator: migrator, redactedDSN: redacted, tmpl: tmpl}
}

// HandleDBError serves the error page when state is DBUnavailable, or redirects
// to the `from` query param (sanitised to local paths only) when the DB has recovered.
func (h *DBErrorHandler) HandleDBError(c *echo.Context) error {
	if h.migrator.State() != migrate.AppStateDBUnavailable {
		from := c.QueryParam("from")
		if from == "" || !strings.HasPrefix(from, "/") {
			from = "/"
		}
		return c.Redirect(http.StatusFound, from)
	}

	lastStr := "unknown"
	if t := h.migrator.LastUnavailableAt(); !t.IsZero() {
		lastStr = t.UTC().Format(time.RFC3339)
	}

	data := struct {
		RedactedDSN       string
		LastUnavailableAt string
	}{
		RedactedDSN:       h.redactedDSN,
		LastUnavailableAt: lastStr,
	}

	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return h.tmpl.Execute(c.Response(), data)
}

// redactDSN replaces the password in a postgres DSN and scrubs password-like query params.
func redactDSN(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid DSN>"
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") {
			q.Set(key, "***")
		}
	}
	u.RawQuery = q.Encode()
	// url.UserPassword percent-encodes '*' as '%2A'; undo that for readability.
	return strings.ReplaceAll(u.String(), "%2A%2A%2A", "***")
}
