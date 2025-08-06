"""Add steam_appid field to games table

Revision ID: e5f5bd9cc304
Revises: 6487696333e5
Create Date: 2025-08-05 11:09:53.446669

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'e5f5bd9cc304'
down_revision: Union[str, Sequence[str], None] = '6487696333e5'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Add steam_appid field to games table
    op.add_column('games', sa.Column('steam_appid', sa.Integer(), nullable=True))
    
    # Create index on steam_appid for fast lookup during two-phase matching
    op.create_index('ix_games_steam_appid', 'games', ['steam_appid'])


def downgrade() -> None:
    """Downgrade schema."""
    # Drop the index first
    op.drop_index('ix_games_steam_appid', table_name='games')
    
    # Drop the steam_appid column
    op.drop_column('games', 'steam_appid')
