package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Format renders a (title, body) pair for an event type + payload, plus any
// payload-decode error. Unknown types and malformed payloads fall back to a
// generic, never-empty message; on a decode failure the body omits the
// untrusted fields rather than rendering zero-valued data. Callers should log
// a non-nil decodeErr (it signals schema drift or a corrupt stored payload).
func Format(eventType string, payload json.RawMessage) (title, body string, decodeErr error) {
	meta, ok := Meta(eventType)
	label := eventType
	if ok {
		label = meta.Label
	}

	switch eventType {
	case TypeSyncFailed:
		var p SyncFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync failed"
		if decodeErr != nil {
			body = "Sync failed."
		} else {
			body = fmt.Sprintf("Your %s sync failed: %s", fallback(p.Storefront, "library"), fallback(p.Error, "unknown error"))
		}

	case TypeSyncAuthExpired:
		var p SyncAuthExpiredPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Storefront needs reconnect"
		if decodeErr != nil {
			body = "A storefront connection has expired. Open Sync settings to reconnect."
		} else {
			sf := fallback(p.Storefront, "A storefront")
			body = fmt.Sprintf("Your %s connection has expired. Open Sync settings to reconnect.", sf)
		}

	case TypeSyncCompleted:
		var p SyncCompletedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync completed"
		if decodeErr != nil {
			body = "Your sync completed successfully."
		} else {
			body = fmt.Sprintf("Your %s sync completed successfully.", fallback(p.Storefront, "library"))
		}

	case TypeSyncCompletedWithErrors:
		var p SyncCompletedWithErrorsPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync completed with errors"
		if decodeErr != nil {
			body = fmt.Sprintf("Your %s sync finished with some failed item(s).", fallback(p.Storefront, "library"))
		} else {
			body = fmt.Sprintf("Your %s sync finished with %d failed item(s).", fallback(p.Storefront, "library"), p.Failed)
		}

	case TypeSyncNeedsReview:
		var p SyncNeedsReviewPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync needs review"
		if decodeErr != nil {
			body = fmt.Sprintf("Your %s sync has item(s) needing review.", fallback(p.Storefront, "library"))
		} else {
			body = fmt.Sprintf("Your %s sync has %d item(s) needing review.", fallback(p.Storefront, "library"), p.Count)
		}

	case TypeSyncDiff:
		var p SyncDiffPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Game library changes"
		if decodeErr != nil {
			body = "Your game library changed."
		} else {
			if p.Storefront != "" {
				title = p.Storefront + " library changes"
			}
			body = formatDiff(p.Added, p.Removed)
		}

	case TypeImportCompleted:
		title, body = "Import completed", "Your import finished successfully."
	case TypeImportFailed:
		var p ImportFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Import failed"
		body = failBody(p.Error, "Your import failed", decodeErr)
	case TypeExportCompleted:
		title, body = "Export completed", "Your export is ready."
	case TypeExportFailed:
		var p ExportFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Export failed"
		body = failBody(p.Error, "Your export failed", decodeErr)

	case TypeAdminBackupCompleted:
		title, body = "Backup completed", "A scheduled backup completed successfully."
	case TypeAdminBackupFailed:
		var p BackupFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Backup failed"
		body = failBody(p.Error, "A scheduled backup failed", decodeErr)
	case TypeAdminMaintCompleted:
		var p MaintPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Maintenance completed"
		body = maintBody(p.Action, p.Error, "Maintenance task completed", decodeErr)
	case TypeAdminMaintFailed:
		var p MaintPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Maintenance failed"
		body = maintBody(p.Action, p.Error, "Maintenance task failed", decodeErr)

	case TypeAdminVersionAvailable:
		var p VersionAvailablePayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "New version available"
		if decodeErr != nil {
			body = "A newer version of Nexorious is available."
		} else {
			body = fmt.Sprintf("Nexorious %s is available (you are running %s): %s",
				fallback(p.AvailableVersion, "a newer version"),
				fallback(p.CurrentVersion, "an unknown version"),
				fallback(p.ReleaseURL, "https://github.com/drzero42/nexorious/releases"))
		}

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
	return title, body, decodeErr
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

// failBody renders "<prefix>: <error>", or "<prefix>." when the error is empty
// or the payload failed to decode.
func failBody(errMsg, prefix string, decodeErr error) string {
	if decodeErr != nil || errMsg == "" {
		return prefix + "."
	}
	return prefix + ": " + errMsg
}

// maintBody renders "<prefix> (action) - error.", omitting any part that is
// absent or that could not be decoded.
func maintBody(action, errMsg, prefix string, decodeErr error) string {
	parts := []string{prefix}
	if decodeErr == nil {
		if action != "" {
			parts = append(parts, "("+action+")")
		}
		if errMsg != "" {
			parts = append(parts, "- "+errMsg)
		}
	}
	return strings.Join(parts, " ") + "."
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
