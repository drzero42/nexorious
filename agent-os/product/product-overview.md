# Product Overview

> Last Updated: 2025-09-11
> Version: 1.0.0

## Vision

Empower knowledgeable gamers to take complete control of their game collections through self-hosted, unified multi-platform tracking that prioritizes data sovereignty and eliminates duplicate purchase frustrations.

## Target Users

**Primary Audience**: Knowledgeable gamers who:
- Own 100+ games distributed across multiple platforms (Steam, Epic, GOG, PlayStation, Xbox, Nintendo, Physical)
- Value data sovereignty and prefer self-hosted solutions over cloud services
- Are comfortable with basic self-hosting setup and maintenance
- Want to avoid duplicate purchases and effectively manage their gaming backlog
- Appreciate functional, practical tools without enterprise complexity

**User Characteristics**:
- Technical comfort with self-hosting applications
- Privacy-conscious and prefer local data control
- Frustrated by platform-specific game libraries that don't provide unified views
- Value KISS (Keep It Simple) approach over feature-heavy alternatives

## Value Proposition

**Complete Data Sovereignty**: Unlike cloud-based game collection tools, Nexorious runs entirely on your infrastructure, ensuring your gaming data remains private and under your complete control.

**IGDB-Powered Professional Experience**: Leverages the comprehensive Internet Game Database (IGDB) for rich metadata, cover art, ratings, and completion time estimates - providing a professional-quality experience without the overhead of maintaining game data internally.

**Unified Multi-Platform Collection**: Seamlessly tracks games across all major platforms and storefronts in a single interface, eliminating the frustration of remembering which platform you own a game on.

**Import-First Design**: Robust CSV and Steam library import with intelligent conflict resolution makes migration from existing systems straightforward and reliable.

## Key Differentiators

1. **Self-Hosting Focus**: Designed specifically for individual deployment, not enterprise or SaaS use cases
2. **IGDB Integration**: Professional-grade metadata without database maintenance overhead  
3. **Multi-Platform Unity**: True cross-platform collection management in a single interface
4. **Practical Simplicity**: KISS and DRY principles - no time tracking, cost estimation, or enterprise bloat
5. **Import Excellence**: Comprehensive import capabilities with conflict resolution and validation
6. **Data Independence**: Complete ownership of your gaming data with local storage

## Current Status

**Development Stage**: Feature-complete application with robust import capabilities
**Current Focus**: Import system enhancements and frontend reliability improvements
**Maturity**: Production-ready for self-hosting with comprehensive test coverage (>80% backend, >70% frontend)

## Core Features

### Collection Management
- Unified game library across all major platforms
- Rich IGDB metadata including ratings, completion times, and cover art
- Personal progress tracking with detailed notes using TipTap rich text editor
- Multi-platform ownership tracking with storefront-specific details

### Import & Integration  
- CSV import from Darkadia format with intelligent conflict resolution
- Steam library import with automatic platform detection and game matching
- Rate-limited IGDB API integration (4 req/s) with automatic retry handling
- Local cover art storage for performance and offline browsing

### Technical Foundation
- FastAPI backend with SQLModel ORM and automatic Alembic migrations
- SvelteKit frontend with TypeScript and responsive Tailwind CSS design
- PostgreSQL (production) and SQLite (development) database support
- JWT authentication with refresh tokens for secure API access
- Comprehensive admin tools for user and system management

## Dependencies

### Critical External Dependency
**IGDB API Access**: REQUIRED - The application cannot function without Internet Game Database (IGDB) API access. All games must have valid IGDB IDs and the application is architecturally dependent on IGDB metadata structure.

### Self-Hosting Requirements
- Python 3.13 with uv package manager
- Database: PostgreSQL recommended, SQLite supported for smaller deployments  
- Network: Internet access required for IGDB API integration
- Storage: Local filesystem access for cover art storage
- Resources: Minimal requirements suitable for personal servers or VPS deployments

## Development Philosophy

**Hobby Project Approach**: Free application for the self-hosting community with focus on practical functionality over enterprise features. No time estimates, cost tracking, or complex project management - just useful tools for gaming enthusiasts who value data sovereignty.