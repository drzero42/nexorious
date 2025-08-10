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

## Steam Games menu item icon
The Steam Games menu item should use the Steam icon rather than the current emoji.

## Steam Games profile setting
Add a profile setting to allow the user to disable the Steam Games integration functionality. It will hide the Steam Games menu item and will make all Steam Games pages (if visited directly) show a message about this being disabled.

## Steam Games depends on platform/storefront
The Steam Games menu item and pages should not only depend on the setting in the profile page, but should also check that a platform called PC (Windows) exists as well as a storefront called Steam.

## Steam Games in sync link to game
The Steam Games that are in sync should have a link to the game in our database.

## Steam Games auto-matching and syncing small batches
Since we are not doing full and proper queueing and background workers, instead of running hundreds of games through either the auto-matching or syncinc functionality at once, the frontend should only trigger batches of 5 or 10 games at a time and do a refresh before starting the next batch. That will give the user some feedback along the way so it won't just be a stuck interface the user has to wait for. It will also allow us to add a cancel button, which will just stop the frontend from triggering the next batch.
This requires a mix of changes to the frontend and backend.
