# Nexorious Backend

FastAPI backend for the Nexorious Game Collection Management Service.

## Setup

1. Create a virtual environment and install dependencies:
```bash
uv sync
```

2. Copy the environment file and configure it:
```bash
cp .env.example .env
```

3. Run database migrations:
```bash
uv run alembic upgrade head
```

4. Start the development server:
```bash
uv run python -m nexorious.main
```

Or use uvicorn directly:
```bash
uv run uvicorn nexorious.main:app --reload
```

## API Documentation

Once the server is running, you can access:
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc
- Health check: http://localhost:8000/health

## Testing

Run the test suite:
```bash
uv run pytest
```

## Database

The application supports both SQLite and PostgreSQL databases. Configure the `DATABASE_URL` in your `.env` file:

- SQLite: `sqlite:///./nexorious.db`
- PostgreSQL: `postgresql://username:password@localhost:5432/nexorious`

## Environment Variables

See `.env.example` for all available configuration options.