from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field
from typing import Optional


class Settings(BaseSettings):
    app_name: str = "Nexorious Game Collection Management"
    app_version: str = "0.1.0"
    debug: bool = False
    log_level: str = Field(
        default="INFO",
        description="Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)"
    )
    
    # Database
    database_url: str = Field(
        default="sqlite:///./nexorious.db",
        description="Database URL. Use sqlite:///./nexorious.db for SQLite or postgresql://user:pass@localhost/db for PostgreSQL"
    )
    
    # Security
    secret_key: str = Field(
        default="your-secret-key-change-this-in-production",
        description="Secret key for JWT token generation"
    )
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
    refresh_token_expire_days: int = 30
    
    # CORS
    cors_origins: list[str] = Field(
        default=["http://localhost:5173", "http://localhost:3000"],
        description="Allowed CORS origins"
    )
    
    # External APIs
    igdb_client_id: Optional[str] = Field(
        default=None,
        description="IGDB API Client ID (Twitch Client ID)"
    )
    igdb_client_secret: Optional[str] = Field(
        default=None,
        description="IGDB API Client Secret (Twitch Client Secret)"
    )
    igdb_access_token: Optional[str] = Field(
        default=None,
        description="IGDB API Access Token (Twitch Bearer Token)"
    )
    
    # IGDB Rate Limiting
    igdb_requests_per_second: float = Field(
        default=4.0,
        description="IGDB API rate limit in requests per second (default: 4.0)"
    )
    igdb_burst_capacity: int = Field(
        default=8,
        description="IGDB API burst capacity - maximum tokens in bucket (default: 8)"
    )
    igdb_backoff_factor: float = Field(
        default=1.0,
        description="Backoff factor for IGDB API retries (default: 1.0 second)"
    )
    igdb_max_retries: int = Field(
        default=3,
        description="Maximum number of retries for IGDB API calls (default: 3)"
    )
    
    # Storage
    storage_path: Optional[str] = Field(
        default="storage",
        description="Path for local file storage"
    )
    temp_storage_dir: str = Field(
        default="/tmp/nexorious_uploads",
        description="Directory for temporary file uploads and processing"
    )
    
    model_config = SettingsConfigDict(env_file=".env", case_sensitive=False)


settings = Settings()