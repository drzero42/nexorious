"""add import job feature parity fields to job model

Revision ID: 44eb16d3a604
Revises: 77a2404c73d8
Create Date: 2025-12-16 17:33:58.402891

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel

# revision identifiers, used by Alembic.
revision: str = '44eb16d3a604'
down_revision: Union[str, Sequence[str], None] = '77a2404c73d8'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Create the enum type first
    importjobsubtype = sa.Enum(
        'LIBRARY_IMPORT', 'AUTO_MATCH', 'BULK_SYNC', 'BULK_UNMATCH', 'BULK_UNSYNC', 'BULK_UNIGNORE',
        name='importjobsubtype'
    )
    importjobsubtype.create(op.get_bind(), checkfirst=True)

    # Add new fields to jobs table for ImportJob feature parity
    with op.batch_alter_table('jobs', schema=None) as batch_op:
        batch_op.add_column(sa.Column('import_subtype', importjobsubtype, nullable=True))
        batch_op.add_column(sa.Column('successful_items', sa.Integer(), nullable=False, server_default='0'))
        batch_op.add_column(sa.Column('failed_items', sa.Integer(), nullable=False, server_default='0'))
        batch_op.add_column(sa.Column('error_log', sqlmodel.sql.sqltypes.AutoString(), nullable=False, server_default='[]'))
        batch_op.create_index(op.f('ix_jobs_import_subtype'), ['import_subtype'], unique=False)


def downgrade() -> None:
    """Downgrade schema."""
    with op.batch_alter_table('jobs', schema=None) as batch_op:
        batch_op.drop_index(op.f('ix_jobs_import_subtype'))
        batch_op.drop_column('error_log')
        batch_op.drop_column('failed_items')
        batch_op.drop_column('successful_items')
        batch_op.drop_column('import_subtype')

    # Drop the enum type
    op.execute('DROP TYPE IF EXISTS importjobsubtype')
