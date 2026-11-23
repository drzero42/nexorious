package api

import (
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

	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// BackupHandler handles admin backup endpoints.
type BackupHandler struct {
	svc *backup.Service
	db  *bun.DB
}

// NewBackupHandler returns a new BackupHandler.
func NewBackupHandler(svc *backup.Service, db *bun.DB) *BackupHandler {
	return &BackupHandler{svc: svc, db: db}
}

// parseCronToSchedule converts a stored schedule_cron into the frontend-friendly
// (schedule, scheduleTime, scheduleDay) triple.
//
//	""              → ("manual", "00:00", 0)
//	"MM HH * * *"   → ("daily",  "HH:MM", 0)
//	"MM HH * * D"   → ("weekly", "HH:MM", (D+6)%7)  — frontend 0=Mon, cron 0=Sun
func parseCronToSchedule(cron string) (schedule, scheduleTime string, scheduleDay int) {
	scheduleTime = "00:00"
	if cron == "" {
		return "manual", scheduleTime, 0
	}
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return "manual", scheduleTime, 0
	}
	h, _ := strconv.Atoi(parts[1]) //nolint:errcheck // malformed cron field defaults to 0
	m, _ := strconv.Atoi(parts[0]) //nolint:errcheck // malformed cron field defaults to 0
	scheduleTime = fmt.Sprintf("%02d:%02d", h, m)
	if parts[4] == "*" {
		return "daily", scheduleTime, 0
	}
	cronDay, _ := strconv.Atoi(parts[4]) //nolint:errcheck // malformed cron field defaults to 0
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

	// Under the service lock, so the schedule does not change during a restore.
	err = h.svc.Exclusive(func() error {
		_, err := h.db.NewUpdate().
			TableExpr("backup_config").
			Set("schedule_cron = ?", cron).
			Set("retention_mode = ?", req.RetentionMode).
			Set("retention_value = ?", req.RetentionValue).
			Set("updated_at = now()").
			Where("id = 1").
			Exec(c.Request().Context())
		return err
	})
	if errors.Is(err, backup.ErrOperationInProgress) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is in progress; try again when it has finished"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update backup config"})
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
		slog.ErrorContext(c.Request().Context(), "backup creation failed", logging.KeyErr, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("backup failed: %v", err)})
	}

	// Apply retention from config
	var cfg models.BackupConfig
	if err := h.db.NewSelect().Model(&cfg).Where("id = 1").Scan(c.Request().Context()); err == nil {
		if retErr := h.svc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue); retErr != nil {
			slog.WarnContext(c.Request().Context(), "retention cleanup failed", logging.KeyErr, retErr)
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
		if errors.Is(err, backup.ErrInvalidBackupID) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid backup id"})
		}
		if errors.Is(err, backup.ErrNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "backup not found"})
		}
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is in progress; try again when it has finished"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to delete: %v", err)})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleDownloadBackup downloads a backup archive (GET /api/admin/backups/:id/download).
func (h *BackupHandler) HandleDownloadBackup(c *echo.Context) error {
	id := c.Param("id")
	path, err := h.svc.GetBackupPath(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid backup id"})
	}
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
	res, err := h.svc.RestoreBackup(id, backup.RestoreOpts{})
	return writeRestoreResponse(c, res, err, false)
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

	res, err := h.svc.RestoreFromUpload(tmpPath, backup.RestoreOpts{})
	return writeRestoreResponse(c, res, err, false)
}

// requireNoUsers enforces the setup-mode gate: the setup-zone restore handlers
// are unauthenticated, so they may only run while the users table is empty.
// It reports whether the handler may continue. When it returns false the
// refusal has already been written and the caller must return err as-is —
// err is nil when the refusal was written successfully, so callers must branch
// on the bool, never on err. A failed count refuses too (fail closed).
func (h *BackupHandler) requireNoUsers(c *echo.Context) (ok bool, err error) {
	ctx := c.Request().Context()
	count, err := h.db.NewSelect().TableExpr("users").Count(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "setup restore gate: count users", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return false, c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "could not verify that setup is still pending; try again"})
	}
	if count > 0 {
		return false, c.JSON(http.StatusForbidden, map[string]string{"error": "restore during setup is only available when no users exist"})
	}
	return true, nil
}

// HandleSetupRestore handles restore during initial setup (POST /api/auth/setup/restore).
func (h *BackupHandler) HandleSetupRestore(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	if ok, err := h.requireNoUsers(c); !ok {
		return err
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

	res, err := h.svc.RestoreFromUpload(tmpPath, backup.RestoreOpts{SetupZone: true})
	return writeRestoreResponse(c, res, err, true)
}

// HandleSetupListBackups lists candidate backup archives in the configured
// backup directory during initial setup (GET /api/auth/setup/backups).
func (h *BackupHandler) HandleSetupListBackups(c *echo.Context) error {
	if ok, err := h.requireNoUsers(c); !ok {
		return err
	}

	maxMigration, minMigration := h.svc.MigrationWindow()

	infos, err := h.svc.ListAvailableArchives(c.Request().Context(), maxMigration, minMigration)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "setup list backups failed", logging.KeyErr, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list backups"})
	}

	type manifestDTO struct {
		CreatedAt        time.Time `json:"created_at"`
		AppVersion       string    `json:"app_version"`
		MigrationVersion string    `json:"migration_version"`
		BackupType       string    `json:"backup_type"`
		Stats            struct {
			Users int `json:"users"`
			Games int `json:"games"`
			Tags  int `json:"tags"`
		} `json:"stats"`
	}
	type entryDTO struct {
		Filename   string       `json:"filename"`
		SizeBytes  int64        `json:"size_bytes"`
		ModTime    time.Time    `json:"mtime"`
		Restorable bool         `json:"restorable"`
		Reason     string       `json:"reason,omitempty"`
		Manifest   *manifestDTO `json:"manifest,omitempty"`
	}

	out := make([]entryDTO, 0, len(infos))
	for _, info := range infos {
		e := entryDTO{
			Filename:   info.Filename,
			SizeBytes:  info.SizeBytes,
			ModTime:    info.ModTime,
			Restorable: info.Restorable,
			Reason:     info.Reason,
		}
		if info.Manifest != nil {
			m := &manifestDTO{
				CreatedAt:        info.Manifest.CreatedAt,
				AppVersion:       info.Manifest.AppVersion,
				MigrationVersion: info.Manifest.MigrationVersion,
				BackupType:       info.Manifest.BackupType,
			}
			m.Stats.Users = info.Manifest.StatsUsers
			m.Stats.Games = info.Manifest.StatsGames
			m.Stats.Tags = info.Manifest.StatsTags
			e.Manifest = m
		}
		out = append(out, e)
	}

	return c.JSON(http.StatusOK, map[string]any{"backups": out})
}

// HandleSetupRestoreFromDisk restores from a backup that already exists in the
// configured backup directory (POST /api/auth/setup/restore/disk). Body:
//
//	{ "filename": "nexorious-backup-20260520-093015.tar.gz" }
//
// Only top-level regular files inside the configured BACKUP_PATH are
// accepted. Symlinks are rejected. The on-disk file is preserved unchanged —
// it is not renamed, moved, or deleted by the restore operation.
//
// Trust model: BACKUP_PATH must be writable only by the nexorious process.
// Hardlinks at top level are not detected by os.Lstat and would be opened
// like a regular file; the manifest validation that follows bounds disclosure
// to "is this a valid Nexorious archive", not arbitrary read.
func (h *BackupHandler) HandleSetupRestoreFromDisk(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	if ok, err := h.requireNoUsers(c); !ok {
		return err
	}

	var body struct {
		Filename string `json:"filename"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
	}

	filename := body.Filename
	if filename == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename is required"})
	}

	// Layered path-traversal defense. None of these checks alone is enough.
	if strings.ContainsAny(filename, `/\`) || strings.Contains(filename, "..") || strings.ContainsRune(filename, 0) || strings.TrimSpace(filename) != filename || filepath.Base(filename) != filename {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	backupDir := h.svc.BackupPath()
	fullPath := filepath.Join(backupDir, filename)
	if filepath.Dir(fullPath) != filepath.Clean(backupDir) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	fi, err := os.Lstat(fullPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "backup not found"})
		}
		slog.ErrorContext(c.Request().Context(), "setup restore-from-disk lstat failed", logging.KeyErr, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to inspect backup file"})
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	res, err := h.svc.RestoreFromArchive(fullPath, backup.RestoreOpts{SetupZone: true})
	return writeRestoreResponse(c, res, err, true)
}

// writeRestoreResponse answers a restore request. Every answer carries
// restarting and the id of this process: once restarting is true the server
// restarts after the response, on success and on failure, and a client waits
// until /health reports a different instance id (#1341). setupZone keeps
// internal error detail out of the answers of the unauthenticated setup zone.
func writeRestoreResponse(c *echo.Context, res backup.RestoreResult, err error, setupZone bool) error {
	ctx := c.Request().Context()
	body := map[string]any{
		"restarting":  res.Restarting,
		"instance_id": instanceID,
	}
	if res.BackupID != "" {
		body["backup_id"] = res.BackupID
	}
	if res.PreRestoreBackupID != "" {
		body["pre_restore_backup_id"] = res.PreRestoreBackupID
	}
	if res.MigrationError != nil {
		slog.ErrorContext(ctx, "restore: migrating the restored database failed", logging.KeyErr, res.MigrationError, logging.Cat(logging.CategoryDB))
		body["migration_error"] = "the restored database could not be migrated; the server offers the migration again when it is back"
	}

	if err == nil {
		body["success"] = true
		if setupZone {
			body["message"] = "Backup restored. The server is restarting; log in with your restored credentials once it is back."
		} else {
			body["message"] = fmt.Sprintf("Restore completed from %s. The server is restarting; log in again once it is back.", res.BackupID)
		}
		return c.JSON(http.StatusOK, body)
	}

	status, msg := http.StatusInternalServerError, "restore failed"
	var incompat *backup.IncompatibleBackupError
	switch {
	case errors.Is(err, backup.ErrOperationInProgress):
		status, msg = http.StatusConflict, "A backup or restore operation is already in progress"
	case errors.Is(err, backup.ErrInvalidBackupID):
		status, msg = http.StatusBadRequest, "invalid backup id"
	case errors.Is(err, backup.ErrNotFound):
		status, msg = http.StatusNotFound, "backup not found"
	case errors.As(err, &incompat):
		status, msg = http.StatusBadRequest, incompat.Error()
	case errors.Is(err, backup.ErrSetupNotPending):
		status, msg = http.StatusForbidden, backup.ErrSetupNotPending.Error()
	case errors.Is(err, backup.ErrSetupCheckFailed):
		status, msg = http.StatusServiceUnavailable, "could not verify that setup is still pending; try again"
	case errors.Is(err, backup.ErrPgDumpUnavailable):
		status, msg = http.StatusServiceUnavailable, backup.ErrPgDumpUnavailable.Error()
	case errors.Is(err, backup.ErrBackgroundStillRunning):
		status, msg = http.StatusServiceUnavailable, "background work did not stop in time, so nothing was restored; the server is restarting, try again once it is back"
	default:
		slog.ErrorContext(ctx, "restore failed", "backup_id", res.BackupID, logging.KeyErr, err)
		if !setupZone {
			msg = fmt.Sprintf("restore failed: %v", err)
		}
	}
	if res.RolledBack {
		body["rolled_back"] = true
	}
	body["error"] = msg
	return c.JSON(status, body)
}
