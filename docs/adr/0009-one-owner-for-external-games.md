# internal/externalgame is the only code that writes External games

The writes to `external_games` and `external_game_platforms` were spread over HTTP handlers in `internal/api` and the sync workers in `internal/worker/tasks`: 18 raw SQL statements in 7 files. The functions behind Rematch and Skip built HTTP errors, `internal/api` took the library write from the worker package, and the scattering had already cost a bug (#1419: Skip took its own inline lock and so accepted a game the lock should have refused).

We decided that `internal/externalgame` is the only non-test Go code that writes the two tables or takes an explicit row lock on them (#1412). It holds the decisions on a game's editions (Rematch, Skip, Unskip on the locked set), the library write, the ingest side of a sync and the remaining thin writes (the store link, the storefront reset, the admin reset). That gives three owners, with dependencies pointing one way:

- `internal/joblifecycle` owns the process: Jobs, items and their transactions (ADR 0001).
- `internal/externalgame` owns the storefront-side entity: External games and their editions.
- `internal/usergame` owns the library entity (ADR 0004).

`internal/externalgame` imports `internal/usergame` and is imported by the Job code, never the other way round.

Derived editions (ADR 0008) make this more than tidiness. Which games are editions follows from the title, the match and the skip of each row, and nothing in the database makes a decision reach every edition. A write from outside the module that changes one of those columns on one game takes it out of its set without the editions lock and without saying so.

## Considered options

- **Column-level ownership** (only the title, match and skip columns). Rejected: the upsert and the match writes have to be in the module anyway, and a table-level rule can be checked with a source scan, a column-level one cannot. The price is a few one-statement functions (`SetStoreLink`, `DeleteForStorefront`, `DeleteForAllUsers`, `PrunePlatforms`, `MarkUnavailable`).
- **An allowlist** for the resets. Rejected: they go through the module like everything else, so the guard has no exceptions for production code. It later gained one for tests, the fixture package `egtest` (#1506, under Consequences).
- **A lint rule or `depguard`.** Rejected: the repo has no `depguard` setup, and a test is enough to fail CI.

## Consequences

- **What stays outside:** reads (the listing handlers and the sync status view keep reading the tables, with the module's edition key), the Job glue (finding the active Job, the open items of a set, item transitions) and the `changes` rows. The implicit key-share locks that foreign-key checks take on insert into `changes`, `job_items` and `user_game_platforms` are not explicit locks and stay outside too.
- **Guard tests in `internal/externalgame`.** One scans the non-test Go sources outside the package for SQL literals and query-builder chains that write the two tables, and fails on any. Another fails if the module links `internal/joblifecycle`, `internal/worker/tasks` or `internal/api`. There is no allowlist for production code; the test-fixture package below is the one exemption.
- **Known gaps:** the write guard reads source, not running queries, so it misses a write it cannot see as one: a builder chain split across variables or run on a typed variable (`NewUpdate().Model(&eg)`), a table named through a placeholder (`TableExpr("?", bun.Ident("external_games"))`), and SQL built at run time. Explicit row locks are part of the rule but are not checked.
- **Test fixtures:** tests write rows directly, through `internal/externalgame/egtest` (#1506). It is the one package the write guard exempts, and a third guard test fails if a non-test package imports it, so the exemption cannot carry a production write. A fixture writes one row as it is, without the editions lock; that is what a test needs and what production code must not do, which is why it is not an operation of the module. Migration tests write the schema as it was before their migration and keep their own SQL.
- **Outside the guards by design:** test files, backup restore (it drops and reloads the whole schema), SQL migrations (not Go), and foreign-key actions. Platform rows cascade from their game, and `external_games.user_id` is `ON DELETE CASCADE`, so deleting a user removes that user's External games.
