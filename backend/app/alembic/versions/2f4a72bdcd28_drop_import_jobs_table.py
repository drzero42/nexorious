"""drop import_jobs table

Revision ID: 2f4a72bdcd28
Revises: d2c65f4979e4
Create Date: 2025-12-16 18:28:11.552622

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

# revision identifiers, used by Alembic.
revision: str = '2f4a72bdcd28'
down_revision: Union[str, Sequence[str], None] = 'd2c65f4979e4'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Drop the import_jobs table.

    All data has been migrated to the unified jobs table in the previous migration.
    """
    # Drop indexes first
    op.drop_index(op.f('ix_import_jobs_source'), table_name='import_jobs')
    op.drop_index(op.f('ix_import_jobs_status'), table_name='import_jobs')
    op.drop_index(op.f('ix_import_jobs_user_id'), table_name='import_jobs')

    # Drop the table
    op.drop_table('import_jobs')

    # Drop the enum types that were only used by import_jobs
    op.execute('DROP TYPE IF EXISTS importtype')
    op.execute('DROP TYPE IF EXISTS importstatus')
    op.execute('DROP TYPE IF EXISTS jobtype')


def downgrade() -> None:
    """Recreate the import_jobs table.

    Note: This only recreates the table structure. Data will NOT be restored.
    Use the data migration's downgrade to remove migrated jobs, then manually
    restore import_jobs data if needed.
    """
    # Recreate enum types
    importtype = postgresql.ENUM(
        'CSV', 'STEAM', 'EPIC', 'GOG', 'XBOX', 'PLAYSTATION', 'DARKADIA',
        name='importtype'
    )
    importtype.create(op.get_bind(), checkfirst=True)

    importstatus = postgresql.ENUM(
        'PENDING', 'PROCESSING', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED',
        name='importstatus'
    )
    importstatus.create(op.get_bind(), checkfirst=True)

    jobtype = postgresql.ENUM(
        'LIBRARY_IMPORT', 'AUTO_MATCH', 'BULK_SYNC', 'BULK_UNMATCH', 'BULK_UNSYNC', 'BULK_UNIGNORE',
        name='jobtype'
    )
    jobtype.create(op.get_bind(), checkfirst=True)

    # Recreate the table
    op.create_table('import_jobs',
        sa.Column('id', sa.VARCHAR(), autoincrement=False, nullable=False),
        sa.Column('user_id', sa.VARCHAR(), autoincrement=False, nullable=False),
        sa.Column('import_type', importtype, autoincrement=False, nullable=False),
        sa.Column('status', importstatus, autoincrement=False, nullable=False),
        sa.Column('total_records', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('processed_records', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('failed_records', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('error_log', sa.VARCHAR(), autoincrement=False, nullable=False),
        sa.Column('job_metadata', sa.VARCHAR(), autoincrement=False, nullable=False),
        sa.Column('job_type', jobtype, autoincrement=False, nullable=True),
        sa.Column('source', sa.VARCHAR(), autoincrement=False, nullable=True),
        sa.Column('started_at', postgresql.TIMESTAMP(), autoincrement=False, nullable=True),
        sa.Column('progress', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('total_items', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('processed_items', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('successful_items', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('failed_items', sa.INTEGER(), autoincrement=False, nullable=False),
        sa.Column('error_message', sa.VARCHAR(), autoincrement=False, nullable=True),
        sa.Column('created_at', postgresql.TIMESTAMP(), autoincrement=False, nullable=False),
        sa.Column('completed_at', postgresql.TIMESTAMP(), autoincrement=False, nullable=True),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], name='import_jobs_user_id_fkey'),
        sa.PrimaryKeyConstraint('id', name='import_jobs_pkey')
    )

    # Recreate indexes
    op.create_index('ix_import_jobs_user_id', 'import_jobs', ['user_id'], unique=False)
    op.create_index('ix_import_jobs_status', 'import_jobs', ['status'], unique=False)
    op.create_index('ix_import_jobs_source', 'import_jobs', ['source'], unique=False)
