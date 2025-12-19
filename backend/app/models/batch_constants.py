"""Constants for batch processing operations."""

from enum import Enum


class BatchOperationType(str, Enum):
    """Types of batch operations."""

    AUTO_MATCH = "auto_match"
    SYNC = "sync"


# Batch sizes for different operation types
BATCH_SIZES = {
    BatchOperationType.AUTO_MATCH: 5,
    BatchOperationType.SYNC: 5,
}
