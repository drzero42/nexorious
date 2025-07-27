"""seed_platforms_storefronts

Revision ID: a1bbb8869bf3
Revises: d94a790338d9
Create Date: 2025-07-27 19:15:52.222393

"""
from typing import Sequence, Union
import logging

from alembic import op
import sqlalchemy as sa
from sqlmodel import Session

# revision identifiers, used by Alembic.
revision: str = 'a1bbb8869bf3'
down_revision: Union[str, Sequence[str], None] = 'd94a790338d9'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None

logger = logging.getLogger(__name__)


def upgrade() -> None:
    """Seed official platforms and storefronts."""
    try:
        # Import seeding functions
        from nexorious.seed_data.seeder import seed_all_official_data
        
        # Get database connection from alembic
        bind = op.get_bind()
        session = Session(bind)
        
        # Run seeding
        logger.info("Seeding official platforms and storefronts...")
        result = seed_all_official_data(session, version="1.0.0")
        logger.info(f"Seeding completed: {result}")
        
    except Exception as e:
        logger.error(f"Error during seeding: {e}")
        # Don't fail the migration if seeding fails - it can be run manually
        logger.warning("Seeding failed but migration will continue. Seed data can be loaded manually.")


def downgrade() -> None:
    """Remove seeded platforms and storefronts."""
    try:
        # Get database connection from alembic
        bind = op.get_bind()
        
        # Remove official seed data only (preserve custom platforms/storefronts)
        bind.execute(sa.text("DELETE FROM platforms WHERE source = 'official'"))
        bind.execute(sa.text("DELETE FROM storefronts WHERE source = 'official'"))
        logger.info("Removed official seed data")
        
    except Exception as e:
        logger.error(f"Error during downgrade: {e}")
        logger.warning("Failed to remove seed data during downgrade")
