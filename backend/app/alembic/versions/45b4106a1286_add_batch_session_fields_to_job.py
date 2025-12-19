"""add_batch_session_fields_to_job

Revision ID: 45b4106a1286
Revises: bf2282cb4c8d
Create Date: 2025-12-19 17:31:29.471702

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel.sql.sqltypes

# revision identifiers, used by Alembic.
revision: str = '45b4106a1286'
down_revision: Union[str, Sequence[str], None] = 'bf2282cb4c8d'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Add batch session tracking fields to jobs table
    op.add_column('jobs', sa.Column('processed_item_ids', sqlmodel.sql.sqltypes.AutoString(), nullable=False, server_default='[]'))
    op.add_column('jobs', sa.Column('failed_item_ids', sqlmodel.sql.sqltypes.AutoString(), nullable=False, server_default='[]'))


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_column('jobs', 'failed_item_ids')
    op.drop_column('jobs', 'processed_item_ids')
