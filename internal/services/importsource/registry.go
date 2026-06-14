// Package importsource is the registry of mapper-based migration import sources.
// Each entry maps a job-source slug to its file mapper plus the display metadata
// the upload handler and the frontend source picker are driven by. Adding a
// source means adding one entry here.
package importsource

import (
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/darkadia"
	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// Mapper parses a source export file into canonical games. On a wrong-shape
// file it returns an error wrapping importmodel.ErrInvalidSignature.
type Mapper interface {
	Parse(raw []byte) ([]importmodel.Game, error)
}

// mapperFunc adapts a plain parse function to the Mapper interface, so a mapper
// package need not import importsource to be registered.
type mapperFunc func(raw []byte) ([]importmodel.Game, error)

func (f mapperFunc) Parse(raw []byte) ([]importmodel.Game, error) { return f(raw) }

// Source is one registered import source.
type Source struct {
	Slug        string   `json:"slug"`         // JobSource* value, e.g. "darkadia"
	DisplayName string   `json:"display_name"` // "Darkadia"
	Description string   `json:"description"`  // picker blurb
	Features    []string `json:"features"`     // picker bullet list
	Accept      []string `json:"accept"`       // file-input accept hints
	Mapper      Mapper   `json:"-"`
}

// registry is the ordered list of sources (stable order drives the picker).
var registry = []Source{
	{
		Slug:        models.JobSourceDarkadia,
		DisplayName: "Darkadia",
		Description: "Migrate a Darkadia collection export. Games are matched to IGDB; ambiguous matches go to review. Requires IGDB to be configured.",
		Features: []string{
			"Preserves ratings, notes & added date",
			"Matches games to IGDB",
			"Interactive review",
		},
		Accept: []string{".csv", "text/csv"},
		Mapper: mapperFunc(darkadia.Parse),
	},
}

// Lookup returns the source for a slug.
func Lookup(slug string) (Source, bool) {
	for _, s := range registry {
		if s.Slug == slug {
			return s, true
		}
	}
	return Source{}, false
}

// All returns every registered source in stable order.
func All() []Source {
	out := make([]Source, len(registry))
	copy(out, registry)
	return out
}

// IsRegistered reports whether slug is a mapper-based import source.
func IsRegistered(slug string) bool {
	_, ok := Lookup(slug)
	return ok
}
