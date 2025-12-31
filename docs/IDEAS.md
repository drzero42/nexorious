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

## Remove dependent relationships from docker-compose
We don't have dependent relationships in Kubernetes, so our software must handle when something is unavailable.
Both API backend and workers/scheduler must gracefully handle when database and/or NATS is unavailable.

## Experiment with slumber
https://github.com/LucasPickering/slumber
Might be better than claude failing to use curl

## Achievements / Trophies
From some platforms/storefronts we can extract information about Achievements/Trophies - at least from Steam this is true.
We should store at least some information about this. Maybe just a percentage of achievements/trophies gained or maybe more detailed...

## Steamctl
Refactor to use https://github.com/ValvePython/steamctl
This will allow auth with Steam Authenticator and will not require public Steam profile. It makes it easier for the user as it should potentially be just a QR code that can be scanned from the Steam mobile app to gain access.

## Epic Games Store
Use https://github.com/derrod/legendary as a CLI tool to pull information about the user's library

## GOG
Use https://github.com/Sude-/lgogdownloader as a CLI tool to pull informatiot about the user's library

## Playstation
https://psnawp.readthedocs.io

## Xbox
https://github.com/OpenXbox/xbox-webapi-python

## Remove unneeded CSV code
We recently removed support for importing CSV, but we still have mentions of CSV as source for importing in the code. This should be removed.

## Keep previous platforms/storefronts association and data
When a game is removed from from a platform/storefront (best example is a PS Plus Extra game) we might have playtime recorded, so instead of deleting the association, it should change status to something like No Longer Owned.

## Allow restore during initial setup
On the initial setup screen, instead of creating an admin user, it should be possible to restore the DB to a backup file that can be uploaded.

## Notifications
Allow notifications to be sent to some external service like Telegram. A helper should be used for this, which can send to many different services (pushover and various others) and which will also help keep the amount of needed code down.
Notifications should be configurable so the user can choose what to receive notifications for. Examples of notifications would be needing to re-auth Epic or that new games were added from sync sources.
