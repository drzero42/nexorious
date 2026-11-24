# Every user-game mutation goes through internal/usergame

Acquiring a game means inserting its platforms, clearing the wishlist flag, and promoting `not_started` to `in_progress` if it has playtime. That sequence was assembled by hand at more than a dozen call sites across the REST handlers, the sync worker and the import pipeline, and the copies had drifted: import skipped the promote, so imported games with playtime stayed `not_started` (#1061).

We decided that each user-game mutation (acquire, platform add/update/remove, set status, record progress, delete, clear library) has exactly one implementation in `internal/usergame` (#1055, #1056). It runs the whole invariant set (clear wishlist, promote if played, remove from pools when finished, tag reconcile) in one transaction. The invariant helpers are unexported, so a caller cannot reassemble a mutation and skip a step.

## Considered options

- **A service layer so REST, MCP and the CLI share handlers.** Rejected: MCP and the CLI are REST clients, not in-process callers, so handler parity buys nothing. The problem was divergent writes, not long handlers.
- **Fix each drifted call site.** Rejected: it fixes #1061 but leaves the next drift possible.

## Consequences

- Converging the paths changed behaviour where they disagreed: import now promotes if played, as sync did.
- New mutations, and new callers such as the library-health auto-fixes, go through `internal/usergame` instead of writing `user_games` or `user_game_platforms` directly.
- Reads are out of scope: a read path written by hand duplicates code but cannot break an invariant.
