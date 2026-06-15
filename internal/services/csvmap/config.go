// Package csvmap is a config-driven engine that maps any CSV export into the
// canonical importmodel.Game shape. A CSV "source" is a Config value (plus an
// optional header Signature), not a hand-written mapper. This package implements
// the simple subset of the Config feature range; the advanced (Darkadia-era)
// sub-structs are declared but rejected by Parse until issue #1016 implements
// their behaviour. See docs/superpowers/specs/2026-06-14-issue-1014-csvmap-engine-design.md.
package csvmap

// Config declaratively describes how to turn one source's CSV into canonical
// importmodel.Game values.
type Config struct {
	Signature    []string        // headers that must all be present; nil = accept any non-empty CSV
	Columns      ColumnMap       // plain scalar field -> source header
	Status       StatusConfig    //
	Platform     PlatformConfig  //
	Notes        NotesConfig     //
	Grouping     GroupingConfig  //
	Rating       *RatingConfig   // nil = ignore ratings
	Duration     *DurationConfig // nil = ignore hours_played
	TruthyValues []string        // Loved truthy set (matched normalized); nil = {"1","true","yes"}
	TagSeparator string          // tag list separator; "" = ","
	DateLayout   string          // Go time layout for date columns; "" = "2006-01-02"
}

// ColumnMap maps each plain scalar canonical field to its source header name.
type ColumnMap struct {
	Title       string // required
	Rating      string
	CreatedAt   string // game "added"/created date
	HoursPlayed string
	Tags        string
	Loved       string
}

// StatusConfig selects how play_status is derived. At most one of Column / Flags.
type StatusConfig struct {
	Column *StatusColumn // SIMPLE (implemented)
	Flags  *StatusFlags  // ADVANCED #1016 (Parse rejects)
}

// StatusColumn derives play_status from a single column via a value map.
type StatusColumn struct {
	Column   string
	ValueMap map[string]string // normalized source value -> play_status
	Default  string            // empty/unmapped -> this; "" falls back to "not_started"
}

// StatusFlags derives play_status from ordered boolean-flag columns (Darkadia).
type StatusFlags struct {
	Rules   []FlagRule // first matching rule (in order) wins
	Default string
}

// FlagRule is one ordered (column is truthy -> status) rule.
type FlagRule struct {
	Column string
	Truthy []string // values meaning "set", e.g. {"1"}
	Status string
}

// PlatformConfig selects how ownership entries are derived. At most one of Simple / Tables.
type PlatformConfig struct {
	Simple *PlatformSimple // SIMPLE (implemented)
	Tables *PlatformTables // ADVANCED #1016 (Parse rejects)
}

// PlatformSimple derives a single (platform, storefront, acquired-date) entry from columns.
type PlatformSimple struct {
	PlatformColumn     string
	StorefrontColumn   string            // optional
	AcquiredDateColumn string            // optional; attaches to the platform entry
	PlatformMap        map[string]string // optional value (normalized) -> slug; nil/miss = passthrough as-is
	StorefrontMap      map[string]string // optional value (normalized) -> slug
}

// PlatformTables is the Darkadia table+precedence model. Behaviour lands in #1016.
type PlatformTables struct {
	AggregateColumn    string                     // comma-separated owned list ("Platforms")
	PlatformColumn     string                     // per-copy ("Copy platform")
	SourceColumn       string                     // digital source ("Copy source")
	SourceOtherColumn  string                     // free-text when SourceColumn == OtherSentinel
	OtherSentinel      string                     // e.g. "Other"
	MediaColumn        string                     // "Copy media"
	MediaPhysicalValue string                     // value meaning physical, e.g. "Physical"
	PurchaseDateColumn string                     // per-copy acquired date
	Platforms          map[string]PlatformMapping // source string -> {slug, inferred storefront}
	Storefronts        map[string]string          // recognized source (lowercased) -> storefront slug
}

// PlatformMapping is a platform slug plus an optional inferred storefront fallback.
type PlatformMapping struct {
	Slug               string
	InferredStorefront *string
}

// NotesConfig is a verbatim notes column plus optional advanced assembly.
type NotesConfig struct {
	Column   string        // SIMPLE: verbatim notes column
	Assembly *NoteAssembly // ADVANCED #1016 (Parse rejects)
}

// NoteAssembly describes extra column-sourced note inputs (Darkadia). Behaviour lands in #1016.
type NoteAssembly struct {
	ReviewSubjectColumn string
	ReviewColumn        string
	CopyNoteColumn      string
}

// GroupingConfig selects how multiple CSV rows collapse into one game.
type GroupingConfig struct {
	MergeByTitle bool             // false = one-row; true = merge rows sharing a title
	CopyRows     *CopyRowGrouping // ADVANCED #1016 (Parse rejects)
}

// CopyRowGrouping is Darkadia's blank-name continuation grouping. Behaviour lands in #1016.
type CopyRowGrouping struct {
	ContinuationColumn string // blank here => row continues the previous game as a copy
}

// RatingConfig normalizes a source rating scale to whole 1-5 stars.
type RatingConfig struct {
	Scale    int  // 5, 10, or 100
	Truncate bool // false = round to nearest whole star; true = truncate toward zero
}

// DurationConfig describes the hours_played format.
type DurationConfig struct {
	Format string // "decimal" (SIMPLE) | "h:mm" (ADVANCED #1016, rejected)
}
