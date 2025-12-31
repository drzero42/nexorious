# Platform & Storefront Slug-Based ID Refactor

## Overview

Refactor platforms and storefronts to use slug-based primary keys instead of UUIDs. This simplifies the data model and provides stable, predictable identifiers across all instances.

## Design Decisions

| Decision | Choice |
|----------|--------|
| Primary key | `name` (slug) replaces UUID `id` |
| Column naming | `platform_id` → `platform`, `storefront_id` → `storefront` |
| Slug format | Hyphenated lowercase (e.g., `pc-windows`, `steam`) |
| API paths | Slug in URL (e.g., `/platforms/pc-windows`) |
| Migration strategy | Delete all migrations, create fresh initial migration |

## Database Schema

### Platform Table

```sql
CREATE TABLE platforms (
    name VARCHAR PRIMARY KEY,           -- slug, e.g., "pc-windows"
    display_name VARCHAR NOT NULL,
    igdb_id INTEGER,
    icon_url VARCHAR,
    color VARCHAR,
    default_storefront VARCHAR REFERENCES storefronts(name),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### Storefront Table

```sql
CREATE TABLE storefronts (
    name VARCHAR PRIMARY KEY,           -- slug, e.g., "steam"
    display_name VARCHAR NOT NULL,
    icon_url VARCHAR,
    color VARCHAR,
    url_template VARCHAR,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### PlatformStorefront Junction Table

```sql
CREATE TABLE platform_storefronts (
    platform VARCHAR REFERENCES platforms(name),
    storefront VARCHAR REFERENCES storefronts(name),
    PRIMARY KEY (platform, storefront)
);
```

### UserGamePlatform Table

```sql
CREATE TABLE user_game_platforms (
    id UUID PRIMARY KEY,                -- keeps own UUID
    user_game_id UUID REFERENCES user_games(id),
    platform VARCHAR REFERENCES platforms(name),
    storefront VARCHAR REFERENCES storefronts(name),
    -- ... other fields
);
```

## Backend Model Changes

### Platform Model

```python
class Platform(SQLModel, table=True):
    __tablename__ = "platforms"

    name: str = Field(primary_key=True)
    display_name: str
    igdb_id: Optional[int] = Field(default=None, index=True)
    icon_url: Optional[str] = None
    color: Optional[str] = None
    default_storefront: Optional[str] = Field(default=None, foreign_key="storefronts.name")
    created_at: datetime
    updated_at: datetime
```

### Storefront Model

```python
class Storefront(SQLModel, table=True):
    __tablename__ = "storefronts"

    name: str = Field(primary_key=True)
    display_name: str
    icon_url: Optional[str] = None
    color: Optional[str] = None
    url_template: Optional[str] = None
    created_at: datetime
    updated_at: datetime
```

### PlatformStorefront Junction

```python
class PlatformStorefront(SQLModel, table=True):
    __tablename__ = "platform_storefronts"

    platform: str = Field(foreign_key="platforms.name", primary_key=True)
    storefront: str = Field(foreign_key="storefronts.name", primary_key=True)
```

### UserGamePlatform

- Rename `platform_id` → `platform`
- Rename `storefront_id` → `storefront`

## Pydantic Schema Changes

### Platform Schemas

```python
class PlatformResponse(BaseModel):
    name: str
    display_name: str
    igdb_id: Optional[int]
    icon_url: Optional[str]
    color: Optional[str]
    default_storefront: Optional[str]
    created_at: datetime
    updated_at: datetime

class PlatformCreateRequest(BaseModel):
    name: str
    display_name: str
    default_storefront: Optional[str]

class StorefrontResponse(BaseModel):
    name: str
    display_name: str
    icon_url: Optional[str]
    color: Optional[str]
    url_template: Optional[str]
    created_at: datetime
    updated_at: datetime
```

### UserGame Schemas

- Rename `platform_id` → `platform`
- Rename `storefront_id` → `storefront`

### Stats Schemas

- `PlatformUsageStats.platform_id` → `PlatformUsageStats.platform`
- Remove redundant `platform_name` field

## API Route Changes

| Current | New |
|---------|-----|
| `GET /platforms/{platform_id}` | `GET /platforms/{platform}` |
| `GET /platforms/{platform_id}/storefronts` | `GET /platforms/{platform}/storefronts` |
| `POST /platforms/{platform_id}/storefronts/{storefront_id}` | `POST /platforms/{platform}/storefronts/{storefront}` |
| `DELETE /platforms/{platform_id}/storefronts/{storefront_id}` | `DELETE /platforms/{platform}/storefronts/{storefront}` |
| `GET /platforms/storefronts/{storefront_id}` | `GET /platforms/storefronts/{storefront}` |

## Frontend Changes

### TypeScript Types

```typescript
export interface Platform {
  name: string;
  display_name: string;
  igdb_id?: number;
  icon_url?: string;
  color?: string;
  default_storefront?: string;
  created_at: string;
  updated_at: string;
}

export interface Storefront {
  name: string;
  display_name: string;
  icon_url?: string;
  color?: string;
  url_template?: string;
  created_at: string;
  updated_at: string;
}

export interface UserGamePlatform {
  id: string;
  platform?: string;
  storefront?: string;
  // ... other fields
}
```

### API Client

- Update URL paths to use slug: `/platforms/${name}`
- Update field references throughout

## Migration Strategy

1. Delete all existing migrations in `backend/app/alembic/versions/`
2. Create fresh initial migration with new schema
3. Reset development database

## Files to Modify

### Backend

- `app/models/platform.py`
- `app/models/user_game.py`
- `app/schemas/platform.py`
- `app/schemas/user_game.py`
- `app/api/platforms.py`
- `app/seed_data/platforms.py`
- `app/seed_data/storefronts.py`
- `app/alembic/versions/*` (delete all, create fresh)
- `scripts/` (Darkadia CSV converter)
- Related test files

### Frontend

- `src/types/platform.ts`
- `src/types/game.ts`
- `src/api/platforms.ts`
- Related components and tests

## Testing

- Update test fixtures to use `name` as identifier
- Update API tests to use slug paths
- Update assertions from `platform_id` → `platform`
- Update frontend mocks to exclude `id` field
