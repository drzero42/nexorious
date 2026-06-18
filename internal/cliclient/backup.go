package cliclient

import (
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
func (c *Client) GetBackupConfig(key string) (*BackupConfig, error) {
	var out BackupConfig
	if err := c.doBearer(http.MethodGet, "/api/admin/backups/config", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateBackupConfig updates the backup schedule and retention settings via
// PUT /api/admin/backups/config and returns the saved configuration. The
// UpdatedAt field of cfg is ignored by the server.
func (c *Client) UpdateBackupConfig(key string, cfg BackupConfig) (*BackupConfig, error) {
	var out BackupConfig
	if err := c.doBearer(http.MethodPut, "/api/admin/backups/config", key, cfg, &out); err != nil {
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
func (c *Client) ListBackups(key string) ([]Backup, error) {
	var env backupListEnvelope
	if err := c.doBearer(http.MethodGet, "/api/admin/backups", key, nil, &env); err != nil {
		return nil, err
	}
	return env.Backups, nil
}

// CreateBackup triggers a manual backup via POST /api/admin/backups and
// returns the new backup's ID and a status message.
func (c *Client) CreateBackup(key string) (*CreateBackupResult, error) {
	var out CreateBackupResult
	if err := c.doBearer(http.MethodPost, "/api/admin/backups", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteBackup removes the backup with the given id via
// DELETE /api/admin/backups/:id (expects 204).
func (c *Client) DeleteBackup(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/admin/backups/"+url.PathEscape(id), key, nil, nil)
}

// DownloadBackup streams the backup archive for id into w via
// GET /api/admin/backups/:id/download. The response body is a raw tar.gz
// stream; it is not JSON-decoded.
func (c *Client) DownloadBackup(key, id string, w io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/admin/backups/"+url.PathEscape(id)+"/download", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpError(resp)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("stream response: %w", err)
	}
	return nil
}

// RestoreBackup restores the server's database from the backup with the given
// id via POST /api/admin/backups/:id/restore. The {success,message} body is
// discarded; any non-2xx response is returned as an error.
func (c *Client) RestoreBackup(key, id string) error {
	return c.doBearer(http.MethodPost, "/api/admin/backups/"+url.PathEscape(id)+"/restore", key, map[string]any{"confirm": true}, nil)
}

// RestoreBackupUpload uploads a backup archive and restores from it via
// POST /api/admin/backups/restore/upload (multipart field "file").
func (c *Client) RestoreBackupUpload(key, filename string, data []byte) error {
	return c.doBearerMultipart(http.MethodPost, "/api/admin/backups/restore/upload", key, filename, data, nil, nil)
}
