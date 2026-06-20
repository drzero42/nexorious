-- Per-user app preferences (#867). Typed columns, one row per user, lazily
-- upserted. Deliberately separate from the auth-owned users table; do NOT
-- reintroduce the generic users.preferences blob removed in #797/#798.
CREATE TABLE user_settings (
    user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    deal_region TEXT NOT NULL DEFAULT 'us',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
