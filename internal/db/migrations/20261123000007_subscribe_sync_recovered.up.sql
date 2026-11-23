-- sync.recovered (#1385) is on by default, but defaults are only seeded for
-- new users and on Reset. It ends an outage that sync.failed reported, so
-- subscribe the existing users who get sync.failed.
INSERT INTO notification_subscriptions (user_id, event_type)
SELECT user_id, 'sync.recovered'
FROM notification_subscriptions
WHERE event_type = 'sync.failed'
ON CONFLICT (user_id, event_type) DO NOTHING;
