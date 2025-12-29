# Ideas

## Choose next game flow
Add functionality to go help choose which game to play next (Next Up view).
Should take wishlist into account as well.
Uses sorting and filtering based on genres, platforms and time-to-beat to help the user choose what game to play next.

## Backlog management
Add a new view that shows all games that are not completed (/mastered/dominated) and not shelved.

## UserGames should be kept unless specifically deleted
Ensure the following logic is followed everywhere.
When removing the last platform/storefront, change ownership to No Longer Owned. If adding a platform/storefront to a UserGame change ownership to Owned.
Only actually delete a UserGame if the user deletes it.

## Use IGDB ID as Game ID
After refactoring to use IGDB ID as the primary key for games, there are schemas and models that feature both igdb_id and game_id. Those are now the same, so we need to refactor to only use one of them. No need to have both. This requires rethinking how steam and darkadia imports work. Maybe igdb_id should just stay in these models or maybe they should also be refactored a bit. It does however not make sense to use the game_id field as an indicator of whether a game has been synced or not, when that field is the same as igdb_id.

## No need to import from IGDB
It should be transparent that data is pulled in from IGDB. Instead of having a workflow of import-igdb and then adding a user-games entry, the user-games add endpoint should just accept an IGDB ID. If no game with that ID exists in our database it should be imported from IGDB. If one already exists, just use that.

## Use knip to keep frontend lean
We should use knip.dev to identify dead code in the frontend and help keep the frontend code lean and fresh.

## Use vulture to keep backend lean
We should use vulture to find dead code in the backend.

## Make sure platform/storefront seed data can be loaded
In the new frontend we don't have a button to load seed data.

## Websocket?
We have support for websocket, but actually use polling. Should we remove websocket support?

## Remove dependent relationships from docker-compose
We don't have dependent relationships in Kubernetes, so our software must handle when something is unavailable.
Both API backend and workers/scheduler must gracefully handle when database and/or NATS is unavailable.

## Import/export is for the current user
Our export format does not contain all information from the database and also does not contain image-files. We should refactor the import/export to make it a user-scoped thing, meaning that only the current users items are exported and imports add the games to the current user.
This is a user-feature - this is not an admin-only feature.

## Backup/restore
Since exports are not usable as full backups, we need to add proper backup/restore functionality. That means dumping all data from the database to a suitable format and creating a compressed archive with that dump along with all relevant static files (probably only cover art). The backups should be stored in a dedicated dir, should be downloadable by the user and we want it to be scheduleable with configuration for retention time.
Restore will be a full reset to the backed up state and must properly warn the user before performing it. Restores can be done from backups the server has available or from a backup uploaded by the user.
This is an admin-only feature.

## Menu refactoring
Sync and Tags should be part of the root menu again.
We don't need the Setting section - it only has profile under it, which is also available when clicking the username at the bottom.
Import/export should be moved under the username at the bottom - same place as Profile.
