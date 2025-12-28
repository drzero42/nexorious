"""change user_game_platforms FK to reference platform and storefront name

Revision ID: f7326f86754b
Revises: 257fba8125ad
Create Date: 2025-12-28 12:16:16.029566

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel


# revision identifiers, used by Alembic.
revision: str = 'f7326f86754b'
down_revision: Union[str, Sequence[str], None] = '257fba8125ad'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema.

    Changes the foreign key references in user_game_platforms from
    platforms.id and storefronts.id (UUIDs) to platforms.name and
    storefronts.name (slugs like 'pc-windows', 'steam').

    This allows using human-readable slugs directly without lookups.
    """
    # First, update existing data to convert UUIDs to names
    # This handles any existing records that used UUIDs
    op.execute("""
        UPDATE user_game_platforms ugp
        SET platform_id = p.name
        FROM platforms p
        WHERE ugp.platform_id = p.id
    """)

    op.execute("""
        UPDATE user_game_platforms ugp
        SET storefront_id = s.name
        FROM storefronts s
        WHERE ugp.storefront_id = s.id
    """)

    # Drop old FK constraints
    op.drop_constraint('user_game_platforms_platform_id_fkey', 'user_game_platforms', type_='foreignkey')
    op.drop_constraint('user_game_platforms_storefront_id_fkey', 'user_game_platforms', type_='foreignkey')

    # Create new FK constraints referencing name columns
    op.create_foreign_key(
        'user_game_platforms_platform_id_fkey',
        'user_game_platforms', 'platforms',
        ['platform_id'], ['name']
    )
    op.create_foreign_key(
        'user_game_platforms_storefront_id_fkey',
        'user_game_platforms', 'storefronts',
        ['storefront_id'], ['name']
    )


def downgrade() -> None:
    """Downgrade schema.

    Reverts foreign key references back to id columns (UUIDs).
    """
    # First, update existing data to convert names back to UUIDs
    op.execute("""
        UPDATE user_game_platforms ugp
        SET platform_id = p.id
        FROM platforms p
        WHERE ugp.platform_id = p.name
    """)

    op.execute("""
        UPDATE user_game_platforms ugp
        SET storefront_id = s.id
        FROM storefronts s
        WHERE ugp.storefront_id = s.name
    """)

    # Drop new FK constraints
    op.drop_constraint('user_game_platforms_platform_id_fkey', 'user_game_platforms', type_='foreignkey')
    op.drop_constraint('user_game_platforms_storefront_id_fkey', 'user_game_platforms', type_='foreignkey')

    # Recreate old FK constraints referencing id columns
    op.create_foreign_key(
        'user_game_platforms_platform_id_fkey',
        'user_game_platforms', 'platforms',
        ['platform_id'], ['id']
    )
    op.create_foreign_key(
        'user_game_platforms_storefront_id_fkey',
        'user_game_platforms', 'storefronts',
        ['storefront_id'], ['id']
    )
