# Ideas

## Choose next game flow
Add functionality to go help choose which game to play next (Next Up view).
Should take wishlist into account as well.
Uses sorting and filtering based on genres, platforms and time-to-beat to help the user choose what game to play next.

## Backlog management
Add a new view that shows all games that are not completed (/mastered/dominated) and not shelved.

## Better handling of games not imported with Darkadia CSV import
During import of Darkadia games with non-interactive strategies, all games that fail should be written to a new CSV file named the same as the one being imported, with an added -failed to the name. The output format should be the same as the Darkadia CSV. This will allow the user to go through all failed games and add by hand.

## No direct SQLAlchemy usage
Go through all direct usage of SQLAlchemy and check if SQLModel could be used instead.

## Darkadia import based on same principles as Steam Games
The Darkadia CSV import should be rebuilt based on the same principles as the Steam Games feature. Biggest differences are that the Darkadia CSV import is unlikely to be much more than a one-off operation, and that Darkadia CSV contains a lot more information (platform/storefront information, date it was added, completion status and more).
The flow should be that the CSV is read into a darkadia_games table, in the same style as steam_games. From there the same matching and syncing functionality as for Steam Games should be available.
