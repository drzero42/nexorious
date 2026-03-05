# backend/Dockerfile
FROM ghcr.io/astral-sh/uv:python3.13-bookworm-slim

WORKDIR /app

# Install PostgreSQL 18 client tools for backup/restore functionality
# Add PostgreSQL APT repository for version 18 (Debian bookworm has v15 by default)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg curl ca-certificates \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends postgresql-client-18 \
    && rm -rf /var/lib/apt/lists/*

# Enable bytecode compilation for faster startup
ENV UV_COMPILE_BYTECODE=1

# Use copy mode for linking (hardlinks don't work across container filesystem layers)
ENV UV_LINK_MODE=copy

# Copy dependency files first (better layer caching)
COPY pyproject.toml uv.lock ./

# Install dependencies (without the project itself)
RUN uv sync --frozen --no-install-project

# Copy source code
COPY . .

# Install the project
RUN uv sync --frozen

# Run with uvicorn (--reload enabled via compose command override)
CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
