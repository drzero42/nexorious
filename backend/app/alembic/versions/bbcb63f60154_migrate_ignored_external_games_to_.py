"""Migrate ignored_external_games to external_games

Revision ID: bbcb63f60154
Revises: dc09220836f0
Create Date: 2026-03-01 08:52:31.626313

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy import text


# revision identifiers, used by Alembic.
revision: str = 'bbcb63f60154'
down_revision: Union[str, Sequence[str], None] = 'dc09220836f0'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None

# Mapping from BackgroundJobSource enum values to storefront FK names
SOURCE_TO_STOREFRONT = {
    "steam": "steam",
    "epic": "epic-games-store",
    "gog": "gog",
}


def upgrade() -> None:
    conn = op.get_bind()

    ignored = conn.execute(
        text("SELECT id, user_id, source, external_id, title, created_at FROM ignored_external_games")
    ).fetchall()

    for row in ignored:
        storefront = SOURCE_TO_STOREFRONT.get(row.source)
        if not storefront:
            # PSN or other sources not in ignored table — skip
            continue

        conn.execute(
            text("""
                INSERT INTO external_games
                    (id, user_id, storefront, external_id, title, is_skipped, is_available,
                     is_subscription, playtime_hours, created_at, updated_at)
                VALUES
                    (gen_random_uuid(), :user_id, :storefront, :external_id, :title,
                     true, true, false, 0, :created_at, now())
                ON CONFLICT (user_id, storefront, external_id) DO UPDATE
                    SET is_skipped = true
            """),
            {
                "user_id": row.user_id,
                "storefront": storefront,
                "external_id": row.external_id,
                "title": row.title,
                "created_at": row.created_at,
            }
        )

    op.drop_table("ignored_external_games")


def downgrade() -> None:
    op.create_table(
        "ignored_external_games",
        sa.Column("id", sa.String(), nullable=False),
        sa.Column("user_id", sa.String(), nullable=False),
        sa.Column("source", sa.String(11), nullable=False),
        sa.Column("external_id", sa.String(100), nullable=False),
        sa.Column("title", sa.String(500), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"]),
        sa.UniqueConstraint("user_id", "source", "external_id",
                            name="uq_ignored_external_games_user_source_external"),
    )
