-- Backfill default notification subscriptions for users created before the
-- notifications feature existed. New users are seeded at creation time
-- (see notify.SeedDefaultSubscriptions); this covers pre-existing users.
-- Source of truth for these defaults: notify.DefaultSubscriptions().
-- Idempotent: ON CONFLICT DO NOTHING skips users already seeded.

INSERT INTO notification_subscriptions (user_id, event_type, created_at)
SELECT u.id, e.event_type, now()
FROM users u
CROSS JOIN (VALUES
    ('sync.completed_with_errors'),
    ('sync.failed'),
    ('import.failed'),
    ('export.failed')
) AS e(event_type)
ON CONFLICT (user_id, event_type) DO NOTHING;

INSERT INTO notification_subscriptions (user_id, event_type, created_at)
SELECT u.id, e.event_type, now()
FROM users u
CROSS JOIN (VALUES
    ('admin.backup.failed'),
    ('admin.maintenance.failed')
) AS e(event_type)
WHERE u.is_admin = true
ON CONFLICT (user_id, event_type) DO NOTHING;
