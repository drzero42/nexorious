-- Composite index for the admin activity feed's keyset scan
-- (ORDER BY occurred_at DESC, id DESC + row-value cursor predicate).
CREATE INDEX events_occurred_at_id_idx ON events (occurred_at DESC, id DESC);
