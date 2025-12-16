"""add unique constraint to review_items job_id source_title

Revision ID: 33f832604949
Revises: 2f4a72bdcd28
Create Date: 2025-12-16 20:00:16.376588

"""
from typing import Sequence, Union

from alembic import op
import sqlmodel

# revision identifiers, used by Alembic.
revision: str = '33f832604949'
down_revision: Union[str, Sequence[str], None] = '2f4a72bdcd28'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    with op.batch_alter_table("review_items", schema=None) as batch_op:
        batch_op.create_unique_constraint(
            'uq_review_items_job_source_title', ['job_id', 'source_title']
        )


def downgrade() -> None:
    """Downgrade schema."""
    with op.batch_alter_table("review_items", schema=None) as batch_op:
        batch_op.drop_constraint('uq_review_items_job_source_title', type_='unique')
