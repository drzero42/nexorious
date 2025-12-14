"""
Models for the matching service.

Defines the data structures used for game matching operations.
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional, List, Dict, Any


class MatchStatus(str, Enum):
    """Status of a game matching attempt."""

    MATCHED = "matched"  # Successfully matched to IGDB ID
    NEEDS_REVIEW = "needs_review"  # Multiple candidates, requires user decision
    NO_MATCH = "no_match"  # No candidates found
    ALREADY_MATCHED = "already_matched"  # Already has IGDB ID
    ERROR = "error"  # Matching failed due to error


class MatchSource(str, Enum):
    """How the match was determined."""

    IGDB_ID_PROVIDED = "igdb_id_provided"  # IGDB ID was in source data
    PLATFORM_LOOKUP = "platform_lookup"  # Found via Steam AppID or similar
    TITLE_SEARCH_AUTO = "title_search_auto"  # High confidence title match
    TITLE_SEARCH_REVIEW = "title_search_review"  # Low confidence, needs review
    MANUAL = "manual"  # User manually selected


@dataclass
class IGDBCandidate:
    """An IGDB game candidate for matching."""

    igdb_id: int
    name: str
    first_release_date: Optional[str] = None
    cover_url: Optional[str] = None
    summary: Optional[str] = None
    platforms: Optional[List[str]] = None
    confidence_score: Optional[float] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "igdb_id": self.igdb_id,
            "name": self.name,
            "first_release_date": self.first_release_date,
            "cover_url": self.cover_url,
            "summary": self.summary,
            "platforms": self.platforms,
            "confidence_score": self.confidence_score,
        }


@dataclass
class MatchRequest:
    """Request to match a game from an external source."""

    source_title: str
    source_platform: str  # "steam", "epic", "darkadia", etc.
    igdb_id: Optional[int] = None  # If already known (e.g., Nexorious JSON export)
    platform_id: Optional[str] = None  # Steam AppID, Epic ID, etc.
    release_year: Optional[int] = None  # Optional hint for matching
    source_metadata: Optional[Dict[str, Any]] = field(default_factory=dict)

    def __post_init__(self):
        if self.source_metadata is None:
            self.source_metadata = {}


@dataclass
class MatchResult:
    """Result of a single game matching attempt."""

    source_title: str
    status: MatchStatus
    match_source: Optional[MatchSource] = None
    igdb_id: Optional[int] = None
    igdb_title: Optional[str] = None
    confidence_score: Optional[float] = None
    candidates: Optional[List[IGDBCandidate]] = field(default=None)
    error_message: Optional[str] = None
    source_metadata: Optional[Dict[str, Any]] = field(default=None)

    def __post_init__(self):
        if self.candidates is None:
            self.candidates = []
        if self.source_metadata is None:
            self.source_metadata = {}

    @property
    def is_matched(self) -> bool:
        """Check if this result has a successful match."""
        return self.status in (MatchStatus.MATCHED, MatchStatus.ALREADY_MATCHED)

    @property
    def needs_review(self) -> bool:
        """Check if this result requires user review."""
        return self.status == MatchStatus.NEEDS_REVIEW

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        candidates_list = self.candidates if self.candidates is not None else []
        return {
            "source_title": self.source_title,
            "status": self.status.value,
            "match_source": self.match_source.value if self.match_source else None,
            "igdb_id": self.igdb_id,
            "igdb_title": self.igdb_title,
            "confidence_score": self.confidence_score,
            "candidates": [c.to_dict() for c in candidates_list],
            "error_message": self.error_message,
            "source_metadata": self.source_metadata,
        }


@dataclass
class BatchMatchResult:
    """Results from matching a batch of games."""

    total_processed: int = 0
    matched: int = 0
    needs_review: int = 0
    no_match: int = 0
    already_matched: int = 0
    errors: int = 0
    results: Optional[List[MatchResult]] = field(default=None)

    def __post_init__(self):
        if self.results is None:
            self.results = []

    def add_result(self, result: MatchResult) -> None:
        """Add a result and update counters."""
        if self.results is None:
            self.results = []
        self.results.append(result)
        self.total_processed += 1

        if result.status == MatchStatus.MATCHED:
            self.matched += 1
        elif result.status == MatchStatus.NEEDS_REVIEW:
            self.needs_review += 1
        elif result.status == MatchStatus.NO_MATCH:
            self.no_match += 1
        elif result.status == MatchStatus.ALREADY_MATCHED:
            self.already_matched += 1
        elif result.status == MatchStatus.ERROR:
            self.errors += 1

    @property
    def success_rate(self) -> float:
        """Calculate the success rate of matching."""
        if self.total_processed == 0:
            return 0.0
        return (self.matched + self.already_matched) / self.total_processed

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        results_list = self.results if self.results is not None else []
        return {
            "total_processed": self.total_processed,
            "matched": self.matched,
            "needs_review": self.needs_review,
            "no_match": self.no_match,
            "already_matched": self.already_matched,
            "errors": self.errors,
            "success_rate": self.success_rate,
            "results": [r.to_dict() for r in results_list],
        }
