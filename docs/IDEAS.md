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

## Games can not be deleted
Instead of having a backend endpoint that deletes a game, games in the `games` table should only be deletable by removing all user_games associations. That means that if a user deletes a game in the frontend, it should delete the user_games entry. When a user_games entry is deleted, the backend should check if that was the last reference that the related games entry had and then delete the games entry if that was the case.
The point is that the user should not need to care if a game exists in the database or not, only if a game is in their collection.
