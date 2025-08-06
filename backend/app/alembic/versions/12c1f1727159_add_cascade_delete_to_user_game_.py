"""Add CASCADE DELETE to user_game relationships

Revision ID: 12c1f1727159
Revises: b48c4db8767d
Create Date: 2025-07-31 15:03:42.657566

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '12c1f1727159'
down_revision: Union[str, Sequence[str], None] = 'b48c4db8767d'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # SQLModel cascade_delete=True handles deletion at ORM level
    # No database schema changes needed - the cascade is handled by SQLAlchemy
    pass


def downgrade() -> None:
    """Downgrade schema."""
    # No database schema changes to revert
    pass
