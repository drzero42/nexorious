-- Subscribe existing admins to the new admin.version.available event type.
-- New users are seeded at creation time (see notify.SeedDefaultSubscriptions);
-- this covers admins created before the type existed.
-- Idempotent: ON CONFLICT DO NOTHING skips admins already subscribed.

INSERT INTO notification_subscriptions (user_id, event_type, created_at)
SELECT u.id, 'admin.version.available', now()
FROM users u
WHERE u.is_admin = true
ON CONFLICT (user_id, event_type) DO NOTHING;
