// Package docs embeds the top-level Markdown guides and reference documents so
// the running binary can serve them in-app. Only direct children of docs/ are
// embedded by the *.md glob; the docs/superpowers/ subtree (specs and plans)
// is intentionally excluded.
package docs

import "embed"

// FS holds the top-level *.md files: the user/admin guides plus the reference
// docs (sync, maintenance, import-export-format, darkadia-import).
//
//go:embed *.md
var FS embed.FS
