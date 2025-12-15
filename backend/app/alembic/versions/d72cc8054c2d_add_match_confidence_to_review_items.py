"""add match_confidence to review_items

Revision ID: d72cc8054c2d
Revises: 6508079ee686
Create Date: 2025-12-15 18:52:10.976230

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision: str = 'd72cc8054c2d'
down_revision: Union[str, Sequence[str], None] = '6508079ee686'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    op.add_column('review_items', sa.Column('match_confidence', sa.Float(), nullable=True))


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_column('review_items', 'match_confidence')
