"""Add NO_LONGER_OWNED ownership status

Revision ID: 1c960000d15e
Revises: 68af15b2b34b
Create Date: 2025-07-30 09:08:55.551348

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '1c960000d15e'
down_revision: Union[str, Sequence[str], None] = '68af15b2b34b'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # SQLite doesn't support ALTER COLUMN for changing enum types
    # We need to recreate the table with the new enum values
    
    # For SQLite, we'll use a simpler approach since it stores enums as strings anyway
    # The new enum value will work automatically as it's just a string constraint
    # No actual schema change is needed for SQLite
    pass


def downgrade() -> None:
    """Downgrade schema."""
    # For SQLite, no schema change was made, so no downgrade needed
    # The constraint is handled at the application level
    pass
