from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.staticfiles import StaticFiles
import logging
import os
from .core.config import settings
from .core.database import run_alembic_migrations
from .api.auth import router as auth_router
from .api.games import router as games_router
from .api.platforms import router as platforms_router
from .api.user_games import router as user_games_router
from .api.import_api import router as import_router
from .services.batch_session_manager import startup_batch_session_manager, shutdown_batch_session_manager

# Configure logging
def get_log_level(level_str: str) -> int:
    """Convert string log level to logging constant."""
    level_map = {
        "DEBUG": logging.DEBUG,
        "INFO": logging.INFO,
        "WARNING": logging.WARNING,
        "ERROR": logging.ERROR,
        "CRITICAL": logging.CRITICAL
    }
    return level_map.get(level_str.upper(), logging.INFO)

# Set log level based on configuration
log_level = get_log_level(settings.log_level)

logging.basicConfig(
    level=log_level,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)

logger = logging.getLogger(__name__)

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan events for FastAPI app"""
    # Startup
    logger.info("Starting up Nexorious Game Collection Management Service")
    run_alembic_migrations()
    logger.info("Database initialized")
    await startup_batch_session_manager()
    logger.info("Batch session manager initialized")
    yield
    # Shutdown
    logger.info("Shutting down Nexorious Game Collection Management Service")
    await shutdown_batch_session_manager()
    logger.info("Batch session manager shutdown completed")

# Create FastAPI app
app = FastAPI(
    title=settings.app_name,
    version=settings.app_version,
    description="A self-hostable web application for managing personal video game collections",
    docs_url="/docs",
    redoc_url="/redoc",
    lifespan=lifespan
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Include API routers
app.include_router(auth_router, prefix="/api")
app.include_router(games_router, prefix="/api")
app.include_router(platforms_router, prefix="/api")
app.include_router(user_games_router, prefix="/api")
app.include_router(import_router, prefix="/api")

# Mount static files for cover art
if settings.storage_path:
    cover_art_path = os.path.join(settings.storage_path, "cover_art")
    os.makedirs(cover_art_path, exist_ok=True)
    app.mount("/static/cover_art", StaticFiles(directory=cover_art_path), name="cover_art")

# Mount static files for logos
logos_path = "static/logos"
os.makedirs(logos_path, exist_ok=True)
app.mount("/static/logos", StaticFiles(directory=logos_path), name="logos")


@app.get("/")
async def root():
    """Root endpoint with basic app information"""
    return {
        "message": f"Welcome to {settings.app_name}",
        "version": settings.app_version,
        "docs": "/docs",
        "health": "/health"
    }

@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": settings.app_name,
        "version": settings.app_version
    }

# Exception handlers
@app.exception_handler(HTTPException)
async def http_exception_handler(request, exc: HTTPException):
    """Handle HTTP exceptions with consistent JSON response"""
    response = JSONResponse(
        status_code=exc.status_code,
        content={
            "error": exc.detail,
            "status_code": exc.status_code
        }
    )
    
    # Ensure CORS headers are included in error responses
    origin = request.headers.get("origin")
    if origin and origin in settings.cors_origins:
        response.headers["Access-Control-Allow-Origin"] = origin
        response.headers["Access-Control-Allow-Credentials"] = "true"
    
    return response

@app.exception_handler(Exception)
async def internal_server_error_handler(request, exc: Exception):
    """Handle internal server errors"""
    # Don't handle HTTPExceptions here - they should be handled by the HTTPException handler
    if isinstance(exc, HTTPException):
        raise exc
    
    logger.error(f"Internal server error: {exc}")
    response = JSONResponse(
        status_code=500,
        content={
            "error": f"Internal server error: {str(exc)}",
            "status_code": 500
        }
    )
    
    # Ensure CORS headers are included in error responses
    origin = request.headers.get("origin")
    if origin and origin in settings.cors_origins:
        response.headers["Access-Control-Allow-Origin"] = origin
        response.headers["Access-Control-Allow-Credentials"] = "true"
    
    return response

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "app.main:app",
        host="0.0.0.0",
        port=8000,
        reload=settings.debug
    )