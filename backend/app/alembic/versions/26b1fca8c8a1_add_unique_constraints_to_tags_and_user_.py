"""add unique constraints to tags and user_game_tags tables

Revision ID: 26b1fca8c8a1
Revises: 22921f6a27d9
Create Date: 2025-08-18 09:57:15.660331

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '26b1fca8c8a1'
down_revision: Union[str, Sequence[str], None] = '22921f6a27d9'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Add unique constraint to tags table (user_id, name)
    with op.batch_alter_table('tags') as batch_op:
        batch_op.create_unique_constraint('uq_tag_user_name', ['user_id', 'name'])
    
    # Add unique constraint to user_game_tags table (user_game_id, tag_id)
    with op.batch_alter_table('user_game_tags') as batch_op:
        batch_op.create_unique_constraint('uq_user_game_tag', ['user_game_id', 'tag_id'])


def downgrade() -> None:
    """Downgrade schema."""
    # Drop unique constraint from user_game_tags table
    with op.batch_alter_table('user_game_tags') as batch_op:
        batch_op.drop_constraint('uq_user_game_tag', type_='unique')
    
    # Drop unique constraint from tags table
    with op.batch_alter_table('tags') as batch_op:
        batch_op.drop_constraint('uq_tag_user_name', type_='unique')
