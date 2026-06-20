-- Play Planning pools (#955). A pool is a sibling of tags: a named, ordered,
-- user-defined collection of games to play, with an optional saved filter that
-- drives suggestions. pool_games is membership AND queue in one table:
-- position IS NULL = Candidate, position NOT NULL = in the Up Next queue.
CREATE TABLE pools (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    color      TEXT,
    position   INTEGER NOT NULL,
    filter     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX pools_user_id_idx ON pools (user_id);

CREATE TABLE pool_games (
    id           TEXT PRIMARY KEY,
    pool_id      TEXT NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
    user_game_id TEXT NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    position     INTEGER,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(pool_id, user_game_id)
);

CREATE INDEX pool_games_pool_id_idx ON pool_games (pool_id);
CREATE INDEX pool_games_user_game_id_idx ON pool_games (user_game_id);
