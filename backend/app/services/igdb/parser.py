"""
IGDB data parsing module.

Handles parsing of IGDB API responses into GameMetadata objects.
"""

import logging
from datetime import datetime
from typing import Optional, Dict, Any

from .models import GameMetadata, IGDB_PLATFORM_MAPPING


logger = logging.getLogger(__name__)


def parse_game_data(game_data: Dict[str, Any]) -> Optional[GameMetadata]:
    """Parse IGDB game data into GameMetadata object."""
    try:
        game_id = game_data.get('id')
        game_name = game_data.get('name', 'Unknown')

        if not game_id:
            logger.warning(f"Game data missing required 'id' field for {game_name}")
            return None

        logger.debug(f"Parsing game data for {game_name} (ID: {game_id})")

        # Extract genres
        genres = game_data.get('genres', [])
        genre_names = [g.get('name') for g in genres if g.get('name')]
        genre = ', '.join(genre_names) if genre_names else None

        # Extract developer and publisher
        companies = game_data.get('involved_companies', [])
        developer = None
        publisher = None

        for company in companies:
            company_name = company.get('company', {}).get('name')
            if company_name:
                if company.get('developer'):
                    developer = company_name
                if company.get('publisher'):
                    publisher = company_name

        # Extract release date
        release_date = None
        if 'first_release_date' in game_data:
            try:
                release_timestamp = game_data['first_release_date']
                release_date = datetime.fromtimestamp(release_timestamp).strftime('%Y-%m-%d')
                logger.debug(f"Parsed release date for {game_name}: {release_date}")
            except (ValueError, OSError) as e:
                logger.warning(f"Invalid release date timestamp for {game_name}: {e}")

        # Extract cover art URL
        cover_art_url = None
        if 'cover' in game_data and 'image_id' in game_data['cover']:
            image_id = game_data['cover']['image_id']
            cover_art_url = f"https://images.igdb.com/igdb/image/upload/t_cover_big/{image_id}.jpg"
            logger.debug(f"Generated cover art URL for {game_name}: {cover_art_url}")

        # Extract platform data
        igdb_platform_ids = []
        platform_names = []
        platforms = game_data.get('platforms', [])
        for platform in platforms:
            platform_id = platform.get('id')
            if platform_id and platform_id in IGDB_PLATFORM_MAPPING:
                igdb_platform_ids.append(platform_id)
                platform_names.append(IGDB_PLATFORM_MAPPING[platform_id])
            elif platform_id:
                logger.debug(f"Unknown platform ID {platform_id} for game {game_name}")

        # Only set platform data if we found any mapped platforms
        igdb_platform_ids = igdb_platform_ids if igdb_platform_ids else None
        platform_names = platform_names if platform_names else None

        # Extract game modes
        game_modes_data = game_data.get('game_modes', [])
        game_modes = [gm.get('name') for gm in game_modes_data if gm.get('name')]

        # Extract themes
        themes_data = game_data.get('themes', [])
        themes = [t.get('name') for t in themes_data if t.get('name')]

        # Extract player perspectives
        perspectives_data = game_data.get('player_perspectives', [])
        player_perspectives = [p.get('name') for p in perspectives_data if p.get('name')]

        logger.debug(f"Successfully parsed {game_name}: genres={genre}, platforms={len(platform_names) if platform_names else 0}")

        return GameMetadata(
            igdb_id=game_id,
            title=game_name,
            igdb_slug=game_data.get('slug'),
            description=game_data.get('summary'),
            genre=genre,
            developer=developer,
            publisher=publisher,
            release_date=release_date,
            cover_art_url=cover_art_url,
            rating_average=game_data.get('rating'),
            rating_count=game_data.get('rating_count'),
            estimated_playtime_hours=None,  # IGDB doesn't provide this directly
            # Time-to-beat data will be populated separately
            hastily=None,
            normally=None,
            completely=None,
            # Platform data from IGDB
            igdb_platform_ids=igdb_platform_ids,
            platform_names=platform_names,
            # Additional IGDB metadata
            game_modes=game_modes if game_modes else None,
            themes=themes if themes else None,
            player_perspectives=player_perspectives if player_perspectives else None,
        )

    except KeyError as e:
        logger.error(f"Missing required field in game data: {e}")
        logger.debug(f"Game data keys available: {list(game_data.keys())}")
        return None
    except Exception as e:
        logger.error(f"Unexpected error parsing game data: {e}", exc_info=True)
        logger.debug(f"Problematic game data: {game_data}")
        return None
