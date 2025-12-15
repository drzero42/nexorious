"""Task definitions for background processing.

Import all task modules here to ensure they are registered with the broker.
The taskiq worker discovers tasks by importing these modules.
"""

# Maintenance tasks (scheduled cleanup operations)
from app.worker.tasks.maintenance import (
    cleanup_task_results,
    cleanup_expired_exports,
    cleanup_expired_sessions,
)

# Sync tasks (platform library synchronization)
from app.worker.tasks.sync import (
    sync_steam_library,
    check_pending_syncs,
)

# Import/export tasks
from app.worker.tasks.import_export import (
    import_nexorious_json,
    import_darkadia_csv,
    import_steam_library,
    export_collection,
)

__all__ = [
    # Maintenance
    "cleanup_task_results",
    "cleanup_expired_exports",
    "cleanup_expired_sessions",
    # Sync
    "sync_steam_library",
    "check_pending_syncs",
    # Import/Export
    "import_nexorious_json",
    "import_darkadia_csv",
    "import_steam_library",
    "export_collection",
]
