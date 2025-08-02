# Ideas

## Choose next game flow
Add functionality to go help choose which game to play next (Next Up view).
Should take wishlist into account as well.
Uses sorting and filtering based on genres, platforms and time-to-beat to help the user choose what game to play next.

## Backlog management
Add a new view that shows all games that are not completed (/mastered/dominated) and not shelved.

## Better handling of games not imported with Darkadia CSV import
During import of Darkadia games with non-interactive strategies, all games that fail should be written to a new CSV file named the same as the one being imported, with an added -failed to the name. The output format should be the same as the Darkadia CSV. This will allow the user to go through all failed games and add by hand.

## Platforms and storefront icons based on the real logos
Platforms and storefronts all have official logos. These should be downloaded and used in the platform badges. We need to be sure to keep it legal, so any ownership of the logos must be attributed correctly in our README.md.

## No direct SQLAlchemy usage
Go through all direct usage of SQLAlchemy and check if SQLModel could be used instead.

