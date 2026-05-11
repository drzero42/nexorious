package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/db/models"
)

// RestoreCallbacks holds the callbacks needed for restore orchestration.
type RestoreCallbacks struct {
	SetMaintenance  func(bool)
	ShutdownPool    func()
	StopScheduler   func()
	CloseDB         func() error
	ReconnectDB     func() (*bun.DB, error)
	RebuildServices func(db *bun.DB) error
	ReinitMigrator   func() error
	SetAppState      func(state string)
	MaxMigration     string
	RebuildBackupJob func(ctx context.Context, cron, retentionMode string, retentionValue int)
}

// BackupHandler handles admin backup endpoints.
type BackupHandler struct {
	svc       *backup.Service
	db        *bun.DB
	callbacks *RestoreCallbacks
}

// NewBackupHandler returns a new BackupHandler.
func NewBackupHandler(svc *backup.Service, db *bun.DB, callbacks *RestoreCallbacks) *BackupHandler {
	return &BackupHandler{svc: svc, db: db, callbacks: callbacks}
}

// parseCronToSchedule converts a stored schedule_cron into the frontend-friendly
// (schedule, scheduleTime, scheduleDay) triple.
//   ""              → ("manual", "00:00", 0)
//   "MM HH * * *"   → ("daily",  "HH:MM", 0)
//   "MM HH * * D"   → ("weekly", "HH:MM", (D+6)%7)  — frontend 0=Mon, cron 0=Sun
func parseCronToSchedule(cron string) (schedule, scheduleTime string, scheduleDay int) {
	scheduleTime = "00:00"
	if cron == "" {
		return "manual", scheduleTime, 0
	}
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return "manual", scheduleTime, 0
	}
	h, _ := strconv.Atoi(parts[1])
	m, _ := strconv.Atoi(parts[0])
	scheduleTime = fmt.Sprintf("%02d:%02d", h, m)
	if parts[4] == "*" {
		return "daily", scheduleTime, 0
	}
	cronDay, _ := strconv.Atoi(parts[4])
	return "weekly", scheduleTime, (cronDay + 6) % 7
}

// buildCronFromSchedule converts the frontend-friendly schedule/time/day into a cron expression.
func buildCronFromSchedule(schedule, scheduleTime string, scheduleDay int) (string, error) {
	if schedule == "manual" {
		return "", nil
	}
	timeParts := strings.SplitN(scheduleTime, ":", 2)
	if len(timeParts) != 2 {
		return "", fmt.Errorf("invalid schedule_time format: %q", scheduleTime)
	}
	hour, err1 := strconv.Atoi(timeParts[0])
	minute, err2 := strconv.Atoi(timeParts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid schedule_time: %q", scheduleTime)
	}
	if schedule == "daily" {
		return fmt.Sprintf("%d %d * * *", minute, hour), nil
	}
	if schedule == "weekly" {
		if scheduleDay < 0 || scheduleDay > 6 {
			return "", fmt.Errorf("schedule_day must be 0–6")
		}
		return fmt.Sprintf("%d %d * * %d", minute, hour, (scheduleDay+1)%7), nil
	}
	return "", fmt.Errorf("unknown schedule: %q", schedule)
}

// HandleGetConfig returns the backup configuration (GET /api/admin/backups/config).
func (h *BackupHandler) HandleGetConfig(c *echo.Context) error {
	var cfg models.BackupConfig
	err := h.db.NewSelect().Model(&cfg).Where("id = 1").Scan(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load backup config"})
	}
	schedule, scheduleTime, scheduleDay := parseCronToSchedule(cfg.ScheduleCron)
	return c.JSON(http.StatusOK, map[string]any{
		"schedule":        schedule,
		"schedule_time":   scheduleTime,
		"schedule_day":    scheduleDay,
		"retention_mode":  cfg.RetentionMode,
		"retention_value": cfg.RetentionValue,
		"updated_at":      cfg.UpdatedAt,
	})
}

// HandleUpdateConfig updates backup config (PUT /api/admin/backups/config).
func (h *BackupHandler) HandleUpdateConfig(c *echo.Context) error {
	var req struct {
		Schedule       string `json:"schedule"`
		ScheduleTime   string `json:"schedule_time"`
		ScheduleDay    int    `json:"schedule_day"`
		RetentionMode  string `json:"retention_mode"`
		RetentionValue int    `json:"retention_value"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Schedule != "manual" && req.Schedule != "daily" && req.Schedule != "weekly" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "schedule must be 'manual', 'daily', or 'weekly'"})
	}
	if req.RetentionMode != "days" && req.RetentionMode != "count" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "retention_mode must be 'days' or 'count'"})
	}
	if req.RetentionValue < 1 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "retention_value must be >= 1"})
	}

	cron, err := buildCronFromSchedule(req.Schedule, req.ScheduleTime, req.ScheduleDay)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	_, err = h.db.NewUpdate().Model((*models.BackupConfig)(nil)).
		TableExpr("backup_config").
		Set("schedule_cron = ?", cron).
		Set("retention_mode = ?", req.RetentionMode).
		Set("retention_value = ?", req.RetentionValue).
		Set("updated_at = now()").
		Where("id = 1").
		Exec(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update backup config"})
	}

	if h.callbacks != nil && h.callbacks.RebuildBackupJob != nil {
		h.callbacks.RebuildBackupJob(c.Request().Context(), cron, req.RetentionMode, req.RetentionValue)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"schedule":        req.Schedule,
		"schedule_time":   req.ScheduleTime,
		"schedule_day":    req.ScheduleDay,
		"retention_mode":  req.RetentionMode,
		"retention_value": req.RetentionValue,
		"updated_at":      time.Now().UTC(),
	})
}

// HandleListBackups lists all backups (GET /api/admin/backups).
func (h *BackupHandler) HandleListBackups(c *echo.Context) error {
	backups, err := h.svc.ListBackups()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list backups"})
	}
	if backups == nil {
		backups = []backup.BackupInfo{}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"backups": backups,
		"total":   len(backups),
	})
}

// HandleCreateBackup creates a manual backup (POST /api/admin/backups).
func (h *BackupHandler) HandleCreateBackup(c *echo.Context) error {
	if !backup.PgDumpAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "pg_dump is not available on this system. Install PostgreSQL client tools to enable backups.",
		})
	}

	id, err := h.svc.CreateBackup("manual")
	if err != nil {
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is already in progress"})
		}
		slog.Error("backup creation failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("backup failed: %v", err)})
	}

	// Apply retention from config
	var cfg models.BackupConfig
	if err := h.db.NewSelect().Model(&cfg).Where("id = 1").Scan(c.Request().Context()); err == nil {
		if retErr := h.svc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue); retErr != nil {
			slog.Warn("retention cleanup failed", "err", retErr)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"backup_id": id,
		"message":   "Backup created successfully",
	})
}

// HandleDeleteBackup deletes a backup (DELETE /api/admin/backups/:id).
func (h *BackupHandler) HandleDeleteBackup(c *echo.Context) error {
	id := c.Param("id")
	err := h.svc.DeleteBackup(id)
	if err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "backup not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to delete: %v", err)})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleDownloadBackup downloads a backup archive (GET /api/admin/backups/:id/download).
func (h *BackupHandler) HandleDownloadBackup(c *echo.Context) error {
	id := c.Param("id")
	path := h.svc.GetBackupPath(id)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "backup file not found"})
	}
	return c.Attachment(path, filepath.Base(path))
}

// HandleRestore restores from an existing backup (POST /api/admin/backups/:id/restore).
func (h *BackupHandler) HandleRestore(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.Bind(&req); err != nil || !req.Confirm {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "confirm must be true"})
	}

	id := c.Param("id")
	opts := h.makeRestoreOpts(false)
	if err := h.svc.RestoreBackup(id, opts); err != nil {
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is already in progress"})
		}
		slog.Error("restore failed", "backup_id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("restore failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Restore completed from: %s. All sessions have been cleared — please log in again.", id),
	})
}

// HandleRestoreUpload restores from an uploaded file (POST /api/admin/backups/restore/upload).
func (h *BackupHandler) HandleRestoreUpload(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file is required"})
	}
	if file.Size > 2<<30 { // 2 GB
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large (max 2GB)"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp("", "nexorious-restore-*.tar.gz")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	const maxUpload = 2 << 30 // 2 GB
	n, err := tmp.ReadFrom(io.LimitReader(src, maxUpload+1))
	if err != nil {
		_ = tmp.Close()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save uploaded file"})
	}
	_ = tmp.Close()
	if n > maxUpload {
		_ = os.Remove(tmpPath)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large (max 2GB)"})
	}

	opts := h.makeRestoreOpts(false)
	id, err := h.svc.RestoreFromUpload(tmpPath, opts)
	if err != nil {
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is already in progress"})
		}
		slog.Error("restore from upload failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("restore failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Restore completed from: %s. All sessions have been cleared — please log in again.", id),
	})
}

// HandleSetupRestore handles restore during initial setup (POST /api/auth/setup/restore).
func (h *BackupHandler) HandleSetupRestore(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	// Check that no users exist (setup mode only)
	count, err := h.db.NewSelect().TableExpr("users").Count(c.Request().Context())
	if err == nil && count > 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "restore during setup is only available when no users exist"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file is required"})
	}
	if file.Size > 2<<30 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large (max 2GB)"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp("", "nexorious-setup-restore-*.tar.gz")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	const maxUpload = 2 << 30 // 2 GB
	n, err := tmp.ReadFrom(io.LimitReader(src, maxUpload+1))
	if err != nil {
		_ = tmp.Close()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save uploaded file"})
	}
	_ = tmp.Close()
	if n > maxUpload {
		_ = os.Remove(tmpPath)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large (max 2GB)"})
	}

	opts := h.makeRestoreOpts(true)
	if _, err := h.svc.RestoreFromUpload(tmpPath, opts); err != nil {
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is already in progress"})
		}
		slog.Error("setup restore failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("restore failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Backup restored successfully. Please log in with your restored credentials.",
	})
}

// makeRestoreOpts constructs RestoreOpts from the handler's callbacks.
func (h *BackupHandler) makeRestoreOpts(skipPreRestore bool) backup.RestoreOpts {
	if h.callbacks == nil {
		return backup.RestoreOpts{SkipPreRestore: skipPreRestore}
	}
	return backup.RestoreOpts{
		SkipPreRestore:  skipPreRestore,
		SetMaintenance:  h.callbacks.SetMaintenance,
		ShutdownPool:    h.callbacks.ShutdownPool,
		StopScheduler:   h.callbacks.StopScheduler,
		CloseDB:         h.callbacks.CloseDB,
		ReconnectDB:     h.callbacks.ReconnectDB,
		RebuildServices: h.callbacks.RebuildServices,
		ReinitMigrator:  h.callbacks.ReinitMigrator,
		SetAppState:     h.callbacks.SetAppState,
		MaxMigration:    h.callbacks.MaxMigration,
	}
}
