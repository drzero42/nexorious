// Package clientops holds the operations over the REST API (via cliclient)
// that the nexctl commands and the MCP tools (internal/mcptools) share: ref
// finders, filtered selection, multi-call game edits, and slug validation.
//
// It is the single implementation for both front-ends, so they cannot drift.
// Functions take no cobra command and never prompt; callers own
// disambiguation. The package must import neither cobra nor the MCP SDK, so
// nexctl can use it without linking the SDK.
package clientops
