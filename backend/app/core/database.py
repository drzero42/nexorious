from contextlib import asynccontextmanager
from sqlmodel import SQLModel, create_engine, Session
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
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

# PostgreSQL only - validate database URL
if not settings.database_url.startswith("postgresql"):
    raise ValueError(
        f"Invalid database URL: {settings.database_url}. "
        "Only PostgreSQL is supported. Use postgresql://user:pass@host:port/db"
    )

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


def get_engine():
    """Get the database engine."""
    return engine


def get_sync_session() -> Session:
    """Get a synchronous database session for use in background tasks.

    Returns:
        Session: A new SQLModel session.
    """
    return Session(engine)


@asynccontextmanager
async def get_session_context():
    """Async context manager for database sessions in background tasks.

    Usage:
        async with get_session_context() as session:
            # Use session here
            ...

    Note: This uses synchronous SQLModel sessions wrapped in an async context
    manager since taskiq tasks run in a synchronous context.
    """
    session = Session(engine)
    try:
        yield session
    finally:
        session.close()