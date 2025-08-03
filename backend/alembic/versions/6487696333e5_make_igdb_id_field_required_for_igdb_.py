"""make igdb_id field required for IGDB-only games

Revision ID: 6487696333e5
Revises: 5992945f2998
Create Date: 2025-08-03 07:00:35.704094

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '6487696333e5'
down_revision: Union[str, Sequence[str], None] = '5992945f2998'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # First, check for any games without IGDB IDs and handle them
    connection = op.get_bind()
    
    # Find games without IGDB IDs
    result = connection.execute(sa.text("SELECT id, title FROM games WHERE igdb_id IS NULL OR igdb_id = ''"))
    games_without_igdb = result.fetchall()
    
    if games_without_igdb:
        # Log warning about games that will be affected
        print(f"WARNING: Found {len(games_without_igdb)} games without IGDB IDs:")
        for game in games_without_igdb:
            print(f"  - Game ID: {game[0]}, Title: {game[1]}")
        
        # For now, we'll assign a placeholder IGDB ID to maintain data integrity
        # In a real-world scenario, these games would need to be manually reviewed
        for game in games_without_igdb:
            placeholder_id = f"manual-{game[0][:8]}"  # Use first 8 chars of game ID
            connection.execute(
                sa.text("UPDATE games SET igdb_id = :igdb_id WHERE id = :game_id"),
                {"igdb_id": placeholder_id, "game_id": game[0]}
            )
            print(f"  - Assigned placeholder IGDB ID '{placeholder_id}' to game '{game[1]}'")
    
    # For SQLite, we need to use batch_alter_table to recreate the table
    with op.batch_alter_table('games', schema=None) as batch_op:
        batch_op.alter_column('igdb_id',
                   existing_type=sa.VARCHAR(length=50),
                   nullable=False)


def downgrade() -> None:
    """Downgrade schema."""
    # For SQLite, we need to use batch_alter_table to recreate the table
    with op.batch_alter_table('games', schema=None) as batch_op:
        batch_op.alter_column('igdb_id',
                   existing_type=sa.VARCHAR(length=50),
                   nullable=True)
