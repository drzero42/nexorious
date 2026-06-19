# `nexctl` Phase 3 — `pool` + `tag` Command Groups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `nexctl tag` (list/create/rename/rm) and `nexctl pool` (list/show/create/edit/rm/add/remove/queue/reorder) command groups over the tags/pools REST API.

**Architecture:** Add ~11 `internal/cliclient` methods behind the existing `doBearer` helper, then `cmd/nexctl/{tag,pool}*.go` commands. Reuse Phase 2's `resolveUserGameRef` (game refs → user-game ids), `cliui`, `resolveProfile`, `flagBool`, `interactive`, and the table/json/-q output idiom.

**Tech Stack:** Go 1.26, `spf13/cobra`, stdlib `net/http`/`encoding/json`/`net/http/httptest`, `internal/cliui`/`cliclient`/`clicfg`.

## Global Constraints

- Module `github.com/drzero42/nexorious`.
- Import boundary: `cmd/nexctl` + `cliclient` import only stdlib + cobra + `internal/clicfg`/`cliclient`/`cliui` — NO server/DB packages.
- All `/api/tags`, `/api/pools` endpoints require `Authorization: Bearer <key>`; commands get it via `resolveProfile(cmd)`.
- errcheck check-blank on non-test code; never blank-discard a `cliui.Confirm` error (`ok, err := …; if err != nil { return err }`). `fmt.Fprint*` to `out` is allowlisted.
- Output: human table/detail default; `--json` via `cliui.EncodeJSON`; `-q` bare ids. Confirms on destructive ops (`tag rm`, `pool rm`) unless `-y`.
- Pools reference games by **user_game_id**; the pool `filter` is JSONB `{"filters":[{…}]}` passed through as a raw JSON string.
- TDD: failing test → see it fail → minimal impl → pass → commit. Frequent commits.

---

## File Structure

- `internal/cliclient/client.go` (modify) — extend `Tag` with `GameCount`; add tag + pool methods and pool types.
- `internal/cliclient/pools_test.go` (create) — httptest tests for the new methods.
- `cmd/nexctl/tag.go` (create) — `newTagCmd` + `resolveTagRef` + list/create/rename/rm.
- `cmd/nexctl/pool.go` (create) — `newPoolCmd` + `resolvePoolRef` + render helpers + list/show.
- `cmd/nexctl/pool_mutate.go` (create) — create/edit/rm.
- `cmd/nexctl/pool_games.go` (create) — add/remove/queue/reorder.
- `cmd/nexctl/{tag,pool}*_test.go` (create) — per-command tests via `newRootCmd()` + httptest.
- `cmd/nexctl/main.go` (modify) — register `newTagCmd()`, `newPoolCmd()`.
- `cmd/nexctl/main_test.go` (modify) — add `"tag"`, `"pool"` to the `want` map.
- `CLAUDE.md` (modify) — note the tag/pool groups.

---

## Task 1: cliclient — tag mutations + `Tag.GameCount`

**Files:** Modify `internal/cliclient/client.go`; Test `internal/cliclient/pools_test.go`.

**Interfaces — Produces:**
- Extend `Tag` with `GameCount int64 json:"game_count"`.
- `func (c *Client) CreateTag(key, name string, color *string) (*Tag, error)` — POST /api/tags
- `func (c *Client) UpdateTag(key, id string, name, color *string) (*Tag, error)` — PUT /api/tags/:id (only non-nil fields sent)
- `func (c *Client) DeleteTag(key, id string) error` — DELETE /api/tags/:id

- [ ] **Step 1: Write the failing test**

```go
package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndDeleteTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "RPG" {
			t.Errorf("name = %v", body["name"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "name": "RPG"})
	})
	mux.HandleFunc("/api/tags/t-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	tag, err := c.CreateTag("k", "RPG", nil)
	if err != nil || tag.ID != "t-1" {
		t.Fatalf("CreateTag: %v %+v", err, tag)
	}
	if err := c.DeleteTag("k", "t-1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/cliclient/... -run TestCreateAndDeleteTag` → FAIL (undefined).

- [ ] **Step 3: Write minimal implementation**

Add `GameCount int64 \`json:"game_count"\`` to the `Tag` struct, then append:

```go
// CreateTag creates a tag with an optional color.
func (c *Client) CreateTag(key, name string, color *string) (*Tag, error) {
	body := map[string]any{"name": name}
	if color != nil {
		body["color"] = *color
	}
	var out Tag
	if err := c.doBearer(http.MethodPost, "/api/tags", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTag updates a tag's name and/or color (only non-nil fields are sent).
func (c *Client) UpdateTag(key, id string, name, color *string) (*Tag, error) {
	body := map[string]any{}
	if name != nil {
		body["name"] = *name
	}
	if color != nil {
		body["color"] = *color
	}
	var out Tag
	if err := c.doBearer(http.MethodPut, "/api/tags/"+url.PathEscape(id), key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTag removes a tag.
func (c *Client) DeleteTag(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/tags/"+url.PathEscape(id), key, nil, nil)
}
```

- [ ] **Step 4: Run test to verify it passes** — `go test ./internal/cliclient/...` → PASS.
- [ ] **Step 5: Commit** — `git add internal/cliclient/ && git commit -m "feat: add cliclient tag create/update/delete methods"`

---

## Task 2: cliclient — pool types + read methods

**Files:** Modify `internal/cliclient/client.go`; Test `internal/cliclient/pools_test.go`.

**Interfaces — Consumes:** `doBearer`, `UserGame`. **Produces:**
- `type Pool struct { ID, Name string; Color *string; Position int; Filter json.RawMessage; HasFilter bool }`
- `type PoolListItem struct { ID, Name string; Color *string; Position int; HasFilter bool; QueueCount, CandidateCount int64 }`
- `type PoolDetail struct { Pool; Queue []UserGame; Candidates []UserGame }`
- `func (c *Client) ListPools(key string) ([]PoolListItem, error)` — GET /api/pools
- `func (c *Client) GetPool(key, id string) (*PoolDetail, error)` — GET /api/pools/:id

Note: `client.go` already imports `encoding/json`.

- [ ] **Step 1: Write the failing test**

```go
func TestListAndGetPool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "p-1", "name": "Backlog", "position": 0, "has_filter": true, "queue_count": 2, "candidate_count": 5},
		})
	})
	mux.HandleFunc("/api/pools/p-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "p-1", "name": "Backlog", "position": 0, "has_filter": false,
			"queue":      []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "A"}}},
			"candidates": []map[string]any{{"id": "ug-2", "game": map[string]any{"title": "B"}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	pools, err := c.ListPools("k")
	if err != nil || len(pools) != 1 || pools[0].Name != "Backlog" || pools[0].QueueCount != 2 {
		t.Fatalf("ListPools: %v %+v", err, pools)
	}
	d, err := c.GetPool("k", "p-1")
	if err != nil || len(d.Queue) != 1 || d.Queue[0].Title() != "A" || len(d.Candidates) != 1 {
		t.Fatalf("GetPool: %v %+v", err, d)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/cliclient/... -run TestListAndGetPool` → FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
// Pool is a play-planning pool's metadata.
type Pool struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Color     *string         `json:"color"`
	Position  int             `json:"position"`
	Filter    json.RawMessage `json:"filter"`
	HasFilter bool            `json:"has_filter"`
}

// PoolListItem is one row of the pool list.
type PoolListItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Color          *string `json:"color"`
	Position       int     `json:"position"`
	HasFilter      bool    `json:"has_filter"`
	QueueCount     int64   `json:"queue_count"`
	CandidateCount int64   `json:"candidate_count"`
}

// PoolDetail is a pool plus its ordered queue and candidate user-games.
type PoolDetail struct {
	Pool
	Queue      []UserGame `json:"queue"`
	Candidates []UserGame `json:"candidates"`
}

// ListPools returns the caller's pools ordered by position.
func (c *Client) ListPools(key string) ([]PoolListItem, error) {
	var out []PoolListItem
	if err := c.doBearer(http.MethodGet, "/api/pools", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPool returns a pool with its queue and candidates.
func (c *Client) GetPool(key, id string) (*PoolDetail, error) {
	var out PoolDetail
	if err := c.doBearer(http.MethodGet, "/api/pools/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

- [ ] **Step 4: Run test to verify it passes** — `go test ./internal/cliclient/...` → PASS.
- [ ] **Step 5: Commit** — `git add internal/cliclient/ && git commit -m "feat: add cliclient pool read methods and types"`

---

## Task 3: cliclient — pool mutation methods

**Files:** Modify `internal/cliclient/client.go`; Test `internal/cliclient/pools_test.go`.

**Interfaces — Consumes:** `doBearer`, `Pool`. **Produces:**
- `func (c *Client) CreatePool(key, name string, color *string, filter json.RawMessage) (*Pool, error)` — POST /api/pools
- `func (c *Client) UpdatePool(key, id string, fields map[string]any) (*Pool, error)` — PUT /api/pools/:id
- `func (c *Client) DeletePool(key, id string) error` — DELETE /api/pools/:id
- `func (c *Client) AddPoolGame(key, poolID, userGameID string) error` — POST /api/pools/:id/games {user_game_id}
- `func (c *Client) BulkAddPoolGames(key, poolID string, userGameIDs []string) (int64, error)` — POST /api/pools/:id/games/bulk {user_game_ids}
- `func (c *Client) RemovePoolGame(key, poolID, userGameID string) error` — DELETE /api/pools/:id/games/:ugid
- `func (c *Client) SetQueue(key, poolID string, userGameIDs []string) error` — PUT /api/pools/:id/queue {ids}
- `func (c *Client) ReorderPools(key string, poolIDs []string) error` — POST /api/pools/reorder {ids}

- [ ] **Step 1: Write the failing test**

```go
func TestPoolMutations(t *testing.T) {
	var queueIDs []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "p-9", "name": "New"})
	})
	mux.HandleFunc("/api/pools/p-9/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		_ = json.NewEncoder(w).Encode(map[string]any{"added": len(b["user_game_ids"].([]any))})
	})
	mux.HandleFunc("/api/pools/p-9/queue", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		queueIDs = b["ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	p, err := c.CreatePool("k", "New", nil, nil)
	if err != nil || p.ID != "p-9" {
		t.Fatalf("CreatePool: %v %+v", err, p)
	}
	n, err := c.BulkAddPoolGames("k", "p-9", []string{"ug-1", "ug-2"})
	if err != nil || n != 2 {
		t.Fatalf("BulkAddPoolGames: %v %d", err, n)
	}
	if err := c.SetQueue("k", "p-9", []string{"ug-2", "ug-1"}); err != nil {
		t.Fatalf("SetQueue: %v", err)
	}
	if len(queueIDs) != 2 || queueIDs[0] != "ug-2" {
		t.Fatalf("queue order = %v", queueIDs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./internal/cliclient/... -run TestPoolMutations` → FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
// CreatePool creates a pool with optional color and filter (raw JSON, server-validated).
func (c *Client) CreatePool(key, name string, color *string, filter json.RawMessage) (*Pool, error) {
	body := map[string]any{"name": name}
	if color != nil {
		body["color"] = *color
	}
	if filter != nil {
		body["filter"] = filter
	}
	var out Pool
	if err := c.doBearer(http.MethodPost, "/api/pools", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePool applies a partial update (name/color/filter).
func (c *Client) UpdatePool(key, id string, fields map[string]any) (*Pool, error) {
	var out Pool
	if err := c.doBearer(http.MethodPut, "/api/pools/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePool removes a pool.
func (c *Client) DeletePool(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/pools/"+url.PathEscape(id), key, nil, nil)
}

// AddPoolGame adds one game (as a candidate) to a pool.
func (c *Client) AddPoolGame(key, poolID, userGameID string) error {
	body := map[string]any{"user_game_id": userGameID}
	return c.doBearer(http.MethodPost, "/api/pools/"+url.PathEscape(poolID)+"/games", key, body, nil)
}

// BulkAddPoolGames adds multiple games (as candidates); returns the count newly inserted.
func (c *Client) BulkAddPoolGames(key, poolID string, userGameIDs []string) (int64, error) {
	body := map[string]any{"user_game_ids": userGameIDs}
	var out struct {
		Added int64 `json:"added"`
	}
	if err := c.doBearer(http.MethodPost, "/api/pools/"+url.PathEscape(poolID)+"/games/bulk", key, body, &out); err != nil {
		return 0, err
	}
	return out.Added, nil
}

// RemovePoolGame removes a game from a pool.
func (c *Client) RemovePoolGame(key, poolID, userGameID string) error {
	return c.doBearer(http.MethodDelete, "/api/pools/"+url.PathEscape(poolID)+"/games/"+url.PathEscape(userGameID), key, nil, nil)
}

// SetQueue declaratively sets the pool's ordered queue (ids must already be members;
// an empty slice clears the queue). Members not listed become candidates.
func (c *Client) SetQueue(key, poolID string, userGameIDs []string) error {
	body := map[string]any{"ids": userGameIDs}
	return c.doBearer(http.MethodPut, "/api/pools/"+url.PathEscape(poolID)+"/queue", key, body, nil)
}

// ReorderPools sets pool positions by the given order.
func (c *Client) ReorderPools(key string, poolIDs []string) error {
	body := map[string]any{"ids": poolIDs}
	return c.doBearer(http.MethodPost, "/api/pools/reorder", key, body, nil)
}
```

- [ ] **Step 4: Run test to verify it passes** — `go test ./internal/cliclient/...` → PASS.
- [ ] **Step 5: Commit** — `git add internal/cliclient/ && git commit -m "feat: add cliclient pool mutation methods"`

---

## Task 4: nexctl `tag` group

**Files:** Create `cmd/nexctl/tag.go`, `cmd/nexctl/tag_test.go`; Modify `cmd/nexctl/main.go`, `cmd/nexctl/main_test.go`.

**Interfaces — Produces:** `func newTagCmd() *cobra.Command`; `func resolveTagRef(c *cliclient.Client, key, ref string) (*cliclient.Tag, error)`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTagListAndCreate(t *testing.T) {
	var created bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			created = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-2", "name": "Co-op"})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "t-1", "name": "RPG", "game_count": 7}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"tag", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tag list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("RPG")) || !bytes.Contains(out.Bytes(), []byte("7")) {
		t.Fatalf("list = %s", out.String())
	}

	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"tag", "create", "Co-op"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tag create: %v", err)
	}
	if !created {
		t.Fatal("create not called")
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./cmd/nexctl/... -run TestTag` → FAIL.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/tag.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newTagCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "Manage your tags"}
	cmd.AddCommand(newTagListCmd(), newTagCreateCmd(), newTagRenameCmd(), newTagRmCmd())
	return cmd
}

// resolveTagRef resolves a tag by id (UUID) or name (case-insensitive).
func resolveTagRef(c *cliclient.Client, key, ref string) (*cliclient.Tag, error) {
	if looksLikeUUID(ref) {
		// No single-tag GET endpoint; fetch the list and match by id.
		tags, err := c.ListTags(key)
		if err != nil {
			return nil, err
		}
		for i := range tags {
			if tags[i].ID == ref {
				return &tags[i], nil
			}
		}
		return nil, fmt.Errorf("no tag with id %s", ref)
	}
	tags, err := c.ListTags(key)
	if err != nil {
		return nil, err
	}
	for i := range tags {
		if strings.EqualFold(tags[i].Name, ref) {
			return &tags[i], nil
		}
	}
	return nil, fmt.Errorf("no tag named %q", ref)
}

func newTagListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your tags",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			tags, err := cliclient.New(p.URL).ListTags(p.Key)
			if err != nil {
				return fmt.Errorf("list tags failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, tags)
			}
			if flagBool(cmd, "quiet") {
				for i := range tags {
					fmt.Fprintln(out, tags[i].ID)
				}
				return nil
			}
			if len(tags) == 0 {
				fmt.Fprintln(out, "No tags.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tCOLOR\tGAMES")
			for i := range tags {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", tags[i].ID, tags[i].Name, deref(tags[i].Color), tags[i].GameCount)
			}
			return tw.Flush()
		},
	}
}

func newTagCreateCmd() *cobra.Command {
	var color string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			var colorPtr *string
			if cmd.Flags().Changed("color") {
				colorPtr = &color
			}
			tag, err := cliclient.New(p.URL).CreateTag(p.Key, args[0], colorPtr)
			if err != nil {
				return fmt.Errorf("create tag failed: %w", err)
			}
			fmt.Fprintf(out, "Created tag %q (%s).\n", tag.Name, tag.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "Tag color (e.g. #6B7280)")
	return cmd
}

func newTagRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <ref> <new-name>",
		Short: "Rename a tag",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			tag, err := resolveTagRef(c, p.Key, args[0])
			if err != nil {
				return err
			}
			newName := args[1]
			if _, err := c.UpdateTag(p.Key, tag.ID, &newName, nil); err != nil {
				return fmt.Errorf("rename tag failed: %w", err)
			}
			fmt.Fprintf(out, "Renamed %q to %q.\n", tag.Name, newName)
			return nil
		},
	}
}

func newTagRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <ref>",
		Short: "Delete a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			tag, err := resolveTagRef(c, p.Key, args[0])
			if err != nil {
				return err
			}
			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete tag %q?", tag.Name), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			if err := c.DeleteTag(p.Key, tag.ID); err != nil {
				return fmt.Errorf("delete tag failed: %w", err)
			}
			fmt.Fprintf(out, "Deleted tag %q.\n", tag.Name)
			return nil
		},
	}
}

// ensure strconv is used (GameCount printed via %d uses fmt; keep import only if referenced).
var _ = strconv.Itoa
```

Note: the `var _ = strconv.Itoa` guard is a placeholder — if `strconv` ends up unused after writing, **remove the import and this line** rather than keeping a noop. (GameCount is printed with `%d`, so `strconv` is likely unneeded; drop it.)

Register: in `cmd/nexctl/main.go` `newRootCmd`, add `root.AddCommand(newTagCmd())`. Add `"tag"` to the `want` map in `main_test.go`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./cmd/nexctl/...` → PASS.
- [ ] **Step 5: Commit** — `git add cmd/nexctl/ && git commit -m "feat: add nexctl tag group"`

---

## Task 5: nexctl `pool` parent + list + show

**Files:** Create `cmd/nexctl/pool.go`, `cmd/nexctl/pool_test.go`; Modify `cmd/nexctl/main.go`, `cmd/nexctl/main_test.go`.

**Interfaces — Produces:** `func newPoolCmd() *cobra.Command`; `func resolvePoolRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.PoolListItem, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestPoolListAndShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "p-1", "name": "Backlog", "position": 0, "has_filter": true, "queue_count": 1, "candidate_count": 2},
		})
	})
	mux.HandleFunc("/api/pools/p-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "p-1", "name": "Backlog", "position": 0,
			"queue":      []map[string]any{{"id": "ug-1", "play_status": "in_progress", "game": map[string]any{"title": "Celeste"}}},
			"candidates": []map[string]any{{"id": "ug-2", "game": map[string]any{"title": "Hades"}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	// list
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Backlog")) {
		t.Fatalf("list = %s", out.String())
	}

	// show by name → resolves via list, then GET detail
	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "show", "Backlog"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool show: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Celeste", "Hades", "QUEUE", "CANDIDATES"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("show missing %q: %s", want, out.String())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./cmd/nexctl/... -run TestPoolListAndShow` → FAIL.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/pool.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newPoolCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pool", Short: "Manage play-planning pools"}
	cmd.AddCommand(newPoolListCmd(), newPoolShowCmd())
	// create/edit/rm (Task 6) and add/remove/queue/reorder (Task 8) are added there.
	return cmd
}

// resolvePoolRef resolves a pool by id (UUID) or name (case-insensitive) via the
// pool list. Many matches prompt a TTY picker or, off-TTY, error with candidates.
func resolvePoolRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.PoolListItem, error) {
	pools, err := c.ListPools(key)
	if err != nil {
		return nil, err
	}
	if looksLikeUUID(ref) {
		for i := range pools {
			if pools[i].ID == ref {
				return &pools[i], nil
			}
		}
		return nil, fmt.Errorf("no pool with id %s", ref)
	}
	var matches []*cliclient.PoolListItem
	for i := range pools {
		if strings.EqualFold(pools[i].Name, ref) {
			matches = append(matches, &pools[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no pool named %q", ref)
	case 1:
		return matches[0], nil
	}
	if interactive(cmd) {
		return pickPool(cmd, matches)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d pools; re-run with an id:", ref, len(matches))
	for _, m := range matches {
		fmt.Fprintf(&b, "\n  %s  %s", m.ID, m.Name)
	}
	return nil, fmt.Errorf("%s", b.String())
}

func pickPool(cmd *cobra.Command, pools []*cliclient.PoolListItem) (*cliclient.PoolListItem, error) {
	out := cmd.OutOrStdout()
	for i, p := range pools {
		fmt.Fprintf(out, "%2d) %s\n", i+1, p.Name)
	}
	fmt.Fprint(out, "Select a pool [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return pools[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(pools) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return pools[n-1], nil
}

func newPoolListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your pools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			pools, err := cliclient.New(p.URL).ListPools(p.Key)
			if err != nil {
				return fmt.Errorf("list pools failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, pools)
			}
			if flagBool(cmd, "quiet") {
				for i := range pools {
					fmt.Fprintln(out, pools[i].ID)
				}
				return nil
			}
			if len(pools) == 0 {
				fmt.Fprintln(out, "No pools.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tPOS\tQUEUE\tCANDIDATES\tFILTER")
			for i := range pools {
				pl := &pools[i]
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%t\n", pl.ID, pl.Name, pl.Position, pl.QueueCount, pl.CandidateCount, pl.HasFilter)
			}
			return tw.Flush()
		},
	}
}

func newPoolShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show a pool's queue and candidates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ref, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			detail, err := c.GetPool(p.Key, ref.ID)
			if err != nil {
				return fmt.Errorf("get pool failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, detail)
			}
			fmt.Fprintf(out, "%s\n  id:     %s\n  filter: %t\n\n", detail.Name, detail.ID, detail.HasFilter)
			fmt.Fprintln(out, "QUEUE:")
			if len(detail.Queue) == 0 {
				fmt.Fprintln(out, "  (empty)")
			}
			for i := range detail.Queue {
				u := &detail.Queue[i]
				fmt.Fprintf(out, "  %2d. %s  [%s]\n", i+1, u.Title(), statusOf(u))
			}
			fmt.Fprintln(out, "\nCANDIDATES:")
			if len(detail.Candidates) == 0 {
				fmt.Fprintln(out, "  (none)")
			}
			for i := range detail.Candidates {
				u := &detail.Candidates[i]
				fmt.Fprintf(out, "  - %s  [%s]\n", u.Title(), statusOf(u))
			}
			return nil
		},
	}
}
```

Register: in `main.go` add `root.AddCommand(newPoolCmd())`; add `"pool"` to the `want` map in `main_test.go`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./cmd/nexctl/...` → PASS.
- [ ] **Step 5: Commit** — `git add cmd/nexctl/ && git commit -m "feat: add nexctl pool list and show"`

---

## Task 6: nexctl `pool create` / `edit` / `rm`

**Files:** Create `cmd/nexctl/pool_mutate.go`, `cmd/nexctl/pool_mutate_test.go`.

**Interfaces — Consumes:** `resolvePoolRef`, `cliclient.{CreatePool,UpdatePool,DeletePool}`, `cliui.Confirm`. **Produces:** `newPoolCreateCmd`/`newPoolEditCmd`/`newPoolRmCmd` (added to `newPoolCmd`).

- [ ] **Step 1: Write the failing test**

```go
func TestPoolCreateWithFilter(t *testing.T) {
	var gotFilter any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotFilter = b["filter"]
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "p-1", "name": "RPGs"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "create", "RPGs", "--filter", `{"filters":[{"genre":["RPG"]}]}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool create: %v\n%s", err, out.String())
	}
	if gotFilter == nil {
		t.Fatal("filter not forwarded")
	}
}

func TestPoolCreateInvalidFilterJSON(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"pool", "create", "X", "--filter", "{not json"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected invalid --filter JSON error before any network call")
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./cmd/nexctl/... -run TestPoolCreate` → FAIL.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/pool_mutate.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// parseFilterFlag validates a --filter JSON string client-side (fail fast before
// any network call) and returns it as raw JSON.
func parseFilterFlag(s string) (json.RawMessage, error) {
	if s == "" {
		return nil, nil
	}
	if !json.Valid([]byte(s)) {
		return nil, fmt.Errorf("--filter is not valid JSON")
	}
	return json.RawMessage(s), nil
}

func newPoolCreateCmd() *cobra.Command {
	var color, filter string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			raw, err := parseFilterFlag(filter)
			if err != nil {
				return err
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			var colorPtr *string
			if cmd.Flags().Changed("color") {
				colorPtr = &color
			}
			pool, err := cliclient.New(p.URL).CreatePool(p.Key, args[0], colorPtr, raw)
			if err != nil {
				return fmt.Errorf("create pool failed: %w", err)
			}
			fmt.Fprintf(out, "Created pool %q (%s).\n", pool.Name, pool.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "Pool color")
	cmd.Flags().StringVar(&filter, "filter", "", `Saved filter as JSON, e.g. {"filters":[{"genre":["RPG"]}]}`)
	return cmd
}

func newPoolEditCmd() *cobra.Command {
	var name, color, filter string
	var clearFilter bool
	cmd := &cobra.Command{
		Use:   "edit <ref>",
		Short: "Edit a pool (name/color/filter)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ch := cmd.Flags().Changed
			fields := map[string]any{}
			if ch("name") {
				fields["name"] = name
			}
			if ch("color") {
				fields["color"] = color
			}
			if clearFilter {
				fields["filter"] = nil
			} else if ch("filter") {
				raw, err := parseFilterFlag(filter)
				if err != nil {
					return err
				}
				fields["filter"] = raw
			}
			if len(fields) == 0 {
				return fmt.Errorf("nothing to change; pass --name/--color/--filter/--clear-filter")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ref, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			if _, err := c.UpdatePool(p.Key, ref.ID, fields); err != nil {
				return fmt.Errorf("edit pool failed: %w", err)
			}
			fmt.Fprintf(out, "Updated pool %q.\n", ref.Name)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "New name")
	f.StringVar(&color, "color", "", "New color")
	f.StringVar(&filter, "filter", "", "New saved filter as JSON")
	f.BoolVar(&clearFilter, "clear-filter", false, "Remove the saved filter")
	return cmd
}

func newPoolRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <ref>",
		Short: "Delete a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ref, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete pool %q?", ref.Name), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			if err := c.DeletePool(p.Key, ref.ID); err != nil {
				return fmt.Errorf("delete pool failed: %w", err)
			}
			fmt.Fprintf(out, "Deleted pool %q.\n", ref.Name)
			return nil
		},
	}
}
```

Register: add `newPoolCreateCmd(), newPoolEditCmd(), newPoolRmCmd()` to `newPoolCmd`'s `AddCommand` in `pool.go`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./cmd/nexctl/...` → PASS.
- [ ] **Step 5: Commit** — `git add cmd/nexctl/ && git commit -m "feat: add nexctl pool create/edit/rm"`

---

## Task 7: nexctl `pool add` / `remove`

**Files:** Create `cmd/nexctl/pool_games.go`, `cmd/nexctl/pool_games_test.go`.

**Interfaces — Consumes:** `resolvePoolRef`, `resolveUserGameRef`, `cliclient.{AddPoolGame,BulkAddPoolGames,RemovePoolGame}`. **Produces:** `newPoolAddCmd`/`newPoolRemoveCmd` + a shared `resolveGameIDs` helper (reused by queue in Task 8).

- [ ] **Step 1: Write the failing test**

```go
func TestPoolAddBulk(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var added int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "p-1", "name": "Backlog"}})
	})
	// game refs are UUIDs → resolveUserGameRef does GET /api/user-games/<id>
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": r.URL.Path[len("/api/user-games/"):], "game": map[string]any{"title": "X"}})
	})
	mux.HandleFunc("/api/pools/p-1/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		added = len(b["user_game_ids"].([]any))
		_ = json.NewEncoder(w).Encode(map[string]any{"added": added})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "add", "Backlog", id, "223e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool add: %v\n%s", err, out.String())
	}
	if added != 2 {
		t.Fatalf("added = %d", added)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./cmd/nexctl/... -run TestPoolAdd` → FAIL.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/pool_games.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// resolveGameIDs resolves each game ref to a user-game id (preserving order).
func resolveGameIDs(cmd *cobra.Command, c *cliclient.Client, key string, refs []string) ([]string, error) {
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

func newPoolAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <pool-ref> <game-ref…>",
		Short: "Add games to a pool (as candidates)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			pool, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ids, err := resolveGameIDs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			if len(ids) == 1 {
				if err := c.AddPoolGame(p.Key, pool.ID, ids[0]); err != nil {
					return fmt.Errorf("add to pool failed: %w", err)
				}
				fmt.Fprintf(out, "Added 1 game to %q.\n", pool.Name)
				return nil
			}
			n, err := c.BulkAddPoolGames(p.Key, pool.ID, ids)
			if err != nil {
				return fmt.Errorf("add to pool failed: %w", err)
			}
			fmt.Fprintf(out, "Added %d game(s) to %q.\n", n, pool.Name)
			return nil
		},
	}
}

func newPoolRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <pool-ref> <game-ref…>",
		Short: "Remove games from a pool",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			pool, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ids, err := resolveGameIDs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := c.RemovePoolGame(p.Key, pool.ID, id); err != nil {
					return fmt.Errorf("remove %s failed: %w", id, err)
				}
			}
			fmt.Fprintf(out, "Removed %d game(s) from %q.\n", len(ids), pool.Name)
			return nil
		},
	}
}
```

Register: add `newPoolAddCmd(), newPoolRemoveCmd()` to `newPoolCmd`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./cmd/nexctl/...` → PASS.
- [ ] **Step 5: Commit** — `git add cmd/nexctl/ && git commit -m "feat: add nexctl pool add/remove"`

---

## Task 8: nexctl `pool queue` / `reorder`

**Files:** Modify `cmd/nexctl/pool_games.go`; Test `cmd/nexctl/pool_queue_test.go`.

**Interfaces — Consumes:** `resolvePoolRef`, `resolveGameIDs`, `cliclient.{BulkAddPoolGames,SetQueue,ReorderPools}`. **Produces:** `newPoolQueueCmd`/`newPoolReorderCmd`.

Behaviour: `queue <pool> [game…]` resolves the pool + game ids, **bulk-adds** them first (idempotent; ensures membership), then `SetQueue(ids)` in order. No game refs → `SetQueue([])` clears. `reorder <pool…>` resolves each pool ref → `ReorderPools(ids)`.

- [ ] **Step 1: Write the failing test**

```go
func TestPoolQueueAddsThenOrders(t *testing.T) {
	const a = "123e4567-e89b-12d3-a456-426614174000"
	const b = "223e4567-e89b-12d3-a456-426614174000"
	var bulkIDs, queueIDs []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "p-1", "name": "Backlog"}})
	})
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": r.URL.Path[len("/api/user-games/"):], "game": map[string]any{"title": "X"}})
	})
	mux.HandleFunc("/api/pools/p-1/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bulkIDs = body["user_game_ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"added": len(bulkIDs)})
	})
	mux.HandleFunc("/api/pools/p-1/queue", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		queueIDs = body["ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "queue", "Backlog", a, b})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool queue: %v\n%s", err, out.String())
	}
	if len(bulkIDs) != 2 || len(queueIDs) != 2 || queueIDs[0] != a {
		t.Fatalf("bulk=%v queue=%v", bulkIDs, queueIDs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails** — `go test ./cmd/nexctl/... -run TestPoolQueue` → FAIL.

- [ ] **Step 3: Write minimal implementation** — append to `cmd/nexctl/pool_games.go`:

```go
func newPoolQueueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "queue <pool-ref> [game-ref…]",
		Short: "Set a pool's ordered queue (no games clears it)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			pool, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ids, err := resolveGameIDs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			// Ensure membership before ordering (the queue endpoint requires it).
			if len(ids) > 0 {
				if _, err := c.BulkAddPoolGames(p.Key, pool.ID, ids); err != nil {
					return fmt.Errorf("ensure pool membership failed: %w", err)
				}
			}
			if err := c.SetQueue(p.Key, pool.ID, ids); err != nil {
				return fmt.Errorf("set queue failed: %w", err)
			}
			if len(ids) == 0 {
				fmt.Fprintf(out, "Cleared the queue for %q.\n", pool.Name)
			} else {
				fmt.Fprintf(out, "Queued %d game(s) in %q.\n", len(ids), pool.Name)
			}
			return nil
		},
	}
}

func newPoolReorderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reorder <pool-ref…>",
		Short: "Reorder pools (positions follow argument order)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ids := make([]string, 0, len(args))
			for _, ref := range args {
				pool, err := resolvePoolRef(cmd, c, p.Key, ref)
				if err != nil {
					return err
				}
				ids = append(ids, pool.ID)
			}
			if err := c.ReorderPools(p.Key, ids); err != nil {
				return fmt.Errorf("reorder pools failed: %w", err)
			}
			fmt.Fprintf(out, "Reordered %d pool(s).\n", len(ids))
			return nil
		},
	}
}
```

Register: add `newPoolQueueCmd(), newPoolReorderCmd()` to `newPoolCmd`.

- [ ] **Step 4: Run test to verify it passes** — `go test ./cmd/nexctl/...` → PASS (whole package).
- [ ] **Step 5: Commit** — `git add cmd/nexctl/ && git commit -m "feat: add nexctl pool queue/reorder"`

---

## Task 9: docs + dead-code reconciliation

**Files:** Modify `CLAUDE.md`.

- [ ] **Step 1** — In CLAUDE.md, extend the `cmd/nexctl/` bullet to mention `tag` (list/create/rename/rm) and `pool` (list/show/create/edit/rm/add/remove/queue/reorder).
- [ ] **Step 2** — `make build && ./nexctl tag --help && ./nexctl pool --help` (both build; help lists the subcommands).
- [ ] **Step 3** — `go test ./internal/cliclient/... ./cmd/nexctl/... && golangci-lint run ./internal/cliclient/... ./cmd/nexctl/...` → PASS, 0 issues.
- [ ] **Step 4** — `make deadcode`; reconcile: no NEW entries in nexctl/cliclient attributable to this branch (all new exported methods are called).
- [ ] **Step 5** — `git add CLAUDE.md && git commit -m "docs: document nexctl tag and pool command groups"`

---

## Self-Review

**Spec coverage:** tag list/create/rename/rm (T4); pool list/show (T5), create/edit/rm (T6), add/remove (T7), queue/reorder (T8); the client methods they need (T1–T3); docs (T9). Pool-ref resolution (T5), tag-ref resolution (T4), game-ref reuse of `resolveUserGameRef` (T7), the `queue` bulk-add-then-order behavior (T8), and raw-JSON `--filter` validation (T6) all match the spec.

**Placeholder scan:** every step carries complete code + commands. The one `var _ = strconv.Itoa` guard in T4 is explicitly called out to be removed if `strconv` is unused — not left as a placeholder.

**Type consistency:** `Pool`/`PoolListItem`/`PoolDetail` (T2) are consumed by `resolvePoolRef` (T5) and the pool commands (T5–T8). `resolveGameIDs` (T7) is reused by `queue` (T8). `resolvePoolRef(cmd, c, key, ref)` and `resolveTagRef(c, key, ref)` signatures are stable across their callers. `cliui.Confirm` error is handled (never blank-discarded) in tag rm / pool rm.

**Known follow-ups (note, not blockers):** `pool reorder` with a partial list sets only those positions (may collide) — documented as "list pools in desired order"; `--filter` is raw JSON (no DSL, no name→UUID); `game list --pool` name resolution is a future nicety now that `resolvePoolRef` exists.
