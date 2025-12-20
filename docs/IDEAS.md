# Ideas

## Choose next game flow
Add functionality to go help choose which game to play next (Next Up view).
Should take wishlist into account as well.
Uses sorting and filtering based on genres, platforms and time-to-beat to help the user choose what game to play next.

## Backlog management
Add a new view that shows all games that are not completed (/mastered/dominated) and not shelved.

## Darkadia import based on same principles as Steam Games
The Darkadia CSV import should be rebuilt based on the same principles as the Steam Games feature. Biggest differences are that the Darkadia CSV import is unlikely to be much more than a one-off operation, and that Darkadia CSV contains a lot more information (platform/storefront information, date it was added, completion status and more).
The flow should be that the CSV is read into a darkadia_games table, in the same style as steam_games. From there the same matching and syncing functionality as for Steam Games should be available.

## UserGames should be kept unless specifically deleted
Ensure the following logic is followed everywhere.
When removing the last platform/storefront, change ownership to No Longer Owned. If adding a platform/storefront to a UserGame change ownership to Owned.
Only actually delete a UserGame if the user deletes it.

## Use IGDB ID as Game ID
After refactoring to use IGDB ID as the primary key for games, there are schemas and models that feature both igdb_id and game_id. Those are now the same, so we need to refactor to only use one of them. No need to have both. This requires rethinking how steam and darkadia imports work. Maybe igdb_id should just stay in these models or maybe they should also be refactored a bit. It does however not make sense to use the game_id field as an indicator of whether a game has been synced or not, when that field is the same as igdb_id.

## No need to import from IGDB
It should be transparent that data is pulled in from IGDB. Instead of having a workflow of import-igdb and then adding a user-games entry, the user-games add endpoint should just accept an IGDB ID. If no game with that ID exists in our database it should be imported from IGDB. If one already exists, just use that.

## Switch from taskiq-pg to PGQueuer
Replace the current taskiq-pg task queue with PGQueuer for true competing consumers semantics.

**Why:** taskiq-pg uses LISTEN/NOTIFY to broadcast tasks to all workers, requiring manual advisory locks to prevent duplicate processing. PGQueuer uses PostgreSQL's `FOR UPDATE SKIP LOCKED` which provides true queue semantics where exactly one worker claims each job atomically.

**Benefits:**
- No wasted work (workers don't wake up just to fail lock acquisition)
- Built-in rate limiting and concurrency control
- Batch operations for high-throughput scenarios
- Built-in dashboard and Prometheus metrics
- Can remove the manual advisory lock code in `app/worker/locking.py`
