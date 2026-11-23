package cliclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// backupOpTimeout bounds one backup or restore request. The server answers
// only once the work has ended: a backup runs pg_dump, whose own deadline
// grows with the database size; a restore also stops background work (up to
// 30 s), takes a pre-restore backup, loads the archive and migrates it. The
// client's usual 30 s would give up long before the answer.
const backupOpTimeout = 2 * time.Hour

// RestoreResult is the body of a restore response, success or failure. Every
// response sent once the restore has started carries Restarting and the
// InstanceID of the server that sent it; Restarting is nil when the body did
// not come from the restore handler (an auth refusal, a proxy's error page).
type RestoreResult struct {
	Success            bool   `json:"success"`
	Message            string `json:"message"`
	Error              string `json:"error"`
	Restarting         *bool  `json:"restarting"`
	InstanceID         string `json:"instance_id"`
	BackupID           string `json:"backup_id"`
	PreRestoreBackupID string `json:"pre_restore_backup_id"`
	MigrationError     string `json:"migration_error"`
	RolledBack         bool   `json:"rolled_back"`
}

// RestoreBackup restores the server's database from the backup with the given
// id via POST /api/admin/backups/:id/restore.
func (c *Client) RestoreBackup(ctx context.Context, key, id string) (*RestoreResult, error) {
	req, err := newJSONRequest(ctx, c.baseURL+"/api/admin/backups/"+url.PathEscape(id)+"/restore", key, map[string]any{"confirm": true})
	if err != nil {
		return nil, err
	}
	return c.doRestore(req)
}

// RestoreBackupUpload uploads a backup archive and restores from it via
// POST /api/admin/backups/restore/upload (multipart field "file"). body is
// streamed into the request, so the caller can pass an *os.File directly
// instead of reading a (potentially multi-GB) archive into memory.
func (c *Client) RestoreBackupUpload(ctx context.Context, key, filename string, body io.Reader) (*RestoreResult, error) {
	req, err := c.newMultipartRequest(ctx, http.MethodPost, "/api/admin/backups/restore/upload", key, filename, body, nil)
	if err != nil {
		return nil, err
	}
	return c.doRestore(req)
}

// SetupRestoreUpload uploads a backup archive and restores a fresh instance
// from it via POST /api/auth/setup/restore (multipart field "file"). The body
// is streamed, so the caller can pass an *os.File directly. Unauthenticated
// (pre-bootstrap), so no API key is sent.
func (c *Client) SetupRestoreUpload(ctx context.Context, filename string, body io.Reader) (*RestoreResult, error) {
	req, err := c.newMultipartRequest(ctx, http.MethodPost, "/api/auth/setup/restore", "", filename, body, nil)
	if err != nil {
		return nil, err
	}
	return c.doRestore(req)
}

// SetupRestoreFromDisk restores a fresh instance from a named on-disk backup
// via POST /api/auth/setup/restore/disk. Unauthenticated (pre-bootstrap).
func (c *Client) SetupRestoreFromDisk(ctx context.Context, filename string) (*RestoreResult, error) {
	req, err := newJSONRequest(ctx, c.baseURL+"/api/auth/setup/restore/disk", "", map[string]string{"filename": filename})
	if err != nil {
		return nil, err
	}
	return c.doRestore(req)
}

func newJSONRequest(ctx context.Context, target, key string, body any) (*http.Request, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req, nil
}

// doRestore sends a restore request with backupOpTimeout and decodes the body
// whatever the status, since a failed restore's body still says whether the
// server is restarting. A non-2xx status is returned as an *HTTPError next to
// the decoded body (nil when the body was not JSON). A request that got no
// answer returns a nil result.
func (c *Client) doRestore(req *http.Request) (*RestoreResult, error) {
	resp, err := c.withTimeout(backupOpTimeout).hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var res *RestoreResult
	if readErr == nil {
		var r RestoreResult
		if json.Unmarshal(raw, &r) == nil {
			res = &r
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		he := &HTTPError{Status: resp.StatusCode, readErr: readErr}
		if res != nil {
			he.Message = res.Error
			if he.Message == "" {
				he.Message = res.Message
			}
		}
		return res, he
	}
	if res == nil {
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}
		return nil, errors.New("decode response: not a restore result")
	}
	return res, nil
}

// HealthInfo is the GET /health body, as far as the CLI uses it.
type HealthInfo struct {
	// Status is "ok" when the server is ready, otherwise the app-state name.
	Status string `json:"status"`
	// InstanceID is new every time the server process starts.
	InstanceID string `json:"instance_id"`
	// RestoreInProgress is true from the start of a restore until the process
	// is replaced.
	RestoreInProgress bool `json:"restore_in_progress"`
}

// HealthInfo reads GET /health.
func (c *Client) HealthInfo(ctx context.Context) (HealthInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return HealthInfo{}, fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return HealthInfo{}, fmt.Errorf("health request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return HealthInfo{}, httpError(resp)
	}
	var out HealthInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return HealthInfo{}, fmt.Errorf("decode health response: %w", err)
	}
	return out, nil
}

// ErrNoRestart is returned by WaitForNewInstance when no new server appeared
// within the grace period.
var ErrNoRestart = errors.New("the server did not come back")

// WaitForNewInstance polls GET /health every interval until a server with an
// instance id other than oldID answers, and returns its health. With no oldID
// it settles for the first answer that reports no restore in progress.
//
// The grace period runs only while the old server is not reporting a restore
// in progress: a restore can go on long after the request that started it was
// given up on, and the grace starts again each time the old server says it is
// still restoring.
func (c *Client) WaitForNewInstance(ctx context.Context, oldID string, grace, interval time.Duration) (HealthInfo, error) {
	deadline := time.Now().Add(grace)
	for {
		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		h, err := c.HealthInfo(pollCtx)
		cancel()
		if err == nil {
			switch {
			case oldID != "" && h.InstanceID != "" && h.InstanceID != oldID:
				return h, nil
			case oldID == "" && !h.RestoreInProgress:
				return h, nil
			case h.RestoreInProgress:
				deadline = time.Now().Add(grace)
			}
		}
		if time.Now().After(deadline) {
			return HealthInfo{}, fmt.Errorf("%w within %s", ErrNoRestart, grace)
		}
		select {
		case <-ctx.Done():
			return HealthInfo{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}
