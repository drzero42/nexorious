"""Taskiq broker configuration for background task processing.

Uses PostgreSQL as both the message broker and result backend via taskiq-pg.

Table Creation Strategy:
    To avoid race conditions when multiple workers start simultaneously, the
    scheduler (singleton) creates the taskiq tables, while workers skip table
    creation. This is controlled via the TASKIQ_SKIP_TABLE_CREATION env var.
"""

import os
from typing import Any, Callable, Optional, Union

import asyncpg
from taskiq.serializers import JSONSerializer
from taskiq_pg import AsyncpgBroker, AsyncpgResultBackend
from taskiq_pg.queries import CREATE_INDEX_QUERY, CREATE_TABLE_QUERY

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


class SafeAsyncpgResultBackend(AsyncpgResultBackend[object]):
    """Result backend that can optionally skip table creation.

    When TASKIQ_SKIP_TABLE_CREATION=true, the startup() method only creates
    the connection pool without attempting to create tables. This prevents
    race conditions when multiple workers start simultaneously.

    The scheduler (which runs as a singleton) should NOT have this env var set,
    so it will create the tables on startup before workers connect.
    """

    def __init__(
        self,
        dsn: Union[Optional[str], Callable[[], str]] = None,
        skip_table_creation: bool = False,
        **kwargs: Any,
    ) -> None:
        super().__init__(dsn=dsn, **kwargs)
        self._skip_table_creation = skip_table_creation

    async def startup(self) -> None:
        """Initialize the result backend.

        Creates connection pool and optionally creates tables (if not skipped).
        """
        _database_pool = await asyncpg.create_pool(
            dsn=self.dsn,
            **self.connect_kwargs,
        )
        if _database_pool is None:
            msg = "Database pool not initialized"
            raise RuntimeError(msg)
        self._database_pool = _database_pool

        if not self._skip_table_creation:
            _ = await self._database_pool.execute(
                CREATE_TABLE_QUERY.format(
                    self.table_name,
                    self.field_for_task_id,
                ),
            )
            _ = await self._database_pool.execute(
                CREATE_INDEX_QUERY.format(
                    self.table_name,
                    self.table_name,
                ),
            )


# Check if we should skip table creation (set for workers, not for scheduler)
_skip_table_creation = os.getenv("TASKIQ_SKIP_TABLE_CREATION", "").lower() == "true"

# Result backend configuration
# - Uses JSONSerializer for human-readable results in the database
# - keep_results=True retains results for later retrieval and debugging
# - skip_table_creation=True for workers to avoid race conditions
result_backend = SafeAsyncpgResultBackend(
    dsn=_get_database_url,
    serializer=JSONSerializer(),
    keep_results=True,
    skip_table_creation=_skip_table_creation,
)

# Broker configuration
# - Uses PostgreSQL LISTEN/NOTIFY for efficient task distribution
# - Workers can be scaled horizontally
broker = AsyncpgBroker(
    dsn=_get_database_url,
).with_result_backend(result_backend)
