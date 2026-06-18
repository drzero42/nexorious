package cliclient

import (
	"net/http"
	"net/url"
)

const adminUsersPath = "/api/auth/admin/users"

// AdminUser is the user record returned by every admin user endpoint.
type AdminUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IsActive  bool   `json:"is_active"`
	IsAdmin   bool   `json:"is_admin"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// DeletionImpact summarises the rows that would be cascade-deleted alongside a user.
type DeletionImpact struct {
	UserID           string `json:"user_id"`
	Username         string `json:"username"`
	TotalGames       int    `json:"total_games"`
	TotalTags        int    `json:"total_tags"`
	TotalImportJobs  int    `json:"total_import_jobs"`
	TotalExportJobs  int    `json:"total_export_jobs"`
	TotalSyncJobs    int    `json:"total_sync_jobs"`
	TotalSyncConfigs int    `json:"total_sync_configs"`
	TotalSessions    int    `json:"total_sessions"`
	Warning          string `json:"warning"`
}

// CreateUser creates a new user via POST /api/auth/admin/users.
func (c *Client) CreateUser(key, username, password string, isAdmin bool) (*AdminUser, error) {
	body := map[string]any{
		"username": username,
		"password": password,
		"is_admin": isAdmin,
	}
	var out AdminUser
	if err := c.doBearer(http.MethodPost, adminUsersPath, key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListUsers returns all users (newest first) via GET /api/auth/admin/users.
func (c *Client) ListUsers(key string) ([]AdminUser, error) {
	var out []AdminUser
	if err := c.doBearer(http.MethodGet, adminUsersPath, key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetUser returns a single user by ID via GET /api/auth/admin/users/:id.
func (c *Client) GetUser(key, id string) (*AdminUser, error) {
	var out AdminUser
	if err := c.doBearer(http.MethodGet, adminUsersPath+"/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUser applies a partial update to a user via PUT /api/auth/admin/users/:id.
// Only keys present in fields are sent; valid keys are username, is_active, is_admin.
func (c *Client) UpdateUser(key, id string, fields map[string]any) (*AdminUser, error) {
	var out AdminUser
	if err := c.doBearer(http.MethodPut, adminUsersPath+"/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResetUserPassword sets a new password for the user via PUT /api/auth/admin/users/:id/password.
func (c *Client) ResetUserPassword(key, id, newPassword string) error {
	body := map[string]any{"new_password": newPassword}
	return c.doBearer(http.MethodPut, adminUsersPath+"/"+url.PathEscape(id)+"/password", key, body, nil)
}

// GetDeletionImpact returns the deletion impact summary for a user
// via GET /api/auth/admin/users/:id/deletion-impact.
func (c *Client) GetDeletionImpact(key, id string) (*DeletionImpact, error) {
	var out DeletionImpact
	if err := c.doBearer(http.MethodGet, adminUsersPath+"/"+url.PathEscape(id)+"/deletion-impact", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteUser removes a user via DELETE /api/auth/admin/users/:id.
func (c *Client) DeleteUser(key, id string) error {
	return c.doBearer(http.MethodDelete, adminUsersPath+"/"+url.PathEscape(id), key, nil, nil)
}

// adminResetResponse is the response body for POST /api/auth/admin/reset.
type adminResetResponse struct {
	Deleted int `json:"deleted"`
}

// AdminReset wipes all user data (except the admin account) via POST /api/auth/admin/reset.
// It returns the number of user_games rows deleted.
func (c *Client) AdminReset(key string) (int, error) {
	var out adminResetResponse
	if err := c.doBearer(http.MethodPost, "/api/auth/admin/reset", key, map[string]any{}, &out); err != nil {
		return 0, err
	}
	return out.Deleted, nil
}
