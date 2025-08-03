# Nexorious Backend API Endpoints Analysis

**Generated:** 2025-08-03  
**Backend Framework:** FastAPI with SQLModel  
**Authentication:** JWT-based with role-based access control  

## Overview

This report provides a comprehensive analysis of all API endpoints in the Nexorious backend, categorized by authentication requirements. The analysis covers 51 total endpoints plus static file serving.

## Authentication Mechanism

The backend uses JWT-based authentication with the following components:

- **Access Tokens**: Short-lived tokens (configurable expiration) for API access
- **Refresh Tokens**: Longer-lived tokens for renewing access tokens
- **Session Management**: Database-tracked sessions with token hashing
- **Role-Based Access**: Regular users vs admin users with different permission levels
- **Security Dependencies**: 
  - `get_current_user()` for authenticated endpoints
  - `get_current_admin_user()` for admin-only endpoints

## 1. Public Endpoints (No Authentication Required)

These endpoints can be accessed without any authentication:

### Core Application
- `GET /` - Root endpoint with basic app information
- `GET /health` - Health check endpoint
- `GET /static/cover_art/{filename}` - Static cover art files

### Authentication & Setup
- `GET /api/auth/setup/status` - Check if initial admin setup is needed
- `POST /api/auth/setup/admin` - Create initial admin user (only when no users exist)
- `POST /api/auth/register` - Register a new user
- `POST /api/auth/login` - User login with JWT token generation
- `POST /api/auth/refresh` - Refresh access token using refresh token
- `POST /api/auth/logout` - User logout (invalidate tokens)
- `GET /api/auth/username/check/{username}` - Check username availability

### Games (Read-Only)
- `GET /api/games/` - List games with filtering and search
- `GET /api/games/{game_id}` - Get specific game details
- `GET /api/games/{game_id}/aliases` - Get game aliases
- `GET /api/games/{game_id}/metadata/status` - Get metadata completeness status

### Platforms (Read-Only)
- `GET /api/platforms/` - List platforms with pagination
- `GET /api/platforms/{platform_id}` - Get specific platform details
- `GET /api/platforms/{platform_id}/storefronts` - Get platform storefronts
- `GET /api/platforms/{platform_id}/default-storefront` - Get platform default storefront
- `GET /api/platforms/storefronts/` - List all storefronts
- `GET /api/platforms/storefronts/{storefront_id}` - Get specific storefront details

**Total Public Endpoints: 18**

## 2. Authenticated Endpoints (Regular Users)

These endpoints require authentication but not admin privileges:

### User Profile Management
- `GET /api/auth/me` - Get current user profile
- `PUT /api/auth/me` - Update user profile
- `PUT /api/auth/change-password` - Change user password
- `PUT /api/auth/username` - Change username

### Game Management
- `POST /api/games/` - Create new game
- `PUT /api/games/{game_id}` - Update game (non-verified games only)
- `POST /api/games/{game_id}/aliases` - Create game alias
- `DELETE /api/games/{game_id}/aliases/{alias_id}` - Delete game alias
- `POST /api/games/search/igdb` - Search IGDB database
- `POST /api/games/igdb-import` - Import game from IGDB
- `POST /api/games/{game_id}/metadata/refresh` - Refresh game metadata (non-verified games)
- `POST /api/games/{game_id}/metadata/populate` - Populate missing metadata (non-verified games)
- `POST /api/games/{game_id}/cover-art/download` - Download cover art (non-verified games)

### User Game Collection
- `GET /api/user-games/` - List user's game collection
- `GET /api/user-games/stats` - Get collection statistics
- `PUT /api/user-games/bulk-update` - Bulk update multiple games
- `DELETE /api/user-games/bulk-delete` - Bulk delete multiple games
- `GET /api/user-games/{user_game_id}` - Get specific user game
- `POST /api/user-games/` - Add game to collection
- `PUT /api/user-games/{user_game_id}` - Update user game entry
- `PUT /api/user-games/{user_game_id}/progress` - Update game progress
- `DELETE /api/user-games/{user_game_id}` - Remove game from collection
- `GET /api/user-games/{user_game_id}/platforms` - Get user game platforms
- `POST /api/user-games/{user_game_id}/platforms` - Add platform to user game
- `PUT /api/user-games/{user_game_id}/platforms/{platform_association_id}` - Update platform association
- `DELETE /api/user-games/{user_game_id}/platforms/{platform_association_id}` - Remove platform association

**Total Authenticated User Endpoints: 23**

## 3. Admin-Only Endpoints

These endpoints require admin privileges:

### User Management
- `POST /api/auth/admin/users` - Create new user (admin)
- `GET /api/auth/admin/users` - List all users
- `GET /api/auth/admin/users/{user_id}` - Get specific user details
- `PUT /api/auth/admin/users/{user_id}` - Update user account
- `PUT /api/auth/admin/users/{user_id}/password` - Reset user password
- `GET /api/auth/admin/users/{user_id}/deletion-impact` - Preview user deletion impact
- `DELETE /api/auth/admin/users/{user_id}` - Delete user account

### Game Management (Admin)
- `DELETE /api/games/{game_id}` - Delete game (admin only)
- `PUT /api/games/{game_id}/verify` - Verify game (mark as verified)
- `POST /api/games/metadata/bulk` - Bulk metadata operations
- `POST /api/games/cover-art/bulk-download` - Bulk cover art download

### Platform & Storefront Management
- `POST /api/platforms/` - Create new platform
- `PUT /api/platforms/{platform_id}` - Update platform
- `DELETE /api/platforms/{platform_id}` - Delete platform
- `POST /api/platforms/{platform_id}/storefronts/{storefront_id}` - Create platform-storefront association
- `DELETE /api/platforms/{platform_id}/storefronts/{storefront_id}` - Remove platform-storefront association
- `PUT /api/platforms/{platform_id}/default-storefront` - Update platform default storefront
- `POST /api/platforms/storefronts/` - Create new storefront
- `PUT /api/platforms/storefronts/{storefront_id}` - Update storefront
- `DELETE /api/platforms/storefronts/{storefront_id}` - Delete storefront
- `GET /api/platforms/stats` - Get platform usage statistics
- `GET /api/platforms/storefronts/stats` - Get storefront usage statistics
- `POST /api/platforms/seed` - Load official platform/storefront seed data

**Total Admin-Only Endpoints: 22**

## Key Security Features

### Permission Model
- **Public Access**: Core information and authentication flows
- **User Access**: Personal collection management and game creation/modification (non-verified only)
- **Admin Access**: System administration, user management, platform management, and game verification

### Game Verification System
- Users can create and modify **unverified** games
- Only admins can **verify** games and modify verified games
- Verified games have stricter edit permissions

### Session Security
- JWT tokens with configurable expiration
- Refresh token rotation
- Session invalidation on password changes
- User agent and IP tracking

## API Documentation

Interactive API documentation is available at:
- **Swagger UI**: `http://localhost:8000/docs`
- **ReDoc**: `http://localhost:8000/redoc`

## Summary Statistics

| Category | Count |
|----------|-------|
| Public Endpoints | 18 |
| Authenticated User Endpoints | 23 |
| Admin-Only Endpoints | 22 |
| **Total API Endpoints** | **63** |

The API follows a clear security model where sensitive operations (user management, platform administration, game verification) are restricted to administrators, while users can manage their own collections and create/modify unverified game entries.

## File Locations

The API endpoints are implemented across these files:
- `/backend/nexorious/main.py` - Main application and core endpoints
- `/backend/nexorious/api/auth.py` - Authentication and user management (48 endpoints)
- `/backend/nexorious/api/games.py` - Game management (19 endpoints)  
- `/backend/nexorious/api/platforms.py` - Platform and storefront management (23 endpoints)
- `/backend/nexorious/api/user_games.py` - User collection management (15 endpoints)
- `/backend/nexorious/core/security.py` - Authentication middleware and dependencies