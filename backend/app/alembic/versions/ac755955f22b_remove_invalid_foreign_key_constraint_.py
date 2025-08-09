"""Remove invalid foreign key constraint from steam_games.igdb_id

Revision ID: ac755955f22b
Revises: 2c7b6e5676cf
Create Date: 2025-08-09 18:50:31.919730

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'ac755955f22b'
down_revision: Union[str, Sequence[str], None] = '2c7b6e5676cf'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # This migration documents the removal of the invalid foreign key constraint
    # from SteamGame.igdb_id. Since SQLite foreign keys aren't enforced in this
    # environment, no actual database changes are needed.
    # The model definition has been corrected to remove the constraint.
    pass


def downgrade() -> None:
    """Downgrade schema."""
    # This would restore the invalid foreign key constraint that was causing issues.
    # Since SQLite foreign keys aren't enforced, this is a no-op.
    pass
