"""NATS JetStream broker configuration for TaskIQ."""

from taskiq_nats import PullBasedJetStreamBroker

from app.core.config import settings

broker = PullBasedJetStreamBroker(
    servers=[settings.NATS_URL],
    stream_name="nexorious_tasks",
    durable="nexorious_workers",
)
