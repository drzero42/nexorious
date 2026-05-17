# Consolidate Symbol Replacement Logic

**Date:** 2026-05-17
**Status:** Approved

## Problem

The ™ and ® trademark symbol replacement is implemented in two separate places:

- `internal/services/matching/normalize.go` — used post-IGDB to normalize titles for fuzzy-match scoring
- `internal/services/igdb/keywords.go` — used pre-IGDB to build clean search query variants

This caused a bug where a fix to one location (replacing symbols with a space rather than removing them) was not applied to the other, leaving manual IGDB search broken for titles like "Velocity®2X".

## Design

### New exported function: `matching.ReplaceSymbols`

Add `ReplaceSymbols(s string) string` to `internal/services/matching/normalize.go`. It applies the existing `reTrademark` regex (`[™®]` → `" "`) and returns the result. It does not lowercase, collapse whitespace, or perform any other transformation — those remain the caller's responsibility.

`NormalizeTitle` calls `ReplaceSymbols` for step 2 instead of the inline `reTrademark.ReplaceAllString` call. Behaviour is identical.

### Changes to `igdb/keywords.go`

Three changes:

1. Remove `kwTrademark` and `kwRegistered` var declarations.
2. Remove their two entries from `keywordRules`.
3. Pre-sanitize the input at the top of `expandQueries`:
   ```go
   original := collapseWhitespace(matching.ReplaceSymbols(strings.TrimSpace(query)))
   ```

The `igdb` package already imports `matching`, so no new import cycle is introduced. The pre-sanitization means the original query sent to IGDB is always clean; the separate symbol-stripped variants that were previously generated are no longer needed.

### Test updates

`igdb_test.go` — `TestExpandQueries` cases for `"FIFA®"` and `"Velocity®2X"` change `wantLen` from 2 to 1. These titles have no other keyword triggers, so after pre-sanitization only one query is generated (the clean original). The `"Batman™: Arkham Knight"` case stays at `wantLen=2` because the colon rule still generates a second variant.

`matching/matching_test.go` — no changes needed.

## Trade-offs Considered

- **`textutil` shared package**: cleaner dependency direction, but introduces a new package for a trivial function. Rejected as premature.
- **Calling `NormalizeTitle` from `expandQueries`**: reuses existing code but applies lowercasing, diacritic folding, and apostrophe removal to the search query, which would produce malformed IGDB queries. Rejected.

## Files Changed

- `internal/services/matching/normalize.go` — add `ReplaceSymbols`, call it from `NormalizeTitle`
- `internal/services/igdb/keywords.go` — remove duplicate rules, pre-sanitize via `matching.ReplaceSymbols`
- `internal/services/igdb/igdb_test.go` — update `wantLen` for trademark-only test cases
