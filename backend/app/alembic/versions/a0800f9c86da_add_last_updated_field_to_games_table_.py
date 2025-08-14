"""add last_updated field to games table for IGDB metadata refresh tracking

Revision ID: a0800f9c86da
Revises: ac755955f22b
Create Date: 2025-08-13 21:31:23.035244

"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa
from sqlalchemy import inspect
import logging

# Configure logging for migration operations
logger = logging.getLogger(__name__)


# revision identifiers, used by Alembic.
revision: str = 'a0800f9c86da'
down_revision: Union[str, Sequence[str], None] = 'ac755955f22b'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema with database-specific handling."""
    
    # Get database dialect for conditional logic
    bind = op.get_bind()
    dialect_name = bind.dialect.name
    
    logger.info(f"Running migration on {dialect_name} database")
    
    # Check if column already exists before adding it
    inspector = inspect(bind)
    columns = [col['name'] for col in inspector.get_columns('games')]
    
    if 'last_updated' not in columns:
        # Add the new column (works on all databases)
        op.add_column('games', sa.Column('last_updated', sa.DateTime(), nullable=True))
        logger.info("Added 'last_updated' column to games table")
    else:
        logger.info("Column 'last_updated' already exists in games table, skipping addition")
    
    # Handle constraint operations based on database type
    if dialect_name == 'postgresql':
        # PostgreSQL: Properly handle foreign key constraints
        try:
            inspector = inspect(bind)
            foreign_keys = inspector.get_foreign_keys('steam_games')
            
            # Look for the invalid constraint
            # (igdb_id should not have a foreign key to games.id)
            for fk in foreign_keys:
                if (fk.get('constrained_columns') == ['igdb_id'] and 
                    fk.get('referred_table') == 'games' and
                    fk.get('referred_columns') == ['id']):
                    
                    constraint_name = fk.get('name')
                    if constraint_name:
                        op.drop_constraint(constraint_name, 'steam_games', type_='foreignkey')
                        logger.info(f"Dropped invalid foreign key constraint: {constraint_name}")
                    break
            else:
                logger.info("Invalid igdb_id foreign key constraint not found (may already be removed)")
                
        except Exception as e:
            logger.error(f"Error handling PostgreSQL constraints: {e}")
            # Re-raise to prevent silent failures
            raise
            
    elif dialect_name == 'sqlite':
        # SQLite: Foreign keys aren't enforced by default
        # Document this as an intentional no-op
        logger.info("SQLite detected: Skipping foreign key constraint operations")
        logger.info("Note: SQLite does not enforce foreign keys by default")
        
    else:
        # Fail safely for unsupported databases
        raise ValueError(f"Unsupported database dialect: {dialect_name}")


def downgrade() -> None:
    """Downgrade schema with database-specific handling."""
    
    bind = op.get_bind()
    dialect_name = bind.dialect.name
    
    logger.info(f"Running downgrade on {dialect_name} database")
    
    # Recreate the constraint only on PostgreSQL
    # (though this constraint is actually invalid and shouldn't exist)
    if dialect_name == 'postgresql':
        # WARNING: This recreates an invalid constraint for rollback compatibility
        # igdb_id should NOT reference games.id
        op.create_foreign_key(
            'steam_games_igdb_id_fkey',  # Explicit constraint name
            'steam_games', 
            'games', 
            ['igdb_id'], 
            ['id']
        )
        logger.warning("Recreated invalid foreign key constraint for rollback compatibility")
        
    elif dialect_name == 'sqlite':
        logger.info("SQLite: Skipping foreign key constraint recreation")
    
    # Remove the column (works on all databases)
    op.drop_column('games', 'last_updated')
    logger.info("Dropped 'last_updated' column from games table")
