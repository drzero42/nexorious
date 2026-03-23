from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field, field_validator, model_validator
from typing import Optional, Union
from urllib.parse import quote as urlquote


class Settings(BaseSettings):
    app_name: str = "Nexorious Game Collection Management"
    app_version: str = "0.1.0"
    debug: bool = False
    log_level: str = Field(
        default="INFO",
        description="Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)"
    )
    
    # Database (PostgreSQL only)
    # Individual vars are used to construct the URL when DATABASE_URL is not set.
    db_host: str = Field(default="localhost", description="PostgreSQL host")
    db_port: int = Field(default=5432, description="PostgreSQL port")
    db_user: str = Field(default="nexorious", description="PostgreSQL username")
    db_password: str = Field(default="nexorious", description="PostgreSQL password")
    db_name: str = Field(default="nexorious", description="PostgreSQL database name")

    database_url: str = Field(
        default="",
        description=(
            "PostgreSQL database URL. If set (non-empty), takes priority over individual "
            "DB_* vars. Format: postgresql://user:pass@host:port/db"
        )
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
    cors_origins: Union[str, list[str]] = Field(
        default=["http://localhost:5173", "http://localhost:3000"],
        description="Allowed CORS origins (comma-separated string or list)"
    )

    @field_validator('cors_origins', mode='after')
    @classmethod
    def parse_cors_origins(cls, v):
        """Parse CORS_ORIGINS from comma-separated string if provided as string"""
        if isinstance(v, str):
            return [origin.strip() for origin in v.split(',') if origin.strip()]
        elif isinstance(v, list):
            return v
        else:
            return ["http://localhost:5173", "http://localhost:3000"]

    @model_validator(mode='after')
    def resolve_database_url(self) -> 'Settings':
        """Construct database_url from individual DB vars when DATABASE_URL is not set."""
        if not self.database_url:
            user = urlquote(self.db_user, safe='')
            password = urlquote(self.db_password, safe='')
            dbname = urlquote(self.db_name, safe='')
            self.database_url = (
                f"postgresql://{user}:{password}"
                f"@{self.db_host}:{self.db_port}/{dbname}"
            )
        return self

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

    # Steam Rate Limiting
    steam_requests_per_second: float = Field(
        default=4.0,
        description="Steam API rate limit in requests per second (default: 4.0)"
    )
    steam_burst_capacity: int = Field(
        default=10,
        description="Steam API burst capacity - maximum tokens in bucket (default: 10)"
    )

    # Distributed Rate Limiting
    rate_limiter_nats_bucket: str = Field(
        default="rate-limiters",
        description="NATS KV bucket name for distributed rate limiting"
    )
    rate_limiter_cas_max_retries: int = Field(
        default=10,
        description="Maximum CAS retry attempts for distributed rate limiter"
    )
    rate_limiter_cas_retry_base_ms: int = Field(
        default=5,
        description="Base delay in ms for CAS retry jitter"
    )
    rate_limiter_cas_retry_max_ms: int = Field(
        default=50,
        description="Maximum delay in ms for CAS retry jitter"
    )

    # Scheduler Connection Resilience
    scheduler_reconnect_initial_delay: float = Field(
        default=5.0,
        description="Initial delay in seconds before reconnection attempt"
    )
    scheduler_reconnect_max_delay: float = Field(
        default=60.0,
        description="Maximum delay in seconds between reconnection attempts"
    )
    scheduler_reconnect_backoff_multiplier: float = Field(
        default=2.0,
        description="Multiplier for exponential backoff between reconnection attempts"
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
    backup_path: str = Field(
        default="storage/backups",
        description="Path for backup file storage"
    )

    # Internal API (worker-to-API communication)
    internal_api_key: str = Field(
        default="nexorious-internal-api-key-change-in-production",
        description="Secret key for internal worker-to-API communication"
    )
    internal_api_url: str = Field(
        default="http://api:8000",
        description="Internal URL for API (used by worker for callbacks)"
    )

    # NATS JetStream
    NATS_URL: str = "nats://localhost:4222"

    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        env_parse_none_str='none'
    )


settings = Settings()