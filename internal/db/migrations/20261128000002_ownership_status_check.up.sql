-- Reject a user_game_platforms.ownership_status or external_games.ownership_status
-- that is not an enum.OwnershipStatus value or NULL (#1500). Every write path
-- already validates; before #1499 a PUT on a platform could store '', so clear
-- anything outside the enum first. A cleared platform shows up in the
-- missing-ownership-status smell; the next sync rewrites a cleared External game.
--
-- A status added to enum.OwnershipStatus must be added here too, by a new
-- migration; TestOwnershipStatusCheckMatchesEnum fails until it is.

UPDATE user_game_platforms
   SET ownership_status = NULL
 WHERE ownership_status NOT IN ('owned', 'borrowed', 'rented', 'subscription', 'no_longer_owned');

UPDATE external_games
   SET ownership_status = NULL
 WHERE ownership_status NOT IN ('owned', 'borrowed', 'rented', 'subscription', 'no_longer_owned');

ALTER TABLE user_game_platforms ADD CONSTRAINT user_game_platforms_ownership_status_check
    CHECK (ownership_status IS NULL OR ownership_status IN ('owned', 'borrowed', 'rented', 'subscription', 'no_longer_owned'));

ALTER TABLE external_games ADD CONSTRAINT external_games_ownership_status_check
    CHECK (ownership_status IS NULL OR ownership_status IN ('owned', 'borrowed', 'rented', 'subscription', 'no_longer_owned'));
