-- Append-only notification/audit outbox. One row per emitted event.
-- dedup_key gives fire-once semantics for job-scoped events (NULL = repeatable;
-- Postgres treats NULLs as distinct so repeatable events always insert).
CREATE TABLE events (
    id            TEXT PRIMARY KEY,
    type          TEXT NOT NULL,
    scope         TEXT NOT NULL,
    actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    dedup_key     TEXT,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX events_dedup_key_idx ON events (dedup_key);
CREATE INDEX events_occurred_at_idx ON events (occurred_at);
CREATE INDEX events_actor_user_id_idx ON events (actor_user_id);
