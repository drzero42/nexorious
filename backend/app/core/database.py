from sqlmodel import SQLModel, create_engine, Session
from .config import settings
import logging
import os
from alembic import command
from alembic.config import Config

# Import all models to ensure they are registered with SQLModel
from ..models import (  # noqa: F401
    User,
    UserSession,
    Platform,
    Storefront,
    PlatformStorefront,
    Game,
    UserGame,
    UserGamePlatform,
    Tag,
    UserGameTag,
    Wishlist,
    ImportJob,
    SteamGame,
    DarkadiaGame,
    DarkadiaImport,
)

logger = logging.getLogger(__name__)

# Determine if we're using SQLite or PostgreSQL
is_sqlite = settings.database_url.startswith("sqlite")

if is_sqlite:
    # SQLite setup
    engine = create_engine(
        settings.database_url,
        echo=settings.debug,
        connect_args={"check_same_thread": False}
    )
else:
    # PostgreSQL setup
    engine = create_engine(
        settings.database_url,
        echo=settings.debug,
        pool_pre_ping=True
    )

def create_db_and_tables():
    """Create database tables"""
    SQLModel.metadata.create_all(engine)
    logger.info("Database tables created")

def run_alembic_migrations():
    """Run Alembic database migrations to upgrade to head"""
    try:
        # Get the directory containing this file (nexorious/core/)
        current_dir = os.path.dirname(os.path.abspath(__file__))
        # Navigate to the backend directory (two levels up from nexorious/core/)
        backend_dir = os.path.dirname(os.path.dirname(current_dir))
        # Path to alembic.ini file
        alembic_ini_path = os.path.join(backend_dir, "alembic.ini")
        
        # Check if alembic.ini exists
        if not os.path.exists(alembic_ini_path):
            logger.error(f"Alembic configuration file not found at: {alembic_ini_path}")
            raise FileNotFoundError(f"Alembic configuration file not found at: {alembic_ini_path}")
        
        # Create Alembic configuration
        alembic_cfg = Config(alembic_ini_path)
        
        # Set the database URL in the configuration
        alembic_cfg.set_main_option("sqlalchemy.url", settings.database_url)
        
        logger.info("Starting database migrations...")
        
        # Configure alembic logging
        alembic_cfg.attributes['configure_logger'] = False

        # Run migrations to head
        command.upgrade(alembic_cfg, "head")
        
        logger.info("Database migrations completed successfully")
        
    except Exception as e:
        logger.error(f"Database migration failed: {e}")
        raise

def get_session():
    """Get database session"""
    with Session(engine) as session:
        yield session