package logging

import "log/slog"

// Category is a fixed, low-cardinality taxonomy for error logs, set at error
// boundaries to make failures aggregatable and greppable.
type Category string

const (
	CategoryExternalAPI Category = "external_api"
	CategoryDB          Category = "db"
	CategoryValidation  Category = "validation"
	CategoryAuth        Category = "auth"
	CategoryConfig      Category = "config"
	CategoryPanic       Category = "panic"
)

// Cat returns the slog attribute for an error category.
func Cat(c Category) slog.Attr {
	return slog.String(KeyCategory, string(c))
}
