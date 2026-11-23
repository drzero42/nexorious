package cliclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// BackupConfig is the backup schedule and retention configuration as returned
// by GET /api/admin/backups/config and PUT /api/admin/backups/config.
type BackupConfig struct {
	Schedule       string `json:"schedule"`
	ScheduleTime   string `json:"schedule_time"`
	ScheduleDay    int    `json:"schedule_day"`
	RetentionMode  string `json:"retention_mode"`
	RetentionValue int    `json:"retention_value"`
	UpdatedAt      string `json:"updated_at"`
}

// BackupStats holds per-backup row counts for the user's library.
type BackupStats struct {
	Users int `json:"users"`
	Games int `json:"games"`
	Tags  int `json:"tags"`
}

// Backup is one entry in the admin backup list.
type Backup struct {
	ID         string      `json:"id"`
	CreatedAt  string      `json:"created_at"`
	BackupType string      `json:"backup_type"`
	SizeBytes  int64       `json:"size_bytes"`
	Stats      BackupStats `json:"stats"`
}

// CreateBackupResult is the response from POST /api/admin/backups.
type CreateBackupResult struct {
	BackupID string `json:"backup_id"`
	Message  string `json:"message"`
}

// GetBackupConfig returns the current backup schedule and retention settings
// from GET /api/admin/backups/config.
func (c *Client) GetBackupConfig(ctx context.Context, key string) (*BackupConfig, error) {
	var out BackupConfig
	if err := c.doBearer(ctx, http.MethodGet, "/api/admin/backups/config", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateBackupConfig updates the backup schedule and retention settings via
// PUT /api/admin/backups/config and returns the saved configuration. The
// UpdatedAt field of cfg is ignored by the server.
func (c *Client) UpdateBackupConfig(ctx context.Context, key string, cfg BackupConfig) (*BackupConfig, error) {
	var out BackupConfig
	if err := c.doBearer(ctx, http.MethodPut, "/api/admin/backups/config", key, cfg, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// backupListEnvelope is the wire envelope for GET /api/admin/backups.
type backupListEnvelope struct {
	Backups []Backup `json:"backups"`
	Total   int      `json:"total"`
}

// ListBackups returns all stored backups from GET /api/admin/backups.
func (c *Client) ListBackups(ctx context.Context, key string) ([]Backup, error) {
	var env backupListEnvelope
	if err := c.doBearer(ctx, http.MethodGet, "/api/admin/backups", key, nil, &env); err != nil {
		return nil, err
	}
	return env.Backups, nil
}

// CreateBackup triggers a manual backup via POST /api/admin/backups and
// returns the new backup's ID and a status message.
func (c *Client) CreateBackup(ctx context.Context, key string) (*CreateBackupResult, error) {
	var out CreateBackupResult
	if err := c.doBearer(ctx, http.MethodPost, "/api/admin/backups", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteBackup removes the backup with the given id via
// DELETE /api/admin/backups/:id (expects 204).
func (c *Client) DeleteBackup(ctx context.Context, key, id string) error {
	return c.doBearer(ctx, http.MethodDelete, "/api/admin/backups/"+url.PathEscape(id), key, nil, nil)
}

// OpenBackupDownload opens the backup archive for id via
// GET /api/admin/backups/:id/download. The response body is a raw tar.gz
// stream; it is not JSON-decoded. It returns the server-suggested filename
// (parsed from Content-Disposition, "" when absent) and the open body, which
// the caller must Close. On a non-2xx response the body is consumed/closed and
// a nil body is returned alongside the error.
func (c *Client) OpenBackupDownload(ctx context.Context, key, id string) (filename string, body io.ReadCloser, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/admin/backups/"+url.PathEscape(id)+"/download", nil)
	if err != nil {
		return "", nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer func() { _ = resp.Body.Close() }()
		return "", nil, httpError(resp)
	}
	return filenameFromContentDisposition(resp.Header.Get("Content-Disposition")), resp.Body, nil
}
