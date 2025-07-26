"""Remove email column from users table

Revision ID: 1dd983a1ed58
Revises: 8d2e10413b18
Create Date: 2025-07-26 21:00:27.193012

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '1dd983a1ed58'
down_revision: Union[str, Sequence[str], None] = '8d2e10413b18'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Drop email index and column from users table
    op.drop_index(op.f('ix_users_email'), table_name='users')
    op.drop_column('users', 'email')


def downgrade() -> None:
    """Downgrade schema."""
    # Add email column and index back to users table
    op.add_column('users', sa.Column('email', sa.VARCHAR(), nullable=False))
    op.create_index(op.f('ix_users_email'), 'users', ['email'], unique=1)
