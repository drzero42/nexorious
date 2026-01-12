"""migrate ignored_external_games to external_games

Revision ID: c860aae6109b
Revises: b11eb1881c9a
Create Date: 2026-01-12 16:53:21.922658

"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa
from datetime import datetime, timezone


# revision identifiers, used by Alembic.
revision: str = 'c860aae6109b'
down_revision: Union[str, Sequence[str], None] = 'b11eb1881c9a'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Migrate data from ignored_external_games to external_games and drop the old table."""
    # Map BackgroundJobSource to storefront names
    source_to_storefront = {
        'STEAM': 'steam',
        'EPIC': 'epic',
        'GOG': 'gog',
        'PSN': 'psn',
    }

    conn = op.get_bind()

    # Get all ignored external games
    ignored_games = conn.execute(
        sa.text("SELECT id, user_id, source, external_id, title, created_at FROM ignored_external_games")
    ).fetchall()

    # Insert into external_games
    for game in ignored_games:
        storefront = source_to_storefront.get(game.source, game.source.lower())
        conn.execute(
            sa.text("""
                INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, created_at, updated_at)
                VALUES (:id, :user_id, :storefront, :external_id, :title, true, true, false, 0, :created_at, :updated_at)
                ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET is_skipped = true
            """),
            {
                "id": game.id,
                "user_id": game.user_id,
                "storefront": storefront,
                "external_id": game.external_id,
                "title": game.title,
                "created_at": game.created_at,
                "updated_at": datetime.now(timezone.utc),
            }
        )

    # Drop ignored_external_games table
    op.drop_table('ignored_external_games')


def downgrade() -> None:
    """Recreate ignored_external_games table and migrate data back."""
    # Recreate ignored_external_games table
    op.create_table(
        'ignored_external_games',
        sa.Column('id', sa.String(), nullable=False),
        sa.Column('user_id', sa.String(), nullable=False),
        sa.Column('source', sa.String(), nullable=False),
        sa.Column('external_id', sa.String(100), nullable=False),
        sa.Column('title', sa.String(500), nullable=False),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id']),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('user_id', 'source', 'external_id', name='uq_ignored_external_games_user_source_external'),
    )

    # Migrate data back (skipped external_games -> ignored_external_games)
    storefront_to_source = {
        'steam': 'STEAM',
        'epic': 'EPIC',
        'gog': 'GOG',
        'psn': 'PSN',
    }

    conn = op.get_bind()
    skipped_games = conn.execute(
        sa.text("SELECT id, user_id, storefront, external_id, title, created_at FROM external_games WHERE is_skipped = true")
    ).fetchall()

    for game in skipped_games:
        source = storefront_to_source.get(game.storefront, game.storefront.upper())
        conn.execute(
            sa.text("""
                INSERT INTO ignored_external_games (id, user_id, source, external_id, title, created_at)
                VALUES (:id, :user_id, :source, :external_id, :title, :created_at)
            """),
            {
                "id": game.id,
                "user_id": game.user_id,
                "source": source,
                "external_id": game.external_id,
                "title": game.title,
                "created_at": game.created_at,
            }
        )
