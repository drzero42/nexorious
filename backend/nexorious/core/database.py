from sqlmodel import SQLModel, create_engine, Session
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from .config import settings
import logging

# Import all models to ensure they are registered with SQLModel
from ..models import *

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

def get_session():
    """Get database session"""
    with Session(engine) as session:
        yield session