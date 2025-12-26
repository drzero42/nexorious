from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field, field_validator
from typing import Optional, Union


class Settings(BaseSettings):
    app_name: str = "Nexorious Game Collection Management"
    app_version: str = "0.1.0"
    debug: bool = False
    log_level: str = Field(
        default="INFO",
        description="Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)"
    )
    
    # Database (PostgreSQL only)
    database_url: str = Field(
        default="postgresql://nexorious:nexorious@localhost:5432/nexorious",
        description="PostgreSQL database URL. Format: postgresql://user:pass@host:port/db"
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

    # NATS JetStream
    NATS_URL: str = "nats://localhost:4222"

    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        env_parse_none_str='none'
    )


settings = Settings()