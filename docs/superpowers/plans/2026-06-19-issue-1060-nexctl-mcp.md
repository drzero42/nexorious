# `nexctl` Phase 8 — MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local stdio MCP server hosted by `nexctl` (`nexctl mcp serve` + `nexctl mcp config`) whose tools are a pure mirror of the `game`/`pool`/`tag`/`sync` CLI command tree, sharing one orchestration core with the CLI so the two surfaces cannot drift.

**Architecture:** The MCP server lives in `package main` under `cmd/nexctl/` and reuses the existing `cliclient` typed methods plus the CLI's orchestration helpers. Each ambiguous ref-resolver is refactored into a pure match-finder (no `*cobra.Command`) that both the CLI disambiguation wrapper and the MCP tool handler call. Tool handlers translate typed input → orchestration → concise JSON projection. Transport is stdio; auth is the active CLI profile captured at `mcp serve` startup.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra`, `github.com/modelcontextprotocol/go-sdk/mcp` (v1.6.1), existing `internal/cliclient` + `internal/clicfg` + `internal/cliui` + `internal/enum`. Tests: stdlib `testing` + `net/http/httptest` (REST stub) + the SDK's `mcp.NewInMemoryTransports` (in-process MCP client↔server).

## Global Constraints

- **Design authority:** `docs/superpowers/specs/2026-06-19-issue-1060-nexctl-mcp-design.md`. Read it before starting.
- **MCP server is `nexctl`-only.** Files live in `cmd/nexctl/` (`package main`). The SDK import must never reach `cmd/nexorious` or any server-side package.
- **No drift.** MCP handlers reuse `cliclient` methods and the shared match-finders/`editOne`/`gameFilter` — never re-implement a multi-call sequence.
- **Pure mirror scope.** Tools mirror `game`/`pool`/`tag`/`sync` only. `admin`/`import`/`export`/`backup`/`config` are out of v1. `sync_connect` is **excluded** (secrets through an agent). The interactive `sync review` loop has no MCP tool; `sync_review` is a read listing, resolution is `sync_resolve`/`sync_skip`.
- **All tools registered in v1** regardless of key scope; a write tool hitting a read-scoped key returns an actionable error (no client-side scope gate).
- **`play_status` enum:** `not_started`, `in_progress`, `completed`, `mastered`, `dominated`, `shelved`, `dropped`, `replay`. Source `internal/enum` — never hardcode.
- **Conventions:** errors wrapped `fmt.Errorf("context: %w", err)`; `errcheck` runs with `check-blank` (handle or `//nolint:errcheck // reason`); `gosec` enabled. After any caller-removing/renaming refactor run `make deadcode`.
- **Module path:** `github.com/drzero42/nexorious`.

---

## File Structure

- `cmd/nexctl/mcp.go` — `newMCPCmd()` (parent + `config` + `serve`), `buildMCPServer(p clicfg.Profile) *mcp.Server` (registers all tools, binds nothing), the shared `mcpProfile` accessor, and the read-key `403`→actionable-error mapping helper.
- `cmd/nexctl/mcp_game.go` — game tool registrations + projection structs.
- `cmd/nexctl/mcp_pool.go` — pool tool registrations.
- `cmd/nexctl/mcp_tag.go` — tag tool registrations.
- `cmd/nexctl/mcp_sync.go` — sync tool registrations.
- `cmd/nexctl/mcp_test.go` — in-memory transport harness + tool tests.
- Modified: `cmd/nexctl/main.go` (register `newMCPCmd()`); `cmd/nexctl/game.go`, `game_add.go`, `game_rm.go`, `pool.go`, `sync.go` (extract pure match-finders); `go.mod`/`go.sum`; `nix/package.nix`, `nix/nexctl.nix` (`vendorHash`); `CLAUDE.md` (command-surface line).

---

## Task 1: Add the MCP SDK dependency and update `vendorHash`

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `nix/package.nix`, `nix/nexctl.nix`

**Interfaces:**
- Produces: the `github.com/modelcontextprotocol/go-sdk/mcp` package is importable; `nix build .#nexctl` succeeds.

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
go mod tidy
```
Expected: `go.mod` gains `github.com/modelcontextprotocol/go-sdk v1.6.1`; `go.sum` updated. If v1.6.1 is unavailable, use the latest `v1.x` tag from `go list -m -versions github.com/modelcontextprotocol/go-sdk` and note it.

- [ ] **Step 2: Confirm the SDK API names in this version**

Run:
```bash
go doc github.com/modelcontextprotocol/go-sdk/mcp AddTool
go doc github.com/modelcontextprotocol/go-sdk/mcp NewServer
go doc github.com/modelcontextprotocol/go-sdk/mcp.Server.Connect
go doc github.com/modelcontextprotocol/go-sdk/mcp NewInMemoryTransports
go doc github.com/modelcontextprotocol/go-sdk/mcp.ClientSession.CallTool
```
Expected: `AddTool[In, Out any](s *Server, t *Tool, h ToolHandlerFor[In, Out])`, `NewServer`, `StdioTransport`, `NewInMemoryTransports`, `ClientSession.CallTool` exist. Note the exact `Connect`/`CallTool` signatures (arg count varies across v1.x) — the later tasks' code follows whatever this prints.

- [ ] **Step 3: Recompute the shared `vendorHash`**

Per CLAUDE.md → Nix Flake Maintenance. In `nix/package.nix` set `vendorHash = pkgs.lib.fakeHash;`, then:
```bash
nix build .#nexorious 2>&1 | grep "got:"
```
Copy the `got:` hash into **both** `nix/package.nix` and `nix/nexctl.nix` (identical value — both vendor the same `go.mod`/`go.sum`).

- [ ] **Step 4: Verify both packages build under nix**

Run:
```bash
nix build .#nexorious && nix build .#nexctl
```
Expected: both succeed (no hash mismatch).

- [ ] **Step 5: Verify the server binary does not link the SDK**

Run:
```bash
go build ./... && go list -deps ./cmd/nexorious | grep modelcontextprotocol && echo "LEAK" || echo "clean"
```
Expected: `clean` (the SDK is not yet imported anywhere, so this also passes trivially now; re-checked in Task 8).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum nix/package.nix nix/nexctl.nix
git commit -m "build: add modelcontextprotocol/go-sdk for nexctl mcp server"
```

---

## Task 2: Extract pure match-finders (refactor; CLI behavior unchanged)

Refactor the ambiguous ref-resolvers so their match-finding core takes no `*cobra.Command`. The CLI wrappers keep their current signatures and behavior; the new finders are what the MCP handlers will call.

**Files:**
- Modify: `cmd/nexctl/game.go` (`resolveUserGameRef`), `cmd/nexctl/game_add.go` (`resolveIGDBCandidate`), `cmd/nexctl/game_rm.go` (`gamesForRefsOrFilter`), `cmd/nexctl/pool.go` (`resolvePoolRef`), `cmd/nexctl/sync.go` (`resolveExternalRef`)
- Test: `cmd/nexctl/mcp_finders_test.go` (new)

**Interfaces:**
- Produces (all in `package main`):
  - `func findUserGamesByRef(c *cliclient.Client, key, ref string) ([]cliclient.UserGame, error)` — UUID → `[]{GetUserGame}`; title → `ListUserGames(q=ref).UserGames`. Returns empty slice (not error) on zero hits.
  - `func findIGDBCandidates(c *cliclient.Client, key string, igdbID int, title string) ([]cliclient.IGDBCandidate, error)` — id → `GetIGDBGame.Games`; title → `SearchIGDB(title,10).Games`.
  - `func gamesByFilter(c *cliclient.Client, key string, f gameFilter) (games []cliclient.UserGame, total int, err error)` — the filter branch of `gamesForRefsOrFilter`, no `cmd`; returns `total` for the truncation warning.
  - `func findPoolsByRef(c *cliclient.Client, key, ref string) ([]cliclient.PoolListItem, error)` — UUID → exact; name → all case-insensitive matches.
  - `func findExternalGamesByRef(c *cliclient.Client, key, sf, ref string) ([]cliclient.ExternalGame, error)` — UUID → exact; title → all case-insensitive matches.
- Consumes: existing `cliclient` methods, `gameFilter`, `resolveTagID`, `looksLikeUUID`.

- [ ] **Step 1: Write the finder unit test (failing)**

Create `cmd/nexctl/mcp_finders_test.go`:
```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestFindUserGamesByRefTitleMany(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "halo" {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}},
				{"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 2, "title": "Halo 2"}},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := findUserGamesByRef(cliclient.New(srv.URL), "k", "halo")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./cmd/nexctl/ -run TestFindUserGamesByRefTitleMany -v`
Expected: FAIL — `undefined: findUserGamesByRef`.

- [ ] **Step 3: Extract `findUserGamesByRef` and rewrite `resolveUserGameRef` as a wrapper**

In `cmd/nexctl/game.go`, replace `resolveUserGameRef` with:
```go
// findUserGamesByRef returns every library game matching ref: a UUID is fetched
// directly (one element); a title is a search query (zero or more). No cmd, no
// picker — callers own disambiguation.
func findUserGamesByRef(c *cliclient.Client, key, ref string) ([]cliclient.UserGame, error) {
	if looksLikeUUID(ref) {
		u, err := c.GetUserGame(key, ref)
		if err != nil {
			return nil, err
		}
		return []cliclient.UserGame{*u}, nil
	}
	res, err := c.ListUserGames(key, urlValues("q", ref))
	if err != nil {
		return nil, err
	}
	return res.UserGames, nil
}

// resolveUserGameRef is the CLI wrapper: one hit is used; many prompt a picker
// (interactive) or error with candidate ids (off-TTY).
func resolveUserGameRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.UserGame, error) {
	games, err := findUserGamesByRef(c, key, ref)
	if err != nil {
		return nil, err
	}
	switch len(games) {
	case 0:
		return nil, fmt.Errorf("no game matching %q in your library", ref)
	case 1:
		return &games[0], nil
	}
	if interactive(cmd) {
		return pickUserGame(cmd, games)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d games; re-run with an id:", ref, len(games))
	for i := range games {
		fmt.Fprintf(&b, "\n  %s  %s", games[i].ID, games[i].Title())
	}
	return nil, fmt.Errorf("%s", b.String())
}
```

- [ ] **Step 4: Run the finder test + the existing game tests**

Run: `go test ./cmd/nexctl/ -run 'TestFindUserGamesByRef|TestGameShow|TestGameRm|TestGameEdit|TestGameAdd' -v`
Expected: PASS (wrapper behavior unchanged).

- [ ] **Step 5: Extract `findIGDBCandidates` (game_add.go), `gamesByFilter` (game_rm.go), `findPoolsByRef` (pool.go), `findExternalGamesByRef` (sync.go)**

Apply the identical pattern to each: pull the cliclient-call + match-collection logic into a `cmd`-free `findX`/`gamesByFilter` function returning the candidate slice (and `total` for `gamesByFilter`), and reduce the existing `resolveX`/`gamesForRefsOrFilter` to: call the finder, then the existing 0/1/many disambiguation + (for `gamesForRefsOrFilter`) the stderr truncation warning using the returned `total`. Keep every existing error string and the `per_page=200` cap.

For `gamesForRefsOrFilter`, the refactored body is:
```go
func gamesForRefsOrFilter(cmd *cobra.Command, c *cliclient.Client, key string, args []string, f gameFilter) ([]cliclient.UserGame, error) {
	if f.use {
		if len(args) > 0 {
			return nil, fmt.Errorf("pass refs or --filter, not both")
		}
		games, total, err := gamesByFilter(c, key, f)
		if err != nil {
			return nil, err
		}
		if total > len(games) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: filter matched %d games but only the first %d are affected; narrow the filter and re-run for the rest\n",
				total, len(games))
		}
		return games, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("provide one or more refs, or --filter")
	}
	games := make([]cliclient.UserGame, 0, len(args))
	for _, ref := range args {
		u, err := resolveUserGameRef(cmd, c, key, ref)
		if err != nil {
			return nil, err
		}
		games = append(games, *u)
	}
	return games, nil
}
```
where `gamesByFilter` holds the `url.Values` build + `resolveTagID` + `ListUserGames`, returning `(res.UserGames, res.Total, nil)`.

- [ ] **Step 6: Run the full nexctl suite + build + deadcode**

Run:
```bash
go test ./cmd/nexctl/...
go build ./...
make deadcode
```
Expected: all pass; `make deadcode` shows no *new* entries (the finders are referenced by the wrappers; they'll also be referenced by MCP tasks).

- [ ] **Step 7: Commit**

```bash
git add cmd/nexctl/
git commit -m "refactor(nexctl): extract cmd-free match-finders for resolver reuse"
```

---

## Task 3: `mcp` command scaffold, `mcp config`, and the server-build harness

**Files:**
- Create: `cmd/nexctl/mcp.go`
- Modify: `cmd/nexctl/main.go`
- Create/extend: `cmd/nexctl/mcp_test.go`

**Interfaces:**
- Produces:
  - `func newMCPCmd() *cobra.Command` — parent with `config` and `serve` subcommands; registered on root.
  - `func buildMCPServer(p clicfg.Profile) *mcp.Server` — creates the server and registers all tool groups (initially empty; tasks 4–7 fill it). Tool handlers close over `p.URL` + `p.Key`.
  - `func mcpToolError(action string, err error) error` — maps a `cliclient` error to an actionable tool error; a `403` becomes the read-only-key message.
- Consumes: `resolveProfile` (main.go), `clicfg.Profile`.

- [ ] **Step 1: Write the `mcp config` test (failing)**

In `cmd/nexctl/mcp_test.go`:
```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMCPConfigStanza(t *testing.T) {
	seedProfile(t, "https://example.test")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"mcp", "config"})
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp config: %v\n%s", err, out.String())
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	s, ok := cfg.MCPServers["nexorious"]
	if !ok || s.Command != "nexctl" || len(s.Args) != 2 || s.Args[0] != "mcp" || s.Args[1] != "serve" {
		t.Fatalf("stanza = %+v", cfg)
	}
}
```
> `seedProfile` is the existing test helper (see `game_stats_test.go`); confirm its signature before use.

- [ ] **Step 2: Run it (fails)**

Run: `go test ./cmd/nexctl/ -run TestMCPConfigStanza -v`
Expected: FAIL — `unknown command "mcp"`.

- [ ] **Step 3: Implement `mcp.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Run or configure the local MCP server"}
	cmd.AddCommand(newMCPConfigCmd(), newMCPServeCmd())
	return cmd
}

func newMCPConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the agent-config stanza for the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args := []string{"mcp", "serve"}
			if name, _ := cmd.Flags().GetString("profile"); name != "" { //nolint:errcheck // absent flag yields ""
				args = append(args, "--profile", name)
			}
			stanza := map[string]any{
				"mcpServers": map[string]any{
					"nexorious": map[string]any{"command": "nexctl", "args": args},
				},
			}
			return cliui.EncodeJSON(cmd.OutOrStdout(), stanza)
		},
	}
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run a local stdio MCP server over the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			srv := buildMCPServer(p)
			return srv.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}

// buildMCPServer registers every mirror tool against a server whose handlers use
// the given profile's URL + key. Transport is bound by the caller.
func buildMCPServer(p clicfg.Profile) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nexorious", Version: version}, nil)
	c := cliclient.New(p.URL)
	registerGameTools(srv, c, p.Key)
	registerPoolTools(srv, c, p.Key)
	registerTagTools(srv, c, p.Key)
	registerSyncTools(srv, c, p.Key)
	return srv
}

// mcpToolError maps a cliclient error to an actionable tool error. A 403 from a
// read-scoped key on a write tool gets a specific, corrective message.
func mcpToolError(action string, err error) error {
	if err == nil {
		return nil
	}
	if isForbidden(err) {
		return fmt.Errorf("%s: this profile's API key is read-only; mint a write-scoped key (`nexctl account api-key …`) to modify the collection", action)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// isForbidden reports whether err is an HTTP 403 from cliclient.
func isForbidden(err error) bool {
	// cliclient surfaces HTTP status in the error message; match 403 robustly.
	var he interface{ StatusCode() int }
	if errors.As(err, &he) {
		return he.StatusCode() == 403
	}
	return strings.Contains(err.Error(), "403")
}
```
> **Step 3a (verify the 403 mechanism):** check how `cliclient` reports HTTP status (`go doc` / read `internal/cliclient/client.go` around `doBearer`). If it exposes a typed error with a status code, use `errors.As` against that concrete type and delete the string fallback. If it only embeds the status in the message string, keep the `strings.Contains` form and add a `//nolint` only if a linter objects. Adjust `isForbidden` to the real type before moving on.
> The four `registerXTools` functions are declared (empty bodies) here so the package compiles; tasks 4–7 fill them.

Add empty registrars at the bottom of `mcp.go` for now:
```go
func registerGameTools(s *mcp.Server, c *cliclient.Client, key string) {}
func registerPoolTools(s *mcp.Server, c *cliclient.Client, key string) {}
func registerTagTools(s *mcp.Server, c *cliclient.Client, key string)  {}
func registerSyncTools(s *mcp.Server, c *cliclient.Client, key string) {}
```

- [ ] **Step 4: Register on the root**

In `cmd/nexctl/main.go`, add after `root.AddCommand(newConfigCmd())`:
```go
	root.AddCommand(newMCPCmd())
```

- [ ] **Step 5: Run the config test + build**

Run: `go test ./cmd/nexctl/ -run TestMCPConfigStanza -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 6: Add the in-memory MCP test harness**

Append to `cmd/nexctl/mcp_test.go` a helper that connects an in-process client to `buildMCPServer` (follow the exact `Connect`/`CallTool` signatures from Task 1 Step 2):
```go
import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/clicfg"
)

// mcpSession spins up buildMCPServer pointed at restURL and returns a connected
// in-memory client session.
func mcpSession(t *testing.T, restURL string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := buildMCPServer(clicfg.Profile{URL: restURL, Key: "test-key"})
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}
```
> Adjust arg counts to the real SDK signatures noted in Task 1. Confirm `clicfg.Profile` field names (`URL`, `Key`) by reading `internal/clicfg`.

- [ ] **Step 7: Commit**

```bash
git add cmd/nexctl/mcp.go cmd/nexctl/main.go cmd/nexctl/mcp_test.go
git commit -m "feat(nexctl): scaffold mcp command, config, and server-build harness"
```

---

## Task 4: Read tools

Register the read tools in `registerGameTools` (game_list/show/stats/filters), `registerPoolTools` (pool_list/show), `registerTagTools` (tag_list), `registerSyncTools` (sync_status/review).

**Files:**
- Create: `cmd/nexctl/mcp_game.go`, `cmd/nexctl/mcp_pool.go`, `cmd/nexctl/mcp_tag.go`, `cmd/nexctl/mcp_sync.go`
- Modify: `cmd/nexctl/mcp.go` (move the empty registrars out; the real ones live in the group files)
- Test: `cmd/nexctl/mcp_test.go`

**Interfaces:**
- Consumes: `cliclient` methods, `findUserGamesByRef`, `findPoolsByRef`, `resolveStorefront`, `enum.AllPlayStatuses`/`AllOwnershipStatuses`, `mcpToolError`.
- Produces: registered read tools; projection structs `gameBrief`, `gameDetail`.

- [ ] **Step 1: Write the `game_list` tool test (failing)**

In `mcp_test.go`:
```go
func TestMCPGameList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("play_status = %q", r.URL.Query().Get("play_status"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "play_status": "completed",
					"game": map[string]any{"id": 1, "title": "Halo"}},
			},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "game_list",
		Arguments: map[string]any{"play_status": "completed"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	// StructuredContent carries the projected payload.
	b, _ := json.Marshal(res.StructuredContent)
	if !bytes.Contains(b, []byte("Halo")) {
		t.Fatalf("missing game: %s", b)
	}
}
```
> Match `CallTool`'s real signature/return (some v1.x return `(*CallToolResult, error)`; confirm from Task 1).

- [ ] **Step 2: Run it (fails)**

Run: `go test ./cmd/nexctl/ -run TestMCPGameList -v`
Expected: FAIL — tool `game_list` not found / `registerGameTools` empty.

- [ ] **Step 3: Implement the game read tools in `mcp_game.go`**

```go
package main

import (
	"context"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/enum"
)

// gameBrief is the concise list projection (human fields + id for chaining).
type gameBrief struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	PlayStatus string   `json:"play_status,omitempty"`
	Rating     *int     `json:"rating,omitempty"`
	Hours      float64  `json:"hours_played"`
	Wishlist   bool     `json:"wishlist"`
	Platforms  []string `json:"platforms,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

func briefOf(u *cliclient.UserGame) gameBrief {
	b := gameBrief{ID: u.ID, Title: u.Title(), Rating: u.PersonalRating,
		Hours: u.HoursPlayed, Wishlist: u.IsWishlisted}
	if u.PlayStatus != nil {
		b.PlayStatus = *u.PlayStatus
	}
	for i := range u.Platforms {
		if u.Platforms[i].Platform != nil {
			b.Platforms = append(b.Platforms, *u.Platforms[i].Platform)
		}
	}
	for _, t := range u.Tags {
		b.Tags = append(b.Tags, t.Name)
	}
	return b
}

type gameListInput struct {
	Q              string `json:"q,omitempty" jsonschema:"free-text title search"`
	PlayStatus     string `json:"play_status,omitempty" jsonschema:"filter by play status: not_started, in_progress, completed, mastered, dominated, shelved, dropped, replay"`
	OwnershipStatus string `json:"ownership_status,omitempty" jsonschema:"filter by ownership status"`
	Wishlist       *bool  `json:"wishlist,omitempty" jsonschema:"true = only wishlisted, false = only library"`
	Genre          string `json:"genre,omitempty"`
	Platform       string `json:"platform,omitempty"`
	Storefront     string `json:"storefront,omitempty"`
	SortBy         string `json:"sort_by,omitempty"`
	SortOrder      string `json:"sort_order,omitempty"`
	Page           int    `json:"page,omitempty"`
	PerPage        int    `json:"per_page,omitempty" jsonschema:"page size, max 200"`
}

type gameListOutput struct {
	Games []gameBrief `json:"games"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Pages int         `json:"pages"`
}

func registerGameTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "game_list", Description: "List/search the collection (concise projection)."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameListInput) (*mcp.CallToolResult, gameListOutput, error) {
			params := url.Values{}
			setIf(params, "q", in.Q)
			setIf(params, "play_status", in.PlayStatus)
			setIf(params, "ownership_status", in.OwnershipStatus)
			setIf(params, "genre", in.Genre)
			setIf(params, "platform", in.Platform)
			setIf(params, "storefront", in.Storefront)
			setIf(params, "sort_by", in.SortBy)
			setIf(params, "sort_order", in.SortOrder)
			if in.Wishlist != nil {
				params.Set("wishlist", strconv.FormatBool(*in.Wishlist))
			}
			if in.Page > 0 {
				params.Set("page", strconv.Itoa(in.Page))
			}
			if in.PerPage > 0 {
				params.Set("per_page", strconv.Itoa(in.PerPage))
			}
			res, err := c.ListUserGames(key, params)
			if err != nil {
				return nil, gameListOutput{}, mcpToolError("game_list", err)
			}
			out := gameListOutput{Total: res.Total, Page: res.Page, Pages: res.Pages}
			for i := range res.UserGames {
				out.Games = append(out.Games, briefOf(&res.UserGames[i]))
			}
			return nil, out, nil
		})

	// game_show, game_stats, game_filters: see Step 4.
}
```
> `setIf` is the existing helper used by `gamesByFilter` — confirm it lives in `package main` (game_rm.go area). If it's unexported there, reuse it directly.

- [ ] **Step 4: Add `game_show`, `game_stats`, `game_filters` in the same registrar**

Specs (each is one `mcp.AddTool`; follow the `game_list` shape):
- `game_show` — input `{ref string}` (title-or-id). Call `findUserGamesByRef(c,key,in.Ref)`; 0 → tool error "no game…"; 1 → project to `gameDetail` (full record: brief fields + per-platform rows with hours + notes); >1 → return a result whose output lists `[]gameBrief` candidates with a message "ambiguous; call again with one of these ids". Output struct `gameShowOutput{Game *gameDetail; Candidates []gameBrief}`.
- `game_stats` — no input; call `c.GetCollectionStats(key)`; **output is `*cliclient.CollectionStats`** (already JSON-tagged — pass through, no projection).
- `game_filters` — no input; output `{PlayStatuses, OwnershipStatuses []string; Genres, GameModes, Themes, PlayerPerspectives []string; Storefronts []string}`. Fill statuses from `enum.AllPlayStatuses()`/`AllOwnershipStatuses()` (stringify), genres/modes/themes/perspectives from `c.GetFilterOptions(key)`, storefronts from `c.ListStorefronts(key)` (slug field). Mirror `game_filters.go` exactly.

- [ ] **Step 5: Implement pool/tag/sync read tools**

- `mcp_pool.go` → `registerPoolTools`: `pool_list` (`c.ListPools(key)` → project to `{id,name,color,game_count}` briefs); `pool_show` (`{ref}` → `findPoolsByRef` → `c.GetPool(key,id)`; ambiguity like `game_show`; output the pool detail incl. ordered queue).
- `mcp_tag.go` → `registerTagTools`: `tag_list` (`c.ListTags(key)` → `[]cliclient.Tag` passthrough).
- `mcp_sync.go` → `registerSyncTools`: `sync_status` (`{storefront string}` optional: empty → `c.ListSyncConfigs(key)` passthrough; set → `resolveStorefront` + `c.GetSyncStatus`); `sync_review` (`{storefront string}` required → `resolveStorefront` + `c.ListExternalGames`, filter `needs_review`, project `{id,title,candidates}`).

Delete the four empty registrar stubs from `mcp.go` (now defined in the group files).

- [ ] **Step 6: Add tests for one read tool per group**

Add `TestMCPGameShowAmbiguous` (asserts `Candidates` populated, `Game` nil for a 2-hit title), `TestMCPGameFilters` (asserts a known play-status value present), `TestMCPPoolList`, `TestMCPTagList`, `TestMCPSyncStatus` — each with a focused `httptest` stub like `TestMCPGameList`.

- [ ] **Step 7: Run, build, deadcode**

Run:
```bash
go test ./cmd/nexctl/ -run TestMCP -v
go build ./... && make deadcode
```
Expected: PASS; no new deadcode.

- [ ] **Step 8: Commit**

```bash
git add cmd/nexctl/mcp*.go
git commit -m "feat(nexctl): mcp read tools (game/pool/tag/sync)"
```

---

## Task 5: Write tools — game group

Add `game_add`, `game_edit`, `game_acquire`, `game_rm` to `registerGameTools`.

**Files:**
- Modify: `cmd/nexctl/mcp_game.go`
- Test: `cmd/nexctl/mcp_test.go`

**Interfaces:**
- Consumes: `findIGDBCandidates`, `c.ImportIGDBGame`, `c.CreateUserGame`, `cliclient.CreateUserGameInput`/`PlatformInput`, `findUserGamesByRef`, `editOne`/`editOpts`, `gameFilter`/`gamesByFilter`, `c.MoveToLibrary`, `c.DeleteUserGame`, `splitPlatform`, `mcpToolError`.

- [ ] **Step 1: Write the `game_edit` test (failing)**

```go
func TestMCPGameEditStatus(t *testing.T) {
	var gotStatus string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "11111111-1111-1111-1111-111111111111",
				"game": map[string]any{"id": 1, "title": "Halo"}})
		case strings.HasSuffix(r.URL.Path, "/progress"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotStatus, _ = body["play_status"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "11111111-1111-1111-1111-111111111111"})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "game_edit",
		Arguments: map[string]any{
			"refs":        []string{"11111111-1111-1111-1111-111111111111"},
			"play_status": "completed",
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("edit: err=%v res=%+v", err, res)
	}
	if gotStatus != "completed" {
		t.Fatalf("play_status sent = %q", gotStatus)
	}
}
```
> Confirm the real progress endpoint path from `cliclient.UpdateProgress` before finalizing the stub.

- [ ] **Step 2: Run it (fails)**

Run: `go test ./cmd/nexctl/ -run TestMCPGameEditStatus -v`
Expected: FAIL — tool not found.

- [ ] **Step 3: Implement `game_edit`**

```go
type gameEditInput struct {
	Refs        []string `json:"refs,omitempty" jsonschema:"one or more game titles or ids"`
	Filter      *gameEditFilter `json:"filter,omitempty" jsonschema:"select games by filter instead of refs"`
	PlayStatus  string   `json:"play_status,omitempty"`
	Rating      *int     `json:"rating,omitempty"`
	Loved       *bool    `json:"loved,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
	AddPlatform string   `json:"add_platform,omitempty" jsonschema:"platform[/storefront] to add"`
	RmPlatform  string   `json:"rm_platform,omitempty"`
	Hours       *float64 `json:"hours,omitempty"`
	HoursPlatform string `json:"hours_platform,omitempty"`
	AddTags     []string `json:"add_tags,omitempty"`
	RmTags      []string `json:"rm_tags,omitempty"`
}
type gameEditFilter struct {
	PlayStatus string `json:"play_status,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Wishlist   *bool  `json:"wishlist,omitempty"`
}
type gameWriteOutput struct {
	Updated    []gameBrief `json:"updated,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}
```
Handler: resolve the target set — if `in.Filter != nil`, build a `gameFilter` and call `gamesByFilter`; else resolve each ref via `findUserGamesByRef`, returning a `Candidates` result (no mutation) if any ref is ambiguous (>1) or empty. Then for each game call `editOne(c, key, &u, editOpts{...})` mapping the optional fields (`statusSet = in.PlayStatus != ""`, `ratingSet = in.Rating != nil`, `loved/noLoved` from `*in.Loved`, etc.). On `editOne` error return `mcpToolError`. Return `Updated` briefs.

- [ ] **Step 4: Implement `game_add`, `game_acquire`, `game_rm`**

- `game_add` — input `{title string; igdb_id int; play_status string; platform string; storefront string; notes *string; wishlist bool; loved bool; rating *int}`. Call `findIGDBCandidates`; 0 → tool error; >1 → return `{Candidates: [{igdb_id,title,release_date}], Message:"ambiguous; call again with igdb_id"}`; 1 → `ImportIGDBGame` then `CreateUserGame` (build `CreateUserGameInput` exactly like `game_add.go`). Output the created `gameBrief`.
- `game_acquire` — input `{ref string; platform string (required); storefront string; ownership string}`. `findUserGamesByRef` (ambiguity → candidates); `c.MoveToLibrary(key,id,[]PlatformInput{...})`. Output the brief.
- `game_rm` — input `{refs []string; filter *gameEditFilter}`. Resolve set as in `game_edit`; `c.DeleteUserGame` each; output `{Removed []gameBrief}`. (No interactive confirm — the agent's call is the intent; mirrors `--yes`.)

- [ ] **Step 5: Add `game_add` ambiguity + `game_rm` tests**

`TestMCPGameAddAmbiguous` (2 IGDB hits → `Candidates` set, no import POST observed), `TestMCPGameRm` (delete called for each id).

- [ ] **Step 6: Run, build**

Run: `go test ./cmd/nexctl/ -run TestMCP -v && go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/nexctl/mcp_game.go cmd/nexctl/mcp_test.go
git commit -m "feat(nexctl): mcp game write tools (add/edit/acquire/rm)"
```

---

## Task 6: Write tools — pool and tag groups

**Files:**
- Modify: `cmd/nexctl/mcp_pool.go`, `cmd/nexctl/mcp_tag.go`
- Test: `cmd/nexctl/mcp_test.go`

**Interfaces:**
- Consumes: `findPoolsByRef`, `resolveGameIDs`-equivalent via `findUserGamesByRef`, `c.CreatePool/UpdatePool/DeletePool/AddPoolGame/BulkAddPoolGames/RemovePoolGame/SetQueue/ReorderPools`, `resolveTagRef`, `c.CreateTag/UpdateTag/DeleteTag`, `mcpToolError`.

- [ ] **Step 1: Write the `pool_create` test (failing)**

```go
func TestMCPPoolCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "33333333-3333-3333-3333-333333333333", "name": "Backlog"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pool_create", Arguments: map[string]any{"name": "Backlog"}})
	if err != nil || res.IsError {
		t.Fatalf("create: err=%v res=%+v", err, res)
	}
}
```

- [ ] **Step 2: Run it (fails)**

Run: `go test ./cmd/nexctl/ -run TestMCPPoolCreate -v`
Expected: FAIL.

- [ ] **Step 3: Implement pool write tools** in `registerPoolTools`

One `mcp.AddTool` each, mirroring the CLI commands (resolve pool ref via `findPoolsByRef` + ambiguity→candidates; resolve game refs via `findUserGamesByRef`):
- `pool_create` `{name string; color *string; filter json.RawMessage}` → `c.CreatePool`.
- `pool_edit` `{ref string; name *string; color *string; filter json.RawMessage; clear_filter bool}` → `c.UpdatePool` (build `fields` map exactly like `pool_mutate.go`).
- `pool_rm` `{ref string}` → `c.DeletePool`.
- `pool_add` `{pool string; games []string}` → resolve ids; 1 → `AddPoolGame`, >1 → `BulkAddPoolGames`.
- `pool_remove` `{pool string; games []string}` → `RemovePoolGame` each.
- `pool_queue` `{pool string; games []string}` → `BulkAddPoolGames` then `SetQueue` (declarative order).
- `pool_reorder` `{pools []string}` → resolve each via `findPoolsByRef`, `ReorderPools(ids)`.

- [ ] **Step 4: Implement tag write tools** in `registerTagTools`

- `tag_create` `{name string; color *string}` → `c.CreateTag`.
- `tag_rename` `{ref string; new_name string}` → `resolveTagRef` + `c.UpdateTag`.
- `tag_rm` `{ref string}` → `resolveTagRef` + `c.DeleteTag`.

- [ ] **Step 5: Add `pool_queue` + `tag_create` tests**

`TestMCPPoolQueue` (asserts bulk-add then set-queue order POSTed), `TestMCPTagCreate`.

- [ ] **Step 6: Run, build**

Run: `go test ./cmd/nexctl/ -run TestMCP -v && go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/nexctl/mcp_pool.go cmd/nexctl/mcp_tag.go cmd/nexctl/mcp_test.go
git commit -m "feat(nexctl): mcp pool and tag write tools"
```

---

## Task 7: Write tools — sync group + read-key error mapping

**Files:**
- Modify: `cmd/nexctl/mcp_sync.go`
- Test: `cmd/nexctl/mcp_test.go`

**Interfaces:**
- Consumes: `resolveStorefront`, `findExternalGamesByRef`, `c.TriggerSync/RematchExternalGame/SkipExternalGame/RetryFailedExternalGames/ResetSyncData/DisconnectStorefront`, `mcpToolError`.

- [ ] **Step 1: Write the `sync_run` + 403-mapping tests (failing)**

```go
func TestMCPSyncRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/configs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"storefront": "steam"}})
	})
	mux.HandleFunc("/api/sync/steam/run", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"job_id": "job-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "sync_run", Arguments: map[string]any{"storefront": "steam"}})
	if err != nil || res.IsError {
		t.Fatalf("run: err=%v res=%+v", err, res)
	}
}

func TestMCPWriteToolReadKey403(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/configs", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"storefront": "steam"}})
	})
	mux.HandleFunc("/api/sync/steam/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "forbidden"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cs := mcpSession(t, srv.URL)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "sync_run", Arguments: map[string]any{"storefront": "steam"}})
	if err != nil {
		t.Fatalf("transport err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error for 403")
	}
	// the message must mention read-only / write-scoped key
	b, _ := json.Marshal(res.Content)
	if !bytes.Contains(bytes.ToLower(b), []byte("read-only")) {
		t.Fatalf("403 message not actionable: %s", b)
	}
}
```
> Confirm the real sync endpoint paths from `cliclient` (`/api/sync/configs`, `/api/sync/:storefront/run`, etc.) before finalizing stubs.

- [ ] **Step 2: Run them (fail)**

Run: `go test ./cmd/nexctl/ -run 'TestMCPSyncRun|TestMCPWriteToolReadKey403' -v`
Expected: FAIL — tool not found.

- [ ] **Step 3: Implement sync write tools** in `registerSyncTools`

- `sync_run` `{storefront string}` → `resolveStorefront` + `c.TriggerSync` → output `{job_id}`.
- `sync_resolve` `{storefront string; ref string; igdb_id int; orphan_action string}` → `resolveStorefront` + `findExternalGamesByRef` (ambiguity→candidates) + `c.RematchExternalGame`.
- `sync_skip` `{storefront string; ref string}` → `findExternalGamesByRef` + `c.SkipExternalGame`.
- `sync_retry` `{storefront string}` → `c.RetryFailedExternalGames`.
- `sync_reset` `{storefront string}` → `c.ResetSyncData`.
- `sync_disconnect` `{storefront string}` → `c.DisconnectStorefront`.

All write paths return errors through `mcpToolError(...)`, so the 403 mapping (Task 3) is exercised. **No `sync_connect`.**

- [ ] **Step 4: Run, build, full suite**

Run:
```bash
go test ./cmd/nexctl/... && go build ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/mcp_sync.go cmd/nexctl/mcp_test.go
git commit -m "feat(nexctl): mcp sync tools and read-key error mapping"
```

---

## Task 8: Docs, deadcode, final verification

**Files:**
- Modify: `CLAUDE.md` (the `cmd/nexctl/` command-surface bullet)
- Modify: `docs/user-guide.md` only if it documents `nexctl` (check first; do not invent a section)

**Interfaces:** none (docs + verification).

- [ ] **Step 1: Update the CLAUDE.md command-surface line**

In the `cmd/nexctl/` bullet under "Project Structure", append the `mcp` group after `config`: note `mcp` (`config`/`serve`) — a local **stdio** MCP server that mirrors the `game`/`pool`/`tag`/`sync` command tree as tools (`game_*`, `pool_*`, `tag_*`, `sync_*`), reusing the same `cliclient` orchestration via the shared match-finders; **all tools registered**, read-key `403` surfaced as an actionable error; `sync_connect`, `admin`/`import`/`export`/`backup`/`config` are **out of MCP v1**. Keep it one dense bullet matching the existing style.

- [ ] **Step 2: Verify the server binary still does not link the SDK**

Run:
```bash
go list -deps ./cmd/nexorious | grep modelcontextprotocol && echo "LEAK — fail" || echo "clean"
```
Expected: `clean`. If `LEAK`, an MCP file was placed in a server-reachable package — move it back under `cmd/nexctl/`.

- [ ] **Step 3: Deadcode + full gates**

Run:
```bash
make deadcode
go test ./cmd/nexctl/... ./internal/cliclient/...
go build ./...
golangci-lint run ./cmd/nexctl/...
```
Expected: no new deadcode entries; all tests pass; lint clean.

- [ ] **Step 4: Manual smoke (optional, if a dev server + profile exist)**

```bash
go build -o /tmp/nexctl ./cmd/nexctl
/tmp/nexctl mcp config           # prints the stanza
# /tmp/nexctl mcp serve          # then drive from an MCP client
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/
git commit -m "docs: document nexctl mcp server (Phase 8)"
```

---

## Self-Review

**Spec coverage:**
- Hosted in `nexctl`, stdio, `config` + `serve` → Task 3. ✓
- Over `cliclient`, one orchestration core (match-finder extraction) → Task 2, reused in 4–7. ✓
- Pure-mirror tool surface (game/pool/tag/sync) → Tasks 4–7. ✓
- `game_stats`/`game_filters` mirror #1094/#1095 → Task 4. ✓
- Concise projection + enum constraints in schema → Task 4 (`gameBrief`, `play_status` jsonschema). ✓
- Title-or-id refs with agent-disambiguation (candidates) → Tasks 4–7 (`Candidates` output). ✓
- All tools registered; read-key 403 actionable error → Task 3 (`mcpToolError`), tested Task 7. ✓
- `sync_connect` excluded; admin/import/export/backup/config out → Task 7 + Global Constraints. ✓
- SDK dependency + shared `vendorHash` in both nix files; server binary doesn't link SDK → Tasks 1, 8. ✓
- Docs (CLAUDE.md command surface) → Task 8. ✓

**Placeholder scan:** Trivial mirror tools are specified by exact input fields + the precise `cliclient` method + output projection, with one fully-coded worked example per group (`game_list`, `game_edit`, `pool_create`, `sync_run`) — no "TBD"/"add error handling"/"similar to". Steps that depend on SDK signature variance across v1.x explicitly defer to the `go doc` output captured in Task 1.

**Type consistency:** `gameBrief`/`briefOf` defined in Task 4 and reused in Task 5 (`gameWriteOutput`); `gameEditFilter` defined Task 5 and reused by `game_rm`; `gameFilter`/`gamesByFilter`/`editOne`/`editOpts` are the existing types from Task 2's refactor; `mcpToolError`/`buildMCPServer`/`registerXTools` names match across Tasks 3–7.

**Open items the implementer must resolve from source (flagged inline, not placeholders):** exact SDK `Connect`/`CallTool`/`AddTool` signatures (Task 1 Step 2); how `cliclient` reports HTTP 403 — typed error vs message string (Task 3 Step 3a); exact REST endpoint paths in the test stubs (Tasks 5, 7); `clicfg.Profile` field names and `seedProfile`/`setIf` reuse (Tasks 3, 4).
