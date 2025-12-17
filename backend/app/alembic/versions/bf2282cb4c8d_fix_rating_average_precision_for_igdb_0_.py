"""fix rating_average precision for IGDB 0-100 scale

Revision ID: bf2282cb4c8d
Revises: 33f832604949
Create Date: 2025-12-17 10:26:35.891077

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision: str = 'bf2282cb4c8d'
down_revision: Union[str, Sequence[str], None] = '33f832604949'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Fix rating_average precision: IGDB ratings are 0-100, not 0-10
    # Previous: Numeric(3,2) = max 9.99
    # New: Numeric(5,2) = max 999.99 (supports 0-100 with decimals)
    op.alter_column('games', 'rating_average',
               existing_type=sa.NUMERIC(precision=3, scale=2),
               type_=sa.Numeric(precision=5, scale=2),
               existing_nullable=True)


def downgrade() -> None:
    """Downgrade schema."""
    # Revert to original precision (will truncate values > 9.99)
    op.alter_column('games', 'rating_average',
               existing_type=sa.Numeric(precision=5, scale=2),
               type_=sa.NUMERIC(precision=3, scale=2),
               existing_nullable=True)
