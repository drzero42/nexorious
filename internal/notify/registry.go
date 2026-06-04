// Package notify implements user/admin notification emission and delivery.
package notify

// Scope constants for events.
const (
	ScopeUser  = "user"
	ScopeAdmin = "admin"
)

// Event type string constants. These are the single source of truth.
const (
	TypeSyncCompleted           = "sync.completed"
	TypeSyncCompletedWithErrors = "sync.completed_with_errors"
	TypeSyncFailed              = "sync.failed"
	TypeSyncAuthExpired         = "sync.auth_expired"
	TypeSyncNeedsReview         = "sync.needs_review"
	TypeSyncDiff                = "sync.diff"
	TypeImportCompleted         = "import.completed"
	TypeImportFailed            = "import.failed"
	TypeExportCompleted         = "export.completed"
	TypeExportFailed            = "export.failed"
	TypeAdminBackupCompleted    = "admin.backup.completed"
	TypeAdminBackupFailed       = "admin.backup.failed"
	TypeAdminMaintCompleted     = "admin.maintenance.completed"
	TypeAdminMaintFailed        = "admin.maintenance.failed"
)

// EventTypeMeta describes one event type for the registry and the settings UI.
type EventTypeMeta struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Category  string `json:"category"`
	Label     string `json:"label"`
	DefaultOn bool   `json:"default_on"`
}

// registry preserves declaration order (used for stable UI ordering).
var registry = []EventTypeMeta{
	{TypeSyncCompleted, ScopeUser, "Sync", "Sync completed", false},
	{TypeSyncCompletedWithErrors, ScopeUser, "Sync", "Sync completed with errors", true},
	{TypeSyncFailed, ScopeUser, "Sync", "Sync failed", true},
	{TypeSyncAuthExpired, ScopeUser, "Sync", "Storefront needs reconnect", true},
	{TypeSyncNeedsReview, ScopeUser, "Sync", "Sync has items needing review", false},
	{TypeSyncDiff, ScopeUser, "Sync", "Game changes digest per sync", false},
	{TypeImportCompleted, ScopeUser, "Import / Export", "Import completed", false},
	{TypeImportFailed, ScopeUser, "Import / Export", "Import failed", true},
	{TypeExportCompleted, ScopeUser, "Import / Export", "Export completed", false},
	{TypeExportFailed, ScopeUser, "Import / Export", "Export failed", true},
	{TypeAdminBackupCompleted, ScopeAdmin, "Backups", "Scheduled backup completed", false},
	{TypeAdminBackupFailed, ScopeAdmin, "Backups", "Scheduled backup failed", true},
	{TypeAdminMaintCompleted, ScopeAdmin, "Maintenance", "Maintenance tasks completed", false},
	{TypeAdminMaintFailed, ScopeAdmin, "Maintenance", "Maintenance tasks failed", true},
}

var metaByType = func() map[string]EventTypeMeta {
	m := make(map[string]EventTypeMeta, len(registry))
	for _, e := range registry {
		m[e.Type] = e
	}
	return m
}()

// Registry returns all event types in declaration order.
func Registry() []EventTypeMeta {
	out := make([]EventTypeMeta, len(registry))
	copy(out, registry)
	return out
}

// Meta returns the metadata for a type and whether it is known.
func Meta(eventType string) (EventTypeMeta, bool) {
	m, ok := metaByType[eventType]
	return m, ok
}

// IsKnownType reports whether eventType is registered.
func IsKnownType(eventType string) bool {
	_, ok := metaByType[eventType]
	return ok
}

// IsAdminType reports whether eventType is admin-scoped. Unknown types -> false.
func IsAdminType(eventType string) bool {
	m, ok := metaByType[eventType]
	return ok && m.Scope == ScopeAdmin
}

// DefaultSubscriptions returns the event types that are on by default
// ("failures on, successes off"). Seeded on user creation and on Reset.
func DefaultSubscriptions() []string {
	var out []string
	for _, e := range registry {
		if e.DefaultOn {
			out = append(out, e.Type)
		}
	}
	return out
}
