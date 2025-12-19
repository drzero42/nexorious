"""
Batch job service for managing persistent batch operations.

Replaces the in-memory BatchSessionManager with database-backed
job tracking using the unified Job model.
"""

import logging
from datetime import datetime, timezone
from typing import List, Optional

from sqlmodel import Session

from app.models.batch_session import BatchOperationType
from app.models.job import (
    BackgroundJobPriority,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobType,
    ImportJobSubtype,
    Job,
)

logger = logging.getLogger(__name__)


class BatchJobService:
    """
    Service for managing batch operations using the Job model.

    Provides a clean interface for creating, updating, and querying
    batch jobs with full database persistence.
    """

    def __init__(self, session: Session):
        self.session = session

    def create_batch_job(
        self,
        user_id: str,
        operation_type: BatchOperationType,
        source: BackgroundJobSource,
        total_items: int,
    ) -> Job:
        """
        Create a new batch job.

        Args:
            user_id: ID of the user starting the operation
            operation_type: Type of batch operation (auto_match or sync)
            source: Import source (darkadia, steam, etc.)
            total_items: Total number of items to process

        Returns:
            Created Job instance
        """
        # Map batch operation type to import subtype
        subtype_map = {
            BatchOperationType.AUTO_MATCH: ImportJobSubtype.AUTO_MATCH,
            BatchOperationType.SYNC: ImportJobSubtype.BULK_SYNC,
        }

        job = Job(
            user_id=user_id,
            job_type=BackgroundJobType.IMPORT,
            source=source,
            import_subtype=subtype_map[operation_type],
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
            progress_total=total_items,
            progress_current=0,
            successful_items=0,
            failed_items=0,
            started_at=datetime.now(timezone.utc),
        )

        self.session.add(job)
        self.session.commit()
        self.session.refresh(job)

        logger.info(
            f"Created batch job {job.id} for user {user_id} "
            f"(type: {operation_type.value}, source: {source.value}, items: {total_items})"
        )

        return job

    def get_batch_job(self, job_id: str) -> Optional[Job]:
        """Get a batch job by ID."""
        return self.session.get(Job, job_id)

    def update_batch_progress(
        self,
        job_id: str,
        processed_count: int,
        successful_count: int,
        failed_count: int,
        processed_ids: List[str],
        failed_ids: List[str],
        errors: List[str],
    ) -> Optional[Job]:
        """
        Update progress for a batch job.

        Args:
            job_id: ID of the job to update
            processed_count: Number of items processed in this batch
            successful_count: Number successfully processed
            failed_count: Number that failed
            processed_ids: List of processed item IDs
            failed_ids: List of failed item IDs
            errors: List of error messages

        Returns:
            Updated Job or None if not found
        """
        job = self.session.get(Job, job_id)
        if not job:
            return None

        # Update counters
        job.progress_current += processed_count
        job.successful_items += successful_count
        job.failed_items += failed_count

        # Append to ID lists
        current_processed = job.get_processed_item_ids()
        current_processed.extend(processed_ids)
        job.set_processed_item_ids(current_processed)

        current_failed = job.get_failed_item_ids()
        current_failed.extend(failed_ids)
        job.set_failed_item_ids(current_failed)

        # Append errors to error log
        for error in errors:
            job.add_error({"message": error, "timestamp": datetime.now(timezone.utc).isoformat()})

        # Check if complete
        if job.progress_current >= job.progress_total:
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.debug(
            f"Updated batch job {job_id}: "
            f"{job.progress_current}/{job.progress_total} processed, "
            f"{job.successful_items} successful, {job.failed_items} failed"
        )

        return job

    def cancel_batch_job(self, job_id: str, user_id: str) -> Optional[Job]:
        """
        Cancel a batch job.

        Args:
            job_id: ID of the job to cancel
            user_id: ID of the user (for authorization)

        Returns:
            Cancelled Job or None if not found/unauthorized
        """
        job = self.session.get(Job, job_id)
        if not job or job.user_id != user_id:
            return None

        if not job.is_active:
            logger.warning(f"Attempted to cancel non-active job {job_id}")
            return job

        job.status = BackgroundJobStatus.CANCELLED
        job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.info(f"Cancelled batch job {job_id} for user {user_id}")

        return job

    def fail_batch_job(self, job_id: str, error_message: str) -> Optional[Job]:
        """
        Mark a batch job as failed.

        Args:
            job_id: ID of the job to fail
            error_message: Error message describing the failure

        Returns:
            Failed Job or None if not found
        """
        job = self.session.get(Job, job_id)
        if not job:
            return None

        job.status = BackgroundJobStatus.FAILED
        job.error_message = error_message
        job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.error(f"Failed batch job {job_id}: {error_message}")

        return job
