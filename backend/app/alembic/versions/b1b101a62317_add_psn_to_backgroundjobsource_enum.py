"""add PSN to BackgroundJobSource enum

Revision ID: b1b101a62317
Revises: 0c6606d41dc1
Create Date: 2026-01-03 19:13:55.381757

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel


# revision identifiers, used by Alembic.
revision: str = 'b1b101a62317'
down_revision: Union[str, Sequence[str], None] = '0c6606d41dc1'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Add psn to the backgroundjobsource enum type (lowercase to match other values)
    # PostgreSQL requires special handling for enum type alterations
    op.execute("ALTER TYPE backgroundjobsource ADD VALUE IF NOT EXISTS 'psn'")


def downgrade() -> None:
    """Downgrade schema."""
    # Note: PostgreSQL does not support removing enum values
    # This would require recreating the entire enum type and updating all references
    # For now, we leave PSN in the enum even on downgrade
    pass
