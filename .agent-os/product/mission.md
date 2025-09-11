# Product Mission

> Last Updated: 2025-09-11
> Version: 1.0.0

## Pitch

Nexorious is a self-hosted game collection management application that empowers knowledgeable gamers to take control of their digital game libraries. By providing complete data sovereignty and unified tracking across multiple platforms, Nexorious eliminates the frustration of forgetting which games you already own and helps you make informed decisions about your gaming backlog.

## Users

**Primary Users**: Knowledgeable gamers who:
- Own games across multiple platforms and storefronts (Steam, Epic, GOG, PlayStation, Xbox, Nintendo, Physical media)
- Value data sovereignty and prefer self-hosted solutions over cloud-based services
- Are comfortable with self-hosting applications and managing their own infrastructure
- Want unified visibility into their entire game collection regardless of platform
- Need to avoid duplicate purchases and manage their gaming backlog effectively

**User Characteristics**:
- Tech-savvy enough to handle self-hosting setup and maintenance
- Own 100+ games distributed across multiple platforms
- Frustrated by platform-specific libraries that don't provide unified views
- Value privacy and control over their gaming data
- Appreciate simple, functional tools over enterprise-grade complexity

## The Problem

Modern gamers face significant challenges managing their game collections:

1. **Platform Fragmentation**: Games are scattered across Steam, Epic Games Store, GOG, PlayStation, Xbox, Nintendo Switch, and physical media, making it impossible to see your complete collection in one place.

2. **Duplicate Purchase Risk**: Without unified visibility, gamers frequently purchase games they already own on different platforms, wasting money and creating frustration.

3. **Backlog Management**: With hundreds of games across multiple platforms, it's difficult to track what you've played, want to play, or have completed.

4. **Data Lock-in**: Platform-specific libraries trap your gaming data in proprietary systems, preventing unified management and analysis.

5. **Privacy Concerns**: Cloud-based solutions require sharing personal gaming habits and collection data with third parties.

## Differentiators

**Complete Data Sovereignty**: Unlike cloud-based alternatives, Nexorious runs entirely on your infrastructure, ensuring your gaming data remains private and under your control.

**IGDB-Powered Metadata**: Leverages the comprehensive IGDB database for rich game metadata, cover art, ratings, and completion time estimates - providing professional-quality data presentation.

**Multi-Platform Unity**: Seamlessly tracks games across all major platforms and storefronts in a single interface, eliminating platform silos.

**Import-First Design**: Robust CSV and Steam library import capabilities with intelligent conflict resolution, making migration from existing systems straightforward.

**Practical Simplicity**: Built with KISS and DRY principles - focuses on essential functionality without enterprise bloat or unnecessary complexity.

**Self-Hosting Friendly**: Designed for easy deployment and maintenance by individual users, not enterprise IT departments.

## Key Features

### Core Collection Management
- Unified game library spanning all major platforms (Steam, Epic, GOG, PlayStation, Xbox, Nintendo, Physical)
- Rich metadata from IGDB including ratings, completion times, genres, and cover art
- Personal progress tracking with play status, ratings, time played, and detailed notes
- Rich text notes with TipTap editor for detailed game commentary

### Import & Integration
- CSV import from Darkadia format with intelligent conflict resolution
- Steam library import with automatic platform detection and game matching
- Rate-limited IGDB API integration (4 req/s) for reliable metadata retrieval
- Automatic cover art download and local storage

### Technical Foundation
- JWT-based user authentication with refresh tokens
- Comprehensive test coverage (>80% backend, >70% frontend)
- Automatic database migrations for seamless updates
- PostgreSQL (production) and SQLite (development) support
- Local filesystem storage for complete data independence

### Administrative Features
- Admin user management and system administration
- Robust error handling and logging
- Development environment with Nix for reproducible builds