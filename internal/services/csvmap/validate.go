package csvmap

import (
	"errors"
	"fmt"
	"strings"
)

// validate checks the config before any data is read. Advanced (Darkadia-era)
// slots are rejected with a descriptive error that is NOT ErrInvalidSignature.
func validate(cfg Config) error {
	if strings.TrimSpace(cfg.Columns.Title) == "" {
		return errors.New("csvmap: Columns.Title is required")
	}
	if cfg.Status.Column != nil && cfg.Status.Flags != nil {
		return errors.New("csvmap: Status.Column and Status.Flags are mutually exclusive")
	}
	if cfg.Platform.Simple != nil && cfg.Platform.Tables != nil {
		return errors.New("csvmap: Platform.Simple and Platform.Tables are mutually exclusive")
	}
	if cfg.Rating != nil {
		switch cfg.Rating.Scale {
		case 5, 10, 100:
		default:
			return fmt.Errorf("csvmap: Rating.Scale must be 5, 10, or 100, got %d", cfg.Rating.Scale)
		}
	}
	if cfg.Duration != nil {
		switch normKey(cfg.Duration.Format) {
		case "decimal":
		case "h:mm":
			return notImplemented(`Duration.Format "h:mm"`)
		default:
			return fmt.Errorf("csvmap: Duration.Format must be %q or %q, got %q", "decimal", "h:mm", cfg.Duration.Format)
		}
	}
	if cfg.Status.Flags != nil {
		return notImplemented("Status.Flags")
	}
	if cfg.Platform.Tables != nil {
		return notImplemented("Platform.Tables")
	}
	if cfg.Notes.Assembly != nil {
		return notImplemented("Notes.Assembly")
	}
	if cfg.Grouping.CopyRows != nil {
		return notImplemented("Grouping.CopyRows")
	}
	return nil
}

// notImplemented is returned for an advanced Config slot whose behaviour lands in #1016.
func notImplemented(feature string) error {
	return fmt.Errorf("csvmap: %s is not implemented yet (advanced feature, see #1016)", feature)
}
