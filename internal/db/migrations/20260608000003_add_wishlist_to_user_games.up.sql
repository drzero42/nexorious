-- Wishlist is a game-level state on user_games (#867). A wishlisted entry has
-- is_wishlisted=true and zero user_game_platforms; attaching any platform
-- clears the flag (auto-promote to library), enforced by the application layer.
ALTER TABLE user_games
    ADD COLUMN is_wishlisted BOOLEAN NOT NULL DEFAULT false;

-- The wishlist page filters "is_wishlisted = true"; a partial index keeps that
-- scan small since the vast majority of rows are library entries.
CREATE INDEX user_games_wishlisted_idx
    ON user_games (user_id)
    WHERE is_wishlisted = true;
