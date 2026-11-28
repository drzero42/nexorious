package api_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/db/models"
)

// TestPlatformHandlers_ErrorShape checks that every error from the user-game
// platform handlers comes back as Echo's {"message": …} body, never the
// {"error": …} body these handlers used to mix in (#1502).
func TestPlatformHandlers_ErrorShape(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	userID, token := setupUserGamesUser(t, testDB, e, "err-shape")
	gameID := insertTestGame(t, testDB, "Error Shape Game")
	insertTestUserGame(t, testDB, "ug-err-shape", userID, int(gameID))

	rec := postJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms", map[string]any{
		"platform": "pc-windows", "storefront": "steam",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create platform: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	platformURL := "/api/user-games/ug-err-shape/platforms/" + created.ID

	// A JSON string cannot bind into the request struct.
	const badBody = "not an object"

	tests := []struct {
		name       string
		send       func() *httptest.ResponseRecorder
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "list: unknown user game",
			send:       func() *httptest.ResponseRecorder { return getAuth(t, e, "/api/user-games/ug-missing/platforms", token) },
			wantStatus: http.StatusNotFound,
			wantMsg:    "user game not found",
		},
		{
			name: "create: invalid body",
			send: func() *httptest.ResponseRecorder {
				return postJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms", badBody, token)
			},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid request body",
		},
		{
			name: "create: unknown platform",
			send: func() *httptest.ResponseRecorder {
				return postJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms", map[string]any{"platform": "no-such-platform"}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "platform not found: no-such-platform",
		},
		{
			name: "create: unknown storefront",
			send: func() *httptest.ResponseRecorder {
				return postJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms", map[string]any{"platform": "pc-windows", "storefront": "no-such-store"}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "storefront not found: no-such-store",
		},
		{
			name: "create: invalid acquired_date",
			send: func() *httptest.ResponseRecorder {
				return postJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms", map[string]any{"platform": "pc-windows", "acquired_date": "yesterday"}, token)
			},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid acquired_date",
		},
		{
			name: "update: unknown user game",
			send: func() *httptest.ResponseRecorder {
				return putJSONAuth(t, e, "/api/user-games/ug-missing/platforms/"+created.ID, map[string]any{}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "user game not found",
		},
		{
			name: "update: unknown platform id",
			send: func() *httptest.ResponseRecorder {
				return putJSONAuth(t, e, "/api/user-games/ug-err-shape/platforms/no-such-id", map[string]any{}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "platform not found",
		},
		{
			name:       "update: invalid body",
			send:       func() *httptest.ResponseRecorder { return putJSONAuth(t, e, platformURL, badBody, token) },
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid request body",
		},
		{
			name: "update: unknown platform",
			send: func() *httptest.ResponseRecorder {
				return putJSONAuth(t, e, platformURL, map[string]any{"platform": "no-such-platform"}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "platform not found: no-such-platform",
		},
		{
			name: "update: unknown storefront",
			send: func() *httptest.ResponseRecorder {
				return putJSONAuth(t, e, platformURL, map[string]any{"storefront": "no-such-store"}, token)
			},
			wantStatus: http.StatusNotFound,
			wantMsg:    "storefront not found: no-such-store",
		},
		{
			name: "update: invalid acquired_date",
			send: func() *httptest.ResponseRecorder {
				return putJSONAuth(t, e, platformURL, map[string]any{"acquired_date": "yesterday"}, token)
			},
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid acquired_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := tt.send()
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal error body %q: %v", rec.Body.String(), err)
			}
			if _, ok := body["error"]; ok {
				t.Errorf("body has an \"error\" key, want only \"message\": %s", rec.Body.String())
			}
			msg, _ := body["message"].(string)
			if !strings.HasPrefix(msg, tt.wantMsg) {
				t.Errorf("message = %q, want prefix %q", msg, tt.wantMsg)
			}
		})
	}
}

// TestPlatformHandlers_DBFailureIs500 checks that a failing database answers
// 500, not the 404 a missing row gets (#1502).
func TestPlatformHandlers_DBFailureIs500(t *testing.T) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(testConnStr)))
	closedDB := bun.NewDB(sqldb, pgdialect.New())
	// The router registers this m2m model; a direct handler call must do it itself.
	closedDB.RegisterModel((*models.PlatformStorefront)(nil))
	if err := closedDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	h := api.NewUserGamesHandler(closedDB, testCfg())

	newCtx := func(method string, body string, params echo.PathValues) *echo.Context {
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		c := echo.New().NewContext(req, httptest.NewRecorder())
		c.Set("user_id", "u-db-failure")
		c.SetPathValues(params)
		return c
	}
	ugParam := echo.PathValue{Name: "id", Value: "ug-db-failure"}
	platParam := echo.PathValue{Name: "platform_id", Value: "plat-db-failure"}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "list: ownership check fails",
			call: func() error {
				return h.HandleListPlatforms(newCtx(http.MethodGet, "", echo.PathValues{ugParam}))
			},
		},
		{
			name: "create: platform lookup fails",
			call: func() error {
				return h.HandleCreatePlatform(newCtx(http.MethodPost, `{"platform":"pc-windows"}`, echo.PathValues{ugParam}))
			},
		},
		{
			name: "update: ownership check fails",
			call: func() error {
				return h.HandleUpdatePlatform(newCtx(http.MethodPut, `{}`, echo.PathValues{ugParam, platParam}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			var he *echo.HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("expected *echo.HTTPError, got %v", err)
			}
			if he.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500 (%s)", he.Code, fmt.Sprint(he.Message))
			}
		})
	}
}
