"""move ownership status and acquired date to platform

Revision ID: 16859bc970d4
Revises: f772653ec5a4
Create Date: 2026-01-09 12:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '16859bc970d4'
down_revision: Union[str, Sequence[str], None] = 'f772653ec5a4'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Add new columns to user_game_platforms
    op.add_column('user_game_platforms', sa.Column('ownership_status', sa.Enum('OWNED', 'BORROWED', 'RENTED', 'SUBSCRIPTION', 'NO_LONGER_OWNED', name='ownershipstatus', create_type=False), nullable=False, server_default='OWNED'))
    op.add_column('user_game_platforms', sa.Column('acquired_date', sa.Date(), nullable=True))

    # Copy ownership_status and acquired_date from user_games to all related user_game_platforms
    op.execute("""
        UPDATE user_game_platforms ugp
        SET ownership_status = ug.ownership_status,
            acquired_date = ug.acquired_date
        FROM user_games ug
        WHERE ugp.user_game_id = ug.id
    """)

    # Remove server default after data migration
    op.alter_column('user_game_platforms', 'ownership_status', server_default=None)

    # Drop columns from user_games
    op.drop_column('user_games', 'ownership_status')
    op.drop_column('user_games', 'acquired_date')


def downgrade() -> None:
    """Downgrade schema."""
    # Add columns back to user_games
    op.add_column('user_games', sa.Column('ownership_status', sa.Enum('OWNED', 'BORROWED', 'RENTED', 'SUBSCRIPTION', 'NO_LONGER_OWNED', name='ownershipstatus'), nullable=False, server_default='OWNED'))
    op.add_column('user_games', sa.Column('acquired_date', sa.Date(), nullable=True))

    # Copy back from first platform association (best effort for downgrade)
    op.execute("""
        UPDATE user_games ug
        SET ownership_status = COALESCE(
            (SELECT ugp.ownership_status FROM user_game_platforms ugp WHERE ugp.user_game_id = ug.id ORDER BY ugp.created_at ASC LIMIT 1),
            'OWNED'
        ),
        acquired_date = (
            SELECT ugp.acquired_date FROM user_game_platforms ugp WHERE ugp.user_game_id = ug.id ORDER BY ugp.created_at ASC LIMIT 1
        )
    """)

    # Remove server default
    op.alter_column('user_games', 'ownership_status', server_default=None)

    # Drop columns from user_game_platforms
    op.drop_column('user_game_platforms', 'acquired_date')
    op.drop_column('user_game_platforms', 'ownership_status')
