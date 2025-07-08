from pydantic_settings import BaseSettings
from pydantic import Field, ConfigDict
from typing import Optional


class Settings(BaseSettings):
    app_name: str = "Nexorious Game Collection Management"
    app_version: str = "0.1.0"
    debug: bool = False
    
    # Database
    database_url: Optional[str] = Field(
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
        description="IGDB API Client ID"
    )
    igdb_client_secret: Optional[str] = Field(
        default=None,
        description="IGDB API Client Secret"
    )
    
    model_config = ConfigDict(env_file=".env", case_sensitive=False)


settings = Settings()