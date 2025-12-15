from contextlib import asynccontextmanager
from typing import Optional
from sqlmodel import SQLModel, create_engine, Session
from sqlalchemy.engine import Engine
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
from app.models.ignored_external_game import IgnoredExternalGame  # noqa: F401

logger = logging.getLogger(__name__)

# Lazy-initialized database engine
_engine: Optional[Engine] = None

# Flag to skip migrations (used in tests)
_skip_migrations = False


def get_engine():
    """Get or create the database engine (lazy initialization).

    The engine is created on first use rather than at module import time.
    This allows tests to inject their own engine before the app tries to
    connect to the configured database URL.
    """
    global _engine
    if _engine is None:
        if not settings.database_url.startswith("postgresql"):
            raise ValueError(
                f"Invalid database URL: {settings.database_url}. "
                "Only PostgreSQL is supported. Use postgresql://user:pass@host:port/db"
            )
        _engine = create_engine(
            settings.database_url,
            echo=settings.debug,
            pool_pre_ping=True
        )
    return _engine


def _reset_engine():
    """Reset the engine for testing. Not for production use."""
    global _engine
    _engine = None


def _set_skip_migrations(skip: bool):
    """Set flag to skip migrations. Used in tests."""
    global _skip_migrations
    _skip_migrations = skip

def create_db_and_tables():
    """Create database tables"""
    SQLModel.metadata.create_all(get_engine())
    logger.info("Database tables created")

def run_alembic_migrations():
    """Run Alembic database migrations to upgrade to head"""
    if _skip_migrations:
        logger.info("Skipping database migrations (test mode)")
        return

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
    with Session(get_engine()) as session:
        yield session


def get_sync_session() -> Session:
    """Get a synchronous database session for use in background tasks.

    Returns:
        Session: A new SQLModel session.
    """
    return Session(get_engine())


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
    session = Session(get_engine())
    try:
        yield session
    finally:
        session.close()