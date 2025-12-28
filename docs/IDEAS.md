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

## Make sure platform/storefront seed data can be loaded
In the new frontend we don't have a button to load seed data.

## Websocket?
We have support for websocket, but actually use polling. Should we remove websocket support?

## Remove dependent relationships from docker-compose
We don't have dependent relationships in Kubernetes, so our software must handle when something is unavailable.
Both API backend and workers/scheduler must gracefully handle when database and/or NATS is unavailable.

## Sync does not need an enabled/disabled state
Sync frequency is set to manual is fine for "disabled".
 