"""Taskiq broker configuration for background task processing.

Uses PostgreSQL as both the message broker and result backend via taskiq-pg.
"""

from taskiq.serializers import JSONSerializer
from taskiq_pg import AsyncpgBroker, AsyncpgResultBackend

from app.core.config import settings


def _get_database_url() -> str:
    """Deferred DSN resolution for broker startup.

    Using a callable avoids config loading at import time, which is important
    for:
    - Testing with different database configurations
    - Type checking tools that import modules
    - Delayed environment variable resolution
    """
    return settings.database_url


# Result backend configuration
# - Uses JSONSerializer for human-readable results in the database
# - keep_results=True retains results for later retrieval and debugging
result_backend: AsyncpgResultBackend[object] = AsyncpgResultBackend(
    dsn=_get_database_url,
    serializer=JSONSerializer(),
    keep_results=True,
)

# Broker configuration
# - Uses PostgreSQL LISTEN/NOTIFY for efficient task distribution
# - Workers can be scaled horizontally
broker = AsyncpgBroker(
    dsn=_get_database_url,
).with_result_backend(result_backend)
