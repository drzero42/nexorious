# `nexctl doctor` CLI + Library-Smells MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `nexctl doctor` command group and a parallel set of MCP tools that expose the already-shipped Library-Smells REST API (`/api/library/smells`) over the CLI and the local MCP server.

**Architecture:** Both front-ends are a *pure mirror* of the REST surface. A new `internal/cliclient/smells.go` defines the request/response types and six client methods once; `cmd/nexctl/doctor.go` (Cobra) and `cmd/nexctl/mcp_doctor.go` (MCP) are two thin consumers of those same methods plus the existing shared finders (`findUserGamesByRef`, `resolveUserGameRef`, `mcpResolveEditTargets`). No server-side change — the engine, the 6 endpoints, and `smell_ignores` already exist.

**Tech Stack:** Go 1.26, Cobra (`spf13/cobra`), the MCP Go SDK (`modelcontextprotocol/go-sdk`, `nexctl`-only), `net/http/httptest` for tests.

## Global Constraints

- **The registry is 9 checks, 3 auto-fixable** — NOT the 11/4 in the epic (#1143) or issue (#1146). `beat-but-not-marked` (#7) was removed in the #1145 follow-up. Auto-fixable check ids: `wishlisted-yet-owned`, `played-but-not-started`, `in-progress-untouched`. The CLI/MCP must derive fixability from the API's `auto_fixable` field and the server's 422 — never hardcode a check list.
- **MCP stays a `nexctl`-only dependency.** Never import `modelcontextprotocol/go-sdk` from the `nexorious` server binary or any `internal/` package it links. MCP code lives only under `cmd/nexctl/`.
- **`jsonschema:"…"` struct tags must be bare description strings** (the `description=…` form panics at `AddTool`).
- **Reuse the shared cmd-free finders** (`findUserGamesByRef`, `mcpResolveEditTargets`) and `cliclient` methods — the two front-ends must not drift, and ref-resolution logic must not be duplicated.
- **Ambiguous refs return candidates, no mutation:** CLI uses `resolveUserGameRef` (picker on TTY / error with ids off-TTY); MCP returns a `candidates` list via `mcpResolveEditTargets`.
- **Surfaced errors:** wrap CLI errors `fmt.Errorf("… failed: %w", err)`; MCP errors go through `mcpToolError(action, err)` so a read-scoped-key 403 becomes the "mint a write-scoped key" message.
- **Persistent flags** `--json`, `-q/--quiet`, `-y/--yes`, `--profile` are inherited from the root command; read them with `flagBool(cmd, "json")` etc. and resolve the profile with `resolveProfile(cmd)`.
- The REST API is the authority for endpoint shapes:
  - `GET    /api/library/smells` → `[]{id,title,description,tier,auto_fixable,count}`
  - `GET    /api/library/smells/:checkID?page=&per_page=` → `{items:[{user_game_id,game_id,title,cover_art_url?,suggested_status?,detail?}],total,page,per_page,pages}`
  - `POST   /api/library/smells/:checkID/apply` body `{user_game_ids:[…]}` → `{applied,skipped}` (422 if not auto-fixable; 400 on empty/>200 ids)
  - `POST   /api/library/smells/:checkID/ignore` body `{user_game_ids:[…]}` → `{ignored}`
  - `DELETE /api/library/smells/:checkID/ignore` body `{user_game_ids:[…]}` → `{restored}`
  - `GET    /api/library/smells/:checkID/ignored?page=&per_page=` → `{items:[{user_game_id,title,created_at}],total,page,per_page,pages}`

---

### Task 1: cliclient — types and six methods

**Files:**
- Create: `internal/cliclient/smells.go`
- Test: `internal/cliclient/smells_test.go`

**Interfaces:**
- Consumes: `(*Client).doBearer(method, path, key string, body, out any) error` (already exists; marshals a body for any method incl. DELETE).
- Produces (later tasks rely on these exact names/types):
  - Types `SmellSummaryItem`, `FlaggedItem`, `FlaggedListResponse`, `IgnoredItem`, `IgnoredListResponse`, `SmellApplyResult`.
  - `(*Client).ListSmells(key string) ([]SmellSummaryItem, error)`
  - `(*Client).ListSmellItems(key, checkID string, page, perPage int) (*FlaggedListResponse, error)`
  - `(*Client).ApplySmell(key, checkID string, userGameIDs []string) (*SmellApplyResult, error)`
  - `(*Client).IgnoreSmell(key, checkID string, userGameIDs []string) (int, error)` (returns `ignored`)
  - `(*Client).RestoreSmell(key, checkID string, userGameIDs []string) (int, error)` (returns `restored`)
  - `(*Client).ListIgnoredSmells(key, checkID string, page, perPage int) (*IgnoredListResponse, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/cliclient/smells_test.go`:

```go
package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSmells(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "tier": "inconsistency", "auto_fixable": false, "count": 2},
			{"id": "wishlisted-yet-owned", "title": "Wishlisted yet owned", "tier": "inconsistency", "auto_fixable": true, "count": 1},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := New(srv.URL).ListSmells("k")
	if err != nil {
		t.Fatalf("ListSmells: %v", err)
	}
	if len(got) != 2 || got[0].ID != "orphan-game" || got[0].Count != 2 || !got[1].AutoFixable {
		t.Fatalf("got = %+v", got)
	}
}

func TestListSmellItems(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/played-but-not-started", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "200" {
			t.Errorf("per_page = %q, want 200", r.URL.Query().Get("per_page"))
		}
		sugg := "in_progress"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 5, "title": "Halo", "suggested_status": sugg},
			},
			"total": 1, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListSmellItems("k", "played-but-not-started", 1, 200)
	if err != nil {
		t.Fatalf("ListSmellItems: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].UserGameID != "u1" ||
		res.Items[0].SuggestedStatus == nil || *res.Items[0].SuggestedStatus != "in_progress" {
		t.Fatalf("res = %+v", res)
	}
}

func TestApplyIgnoreRestoreSmell(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body["user_game_ids"]) != 2 {
			t.Errorf("ids = %v", body["user_game_ids"])
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 2, "skipped": 0})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/ignore", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ap, err := c.ApplySmell("k", "wishlisted-yet-owned", []string{"a", "b"})
	if err != nil || ap.Applied != 2 {
		t.Fatalf("ApplySmell: %+v err=%v", ap, err)
	}
	ig, err := c.IgnoreSmell("k", "wishlisted-yet-owned", []string{"a"})
	if err != nil || ig != 1 {
		t.Fatalf("IgnoreSmell: %d err=%v", ig, err)
	}
	rs, err := c.RestoreSmell("k", "wishlisted-yet-owned", []string{"a"})
	if err != nil || rs != 1 {
		t.Fatalf("RestoreSmell: %d err=%v", rs, err)
	}
}

func TestListIgnoredSmells(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/orphan-game/ignored", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u9", "title": "Tetris", "created_at": "2026-06-22T00:00:00Z"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListIgnoredSmells("k", "orphan-game", 1, 25)
	if err != nil {
		t.Fatalf("ListIgnoredSmells: %v", err)
	}
	if res.Total != 1 || res.Items[0].UserGameID != "u9" || res.Items[0].Title != "Tetris" {
		t.Fatalf("res = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliclient/ -run 'TestListSmells|TestListSmellItems|TestApplyIgnoreRestoreSmell|TestListIgnoredSmells' -v`
Expected: FAIL — undefined: `(*Client).ListSmells` etc.

- [ ] **Step 3: Write the implementation**

Create `internal/cliclient/smells.go`:

```go
package cliclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// SmellSummaryItem is one row of GET /api/library/smells (per-check counts).
type SmellSummaryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	AutoFixable bool   `json:"auto_fixable"`
	Count       int    `json:"count"`
}

// FlaggedItem is one flagged game returned by GET /api/library/smells/:checkID.
type FlaggedItem struct {
	UserGameID      string  `json:"user_game_id"`
	GameID          int32   `json:"game_id"`
	Title           string  `json:"title"`
	CoverArtURL     *string `json:"cover_art_url,omitempty"`
	SuggestedStatus *string `json:"suggested_status,omitempty"`
	Detail          *string `json:"detail,omitempty"`
}

// FlaggedListResponse is the paginated flagged-items response.
type FlaggedListResponse struct {
	Items   []FlaggedItem `json:"items"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// IgnoredItem is one dismissed game from GET /api/library/smells/:checkID/ignored.
type IgnoredItem struct {
	UserGameID string `json:"user_game_id"`
	Title      string `json:"title"`
	CreatedAt  string `json:"created_at"`
}

// IgnoredListResponse is the paginated dismissed-items response.
type IgnoredListResponse struct {
	Items   []IgnoredItem `json:"items"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// SmellApplyResult is the POST .../apply response.
type SmellApplyResult struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
}

func smellPagePath(base string, page, perPage int) string {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
	if enc := q.Encode(); enc != "" {
		return base + "?" + enc
	}
	return base
}

// ListSmells returns the per-check summary (counts post-ignore).
func (c *Client) ListSmells(key string) ([]SmellSummaryItem, error) {
	var out []SmellSummaryItem
	if err := c.doBearer(http.MethodGet, "/api/library/smells", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSmellItems returns one page of flagged items for a check.
func (c *Client) ListSmellItems(key, checkID string, page, perPage int) (*FlaggedListResponse, error) {
	var out FlaggedListResponse
	path := smellPagePath("/api/library/smells/"+url.PathEscape(checkID), page, perPage)
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplySmell applies the auto-fix for a check to the given user-game ids.
func (c *Client) ApplySmell(key, checkID string, userGameIDs []string) (*SmellApplyResult, error) {
	var out SmellApplyResult
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/apply"
	if err := c.doBearer(http.MethodPost, path, key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IgnoreSmell dismisses the given user-games for a check; returns the count newly ignored.
func (c *Client) IgnoreSmell(key, checkID string, userGameIDs []string) (int, error) {
	var out struct {
		Ignored int `json:"ignored"`
	}
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/ignore"
	if err := c.doBearer(http.MethodPost, path, key, body, &out); err != nil {
		return 0, err
	}
	return out.Ignored, nil
}

// RestoreSmell un-dismisses the given user-games for a check; returns the count restored.
func (c *Client) RestoreSmell(key, checkID string, userGameIDs []string) (int, error) {
	var out struct {
		Restored int `json:"restored"`
	}
	body := map[string][]string{"user_game_ids": userGameIDs}
	path := "/api/library/smells/" + url.PathEscape(checkID) + "/ignore"
	if err := c.doBearer(http.MethodDelete, path, key, body, &out); err != nil {
		return 0, err
	}
	return out.Restored, nil
}

// ListIgnoredSmells returns one page of dismissed items for a check.
func (c *Client) ListIgnoredSmells(key, checkID string, page, perPage int) (*IgnoredListResponse, error) {
	var out IgnoredListResponse
	path := smellPagePath("/api/library/smells/"+url.PathEscape(checkID)+"/ignored", page, perPage)
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ensure fmt is used (kept for future error wrapping in this file).
var _ = fmt.Sprintf
```

> Note: drop the trailing `var _ = fmt.Sprintf` line and the `"fmt"` import if `gofmt`/`golangci-lint` flags `fmt` as unused — it is included only as a guard and should be removed if not needed. The PostToolUse hook will surface this; reconcile it before committing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliclient/ -run 'TestListSmells|TestListSmellItems|TestApplyIgnoreRestoreSmell|TestListIgnoredSmells' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/smells.go internal/cliclient/smells_test.go
git commit -m "feat: cliclient methods for library-smells REST API"
```

---

### Task 2: `nexctl doctor` — summary + per-check detail (read-only)

**Files:**
- Create: `cmd/nexctl/doctor.go`
- Modify: `cmd/nexctl/main.go` (register `newDoctorCmd()`)
- Test: `cmd/nexctl/doctor_test.go`

**Interfaces:**
- Consumes: `(*Client).ListSmells`, `(*Client).ListSmellItems` (Task 1); `resolveProfile`, `flagBool`, `cliclient.New`, `cliui.EncodeJSON`, `deref` (existing).
- Produces (later tasks rely on these):
  - `newDoctorCmd() *cobra.Command` — has a `--check` string flag; its `RunE` prints the summary, or the per-check detail when `--check` is set. Sub-commands are attached here in Tasks 3 & 4.
  - `collectFlaggedIDs(c *cliclient.Client, key, checkID string) ([]string, error)` — fetches every flagged `user_game_id` across all pages.

- [ ] **Step 1: Write the failing test**

Create `cmd/nexctl/doctor_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// doctorServer stubs the summary + a couple of per-check listings.
func doctorServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		// Exact match only — sub-paths are handled below.
		if r.URL.Path != "/api/library/smells" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "description": "d", "tier": "inconsistency", "auto_fixable": false, "count": 2},
			{"id": "wishlisted-yet-owned", "title": "Wishlisted yet owned", "description": "d", "tier": "inconsistency", "auto_fixable": true, "count": 1},
			{"id": "unrated-after-finishing", "title": "Unrated after finishing", "description": "d", "tier": "nudge", "auto_fixable": false, "count": 0},
		})
	})
	mux.HandleFunc("/api/library/smells/orphan-game", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 1, "title": "Tetris"},
				{"user_game_id": "u2", "game_id": 2, "title": "Pong"},
			},
			"total": 2, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDoctorSummaryTable(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor: %v\n%s", err, out.String())
	}
	for _, want := range []string{"orphan-game", "Orphan game", "inconsistency", "2", "wishlisted-yet-owned", "yes"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("summary missing %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorSummaryQuietOnlyNonzero(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "-q"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor -q: %v", err)
	}
	got := out.String()
	if !bytes.Contains(out.Bytes(), []byte("orphan-game")) ||
		!bytes.Contains(out.Bytes(), []byte("wishlisted-yet-owned")) {
		t.Fatalf("quiet missing nonzero ids:\n%s", got)
	}
	if bytes.Contains(out.Bytes(), []byte("unrated-after-finishing")) {
		t.Fatalf("quiet should omit zero-count checks:\n%s", got)
	}
}

func TestDoctorDetail(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "--check", "orphan-game"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --check: %v\n%s", err, out.String())
	}
	for _, want := range []string{"u1", "Tetris", "u2", "Pong"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("detail missing %q:\n%s", want, out.String())
		}
	}
}
```

> `seedProfile(t, srv.URL)` is the existing helper used by every `cmd/nexctl` CLI test (see `tag_test.go`, `game_stats_test.go`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/ -run TestDoctor -v`
Expected: FAIL — `unknown command "doctor"` (the command isn't registered yet).

- [ ] **Step 3: Write the implementation**

Create `cmd/nexctl/doctor.go`:

```go
package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newDoctorCmd() *cobra.Command {
	var check string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect and fix library health issues (\"smells\")",
		Long: "doctor scans your collection for data-quality issues. With no flags it\n" +
			"prints a summary of every check; --check <id> lists the flagged games for\n" +
			"one check. Use the apply/ignore/restore subcommands to act on findings.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if check != "" {
				return runDoctorDetail(cmd, check)
			}
			return runDoctorSummary(cmd)
		},
	}
	cmd.Flags().StringVar(&check, "check", "", "List the flagged games for one check id")
	cmd.AddCommand(newDoctorApplyCmd(), newDoctorIgnoreCmd(), newDoctorRestoreCmd(), newDoctorIgnoredCmd())
	return cmd
}

func runDoctorSummary(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	p, _, err := resolveProfile(cmd)
	if err != nil {
		return err
	}
	checks, err := cliclient.New(p.URL).ListSmells(p.Key)
	if err != nil {
		return fmt.Errorf("list smells failed: %w", err)
	}
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, checks)
	}
	if flagBool(cmd, "quiet") {
		for i := range checks {
			if checks[i].Count > 0 {
				fmt.Fprintln(out, checks[i].ID)
			}
		}
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCHECK\tTIER\tFIXABLE\tCOUNT")
	for i := range checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n",
			checks[i].ID, checks[i].Title, checks[i].Tier, yesNo(checks[i].AutoFixable), checks[i].Count)
	}
	return tw.Flush()
}

func runDoctorDetail(cmd *cobra.Command, checkID string) error {
	out := cmd.OutOrStdout()
	p, _, err := resolveProfile(cmd)
	if err != nil {
		return err
	}
	res, err := cliclient.New(p.URL).ListSmellItems(p.Key, checkID, 1, 200)
	if err != nil {
		return fmt.Errorf("list smell items failed: %w", err)
	}
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, res)
	}
	if flagBool(cmd, "quiet") {
		for i := range res.Items {
			fmt.Fprintln(out, res.Items[i].UserGameID)
		}
		return nil
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(out, "No games flagged by this check.")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "USER_GAME_ID\tTITLE\tSUGGESTION")
	for i := range res.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", res.Items[i].UserGameID, res.Items[i].Title, suggestionOf(res.Items[i]))
	}
	return tw.Flush()
}

// suggestionOf renders the per-item hint: the suggested status (auto-fix
// checks) or the human-readable detail (e.g. impossible-acquired-date), else "-".
func suggestionOf(it cliclient.FlaggedItem) string {
	if it.SuggestedStatus != nil && *it.SuggestedStatus != "" {
		return "→ " + *it.SuggestedStatus
	}
	if it.Detail != nil && *it.Detail != "" {
		return *it.Detail
	}
	return "-"
}

// collectFlaggedIDs returns every flagged user_game_id for a check, across pages.
func collectFlaggedIDs(c *cliclient.Client, key, checkID string) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		res, err := c.ListSmellItems(key, checkID, page, 200)
		if err != nil {
			return nil, err
		}
		for i := range res.Items {
			ids = append(ids, res.Items[i].UserGameID)
		}
		if res.Pages <= page || res.Pages == 0 {
			break
		}
	}
	return ids, nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
```

Modify `cmd/nexctl/main.go` — register the command. After the line `root.AddCommand(newChangelogCmd())`:

```go
	root.AddCommand(newChangelogCmd())
	root.AddCommand(newDoctorCmd())
```

- [ ] **Step 2b: Stub the subcommand constructors so the package compiles**

The four subcommand constructors are implemented in Tasks 3–4, but `newDoctorCmd()` references them now. Add minimal stubs at the bottom of `cmd/nexctl/doctor.go` to compile this task; Tasks 3–4 replace them:

```go
// Replaced in Tasks 3 & 4.
func newDoctorApplyCmd() *cobra.Command   { return &cobra.Command{Use: "apply", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func newDoctorIgnoreCmd() *cobra.Command  { return &cobra.Command{Use: "ignore", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func newDoctorRestoreCmd() *cobra.Command { return &cobra.Command{Use: "restore", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func newDoctorIgnoredCmd() *cobra.Command { return &cobra.Command{Use: "ignored", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./cmd/nexctl/ -run TestDoctor -v`
Expected: PASS (`TestDoctorSummaryTable`, `TestDoctorSummaryQuietOnlyNonzero`, `TestDoctorDetail`).

- [ ] **Step 4: Commit**

```bash
git add cmd/nexctl/doctor.go cmd/nexctl/doctor_test.go cmd/nexctl/main.go
git commit -m "feat: nexctl doctor summary + per-check detail"
```

---

### Task 3: `nexctl doctor apply`

**Files:**
- Modify: `cmd/nexctl/doctor.go` (replace `newDoctorApplyCmd` stub)
- Test: `cmd/nexctl/doctor_test.go` (add cases)

**Interfaces:**
- Consumes: `collectFlaggedIDs` (Task 2), `(*Client).ApplySmell` (Task 1), `resolveUserGameRef` (existing, in `game.go`), `cliui.Confirm`, `interactive` (existing).
- Produces: `newDoctorApplyCmd() *cobra.Command` — `doctor apply <check> [refs…]`. With no refs, applies to every flagged game for the check; with refs, resolves each to a user-game id. Confirms (unless `-y`). Prints `{applied, skipped}`.

- [ ] **Step 1: Write the failing test**

Add to `cmd/nexctl/doctor_test.go`:

```go
func TestDoctorApplyAll(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 1, "title": "A"},
				{"user_game_id": "u2", "game_id": 2, "title": "B"},
			},
			"total": 2, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 2, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "apply", "wishlisted-yet-owned", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	if len(gotIDs) != 2 {
		t.Fatalf("applied ids = %v", gotIDs)
	}
	if !bytes.Contains(out.Bytes(), []byte("Applied 2")) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDoctorApplyByRef(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	// UUID ref → direct GET of the user-game.
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 1, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "apply", "wishlisted-yet-owned",
		"11111111-1111-1111-1111-111111111111", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply by ref: %v\n%s", err, out.String())
	}
	if len(gotIDs) != 1 || gotIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("applied ids = %v", gotIDs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/ -run 'TestDoctorApply' -v`
Expected: FAIL — the stub `apply` does nothing, so `gotIDs` is empty / output missing "Applied 2".

- [ ] **Step 3: Write the implementation**

In `cmd/nexctl/doctor.go`, replace the `newDoctorApplyCmd` stub with:

```go
func newDoctorApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <check> [refs...]",
		Short: "Apply an auto-fix check's suggestion (all flagged games, or specific refs)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			checkID := args[0]
			refs := args[1:]

			var ids []string
			if len(refs) == 0 {
				ids, err = collectFlaggedIDs(c, p.Key, checkID)
				if err != nil {
					return fmt.Errorf("list smell items failed: %w", err)
				}
			} else {
				for _, ref := range refs {
					u, rErr := resolveUserGameRef(cmd, c, p.Key, ref)
					if rErr != nil {
						return rErr
					}
					ids = append(ids, u.ID)
				}
			}
			if len(ids) == 0 {
				fmt.Fprintln(out, "Nothing to apply — no games flagged by this check.")
				return nil
			}

			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Apply %q suggestion to %d game(s)?", checkID, len(ids)), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}

			res, err := c.ApplySmell(p.Key, checkID, ids)
			if err != nil {
				return fmt.Errorf("apply failed: %w", err)
			}
			fmt.Fprintf(out, "Applied %d, skipped %d.\n", res.Applied, res.Skipped)
			return nil
		},
	}
}
```

Add `"bufio"` to the import block of `cmd/nexctl/doctor.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/ -run 'TestDoctorApply' -v`
Expected: PASS (`TestDoctorApplyAll`, `TestDoctorApplyByRef`).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/doctor.go cmd/nexctl/doctor_test.go
git commit -m "feat: nexctl doctor apply"
```

---

### Task 4: `nexctl doctor ignore` / `restore` / `ignored`

**Files:**
- Modify: `cmd/nexctl/doctor.go` (replace the three stubs)
- Test: `cmd/nexctl/doctor_test.go` (add cases)

**Interfaces:**
- Consumes: `(*Client).IgnoreSmell`, `(*Client).RestoreSmell`, `(*Client).ListIgnoredSmells` (Task 1), `resolveUserGameRef` (existing).
- Produces:
  - `newDoctorIgnoreCmd()` — `doctor ignore <check> <ref> [refs…]`, dismisses each resolved game; prints count ignored.
  - `newDoctorRestoreCmd()` — `doctor restore <check> <ref> [refs…]`, un-dismisses; prints count restored.
  - `newDoctorIgnoredCmd()` — `doctor ignored <check>`, lists dismissed items (table / `--json` / `-q` ids).

- [ ] **Step 1: Write the failing test**

Add to `cmd/nexctl/doctor_test.go`:

```go
func TestDoctorIgnoreAndRestore(t *testing.T) {
	var ignoreIDs, restoreIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 1, "title": "Doom"}})
	})
	mux.HandleFunc("/api/library/smells/orphan-game/ignore", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.Method {
		case http.MethodPost:
			ignoreIDs = body["user_game_ids"]
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			restoreIDs = body["user_game_ids"]
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	ref := "22222222-2222-2222-2222-222222222222"

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "ignore", "orphan-game", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("ignore: %v\n%s", err, out.String())
	}
	if len(ignoreIDs) != 1 || ignoreIDs[0] != ref {
		t.Fatalf("ignore ids = %v", ignoreIDs)
	}

	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "restore", "orphan-game", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("restore: %v\n%s", err, out.String())
	}
	if len(restoreIDs) != 1 || restoreIDs[0] != ref {
		t.Fatalf("restore ids = %v", restoreIDs)
	}
}

func TestDoctorIgnoredList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/orphan-game/ignored", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u9", "title": "Myst", "created_at": "2026-06-22T10:00:00Z"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "ignored", "orphan-game"})
	if err := root.Execute(); err != nil {
		t.Fatalf("ignored: %v\n%s", err, out.String())
	}
	for _, want := range []string{"u9", "Myst"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("ignored list missing %q:\n%s", want, out.String())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/ -run 'TestDoctorIgnore|TestDoctorIgnored' -v`
Expected: FAIL — stubs do nothing, captured ids empty / list output missing "Myst".

- [ ] **Step 3: Write the implementation**

In `cmd/nexctl/doctor.go`, replace the three stubs:

```go
// resolveDoctorRefs resolves one-or-more game refs to user-game ids.
func resolveDoctorRefs(cmd *cobra.Command, c *cliclient.Client, key string, refs []string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		u, err := resolveUserGameRef(cmd, c, key, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, u.ID)
	}
	return ids, nil
}

func newDoctorIgnoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ignore <check> <ref> [refs...]",
		Short: "Dismiss flagged games for a check",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ids, err := resolveDoctorRefs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			n, err := c.IgnoreSmell(p.Key, args[0], ids)
			if err != nil {
				return fmt.Errorf("ignore failed: %w", err)
			}
			fmt.Fprintf(out, "Dismissed %d game(s) for %q.\n", n, args[0])
			return nil
		},
	}
}

func newDoctorRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <check> <ref> [refs...]",
		Short: "Un-dismiss previously ignored games for a check",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ids, err := resolveDoctorRefs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			n, err := c.RestoreSmell(p.Key, args[0], ids)
			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}
			fmt.Fprintf(out, "Restored %d game(s) for %q.\n", n, args[0])
			return nil
		},
	}
}

func newDoctorIgnoredCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ignored <check>",
		Short: "List dismissed games for a check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			res, err := cliclient.New(p.URL).ListIgnoredSmells(p.Key, args[0], 1, 200)
			if err != nil {
				return fmt.Errorf("list dismissed failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, res)
			}
			if flagBool(cmd, "quiet") {
				for i := range res.Items {
					fmt.Fprintln(out, res.Items[i].UserGameID)
				}
				return nil
			}
			if len(res.Items) == 0 {
				fmt.Fprintln(out, "No dismissed games for this check.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "USER_GAME_ID\tTITLE\tDISMISSED_AT")
			for i := range res.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", res.Items[i].UserGameID, res.Items[i].Title, res.Items[i].CreatedAt)
			}
			return tw.Flush()
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/ -run 'TestDoctor' -v`
Expected: PASS (all doctor CLI tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/doctor.go cmd/nexctl/doctor_test.go
git commit -m "feat: nexctl doctor ignore/restore/ignored"
```

---

### Task 5: MCP tools (`library_smells_*`) + registration

**Files:**
- Create: `cmd/nexctl/mcp_doctor.go`
- Modify: `cmd/nexctl/mcp.go` (register in `buildMCPServer`)
- Test: `cmd/nexctl/mcp_doctor_test.go`

**Interfaces:**
- Consumes: every `cliclient` smells method (Task 1); `mcpResolveEditTargets`, `briefOf`, `gameBrief`, `mcpToolError` (existing in `mcp_game.go`/`mcp.go`); `collectFlaggedIDs` (Task 2).
- Produces: `registerDoctorTools(s *mcp.Server, c *cliclient.Client, key string)` registering six tools: `library_smells_list`, `library_smells_detail`, `library_smells_apply`, `library_smells_ignore`, `library_smells_restore`, `library_smells_ignored`.

- [ ] **Step 1: Write the failing test**

Create `cmd/nexctl/mcp_doctor_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPSmellsListAndDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/library/smells" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "tier": "inconsistency", "auto_fixable": false, "count": 1},
		})
	})
	mux.HandleFunc("/api/library/smells/orphan-game", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u1", "game_id": 1, "title": "Tetris"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "library_smells_list"})
	if err != nil || res.IsError {
		t.Fatalf("list: err=%v res=%+v", err, res)
	}
	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_detail", Arguments: map[string]any{"check_id": "orphan-game"}})
	if err != nil || res.IsError {
		t.Fatalf("detail: err=%v res=%+v", err, res)
	}
}

func TestMCPSmellsApplyByRef(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 1, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_apply",
		Arguments: map[string]any{
			"check_id": "wishlisted-yet-owned",
			"refs":     []string{"11111111-1111-1111-1111-111111111111"},
		}})
	if err != nil || res.IsError {
		t.Fatalf("apply: err=%v res=%+v", err, res)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("applied ids = %v", gotIDs)
	}
}

func TestMCPSmellsIgnoreRestore(t *testing.T) {
	var ignored, restored bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 1, "title": "Doom"}})
	})
	mux.HandleFunc("/api/library/smells/orphan-game/ignore", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ignored = true
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			restored = true
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	ref := "22222222-2222-2222-2222-222222222222"

	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_ignore", Arguments: map[string]any{"check_id": "orphan-game", "refs": []string{ref}}}); err != nil || res.IsError {
		t.Fatalf("ignore: err=%v res=%+v", err, res)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "library_smells_restore", Arguments: map[string]any{"check_id": "orphan-game", "refs": []string{ref}}}); err != nil || res.IsError {
		t.Fatalf("restore: err=%v res=%+v", err, res)
	}
	if !ignored || !restored {
		t.Fatalf("ignored=%v restored=%v", ignored, restored)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/ -run 'TestMCPSmells' -v`
Expected: FAIL — tools `library_smells_*` are not registered (CallTool returns an error / `IsError`).

- [ ] **Step 3: Write the implementation**

Create `cmd/nexctl/mcp_doctor.go`:

```go
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// --- input schemas ---

type smellsDetailInput struct {
	CheckID string `json:"check_id" jsonschema:"check id (from library_smells_list)"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"page size, max 200"`
}

type smellsRefsInput struct {
	CheckID string   `json:"check_id" jsonschema:"check id (from library_smells_list)"`
	Refs    []string `json:"refs" jsonschema:"one or more game ids (UUID) or title substrings"`
}

// --- output projections ---

type smellsListOutput struct {
	Checks []cliclient.SmellSummaryItem `json:"checks"`
}

type smellsDetailOutput struct {
	Items []cliclient.FlaggedItem `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Pages int                     `json:"pages"`
}

type smellsApplyOutput struct {
	Applied    int         `json:"applied,omitempty"`
	Skipped    int         `json:"skipped,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type smellsMutateOutput struct {
	Ignored    int         `json:"ignored,omitempty"`
	Restored   int         `json:"restored,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type smellsIgnoredOutput struct {
	Items []cliclient.IgnoredItem `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Pages int                     `json:"pages"`
}

// resolveSmellRefIDs resolves refs to user-game ids, returning candidates (no
// mutation) when any ref is ambiguous or unmatched.
func resolveSmellRefIDs(c *cliclient.Client, key string, refs []string) (ids []string, cands []gameBrief, msg string, err error) {
	games, cands, msg, err := mcpResolveEditTargets(c, key, refs, nil)
	if err != nil || cands != nil || msg != "" {
		return nil, cands, msg, err
	}
	ids = make([]string, len(games))
	for i := range games {
		ids[i] = games[i].ID
	}
	return ids, nil, "", nil
}

func registerDoctorTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_list", Description: "List every library-health check with its tier, whether it is auto-fixable, and how many games it currently flags."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, smellsListOutput, error) {
			checks, err := c.ListSmells(key)
			if err != nil {
				return nil, smellsListOutput{}, mcpToolError("library_smells_list", err)
			}
			return nil, smellsListOutput{Checks: checks}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_detail", Description: "List the flagged games for one check. suggested_status (auto-fix checks) or detail describes each finding."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsDetailInput) (*mcp.CallToolResult, smellsDetailOutput, error) {
			res, err := c.ListSmellItems(key, in.CheckID, in.Page, in.PerPage)
			if err != nil {
				return nil, smellsDetailOutput{}, mcpToolError("library_smells_detail", err)
			}
			return nil, smellsDetailOutput{Items: res.Items, Total: res.Total, Page: res.Page, Pages: res.Pages}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_apply", Description: "Apply an auto-fixable check's suggestion. With refs, fixes those games; with no refs, fixes every game the check flags. Ambiguous refs return candidates — call again with an id. Non-fixable checks return an error."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsApplyOutput, error) {
			var ids []string
			if len(in.Refs) == 0 {
				var err error
				ids, err = collectFlaggedIDs(c, key, in.CheckID)
				if err != nil {
					return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
				}
				if len(ids) == 0 {
					return nil, smellsApplyOutput{Message: "no games flagged by this check"}, nil
				}
			} else {
				resolved, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
				if err != nil {
					return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
				}
				if cands != nil || msg != "" {
					return nil, smellsApplyOutput{Candidates: cands, Message: msg}, nil
				}
				ids = resolved
			}
			res, err := c.ApplySmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
			}
			return nil, smellsApplyOutput{Applied: res.Applied, Skipped: res.Skipped,
				Message: fmt.Sprintf("applied %d, skipped %d", res.Applied, res.Skipped)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_ignore", Description: "Dismiss flagged games for a check so they stop appearing. Ambiguous refs return candidates — call again with an id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsMutateOutput, error) {
			ids, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_ignore", err)
			}
			if cands != nil || msg != "" {
				return nil, smellsMutateOutput{Candidates: cands, Message: msg}, nil
			}
			n, err := c.IgnoreSmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_ignore", err)
			}
			return nil, smellsMutateOutput{Ignored: n, Message: fmt.Sprintf("dismissed %d game(s)", n)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_restore", Description: "Un-dismiss previously ignored games for a check. Ambiguous refs return candidates — call again with an id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsMutateOutput, error) {
			ids, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_restore", err)
			}
			if cands != nil || msg != "" {
				return nil, smellsMutateOutput{Candidates: cands, Message: msg}, nil
			}
			n, err := c.RestoreSmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_restore", err)
			}
			return nil, smellsMutateOutput{Restored: n, Message: fmt.Sprintf("restored %d game(s)", n)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_ignored", Description: "List the games currently dismissed for one check."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsDetailInput) (*mcp.CallToolResult, smellsIgnoredOutput, error) {
			res, err := c.ListIgnoredSmells(key, in.CheckID, in.Page, in.PerPage)
			if err != nil {
				return nil, smellsIgnoredOutput{}, mcpToolError("library_smells_ignored", err)
			}
			return nil, smellsIgnoredOutput{Items: res.Items, Total: res.Total, Page: res.Page, Pages: res.Pages}, nil
		})
}
```

Modify `cmd/nexctl/mcp.go` — register inside `buildMCPServer`, after `registerSyncTools(srv, c, p.Key)`:

```go
	registerSyncTools(srv, c, p.Key)
	registerDoctorTools(srv, c, p.Key)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/ -run 'TestMCPSmells' -v`
Expected: PASS (`TestMCPSmellsListAndDetail`, `TestMCPSmellsApplyByRef`, `TestMCPSmellsIgnoreRestore`).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/mcp_doctor.go cmd/nexctl/mcp.go cmd/nexctl/mcp_doctor_test.go
git commit -m "feat: MCP library_smells_* tools"
```

---

### Task 6: Documentation + full verification

**Files:**
- Modify: `CLAUDE.md` (extend the `cmd/nexctl/` command-tree description)

- [ ] **Step 1: Update CLAUDE.md**

In the `cmd/nexctl/` bullet under "Project Structure", add `doctor` to the command list and append a sentence describing it. Insert after the existing `mcp` clause description, mirroring the existing dense style. Add to the command-list enumeration near the top of that bullet:

```
…, `mcp` (config/serve), `doctor` (library-health smells: summary/--check/apply/ignore/restore/ignored), `setup` …
```

And append a description sentence at the end of the bullet:

```
`doctor` is the CLI/MCP front-end over the library-smells REST API (`/api/library/smells`, engine in `internal/librarysmells`): default summary table (id/check/tier/fixable/count), `--check <id>` lists flagged items, `apply <check> [refs…]` fixes auto-fixable checks (no refs = all flagged; confirms unless `-y`; server 422s a non-fixable check), and `ignore`/`restore`/`ignored <check>` manage per-item dismissals. Refs resolve via the shared `resolveUserGameRef`/`findUserGamesByRef`. The MCP mirror is `library_smells_list/detail/apply/ignore/restore/ignored` (full parity), built on the same `cliclient` methods + `mcpResolveEditTargets`; ambiguous refs return a `candidates` list with no mutation. Registry is **9 checks, 3 auto-fixable** (`beat-but-not-marked` was removed in the #1145 follow-up) — fixability is read from the API's `auto_fixable`, never hardcoded.
```

- [ ] **Step 2: Run the full nexctl + cliclient suites**

Run: `go test ./cmd/nexctl/... ./internal/cliclient/... -v`
Expected: PASS (all doctor + MCP + client tests, plus the pre-existing suites unchanged).

- [ ] **Step 3: Build both binaries and run deadcode**

Run:
```bash
go build ./...
go run golang.org/x/tools/cmd/deadcode@latest -test ./... | grep -iE 'doctor|smell' || echo "no new doctor/smell deadcode"
```
Expected: build succeeds; deadcode reports no new reachable-but-unused exported symbols from this change. (`collectFlaggedIDs` is used by both `doctor apply` and the MCP apply tool; every `cliclient` smells method has a consumer.)

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document nexctl doctor + library_smells MCP tools"
```

---

## Self-Review Notes

- **Spec coverage (issue #1146):** CLI summary (Task 2), `--check` drill (Task 2), `apply <check> [refs…]` with `-y` (Task 3), `ignore`/`restore` + dismissed listing (Task 4) ✓. MCP `library_smells_list/detail/apply/ignore` + the agreed `restore`/`ignored` for full parity (Task 5) ✓. Bare-string `jsonschema` tags, concise JSON projection, `mcpToolError` 403 surfacing, shared cmd-free finders, MCP-only dependency, in-process `NewInMemoryTransports` test (Task 5) ✓.
- **#1145 reshape:** auto-fixable set is 3, not 4; fixability is read from `auto_fixable`/422, never hardcoded (Global Constraints, Task 3, Task 6) ✓.
- **Type consistency:** `collectFlaggedIDs` (Task 2) reused verbatim in Task 5; `resolveSmellRefIDs` wraps the existing `mcpResolveEditTargets`; client method names match across the CLI and MCP consumers.
- **No back-compat shims** needed (solo user) — pure addition.
