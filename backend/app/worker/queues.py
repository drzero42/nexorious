"""Queue configuration for NATS subject-based routing."""

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH_IMPORT = "tasks.high.import"
SUBJECT_HIGH_SYNC = "tasks.high.sync"
SUBJECT_HIGH_EXPORT = "tasks.high.export"

SUBJECT_LOW_IMPORT = "tasks.low.import"
SUBJECT_LOW_SYNC = "tasks.low.sync"
SUBJECT_LOW_MAINTENANCE = "tasks.low.maintenance"

# Legacy compatibility (will be removed after full migration)
QUEUE_HIGH = "high"
QUEUE_LOW = "low"
