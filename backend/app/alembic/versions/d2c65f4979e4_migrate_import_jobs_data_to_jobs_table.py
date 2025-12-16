"""migrate import_jobs data to jobs table

Revision ID: d2c65f4979e4
Revises: 44eb16d3a604
Create Date: 2025-12-16 18:25:03.741905

"""
from typing import Sequence, Union
import json

from alembic import op
import sqlalchemy as sa
import sqlmodel


# revision identifiers, used by Alembic.
revision: str = 'd2c65f4979e4'
down_revision: Union[str, Sequence[str], None] = '44eb16d3a604'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


# Mapping from ImportType to BackgroundJobSource
IMPORT_TYPE_TO_SOURCE = {
    'CSV': 'CSV',
    'STEAM': 'STEAM',
    'EPIC': 'EPIC',
    'GOG': 'GOG',
    'XBOX': 'XBOX',
    'PLAYSTATION': 'PLAYSTATION',
    'DARKADIA': 'DARKADIA',
}

# Mapping from ImportStatus to BackgroundJobStatus
IMPORT_STATUS_TO_JOB_STATUS = {
    'PENDING': 'PENDING',
    'PROCESSING': 'PROCESSING',
    'RUNNING': 'PROCESSING',  # RUNNING maps to PROCESSING
    'COMPLETED': 'COMPLETED',
    'FAILED': 'FAILED',
    'CANCELLED': 'CANCELLED',
}

# Mapping from old JobType to ImportJobSubtype
JOB_TYPE_TO_IMPORT_SUBTYPE = {
    'LIBRARY_IMPORT': 'LIBRARY_IMPORT',
    'AUTO_MATCH': 'AUTO_MATCH',
    'BULK_SYNC': 'BULK_SYNC',
    'BULK_UNMATCH': 'BULK_UNMATCH',
    'BULK_UNSYNC': 'BULK_UNSYNC',
    'BULK_UNIGNORE': 'BULK_UNIGNORE',
}


def upgrade() -> None:
    """Migrate data from import_jobs to jobs table."""
    conn = op.get_bind()

    # Select all existing import_jobs
    import_jobs_table = sa.table(
        'import_jobs',
        sa.column('id', sa.String),
        sa.column('user_id', sa.String),
        sa.column('import_type', sa.String),
        sa.column('status', sa.String),
        sa.column('total_records', sa.Integer),
        sa.column('processed_records', sa.Integer),
        sa.column('failed_records', sa.Integer),
        sa.column('error_log', sa.String),
        sa.column('job_metadata', sa.String),
        sa.column('job_type', sa.String),
        sa.column('source', sa.String),
        sa.column('started_at', sa.DateTime),
        sa.column('progress', sa.Integer),
        sa.column('total_items', sa.Integer),
        sa.column('processed_items', sa.Integer),
        sa.column('successful_items', sa.Integer),
        sa.column('failed_items', sa.Integer),
        sa.column('error_message', sa.String),
        sa.column('created_at', sa.DateTime),
        sa.column('completed_at', sa.DateTime),
    )

    jobs_table = sa.table(
        'jobs',
        sa.column('id', sa.String),
        sa.column('user_id', sa.String),
        sa.column('job_type', sa.String),
        sa.column('source', sa.String),
        sa.column('status', sa.String),
        sa.column('priority', sa.String),
        sa.column('import_subtype', sa.String),
        sa.column('progress_current', sa.Integer),
        sa.column('progress_total', sa.Integer),
        sa.column('successful_items', sa.Integer),
        sa.column('failed_items', sa.Integer),
        sa.column('result_summary', sa.String),
        sa.column('error_log', sa.String),
        sa.column('error_message', sa.String),
        sa.column('file_path', sa.String),
        sa.column('taskiq_task_id', sa.String),
        sa.column('created_at', sa.DateTime),
        sa.column('started_at', sa.DateTime),
        sa.column('completed_at', sa.DateTime),
    )

    # Fetch all import_jobs
    result = conn.execute(sa.select(import_jobs_table))
    rows = result.fetchall()

    for row in rows:
        # Map import_type to source
        source = IMPORT_TYPE_TO_SOURCE.get(row.import_type, 'SYSTEM')

        # Map status
        status = IMPORT_STATUS_TO_JOB_STATUS.get(row.status, 'PENDING')

        # Map job_type to import_subtype (if exists)
        import_subtype = None
        if row.job_type:
            import_subtype = JOB_TYPE_TO_IMPORT_SUBTYPE.get(row.job_type)

        # Build result_summary from job_metadata, error_log, and failed_records
        try:
            metadata = json.loads(row.job_metadata) if row.job_metadata else {}
        except (json.JSONDecodeError, TypeError):
            metadata = {}

        result_summary = {
            'failed_records': row.failed_records or 0,
            'legacy_metadata': metadata,
        }
        result_summary_json = json.dumps(result_summary)

        # Use error_log directly (already JSON array string)
        error_log_json = row.error_log if row.error_log else '[]'

        # Calculate progress values
        # Use newer fields if available, otherwise fall back to legacy fields
        progress_total = row.total_items if row.total_items else row.total_records or 0
        progress_current = row.processed_items if row.processed_items else row.processed_records or 0
        successful_items = row.successful_items or 0
        failed_items = row.failed_items if row.failed_items else row.failed_records or 0

        # Insert into jobs table
        conn.execute(
            sa.insert(jobs_table).values(
                id=row.id,
                user_id=row.user_id,
                job_type='IMPORT',
                source=source,
                status=status,
                priority='HIGH',
                import_subtype=import_subtype,
                progress_current=progress_current,
                progress_total=progress_total,
                successful_items=successful_items,
                failed_items=failed_items,
                result_summary=result_summary_json,
                error_log=error_log_json,
                error_message=row.error_message,
                file_path=None,
                taskiq_task_id=None,
                created_at=row.created_at,
                started_at=row.started_at,
                completed_at=row.completed_at,
            )
        )


def downgrade() -> None:
    """Remove migrated jobs from jobs table.

    Note: This deletes all jobs with job_type=IMPORT that were migrated.
    The original import_jobs records are NOT affected.
    """
    conn = op.get_bind()

    jobs_table = sa.table(
        'jobs',
        sa.column('id', sa.String),
        sa.column('job_type', sa.String),
    )

    import_jobs_table = sa.table(
        'import_jobs',
        sa.column('id', sa.String),
    )

    # Get all import_jobs IDs
    result = conn.execute(sa.select(import_jobs_table.c.id))
    import_job_ids = [row.id for row in result.fetchall()]

    # Delete jobs that were migrated from import_jobs
    if import_job_ids:
        conn.execute(
            sa.delete(jobs_table).where(
                sa.and_(
                    jobs_table.c.job_type == 'IMPORT',
                    jobs_table.c.id.in_(import_job_ids)
                )
            )
        )
