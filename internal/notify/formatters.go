package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DiffGame is one entry in a sync.diff payload.
type DiffGame struct {
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}

// Format renders a (title, body) pair for an event type + payload. Unknown
// types and malformed payloads fall back to a generic, never-empty message.
func Format(eventType string, payload json.RawMessage) (title, body string) {
	meta, ok := Meta(eventType)
	label := eventType
	if ok {
		label = meta.Label
	}

	switch eventType {
	case TypeSyncFailed:
		var p struct {
			Storefront string `json:"storefront"`
			Error      string `json:"error"`
		}
		if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
			_ = err
		}
		title = "Sync failed"
		body = fmt.Sprintf("Your %s sync failed: %s", fallback(p.Storefront, "library"), fallback(p.Error, "unknown error"))

	case TypeSyncCompleted:
		var p struct {
			Storefront string `json:"storefront"`
		}
		if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
			_ = err
		}
		title = "Sync completed"
		body = fmt.Sprintf("Your %s sync completed successfully.", fallback(p.Storefront, "library"))

	case TypeSyncCompletedWithErrors:
		var p struct {
			Storefront string `json:"storefront"`
			Failed     int    `json:"failed"`
		}
		if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
			_ = err
		}
		title = "Sync completed with errors"
		body = fmt.Sprintf("Your %s sync finished with %d failed item(s).", fallback(p.Storefront, "library"), p.Failed)

	case TypeSyncNeedsReview:
		var p struct {
			Storefront string `json:"storefront"`
			Count      int    `json:"count"`
		}
		if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
			_ = err
		}
		title = "Sync needs review"
		body = fmt.Sprintf("Your %s sync has %d item(s) needing review.", fallback(p.Storefront, "library"), p.Count)

	case TypeSyncDiff:
		var p struct {
			Added   []DiffGame `json:"added"`
			Removed []DiffGame `json:"removed"`
		}
		if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
			_ = err
		}
		title = "Game library changes"
		body = formatDiff(p.Added, p.Removed)

	case TypeImportCompleted:
		title, body = "Import completed", "Your import finished successfully."
	case TypeImportFailed:
		title, body = "Import failed", failBody(payload, "Your import failed")
	case TypeExportCompleted:
		title, body = "Export completed", "Your export is ready."
	case TypeExportFailed:
		title, body = "Export failed", failBody(payload, "Your export failed")

	case TypeAdminBackupCompleted:
		title, body = "Backup completed", "A scheduled backup completed successfully."
	case TypeAdminBackupFailed:
		title, body = "Backup failed", failBody(payload, "A scheduled backup failed")
	case TypeAdminMaintCompleted:
		title, body = "Maintenance completed", maintBody(payload, "Maintenance task completed")
	case TypeAdminMaintFailed:
		title, body = "Maintenance failed", maintBody(payload, "Maintenance task failed")

	default:
		title = label
		body = "An event occurred: " + eventType
	}

	if title == "" {
		title = label
	}
	if body == "" {
		body = "An event occurred: " + eventType
	}
	return title, body
}

func formatDiff(added, removed []DiffGame) string {
	var b strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&b, "Added (%d):\n", len(added))
		for _, g := range added {
			suffix := ""
			if len(g.Platforms) > 0 {
				suffix = " [" + strings.Join(g.Platforms, ", ") + "]"
			}
			fmt.Fprintf(&b, "  + %s%s\n", g.Title, suffix)
		}
	}
	if len(removed) > 0 {
		fmt.Fprintf(&b, "Removed (%d):\n", len(removed))
		for _, g := range removed {
			suffix := ""
			if len(g.Platforms) > 0 {
				suffix = " [" + strings.Join(g.Platforms, ", ") + "]"
			}
			fmt.Fprintf(&b, "  - %s%s\n", g.Title, suffix)
		}
	}
	if b.Len() == 0 {
		return "No changes."
	}
	return strings.TrimRight(b.String(), "\n")
}

func failBody(payload json.RawMessage, prefix string) string {
	var p struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
		_ = err
	}
	if p.Error == "" {
		return prefix + "."
	}
	return prefix + ": " + p.Error
}

func maintBody(payload json.RawMessage, prefix string) string {
	var p struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(payload, &p); err != nil { //nolint:errcheck // best-effort decode; falls back to defaults on error
		_ = err
	}
	parts := []string{prefix}
	if p.Action != "" {
		parts = append(parts, "("+p.Action+")")
	}
	if p.Error != "" {
		parts = append(parts, "- "+p.Error)
	}
	return strings.Join(parts, " ") + "."
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
