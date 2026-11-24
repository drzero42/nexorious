# Library health does not infer that a game was beaten from its playtime

The Library Smells epic (#1143) planned an auto-fixable `beat-but-not-marked` check: flag a `not_started` or `in_progress` game whose playtime reaches its HowLongToBeat main time, and fix it by setting the status to `completed`.

We removed the check before release (#1145). Playtime past the time-to-beat does not mean a game was beaten: co-op sessions, replays and idle time all inflate it, so the check flagged games that were not finished. Its fix also did real damage when wrong, because a finished status removes the game from its pools.

## Consequences

- Library health ships 9 checks, 3 of them auto-fixable.
- `played-but-not-started` no longer defers to the removed check: it flags a `not_started` game with playtime whatever its HowLongToBeat time.
- An auto-fix that sets a finished status needs a stronger signal than playtime. Bring such a check back only with one.
