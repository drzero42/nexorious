# Tag-assignment endpoint for user-games (issue #1054)

## Problem

There is no write path to attach or detach a tag from a user-game. Tag
*definitions* can be created, renamed, deleted, and filtered on, but the only
code that ever populates the `user_game_tags` join table is the import pipeline
(`internal/worker/tasks/import_pipeline.go`, `import_item.go`). Consequently a
user cannot manually tag a game in their collection through the API.

The frontend is not missing a tag editor — the game edit form
(`ui/frontend/src/components/games/game-edit-form.tsx`) already renders one and
calls three hooks that target backend routes which **were never built**:

- `POST /api/tags/assign/:userGameId` (assign by tag IDs)
- `DELETE /api/tags/remove/:userGameId` (remove by tag IDs)
- `POST /api/tags/create-or-get` (inline create)

These calls fail at runtime (observed as a 500). The client also ships two
functions — `bulkAssignTags` / `bulkRemoveTags` (`POST /tags/bulk-assign`,
`DELETE /tags/bulk-remove`) — that no UI component uses at all.

This gap also blocks the MCP server work (#518), whose revised design folds tag
assignment into a consolidated `update_game` tool that assigns by name and
auto-creates definitions as needed.

## Goals

- Add a backend write path to set the tags on a user-game.
- Rewire the existing edit form to that endpoint so manual tagging works.
- Remove the dead frontend client code left over from the never-built API.
- Keep the import-side tag write logic and the new endpoint in agreement by
  sharing one reconcile helper.

## Non-goals

- Bulk tagging across multiple games in one call (no UI consumer; the dead
  `bulk-*` client functions are removed, not implemented). A follow-up issue can
  add this if a multi-select bulk-tag UI is built.
- Choosing a tag color at assignment time. Auto-created tags get the default
  color; color is managed through existing tag CRUD.

## Design

### API: replace-set endpoint

A new route on the user-games group:

```
PUT /api/user-games/:id/tags
Body: { "tags": ["RPG", "Backlog", "Co-op"] }
```

The body is the **complete desired set** of tag names. The handler runs in a
single transaction:

1. Verify the user-game exists and belongs to the authenticated caller; 404
   otherwise. 401 if unauthenticated.
2. Validate names: trim, reject empty, reject > 100 chars (mirrors
   `HandleCreateTag`), de-duplicate case-insensitively.
3. Resolve each name case-insensitively against the caller's tags,
   auto-creating any that do not exist (default color).
4. Reconcile `user_game_tags`: insert links that are missing, delete links no
   longer present. `{"tags": []}` clears all tags on the game.
5. Return the updated user-game with its `Tags` relation eager-loaded — the same
   response shape `HandleUpdateUserGame` already produces
   (`toUserGameWithPlatformsResponse`).

Resolving/creating only ever happens within the caller's own tags, so
cross-user tag assignment is impossible by construction; combined with the
ownership check on the user-game, there is no path to touch another user's data.

This is a dedicated sub-resource rather than a field folded into
`PUT /api/user-games/:id`. The existing PUT handler builds dynamic SQL per
scalar column; an arbitrary-length set-reconcile does not fit that shape, and a
dedicated replace-set endpoint maps cleanly onto both the UI tag editor and
MCP's `update_game`.

### Shared reconcile helper

Today `findOrCreateTag` is unexported in package `tasks` and duplicated in
effect across the two import workers. Extract the shared logic into
`internal/usergame` (which already holds `RemoveFromPoolsIfFinished`,
`ClearWishlistOnAcquire`, `PromoteToInProgressIfPlayed`, all on a `bun.IDB`
signature):

- `usergame.ResolveOrCreateTag(ctx, db bun.IDB, userID, name string, color *string) (tagID string, err error)`
  — case-insensitive find-or-create, matching today's `findOrCreateTag`.
- `usergame.ReplaceTags(ctx, db bun.IDB, userGameID, userID string, names []string) error`
  — resolve/create each name, then reconcile the join table.

Both import workers switch to `ResolveOrCreateTag` (their additive,
skip-on-error merge behavior is preserved — they do not use the replace-set
reconcile). The new handler calls `ReplaceTags`. No behavior change is intended
for the import path.

### Frontend rewire & cleanup

Rewire `game-edit-form.tsx`: replace the three-step add/remove/create-or-get tag
logic on save with a single replace-set call. The form already has `useAllTags`
and `selectedTagIds`, so it maps the selected IDs to names and PUTs the full
set:

```ts
await replaceTags.mutateAsync({ userGameId: game.id, tags: selectedTagNames });
```

- The `TagSelector` component is unchanged. It is ID-based and shared with
  `game-filters.tsx`.
- Inline tag creation (`handleCreateTag`) switches from the broken
  `createOrGetTag` to the existing, working `POST /api/tags` (`createTag`) so a
  newly created tag still gets a real ID and color in the selector before save.
  A 409 on an accidental duplicate is handled by selecting the existing tag.
- Add a `replaceUserGameTags(id, names)` client function and a
  `useReplaceUserGameTags` hook that invalidates the affected game and tag
  queries.

Delete as dead code (keeps `knip` green):

- `api/tags.ts`: `createOrGetTag`, `assignTagsToGame`, `removeTagsFromGame`,
  `bulkAssignTags`, `bulkRemoveTags`, and their now-orphaned request/response
  types.
- `use-tags.ts`: `useCreateOrGetTag`, `useAssignTagsToGame`,
  `useRemoveTagsFromGame` (and any bulk hooks), plus the re-exports in
  `hooks/index.ts`.

Known minor wrinkle: with by-name, the UI maps IDs → names and the backend maps
names → IDs — a small round-trip. It is cheap (the form already has the tag
list) and keeps a single contract MCP #518 can reuse directly.

## Testing

Backend (`internal/api`, shared test container per the test policy):

- Replace set on a game with no tags → links created.
- Replace with a subset → surplus links removed, kept links untouched.
- Replace with `[]` → all links cleared.
- New name auto-creates a definition; existing name (case-insensitive) reuses it
  — no duplicate definition.
- Ownership/isolation: a name resolves/creates only within the caller's tags;
  user A cannot tag user B's game (404) or attach B's tags.
- Validation: empty name rejected, > 100 chars rejected, duplicate names in one
  body de-duped.
- Unknown user-game id → 404; unauthenticated → 401.

Shared helper: a focused unit test for `usergame.ReplaceTags` /
`ResolveOrCreateTag` if the extracted logic is non-trivial; otherwise the API
tests exercise it. Confirm the existing import-worker tests still pass after the
extraction.

Frontend: update `game-edit-form.test.tsx` to assert the single replace-set call
(add, remove, clear) instead of the old granular mocks. Confirm `npm run check`,
`npm run knip`, and `npm run test` pass after the deletions.
