# `nexctl` Phase 1 — Scaffold + Account/Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a separate `nexctl` client binary, move the account commands (`login`/`logout`/`whoami`/`api-key`) out of the `nexorious` server binary, and add multi-profile management — all over the existing REST/`clicfg`/`cliclient` stack.

**Architecture:** Extract the login bootstrap and terminal helpers shared by `nexorious setup` and the client commands into two new `internal/` packages (`cliui`, `cliauth`). Repoint `nexorious setup` at them, delete the moved commands from `cmd/nexorious`, then build `cmd/nexctl` (cobra root + global flags `--profile`/`--json`/`-q`/`-y`) and re-home the account commands plus a new `profile` group there.

**Tech Stack:** Go 1.26, `spf13/cobra`, `golang.org/x/term`, `gopkg.in/yaml.v3` (via `clicfg`), stdlib `net/http/httptest` for tests.

## Global Constraints

- Module path: `github.com/drzero42/nexorious`.
- No back-compat aliases — `login`/`logout`/`whoami`/`api-key` are **removed** from `nexorious`, not deprecated ([[project_no_backcompat_solo_user]]).
- `nexctl` may import **only** `internal/clicfg`, `internal/cliclient`, `internal/cliui`, `internal/cliauth` (+ stdlib + cobra/term). It must **not** import any server package.
- Every `_ =` error discard fails CI (`errcheck` `check-blank`); handle errors or annotate with `//nolint:errcheck // <reason>`. Exempt in `_test.go`.
- `gosec` is enabled for non-test code; config files already carry the right `//nolint:gosec` annotations — preserve them when moving code.
- Account-group user-facing strings must say `nexctl account login` (not `nexorious login`).
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit. Frequent commits.
- Build version/commit are injected via `-ldflags "-X main.version=… -X main.commit=…"`; `nexctl` declares its own `version`/`commit` package vars like `nexorious` does.

---

## File Structure

- `internal/cliui/cliui.go` (new) — TTY detection, text/secret prompts, confirm, JSON encode, `FirstNonEmpty`. Imported by `cmd/nexorious` (setup) and `cmd/nexctl`.
- `internal/cliauth/cliauth.go` (new) — `DefaultServerURL`, `LoginAndStoreKey`. Built on `cliclient`+`clicfg`. Imported by `cmd/nexorious` (setup) and `cmd/nexctl` (account login).
- `internal/clicfg/config.go` (modify) — add `Profile`, `RemoveProfile`, `SetCurrent`, `Names`.
- `cmd/nexorious/setup.go` (modify) — use `cliauth`/`cliui`.
- `cmd/nexorious/{login,logout,whoami,api_key}.go` + tests (delete).
- `cmd/nexorious/main.go` + `main_test.go` (modify) — drop the 4 commands.
- `cmd/nexctl/main.go` (new) — root cobra, global flags, profile-resolution helpers, `main()`.
- `cmd/nexctl/version.go` (new) — `version` subcommand.
- `cmd/nexctl/account.go` (new) — `account` parent + `login`/`logout`/`whoami`.
- `cmd/nexctl/api_key.go` (new) — `account api-key` group.
- `cmd/nexctl/profile.go` (new) — `profile` group.
- `Makefile` (modify) — build `nexctl`.
- `CLAUDE.md` (modify) — document `cmd/nexctl` + the two new packages.

---

## Task 1: `internal/cliui` shared terminal helpers

**Files:**
- Create: `internal/cliui/cliui.go`
- Test: `internal/cliui/cliui_test.go`

**Interfaces:**
- Produces:
  - `func FirstNonEmpty(vals ...string) string`
  - `func IsTTY(f *os.File) bool`
  - `func Prompt(in *bufio.Reader, out io.Writer, label string) (string, error)`
  - `func ReadPassword(in *bufio.Reader, out io.Writer) (string, error)` — reads `NEXORIOUS_PASSWORD` env, else no-echo TTY prompt, else a plain line from `in`.
  - `func Confirm(in *bufio.Reader, out io.Writer, question string, assumeYes bool) (bool, error)`
  - `func EncodeJSON(out io.Writer, v any) error` — indented JSON + trailing newline.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliui/...`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package cliui holds front-end-agnostic terminal helpers shared by the
// nexorious and nexctl binaries: TTY detection, prompts, confirmation, and
// machine-readable output.
package cliui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// FirstNonEmpty returns the first non-empty string, or "".
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// IsTTY reports whether f is an interactive terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Prompt writes label to out and reads one trimmed line from in.
func Prompt(in *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// ReadPassword resolves a password from the NEXORIOUS_PASSWORD env var, else a
// no-echo TTY prompt, else a plain line from in (piped input). The non-TTY path
// reuses the caller's reader so it does not race with other prompts on stdin.
func ReadPassword(in *bufio.Reader, out io.Writer) (string, error) {
	if env := os.Getenv("NEXORIOUS_PASSWORD"); env != "" {
		return env, nil
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(out, "Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// Confirm asks a yes/no question. When assumeYes is true it returns true without
// prompting. Anything other than y/yes (case-insensitive) is false.
func Confirm(in *bufio.Reader, out io.Writer, question string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	line, _ := in.ReadString('\n') //nolint:errcheck // EOF/partial line still yields the typed answer
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// EncodeJSON writes v as indented JSON followed by a newline.
func EncodeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliui/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliui/
git commit -m "feat: add internal/cliui shared terminal helpers"
```

---

## Task 2: `internal/clicfg` profile-management helpers

**Files:**
- Modify: `internal/clicfg/config.go`
- Test: `internal/clicfg/config_test.go`

**Interfaces:**
- Produces (methods on `*Config`):
  - `func (c *Config) Profile(name string) (Profile, bool)` — defaults `""` to `"default"`.
  - `func (c *Config) RemoveProfile(name string)` — deletes the entry; if it was current, resets `Current` to `""`.
  - `func (c *Config) SetCurrent(name string) error` — errors if the profile doesn't exist.
  - `func (c *Config) Names() []string` — sorted profile names.

- [ ] **Step 1: Write the failing test**

```go
func TestProfileHelpers(t *testing.T) {
	cfg := &Config{}
	cfg.SetProfile("default", Profile{URL: "u1", Key: "k1"})
	cfg.SetProfile("work", Profile{URL: "u2", Key: "k2"})

	if got := cfg.Names(); len(got) != 2 || got[0] != "default" || got[1] != "work" {
		t.Fatalf("Names = %v, want [default work]", got)
	}
	if p, ok := cfg.Profile("work"); !ok || p.Key != "k2" {
		t.Fatalf("Profile(work) = %+v,%v", p, ok)
	}
	if err := cfg.SetCurrent("missing"); err == nil {
		t.Fatal("SetCurrent(missing) should error")
	}
	if err := cfg.SetCurrent("work"); err != nil || cfg.Current != "work" {
		t.Fatalf("SetCurrent(work) = %v, Current=%q", err, cfg.Current)
	}
	cfg.RemoveProfile("work")
	if _, ok := cfg.Profile("work"); ok {
		t.Fatal("work should be removed")
	}
	if cfg.Current != "" {
		t.Fatalf("Current should reset after removing the current profile, got %q", cfg.Current)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/clicfg/... -run TestProfileHelpers`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/clicfg/config.go` (add `"sort"` to the import block):

```go
// Profile returns the named profile (defaulting "" to the default profile).
func (c *Config) Profile(name string) (Profile, bool) {
	if name == "" {
		name = defaultProfile
	}
	p, ok := c.Profiles[name]
	return p, ok
}

// RemoveProfile deletes a profile. If it was the current profile, Current is
// cleared so CurrentName falls back to the default.
func (c *Config) RemoveProfile(name string) {
	if name == "" {
		name = defaultProfile
	}
	delete(c.Profiles, name)
	if c.Current == name {
		c.Current = ""
	}
}

// SetCurrent marks an existing profile as current, erroring if it is unknown.
func (c *Config) SetCurrent(name string) error {
	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("no profile named %q", name)
	}
	c.Current = name
	return nil
}

// Names returns the profile names in sorted order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/clicfg/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/clicfg/
git commit -m "feat: add profile-management helpers to clicfg"
```

---

## Task 3: `internal/cliauth` login bootstrap

**Files:**
- Create: `internal/cliauth/cliauth.go`
- Test: `internal/cliauth/cliauth_test.go`

**Interfaces:**
- Consumes: `cliclient.Client`, `clicfg.Config`, `clicfg.Profile`, `Config.Profile` (Task 2).
- Produces:
  - `const DefaultServerURL = "http://localhost:8000"`
  - `func LoginAndStoreKey(out io.Writer, client *cliclient.Client, cfg *clicfg.Config, profileName, url, username, password string) error` — logs in, rotates out any key already stored under `profileName`, mints a fresh CLI key, drops the bootstrap session, and saves it under `profileName`.

This is lifted from `cmd/nexorious/login.go`'s `loginAndStoreKey`, parameterized by `profileName` (was hard-coded to `cfg.CurrentName()`), with its private `hostname`/`maskKey` helpers.

- [ ] **Step 1: Write the failing test**

```go
package cliauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestLoginAndStoreKeyWritesNamedProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-1"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &clicfg.Config{}
	if err := LoginAndStoreKey(&bytes.Buffer{}, cliclient.New(srv.URL), cfg, "work", srv.URL, "alice", "pw"); err != nil {
		t.Fatalf("LoginAndStoreKey: %v", err)
	}
	p, ok := cfg.Profile("work")
	if !ok || p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("profile work = %+v ok=%v", p, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliauth/...`
Expected: FAIL — package undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// Package cliauth holds the API-key login bootstrap shared by `nexctl account
// login` and `nexorious setup --login`.
package cliauth

import (
	"fmt"
	"io"
	"os"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// DefaultServerURL is the URL assumed when none is provided.
const DefaultServerURL = "http://localhost:8000"

// LoginAndStoreKey logs in with the given credentials, rotates out any key
// already stored under profileName, mints a fresh CLI key, drops the throwaway
// session, and saves the key to the named profile (marking it current).
func LoginAndStoreKey(out io.Writer, client *cliclient.Client, cfg *clicfg.Config, profileName, url, username, password string) error {
	sessionID, err := client.Login(username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Rotate: revoke a previously stored key for this profile before minting a new one.
	if existing, _ := cfg.Profile(profileName); existing.KeyID != "" {
		if err := client.RevokeAPIKeyWithCookie(sessionID, existing.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke previous key %s: %v\n", existing.KeyID, err)
		}
	}

	keyName := "cli@" + hostname()
	key, keyID, err := client.CreateAPIKey(sessionID, keyName)
	if err != nil {
		return fmt.Errorf("create API key failed: %w", err)
	}

	// Drop the throwaway session; failure here is non-fatal.
	if err := client.Logout(sessionID); err != nil {
		fmt.Fprintf(out, "warning: could not close bootstrap session: %v\n", err)
	}

	cfg.SetProfile(profileName, clicfg.Profile{
		URL:      url,
		Username: username,
		KeyName:  keyName,
		KeyID:    keyID,
		Key:      key,
	})
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("API key %q (id %s) was created but saving config failed; "+
			"revoke it from the web UI to avoid an orphaned key: %w", keyName, keyID, err)
	}

	fmt.Fprintf(out, "Logged in to %s as %s.\nStored API key %q (%s)", url, username, keyName, maskKey(key))
	if path, err := clicfg.Path(); err == nil {
		fmt.Fprintf(out, " in %s", path)
	}
	fmt.Fprintln(out, ".")
	return nil
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliauth/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliauth/
git commit -m "feat: add internal/cliauth login bootstrap"
```

---

## Task 4: Repoint `nexorious setup` and remove the moved commands

**Files:**
- Modify: `cmd/nexorious/setup.go`
- Modify: `cmd/nexorious/main.go`
- Modify: `cmd/nexorious/main_test.go`
- Delete: `cmd/nexorious/login.go`, `login_test.go`, `logout.go`, `logout_test.go`, `whoami.go`, `whoami_test.go`, `api_key.go`, `api_key_test.go`

**Interfaces:**
- Consumes: `cliauth.DefaultServerURL`, `cliauth.LoginAndStoreKey` (Task 3); `cliui.FirstNonEmpty`, `cliui.Prompt` (Task 1).

- [ ] **Step 1: Update `setup.go` to use the shared packages**

In `cmd/nexorious/setup.go`: add imports `"github.com/drzero42/nexorious/internal/cliauth"` and `"github.com/drzero42/nexorious/internal/cliui"`, then replace the three call sites:

```go
	url := cliui.FirstNonEmpty(opts.url, cliauth.DefaultServerURL)
```
```go
		username, err = cliui.Prompt(in, out, "Username: ")
```
```go
		if err := cliauth.LoginAndStoreKey(out, client, cfg, cfg.CurrentName(), url, username, password); err != nil {
			return fmt.Errorf("admin created, but --login failed (run \"nexctl account login\"): %w", err)
		}
```
Also update the earlier `--login` config-load error string in `setup.go` to reference `nexctl account login`. Replace the `--url` flag default help text `defaultServerURL` reference with `cliauth.DefaultServerURL`.

- [ ] **Step 2: Delete the moved command files**

```bash
git rm cmd/nexorious/login.go cmd/nexorious/login_test.go \
       cmd/nexorious/logout.go cmd/nexorious/logout_test.go \
       cmd/nexorious/whoami.go cmd/nexorious/whoami_test.go \
       cmd/nexorious/api_key.go cmd/nexorious/api_key_test.go
```

- [ ] **Step 3: Drop the commands from the root**

In `cmd/nexorious/main.go`, remove these four lines:

```go
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newWhoamiCmd())
	root.AddCommand(newAPIKeyCmd())
```

In `cmd/nexorious/main_test.go`, remove `"login"`, `"logout"`, `"whoami"` from the `wantSubcommands` map in `TestRootCmd_StructureAndSubcommands` and from the slice in `TestHelp_MentionsAllSubcommands`.

- [ ] **Step 4: Verify the server binary still builds and tests pass**

Run: `go build ./cmd/nexorious && go test ./cmd/nexorious/...`
Expected: PASS — no references to the deleted commands remain; `setup` still works via the shared packages.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/
git commit -m "refactor: move account commands' shared bootstrap to internal packages"
```

---

## Task 5: `cmd/nexctl` scaffold — root, global flags, version

**Files:**
- Create: `cmd/nexctl/main.go`
- Create: `cmd/nexctl/version.go`
- Test: `cmd/nexctl/main_test.go`

**Interfaces:**
- Produces (used by Tasks 6–8):
  - `func newRootCmd() *cobra.Command` — root `nexctl` with persistent flags `--profile` (string), `--json` (bool), `--quiet`/`-q` (bool), `--yes`/`-y` (bool).
  - `func profileName(cmd *cobra.Command, cfg *clicfg.Config) string` — value of `--profile`, defaulting to `cfg.CurrentName()`.
  - `func resolveProfile(cmd *cobra.Command) (clicfg.Profile, *clicfg.Config, error)` — loads config, resolves the active profile, errors with a `nexctl account login` hint if there is no stored key.
  - `func flagBool(cmd *cobra.Command, name string) bool` — reads an inherited persistent bool flag, ignoring lookup errors.
  - package vars `version`, `commit`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Structure(t *testing.T) {
	root := newRootCmd()
	if root.Use != "nexctl" {
		t.Errorf("root.Use = %q, want nexctl", root.Use)
	}
	for _, f := range []string{"profile", "json", "quiet", "yes"} {
		if root.PersistentFlags().Lookup(f) == nil {
			t.Errorf("expected persistent flag --%s", f)
		}
	}
	want := map[string]bool{"account": false, "profile": false, "version": false}
	for _, sub := range root.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q", name)
		}
	}
}

func TestVersionCmd(t *testing.T) {
	prevV, prevC := version, commit
	version, commit = "9.9.9-test", "cafef00d"
	t.Cleanup(func() { version, commit = prevV, prevC })

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "nexctl ") || !strings.Contains(got, "9.9.9-test") || !strings.Contains(got, "cafef00d") {
		t.Errorf("version output = %q", got)
	}
}
```

Note: `account` and `profile` subcommands referenced here are added in Tasks 6 and 8; this test will not fully pass until those tasks land. Until then, assert only `version` is registered, then extend the `want` map in Tasks 6/8. (Implementer: in this task, register only `version`; add `account`/`profile` to the test's `want` map as those tasks complete.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/...`
Expected: FAIL — package undefined.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/main.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

var errNoSubcommand = errors.New("no subcommand provided")

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nexctl",
		Short: "Nexorious CLI client — manage a remote collection from the terminal",
		Long: "nexctl is a REST client for a Nexorious server. Authenticate with\n" +
			"`nexctl account login`, then manage your collection, pools, tags, sync,\n" +
			"and more. Use --profile to target one of several configured servers.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errNoSubcommand
		},
	}

	pf := root.PersistentFlags()
	pf.String("profile", "", "Config profile to use (default: the current profile)")
	pf.Bool("json", false, "Emit machine-readable JSON")
	pf.BoolP("quiet", "q", false, "Emit only bare ids/values for piping")
	pf.BoolP("yes", "y", false, "Skip confirmation prompts")

	root.AddCommand(newVersionCmd())

	return root
}

// profileName returns the --profile value, defaulting to the current profile.
func profileName(cmd *cobra.Command, cfg *clicfg.Config) string {
	name, _ := cmd.Flags().GetString("profile") //nolint:errcheck // absent flag yields ""
	if name == "" {
		name = cfg.CurrentName()
	}
	return name
}

// resolveProfile loads config and returns the active profile, erroring with a
// login hint when no API key is stored for it.
func resolveProfile(cmd *cobra.Command) (clicfg.Profile, *clicfg.Config, error) {
	cfg, err := clicfg.Load()
	if err != nil {
		return clicfg.Profile{}, nil, err
	}
	name := profileName(cmd, cfg)
	p, ok := cfg.Profile(name)
	if !ok || p.Key == "" {
		return clicfg.Profile{}, nil, fmt.Errorf("not logged in to profile %q (run `nexctl account login` first)", name)
	}
	return p, cfg, nil
}

// flagBool reads an inherited persistent bool flag.
func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name) //nolint:errcheck // absent flag yields false
	return v
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		if !errors.Is(err, errNoSubcommand) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
```

`cmd/nexctl/version.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "nexctl %s (%s)\n", version, commit)
		},
	}
}
```

In the test from Step 1, narrow the `want` map to `{"version": false}` for now.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: scaffold nexctl binary with root, global flags, and version"
```

---

## Task 6: `account` group — login / logout / whoami

**Files:**
- Create: `cmd/nexctl/account.go`
- Test: `cmd/nexctl/account_test.go`

**Interfaces:**
- Consumes: `cliauth.LoginAndStoreKey`, `cliui.{Prompt,ReadPassword,FirstNonEmpty}`, `resolveProfile`, `profileName`, `flagBool`.
- Produces: `func newAccountCmd() *cobra.Command` registered on root.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func accountTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountLoginWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s", err, out.String())
	}

	cfg, _ := clicfg.Load()
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key != "nxr_minted" || p.URL != srv.URL {
		t.Fatalf("stored profile = %+v ok=%v", p, ok)
	}
}

func TestAccountWhoami(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := accountTestServer(t)
	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_x", KeyID: "key-1"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "whoami"})
	if err := root.Execute(); err != nil {
		t.Fatalf("whoami: %v\n%s", err, out.String())
	}
	if got := out.String(); got == "" || !bytes.Contains([]byte(got), []byte("alice")) {
		t.Fatalf("whoami output = %q", got)
	}
}

// TestTopLevelLoginAlias verifies `nexctl login` works identically to
// `nexctl account login`.
func TestTopLevelLoginAlias(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("top-level login: %v\n%s", err, out.String())
	}
	cfg, _ := clicfg.Load()
	if p, ok := cfg.CurrentProfile(); !ok || p.Key != "nxr_minted" {
		t.Fatalf("top-level login did not store key: %+v ok=%v", p, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestAccount`
Expected: FAIL — `account` command undefined.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/account.go`:

```go
package main

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Authenticate and inspect the active session",
	}
	cmd.AddCommand(newLoginCmd(), newLogoutCmd(), newWhoamiCmd(), newAPIKeyCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	var urlFlag, usernameFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a Nexorious server and store an API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, urlFlag, usernameFlag)
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Server URL (prompted if omitted)")
	cmd.Flags().StringVar(&usernameFlag, "username", "", "Username (prompted if omitted)")
	return cmd
}

func runLogin(cmd *cobra.Command, urlFlag, usernameFlag string) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	name := profileName(cmd, cfg)
	existing, _ := cfg.Profile(name)

	in := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	url := cliui.FirstNonEmpty(urlFlag, existing.URL)
	if url == "" {
		url, err = cliui.Prompt(in, out, fmt.Sprintf("Server URL [%s]: ", cliauth.DefaultServerURL))
		if err != nil {
			return err
		}
		url = cliui.FirstNonEmpty(url, cliauth.DefaultServerURL)
	}

	username := cliui.FirstNonEmpty(usernameFlag, existing.Username)
	if username == "" {
		username, err = cliui.Prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}

	password, err := cliui.ReadPassword(in, out)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	return cliauth.LoginAndStoreKey(out, cliclient.New(url), cfg, name, url, username, password)
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored API key and clear it from config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, cfg, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if p.KeyID != "" {
				if err := cliclient.New(p.URL).RevokeAPIKeyWithBearer(p.Key, p.KeyID); err != nil {
					fmt.Fprintf(out, "warning: could not revoke key on server: %v\n", err)
				}
			}
			if err := clearStoredKey(cfg, profileName(cmd, cfg)); err != nil {
				return err
			}
			fmt.Fprintf(out, "Logged out of %s.\n", p.URL)
			return nil
		},
	}
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the user authenticated by the stored API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			username, err := cliclient.New(p.URL).Me(p.Key)
			if err != nil {
				return fmt.Errorf("could not verify stored key (it may be revoked or expired): %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s @ %s\n", username, p.URL)
			return nil
		},
	}
}

// clearStoredKey wipes the API key from the named profile and saves config.
func clearStoredKey(cfg *clicfg.Config, name string) error {
	p, _ := cfg.Profile(name)
	p.Key, p.KeyID, p.KeyName = "", "", ""
	cfg.SetProfile(name, p)
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
```

Register it: in `cmd/nexctl/main.go`'s `newRootCmd`, add the account group **and** the two top-level convenience aliases:

```go
	root.AddCommand(newAccountCmd())
	root.AddCommand(newLoginCmd(), newLogoutCmd()) // top-level aliases for `account login`/`logout`
```

The constructors return a fresh command instance per call, so the root-level `login`/`logout` and the `account login`/`logout` instances are independent commands sharing the same run logic. Both appearing in help is intentional. Add `"account"`, `"login"`, and `"logout"` to the `want` map in `main_test.go`'s `TestRootCmd_Structure`.

Note: `clearStoredKey` keeps the profile entry (URL/username) and is `SetProfile`, which also marks it current — acceptable for the solo-profile common case. `newAPIKeyCmd` is provided by Task 7; until then, temporarily omit it from `newAccountCmd`'s `AddCommand` call and add it in Task 7.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestAccount`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl account login/logout/whoami"
```

---

## Task 7: `account api-key` group — generate / list / revoke

**Files:**
- Create: `cmd/nexctl/api_key.go`
- Test: `cmd/nexctl/api_key_test.go`

**Interfaces:**
- Consumes: `cliclient.{ListAPIKeys,CreateAPIKeyWithBearer,RevokeAPIKeyWithBearer,APIKey}`, `resolveProfile`, `clearStoredKey`, `profileName`, `flagBool`, `cliui.{EncodeJSON,Confirm}`.
- Produces: `func newAPIKeyCmd() *cobra.Command`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestAPIKeyListJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "key-1", "name": "cli@host", "scopes": "write", "created_at": "2026-01-01T00:00:00Z"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Key: "nxr_x", KeyID: "key-0"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--json", "account", "api-key", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("\"id\": \"key-1\"")) {
		t.Fatalf("json output = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestAPIKey`
Expected: FAIL — `api-key` command undefined.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/api_key.go` — ported from the old `cmd/nexorious/api_key.go`, with: `currentProfile()` replaced by `resolveProfile(cmd)`; the per-command `--json` flag replaced by the global `flagBool(cmd, "json")` + `cliui.EncodeJSON`; the inline revoke prompt replaced by `cliui.Confirm(in, out, "...", flagBool(cmd,"yes"))`; `clearStoredKey(cfg, profileName(cmd, cfg))` for self-revoke; and login hints saying `nexctl account login`.

```go
package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newAPIKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "api-key",
		Aliases: []string{"keys"},
		Short:   "Manage your API keys on a Nexorious server",
	}
	cmd.AddCommand(newAPIKeyGenerateCmd(), newAPIKeyListCmd(), newAPIKeyRevokeCmd())
	return cmd
}

func formatNullableTime(t *time.Time, zero string) string {
	if t == nil {
		return zero
	}
	return t.Local().Format("2006-01-02 15:04")
}

func newAPIKeyGenerateCmd() *cobra.Command {
	var name, scopes, expiresAt string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Create a new API key and print it once",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if scopes != "read" && scopes != "write" {
				return fmt.Errorf("--scopes must be 'read' or 'write'")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			client := cliclient.New(p.URL)
			if existing, err := client.ListAPIKeys(p.Key); err == nil {
				for _, k := range existing {
					if k.Name == name {
						fmt.Fprintf(out, "warning: an API key named %q already exists\n", name)
						break
					}
				}
			}
			var expPtr *string
			if expiresAt != "" {
				expPtr = &expiresAt
			}
			key, err := client.CreateAPIKeyWithBearer(p.Key, name, scopes, expPtr)
			if err != nil {
				return fmt.Errorf("create API key failed: %w", err)
			}
			fmt.Fprintf(out, "API key created. Copy it now — it will not be shown again:\n\n  %s\n\n", key.Key)
			fmt.Fprintf(out, "id:      %s\nname:    %s\nscopes:  %s\nexpires: %s\n",
				key.ID, key.Name, key.Scopes, formatNullableTime(key.ExpiresAt, "never"))
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Label for the key (required)")
	cmd.Flags().StringVar(&scopes, "scopes", "write", "Key scopes: read or write")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "Optional expiry as an RFC3339 timestamp")
	return cmd
}

func newAPIKeyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your API keys",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			keys, err := cliclient.New(p.URL).ListAPIKeys(p.Key)
			if err != nil {
				return fmt.Errorf("list API keys failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, keys)
			}
			if len(keys) == 0 {
				fmt.Fprintln(out, "No API keys.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tSCOPES\tCREATED\tLAST USED\tEXPIRES")
			for _, k := range keys {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					k.ID, k.Name, k.Scopes,
					k.CreatedAt.Local().Format("2006-01-02 15:04"),
					formatNullableTime(k.LastUsedAt, "never"),
					formatNullableTime(k.ExpiresAt, "–"))
			}
			return tw.Flush()
		},
	}
}

func newAPIKeyRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <id-or-name>",
		Short: "Revoke an API key by id or name (from `api-key list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, cfg, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			client := cliclient.New(p.URL)
			keys, err := client.ListAPIKeys(p.Key)
			if err != nil {
				return fmt.Errorf("list API keys failed: %w", err)
			}
			targetID, err := resolveKeyID(keys, args[0])
			if err != nil {
				return err
			}
			self := targetID == p.KeyID
			if self {
				ok, _ := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
					"Revoke the key this CLI is currently using? This will log you out.", flagBool(cmd, "yes"))
				if !ok {
					return fmt.Errorf("aborted")
				}
			}
			if err := client.RevokeAPIKeyWithBearer(p.Key, targetID); err != nil {
				return fmt.Errorf("revoke failed: %w", err)
			}
			if self {
				url := p.URL
				if err := clearStoredKey(cfg, profileName(cmd, cfg)); err != nil {
					return err
				}
				fmt.Fprintf(out, "Revoked API key %s.\nThat was the key this CLI was using — you have been logged out of %s.\n", targetID, url)
				return nil
			}
			fmt.Fprintf(out, "Revoked API key %s.\n", targetID)
			return nil
		},
	}
}

func resolveKeyID(keys []cliclient.APIKey, idOrName string) (string, error) {
	for _, k := range keys {
		if k.ID == idOrName {
			return k.ID, nil
		}
	}
	var matches []string
	for _, k := range keys {
		if k.Name == idOrName {
			matches = append(matches, k.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no API key with id or name %q", idOrName)
	default:
		return "", fmt.Errorf("multiple active keys named %q; revoke by id instead (see `api-key list`)", idOrName)
	}
}
```

Re-add `newAPIKeyCmd()` to `newAccountCmd`'s `AddCommand` call (deferred from Task 6).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestAPIKey`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl account api-key generate/list/revoke"
```

---

## Task 8: `profile` group — list / use / add / rm

**Files:**
- Create: `cmd/nexctl/profile.go`
- Test: `cmd/nexctl/profile_test.go`

**Interfaces:**
- Consumes: `clicfg.{Load,Save,Names,Profile,SetProfile,SetCurrent,RemoveProfile,CurrentName}`, `cliui.{EncodeJSON,Confirm}`, `flagBool`.
- Produces: `func newProfileCmd() *cobra.Command`.

Semantics:
- `list` — table (or `--json` / `-q` bare names) of profiles, marking the current one.
- `use <name>` — make an existing profile current (error if unknown).
- `add <name> [--url]` — create an empty/url-only profile and mark it current; a later `account login` fills in the key.
- `rm <name>` — delete a profile (confirm on TTY unless `--yes`).

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestProfileAddUseRm(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	run := func(args ...string) (string, error) {
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetIn(bytes.NewReader(nil))
		root.SetArgs(args)
		return out.String(), root.Execute()
	}

	if _, err := run("profile", "add", "work", "--url", "http://work:8000"); err != nil {
		t.Fatalf("add: %v", err)
	}
	cfg, _ := clicfg.Load()
	if p, ok := cfg.Profile("work"); !ok || p.URL != "http://work:8000" {
		t.Fatalf("work profile = %+v ok=%v", p, ok)
	}
	if cfg.CurrentName() != "work" {
		t.Fatalf("add should switch current, got %q", cfg.CurrentName())
	}

	out, err := run("-q", "profile", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("work")) {
		t.Fatalf("list -q = %q", out)
	}

	if _, err := run("profile", "rm", "work", "--yes"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	cfg, _ = clicfg.Load()
	if _, ok := cfg.Profile("work"); ok {
		t.Fatal("work should be gone")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestProfile`
Expected: FAIL — `profile` command undefined.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/profile.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configured server profiles",
	}
	cmd.AddCommand(newProfileListCmd(), newProfileUseCmd(), newProfileAddCmd(), newProfileRmCmd())
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			names := cfg.Names()
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, cfg.Profiles)
			}
			if flagBool(cmd, "quiet") {
				for _, n := range names {
					fmt.Fprintln(out, n)
				}
				return nil
			}
			if len(names) == 0 {
				fmt.Fprintln(out, "No profiles. Run `nexctl account login` to create one.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "CURRENT\tNAME\tURL\tUSER")
			for _, n := range names {
				p, _ := cfg.Profile(n)
				marker := ""
				if n == cfg.CurrentName() {
					marker = "*"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", marker, n, p.URL, p.Username)
			}
			return tw.Flush()
		},
	}
}

func newProfileUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if err := cfg.SetCurrent(args[0]); err != nil {
				return err
			}
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Now using profile %q.\n", args[0])
			return nil
		},
	}
}

func newProfileAddCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a profile and switch to it (run `account login` to authenticate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profile(args[0]); ok {
				return fmt.Errorf("profile %q already exists", args[0])
			}
			cfg.SetProfile(args[0], clicfg.Profile{URL: url})
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created profile %q (now current). Run `nexctl account login` to authenticate.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "Server URL for the new profile")
	return cmd
}

func newProfileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profile(args[0]); !ok {
				return fmt.Errorf("no profile named %q", args[0])
			}
			ok, _ := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete profile %q?", args[0]), flagBool(cmd, "yes"))
			if !ok {
				return fmt.Errorf("aborted")
			}
			cfg.RemoveProfile(args[0])
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(out, "Deleted profile %q.\n", args[0])
			return nil
		},
	}
}
```

Register it: in `newRootCmd`, add `root.AddCommand(newProfileCmd())`. Add `"profile"` to the `want` map in `main_test.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/...`
Expected: PASS (full suite, including the extended `TestRootCmd_Structure`).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl profile list/use/add/rm"
```

---

## Task 9: Build wiring, docs, and dead-code reconciliation

**Files:**
- Modify: `Makefile`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Build `nexctl` in the Makefile**

In `Makefile`, change the `build` target to build both binaries:

```make
build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious
	go build $(LDFLAGS) -o nexctl ./cmd/nexctl
```

- [ ] **Step 2: Verify both binaries build**

Run: `make build`
Expected: produces `./nexorious` and `./nexctl` with no errors.

- [ ] **Step 3: Smoke-test the new binary**

Run: `./nexctl version && ./nexctl --help && ./nexctl account --help`
Expected: prints `nexctl <version> (<commit>)`, the root help listing `account`/`profile`/`version`, and the account subcommands.

- [ ] **Step 4: Document the new layout in CLAUDE.md**

Under **Project Structure**, add entries for `cmd/nexctl/` (the client binary), `internal/cliui/` (shared terminal helpers), and `internal/cliauth/` (login bootstrap), and note that `cmd/nexorious` no longer hosts `login`/`logout`/`whoami`/`api-key` (now `nexctl account …`).

- [ ] **Step 5: Reconcile dead code and commit**

Run: `make deadcode`
Expected: no *new* entries attributable to this change (the old `cmd/nexorious` helpers were deleted, not orphaned; the new exported `clicfg` methods are all called).

```bash
git add Makefile CLAUDE.md
git commit -m "build: build nexctl binary and document the client layout"
```

---

## Self-Review

**Spec coverage:** Phase 1 of the spec = scaffold + global flags + TTY helper + shared `cliui`/`cliauth` extraction + move account commands + profile management. Tasks 1–9 cover each: cliui (T1), clicfg helpers (T2), cliauth (T3), nexorious repoint+removal (T4), nexctl root/flags/version (T5), account login/logout/whoami (T6), api-key (T7), profile (T8), build/docs (T9). `account passwd` and packaging are explicitly deferred in the spec — not in this plan.

**Placeholder scan:** No TBD/TODO; every code and test step carries complete code and exact run commands.

**Type consistency:** `LoginAndStoreKey` signature (with `profileName`) is consistent between T3 (definition), T4 (setup call), and T6 (login call). `clearStoredKey(cfg, name)` is defined in T6 and reused in T7. `flagBool`/`profileName`/`resolveProfile` are defined in T5 and consumed in T6–T8. `cliui.Confirm`/`EncodeJSON`/`FirstNonEmpty`/`Prompt`/`ReadPassword` signatures match across T1 and their callers. The `TestRootCmd_Structure` `want` map is grown incrementally across T5/T6/T8 (noted in each task).
