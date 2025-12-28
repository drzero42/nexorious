"""Queue configuration for NATS subject-based routing."""

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH = "tasks.high"
SUBJECT_LOW = "tasks.low"

# Legacy compatibility (will be removed after full migration)
QUEUE_HIGH = "high"
QUEUE_LOW = "low"

# Keep old subjects as aliases during migration
SUBJECT_HIGH_IMPORT = SUBJECT_HIGH
SUBJECT_HIGH_SYNC = SUBJECT_HIGH
SUBJECT_HIGH_EXPORT = SUBJECT_HIGH
SUBJECT_LOW_IMPORT = SUBJECT_LOW
SUBJECT_LOW_SYNC = SUBJECT_LOW
SUBJECT_LOW_MAINTENANCE = SUBJECT_LOW
