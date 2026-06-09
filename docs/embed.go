// Package docs embeds the in-app Markdown guides so the running binary can
// serve them at /api/docs/:slug. Only the user and admin guides are embedded;
// the reference docs (sync, maintenance, import-export-format, darkadia-import)
// and the docs/superpowers/ subtree (specs and plans) live in the repo for
// GitHub viewing but are intentionally not embedded or served in-app.
package docs

import "embed"

// FS holds the two in-app guides. They cross-link only to each other; any other
// relative *.md link inside a guide falls back to a GitHub source URL (see
// ui/frontend/src/lib/doc-links.ts), so no further docs need embedding.
//
//go:embed user-guide.md admin-guide.md
var FS embed.FS
