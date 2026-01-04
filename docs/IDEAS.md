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
After refactoring to use IGDB ID as the primary key for games, there are schemas and models that feature both igdb_id and game_id. Those are now the same, so we need to refactor to only use one of them. No need to have both. This maybe requires rethinking how sync and imports work. Maybe igdb_id should just stay in these models or maybe they should also be refactored a bit. It does however not make sense to use the game_id field as an indicator of whether a game has been synced or not, when that field is the same as igdb_id.

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

## GOG
Use https://github.com/Sude-/lgogdownloader as a CLI tool to pull informatiot about the user's library

## Playstation
https://psnawp.readthedocs.io

## Xbox
https://github.com/OpenXbox/xbox-webapi-python

## Remove unneeded CSV code
We recently removed support for importing CSV, but we still have mentions of CSV as source for importing in the code. This should be removed.

## Notifications
Allow notifications to be sent to some external service like Telegram. A helper should be used for this, which can send to many different services (pushover and various others) and which will also help keep the amount of needed code down.
Notifications should be configurable so the user can choose what to receive notifications for. Examples of notifications would be needing to re-auth Epic or that new games were added from sync sources.

## Next.js proxy
The frontend reports: The "middleware" file convention is deprecated. Please use "proxy" instead. Learn more: https://nextjs.org/docs/messages/middleware-to-proxy

## Get rid of all "coming soon" messages
We have multiple places in the app where we claim things are "coming soon". This is not helpful and should be removed.

## Maintenance jobs
We need scheduled jobs that take care of cleaning up expired jobs and orphaned files.

## Epic Games Store sync configuration
When clicking Connect we have a box pop-up with a button that says Start Authentication, after which another box pops up with the link to authenticate and an input for the code. We don't need the first box - instead incorporate the information in a single box to handle the full process.

## Darkadia unknown platform and/or storefront
When we convert and import Darkadia CSV as nexorious JSON we should make sure missing platforms and storefronts end up as unknown associations in our database. This way the user can go through and handle it after importing. Some games may need to be hunted down.
A function to sort out missing platform/storefronts for games may be a good idea to add.

## Apostrophe titles
When searching IGDB for titles with apostrophes in the name, it can not find any results.

## Icons for platforms and storefronts
We have icons for all platforms and storefronts, but they are not used on the /games page or the game details pages.

## The needs review badges and number of items needing review on the sync page
They look bad and must be improved.
