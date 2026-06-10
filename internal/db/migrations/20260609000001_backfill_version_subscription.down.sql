-- No-op: the backfill cannot be safely reversed (backfilled rows are
-- indistinguishable from subscriptions the user has since chosen to keep).
SELECT 1;
