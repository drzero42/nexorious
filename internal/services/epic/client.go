package epic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client invokes the Legendary CLI as a subprocess to manage an Epic Games
// Store library. workDir is the value of LEGENDARY_WORK_DIR.
type Client struct {
	workDir string
}

// NewClient creates a new epic Client. workDir may be empty, in which case
// Configured() returns false and all operations return an error.
func NewClient(workDir string) *Client {
	return &Client{workDir: workDir}
}

// Configured reports whether LEGENDARY_WORK_DIR is set.
func (c *Client) Configured() bool {
	return c.workDir != ""
}

// EpicAccountInfo holds the account details extracted from legendary's user.json.
type EpicAccountInfo struct {
	DisplayName string
	AccountID   string
}

// ExternalGameEntry is a normalised game entry from the Epic library.
type ExternalGameEntry struct {
	ExternalID      string // legendary app_name
	Title           string
	Namespace       string
	CatalogItemID   string
	OwnershipStatus string // always "owned"
}

// userDir returns the per-user base directory (XDG_CONFIG_HOME equivalent).
func (c *Client) userDir(userID string) string {
	return filepath.Join(c.workDir, userID)
}

// legendaryDir returns the directory legendary writes its config into
// (legendary appends "/legendary" to XDG_CONFIG_HOME itself).
func (c *Client) legendaryDir(userID string) string {
	return filepath.Join(c.userDir(userID), "legendary")
}

// Authenticate runs `legendary auth --code <code>` in a fresh per-user dir,
// reads user.json to extract account info, captures the resulting snapshot,
// and returns both. The snapshot must be stored in the DB by the caller.
func (c *Client) Authenticate(ctx context.Context, userID, authCode string) (*EpicAccountInfo, map[string]string, error) {
	if !c.Configured() {
		return nil, nil, fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}
	if _, err := exec.LookPath("legendary"); err != nil {
		return nil, nil, fmt.Errorf("epic: legendary not found in PATH")
	}
	if err := os.MkdirAll(c.legendaryDir(userID), 0o750); err != nil {
		return nil, nil, fmt.Errorf("epic: create user dir: %w", err)
	}
	if _, err := c.runLegendary(ctx, userID, "auth", "--code", authCode); err != nil {
		return nil, nil, err
	}
	info, err := c.readUserJSON(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("epic: read user.json after auth: %w", err)
	}
	snapshot, err := c.CaptureSnapshot(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("epic: capture snapshot after auth: %w", err)
	}
	return info, snapshot, nil
}

// GetLibrary runs `legendary list --json`, parses the output, skips DLC entries,
// and streams results to onBatch. The caller is responsible for restoring the
// snapshot before calling this method and capturing it afterward.
func (c *Client) GetLibrary(ctx context.Context, userID string, onBatch func([]ExternalGameEntry) error) error {
	if !c.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}
	out, err := c.runLegendary(ctx, userID, "list", "--json")
	if err != nil {
		return err
	}

	var raw []struct {
		AppName         string `json:"app_name"`
		AppTitle        string `json:"app_title"`
		Namespace       string `json:"namespace"`
		CatalogItemID   string `json:"catalog_item_id"`
		MainGameAppName string `json:"main_game_appname"` // non-empty for DLC
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return fmt.Errorf("epic: parse legendary list output: %w", err)
	}

	entries := make([]ExternalGameEntry, 0, len(raw))
	for _, r := range raw {
		if r.MainGameAppName != "" {
			continue // skip DLC
		}
		entries = append(entries, ExternalGameEntry{
			ExternalID:      r.AppName,
			Title:           r.AppTitle,
			Namespace:       r.Namespace,
			CatalogItemID:   r.CatalogItemID,
			OwnershipStatus: "owned",
		})
	}

	slog.Info("epic: library parsed", "user_id", userID, "total", len(raw), "after_dlc_filter", len(entries))

	if len(entries) == 0 {
		return nil
	}
	return onBatch(entries)
}

// Cleanup removes the per-user working directory.
func (c *Client) Cleanup(_ context.Context, userID string) error {
	return os.RemoveAll(c.userDir(userID))
}

// RestoreSnapshot writes each file from snapshot into the per-user legendary
// config directory, creating subdirectories as needed.
func (c *Client) RestoreSnapshot(userID string, snapshot map[string]string) error {
	legendaryDir := c.legendaryDir(userID)
	for relPath, content := range snapshot {
		if filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
			return fmt.Errorf("epic: restore snapshot: unsafe path %q", relPath)
		}
		fullPath := filepath.Join(legendaryDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			return fmt.Errorf("epic: restore snapshot mkdir %s: %w", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("epic: restore snapshot write %s: %w", relPath, err)
		}
	}
	return nil
}

// CaptureSnapshot reads all files from the per-user legendary config directory
// into a map of relative-path → content, excluding *.lock, tmp/, and manifests/.
// Returns an empty map (not an error) if the directory does not exist.
func (c *Client) CaptureSnapshot(userID string) (map[string]string, error) {
	legendaryDir := c.legendaryDir(userID)
	if _, err := os.Stat(legendaryDir); os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	snapshot := make(map[string]string)
	err := filepath.WalkDir(legendaryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "tmp" || name == "manifests" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".lock") {
			return nil
		}
		relPath, err := filepath.Rel(legendaryDir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("epic: capture snapshot read %s: %w", relPath, err)
		}
		snapshot[relPath] = string(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("epic: capture snapshot walk: %w", err)
	}
	return snapshot, nil
}

// runLegendary executes legendary with the given args, setting XDG_CONFIG_HOME
// to the per-user directory. Returns stdout on success, or an error wrapping
// stderr on non-zero exit.
func (c *Client) runLegendary(ctx context.Context, userID string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "legendary", args...)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+c.userDir(userID))
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("epic: legendary %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("epic: legendary %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type userJSON struct {
	DisplayName string `json:"displayName"`
	AccountID   string `json:"account_id"`
}

func (c *Client) readUserJSON(userID string) (*EpicAccountInfo, error) {
	path := filepath.Join(c.legendaryDir(userID), "user.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("epic: read user.json: %w", err)
	}
	var u userJSON
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("epic: parse user.json: %w", err)
	}
	if u.DisplayName == "" && u.AccountID == "" {
		return nil, fmt.Errorf("epic: user.json missing displayName and account_id")
	}
	return &EpicAccountInfo{DisplayName: u.DisplayName, AccountID: u.AccountID}, nil
}
