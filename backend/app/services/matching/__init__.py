"""
Shared matching service for games from various sources.

This module provides a unified interface for matching games to IGDB IDs,
used by both sync tasks (Steam, etc.) and import tasks (Darkadia CSV, etc.).

The matching flow is:
1. IGDB ID if present in source data - Immediate match
2. Platform ID lookup (Steam AppID, etc.) - Check existing DB records
3. Title-based IGDB search - Auto-match if high confidence, queue for review otherwise
"""

from app.services.matching.models import (
    MatchResult,
    MatchStatus,
    MatchSource,
    IGDBCandidate,
    MatchRequest,
    BatchMatchResult,
)
from app.services.matching.service import MatchingService

__all__ = [
    "MatchingService",
    "MatchResult",
    "MatchStatus",
    "MatchSource",
    "IGDBCandidate",
    "MatchRequest",
    "BatchMatchResult",
]
