-- A user's notification channels. encrypted_url holds the Shoutrrr URL
-- encrypted at rest (enc:v1: prefix), same pattern as storefront_credentials.
CREATE TABLE notification_channels (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    encrypted_url TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX notification_channels_user_id_idx ON notification_channels (user_id);
