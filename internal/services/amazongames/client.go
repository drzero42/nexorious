// Package amazongames syncs an Amazon Games library through the nile CLI
// (https://github.com/imLinguin/nile), the open-source Amazon Games client
// Heroic uses, the way internal/services/epicgamesstore drives legendary.
//
// nile keeps its sign-in in its own config directory. Nexorious stores nile's
// decrypted user data (the device registration and its tokens) encrypted in
// user_sync_configs; for every nile call the client rebuilds nile's account
// files in a fresh temporary NILE_CONFIG_PATH, runs nile, reads the possibly
// refreshed user data back, and deletes the directory.
package amazongames

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrAuthFailed is returned when Amazon rejects the stored sign-in: the
// refresh token was refused, or nile has no signed-in account to use.
var ErrAuthFailed = errors.New("amazon: sign-in rejected")

// Login is one sign-in attempt started by `nile auth --login --non-interactive`.
// ClientID, CodeVerifier and Serial must be kept server-side until Register.
type Login struct {
	URL          string `json:"url"`
	ClientID     string `json:"client_id"`
	CodeVerifier string `json:"code_verifier"`
	Serial       string `json:"serial"`
}

// Account identifies the Amazon account a registration belongs to.
type Account struct {
	UserID string
	Name   string
}

// Entitlement is one game the Amazon account owns.
type Entitlement struct {
	ProductID string
	Title     string
}

// runFunc runs nile with NILE_CONFIG_PATH set to configDir and returns its
// stdout and stderr. err is non-nil when nile could not run or exited non-zero.
type runFunc func(ctx context.Context, configDir string, args ...string) (stdout, stderr []byte, err error)

// Client invokes the nile CLI as a subprocess.
type Client struct {
	run      runFunc
	lookPath func(string) (string, error)
}

// NewClient returns a Client that runs the `nile` binary found on PATH.
func NewClient() *Client {
	return &Client{run: execNile, lookPath: exec.LookPath}
}

// Configured reports whether the nile binary is on PATH. Without it Amazon
// sync is unavailable.
func (c *Client) Configured() bool {
	_, err := c.lookPath("nile")
	return err == nil
}

// LoginURL starts a sign-in: nile generates a device serial, client id and
// PKCE code verifier, and the Amazon sign-in URL built from them.
func (c *Client) LoginURL(ctx context.Context) (*Login, error) {
	dir, err := newConfigDir()
	if err != nil {
		return nil, err
	}
	defer removeConfigDir(dir)

	stdout, stderr, err := c.run(ctx, dir, "auth", "--login", "--non-interactive")
	if err != nil {
		return nil, fmt.Errorf("amazon: nile auth --login failed: %s", sanitize(lastLines(stderr), err.Error()))
	}
	var login Login
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &login); err != nil {
		return nil, errors.New("amazon: nile auth --login printed no login data")
	}
	if login.URL == "" || login.ClientID == "" || login.CodeVerifier == "" || login.Serial == "" {
		return nil, errors.New("amazon: nile auth --login printed incomplete login data")
	}
	return &login, nil
}

// Register finishes a sign-in with the authorization code Amazon put on the
// landing address: `nile register` registers this device and writes the user
// data, which is returned for the caller to store encrypted.
func (c *Client) Register(ctx context.Context, code string, login Login) (*Account, json.RawMessage, error) {
	dir, err := newConfigDir()
	if err != nil {
		return nil, nil, err
	}
	defer removeConfigDir(dir)

	_, stderr, runErr := c.run(ctx, dir, "register",
		"--code", code,
		"--client-id", login.ClientID,
		"--code-verifier", login.CodeVerifier,
		"--serial", login.Serial)

	// nile registers, writes the user data, and then syncs the library in the
	// same command; a failure in that trailing sync exits non-zero after the
	// registration succeeded. The user data on disk is the registration's
	// outcome, so it decides.
	userData, readErr := readNileConfig(dir)
	if readErr != nil {
		detail := lastLines(stderr)
		if runErr != nil {
			detail += "\n" + runErr.Error()
		}
		return nil, nil, fmt.Errorf("amazon: device registration failed: %s",
			sanitize(detail, code, login.ClientID, login.CodeVerifier, login.Serial))
	}
	account, err := accountOf(userData)
	if err != nil {
		return nil, nil, err
	}
	return account, userData, nil
}

// Library reads the account's entitlements with `nile library sync --force`.
// nile reads every page before it writes library.json, so the list is
// complete or an error is returned. refreshed is nile's user data as it was
// left after the run (nile refreshes an expired access token first); it is
// returned whenever it can be read, even alongside an error, so the caller can
// keep a refreshed token.
func (c *Client) Library(ctx context.Context, userData json.RawMessage) (entitlements []Entitlement, refreshed json.RawMessage, err error) {
	if _, err := accountOf(userData); err != nil {
		return nil, nil, err
	}
	dir, err := newConfigDir()
	if err != nil {
		return nil, nil, err
	}
	defer removeConfigDir(dir)

	if err := writeNileConfig(dir, userData); err != nil {
		return nil, nil, err
	}

	_, stderr, runErr := c.run(ctx, dir, "library", "sync", "--force")

	refreshed, readErr := readNileConfig(dir)
	if readErr != nil {
		refreshed = nil
	}
	secrets := secretsOf(userData)
	if runErr != nil {
		if isAuthFailure(stderr) {
			return nil, refreshed, ErrAuthFailed
		}
		return nil, refreshed, fmt.Errorf("amazon: nile library sync failed: %s",
			sanitize(lastLines(stderr)+"\n"+runErr.Error(), secrets...))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "nile", "library.json")) //nolint:gosec // path inside the temp config dir this call created
	if err != nil {
		return nil, refreshed, errors.New("amazon: nile library sync wrote no library")
	}
	entitlements, err = parseLibrary(raw)
	if err != nil {
		return nil, refreshed, err
	}
	return entitlements, refreshed, nil
}

// parseLibrary reads nile's library.json: a list of entitlements, each with
// the product it grants.
func parseLibrary(raw []byte) ([]Entitlement, error) {
	var games []struct {
		Product struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"product"`
	}
	if err := json.Unmarshal(raw, &games); err != nil {
		return nil, errors.New("amazon: nile library.json is not a list of entitlements")
	}
	out := make([]Entitlement, 0, len(games))
	for _, g := range games {
		out = append(out, Entitlement{ProductID: g.Product.ID, Title: g.Product.Title})
	}
	return out, nil
}

// refreshRejected matches nile's log line when Amazon answers a token refresh
// with a 4xx (an invalid or revoked refresh token). A connection failure logs
// the same prefix without "<Response [4", and is not a credentials problem.
var refreshRejected = regexp.MustCompile(`Failed to refresh the token <Response \[4\d\d\]>`)

func isAuthFailure(stderr []byte) bool {
	return refreshRejected.Match(stderr) || bytes.Contains(stderr, []byte("User not logged in"))
}

// execNile runs the real nile binary.
func execNile(ctx context.Context, configDir string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "nile", args...) //nolint:gosec // fixed binary; args are nile subcommands and values nile itself generated or Amazon returned
	cmd.Env = append(os.Environ(), "NILE_CONFIG_PATH="+configDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func newConfigDir() (string, error) {
	dir, err := os.MkdirTemp("", "nexorious-nile-")
	if err != nil {
		return "", fmt.Errorf("amazon: create nile config dir: %w", err)
	}
	// nile downloads the Amazon Games SDK into <config>/nile/SDK on start
	// unless that directory exists; Nexorious never installs games, so an
	// empty one skips the download.
	if err := os.MkdirAll(filepath.Join(dir, "nile", "SDK"), 0o700); err != nil {
		removeConfigDir(dir)
		return "", fmt.Errorf("amazon: create nile config dir: %w", err)
	}
	return dir, nil
}

func removeConfigDir(dir string) {
	_ = os.RemoveAll(dir) //nolint:errcheck // best-effort removal of a temp dir; nothing to surface
}

// lastLines keeps the end of nile's stderr, where its error and a Python
// traceback's final exception line are.
func lastLines(stderr []byte) string {
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	return strings.Join(lines, "\n")
}

// tokenLike matches values that must never reach an error, a log line or a
// job: Amazon tokens (Atna|…, Atnr|…, Atc|…), long hex strings (device serials
// and client ids) and long base64 runs (private keys, ADP tokens).
var tokenLike = regexp.MustCompile(`At[a-z]{1,3}\|[^\s"',}]+|[0-9A-Fa-f]{24,}|[A-Za-z0-9+/=_-]{48,}`)

// sanitize removes every known secret, then anything shaped like one.
func sanitize(text string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, "[redacted]")
		}
	}
	return strings.TrimSpace(tokenLike.ReplaceAllString(text, "[redacted]"))
}

// secretsOf lists the user data's secret values, for sanitize.
func secretsOf(userData json.RawMessage) []string {
	var u struct {
		Tokens struct {
			Bearer struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
			} `json:"bearer"`
			MacDMS struct {
				DevicePrivateKey string `json:"device_private_key"`
				ADPToken         string `json:"adp_token"`
			} `json:"mac_dms"`
		} `json:"tokens"`
		Extensions struct {
			DeviceInfo struct {
				Serial string `json:"device_serial_number"`
			} `json:"device_info"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(userData, &u); err != nil {
		return nil
	}
	return []string{
		u.Tokens.Bearer.AccessToken, u.Tokens.Bearer.RefreshToken,
		u.Tokens.MacDMS.DevicePrivateKey, u.Tokens.MacDMS.ADPToken,
		u.Extensions.DeviceInfo.Serial,
	}
}
