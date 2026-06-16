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
		case "decimal", "h:mm":
		default:
			return fmt.Errorf("csvmap: Duration.Format must be %q or %q, got %q", "decimal", "h:mm", cfg.Duration.Format)
		}
	}
	if cfg.Status.Column != nil {
		if err := validateColumnFormat("Status.Column", cfg.Status.Column.Format); err != nil {
			return err
		}
	}
	if cfg.Platform.Simple != nil {
		if err := validateColumnFormat("Platform.Simple", cfg.Platform.Simple.PlatformFormat); err != nil {
			return err
		}
		if cfg.Platform.Simple.PlatformSeparator != "" && cfg.Platform.Simple.PlatformFormat == FormatJSONKeys {
			return errors.New("csvmap: Platform.Simple PlatformSeparator and json-keys format are mutually exclusive")
		}
	}
	if cfg.PlayLog != nil {
		if cfg.Duration != nil {
			return errors.New("csvmap: Duration and PlayLog are mutually exclusive")
		}
		if strings.TrimSpace(cfg.PlayLog.Column) == "" ||
			strings.TrimSpace(cfg.PlayLog.SecondsField) == "" ||
			strings.TrimSpace(cfg.PlayLog.CompletionField) == "" {
			return errors.New("csvmap: PlayLog requires Column, SecondsField, and CompletionField")
		}
	}
	if cfg.Grouping.CopyRows != nil {
		if strings.TrimSpace(cfg.Grouping.CopyRows.ContinuationColumn) == "" {
			return errors.New("csvmap: Grouping.CopyRows requires ContinuationColumn")
		}
		if cfg.Grouping.MergeByTitle {
			return errors.New("csvmap: Grouping.CopyRows and MergeByTitle are mutually exclusive")
		}
	}
	return nil
}

func validateColumnFormat(name string, f ColumnFormat) error {
	switch f {
	case FormatScalar, FormatJSONKeys:
		return nil
	}
	return fmt.Errorf("csvmap: %s format %q must be %q or %q", name, f, FormatScalar, FormatJSONKeys)
}
