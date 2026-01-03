"""NATS JetStream broker configuration for TaskIQ."""

from nats.js.api import ConsumerConfig
from taskiq_nats import PullBasedJetStreamBroker

from app.core.config import settings

# Configure JetStream consumer with increased ack_wait for long-running tasks
# PSN sync can take 2-3 minutes to fetch library, so we need a longer timeout
consumer_config = ConsumerConfig(
    ack_wait=300,  # 5 minutes - enough for PSN library fetch + processing
)

broker = PullBasedJetStreamBroker(
    servers=[settings.NATS_URL],
    stream_name="nexorious_tasks",
    durable="nexorious_workers",
    consumer_config=consumer_config,
)
