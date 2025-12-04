"""
IGDB search module.

Handles search query expansion, keyword detection, and fuzzy matching.
"""

import logging
import re
from typing import List, Dict

from .models import GameMetadata, KEYWORD_EXPANSIONS


logger = logging.getLogger(__name__)


def detect_pattern_keyword(pattern_key: str, expansion: str, query: str) -> Dict[str, str]:
    """
    Detect pattern-based keywords in search query.

    Args:
        pattern_key: Pattern key from KEYWORD_EXPANSIONS (e.g., "_pattern_year_parentheses")
        expansion: Expansion string for the pattern
        query: Search query string

    Returns:
        Dictionary mapping found pattern instances to their expansions
    """
    detected = {}

    if pattern_key == "_pattern_year_parentheses":
        # Detect years in parentheses (4 digits)
        year_pattern = r'\(\d{4}\)'
        matches = re.findall(year_pattern, query)
        for match in matches:
            detected[match] = expansion
            logger.debug(f"Detected year pattern '{match}' in query '{query}'")

    elif pattern_key == "_pattern_standalone_one":
        # Detect standalone number "1" but avoid Episode/Chapter/Part/Version references
        # Improved pattern to avoid false positives like "Episode 1", "Chapter 1", etc.
        # Look for "1" that is:
        # - Word boundary at start
        # - Not followed by a dot or dash (version numbers)
        # - Not preceded by Episode/Chapter/Part/Season/Vol/Volume (case insensitive)
        if re.search(r'\b1\b', query) and not re.search(r'\b1[\.\-]', query):
            # Check if it's NOT preceded by episode/chapter type words
            episode_pattern = r'\b(?:episode|chapter|part|season|vol|volume|series)\s+1\b'
            if not re.search(episode_pattern, query, re.IGNORECASE):
                # Find the actual match for replacement (include trailing space if present)
                match = re.search(r'\b1\s*(?=:|$)|\b1\s+', query)
                if match:
                    matched_text = match.group()
                    detected[matched_text] = expansion
                    logger.debug(f"Detected standalone '1' pattern '{matched_text}' in query '{query}'")

    return detected


def detect_keywords(query: str) -> Dict[str, str]:
    """
    Detect keywords in search query that need expansion.

    Args:
        query: Search query string

    Returns:
        Dictionary mapping found keywords/patterns to their expansions
    """
    detected = {}
    query_lower = query.lower()

    for keyword, expansion in KEYWORD_EXPANSIONS.items():
        # Check if this is a pattern-based keyword
        if keyword.startswith('_pattern_'):
            detected.update(detect_pattern_keyword(keyword, expansion, query))
        # Special case for (classic) - make it case insensitive
        elif keyword == "(classic)":
            pattern = re.escape(keyword)
            if re.search(pattern, query, re.IGNORECASE):
                # Find the actual match in the original query to preserve case for replacement
                match = re.search(pattern, query, re.IGNORECASE)
                if match:
                    actual_match = match.group()
                    detected[actual_match] = expansion
                    logger.debug(f"Detected case-insensitive '{keyword}' as '{actual_match}' in query '{query}'")
        # Check if keyword is a symbol (no alphanumeric characters)
        elif re.match(r'^[^\w\s]+$', keyword):
            # For symbols, use simple pattern without word boundaries
            pattern = re.escape(keyword)
            if re.search(pattern, query):  # Use original query for case-sensitive symbols
                detected[keyword] = expansion
                logger.debug(f"Detected symbol '{keyword}' in query '{query}'")
        else:
            # Use word boundaries to match whole words only for text keywords
            pattern = r'\b' + re.escape(keyword.lower()) + r'\b'
            if re.search(pattern, query_lower):
                detected[keyword] = expansion
                logger.debug(f"Detected keyword '{keyword}' in query '{query}'")

    return detected


def apply_single_transformation(query: str, keyword: str, expansion: str) -> str:
    """
    Apply a single keyword transformation to a query.

    Args:
        query: Query to transform
        keyword: Keyword to replace
        expansion: Replacement text

    Returns:
        Transformed query string
    """
    # Check if keyword is a year in parentheses pattern
    if keyword.startswith('(') and keyword.endswith(')') and re.match(r'\(\d{4}\)', keyword):
        # For year patterns like (2023), use exact pattern matching
        pattern = re.escape(keyword)
        expanded_query = re.sub(pattern, expansion, query)
    # Check if keyword contains trailing space (from standalone "1" detection)
    elif keyword.endswith(' ') or keyword.endswith('\t'):
        # For patterns that include whitespace, use exact replacement
        expanded_query = query.replace(keyword, expansion)
    # Check if keyword is (classic) variants - case-insensitive matches need exact replacement
    elif keyword.lower() == "(classic)":
        # For case-insensitive (classic) matches, use exact replacement of the found match
        expanded_query = query.replace(keyword, expansion)
    # Check if keyword is a symbol (no alphanumeric characters)
    elif re.match(r'^[^\w\s]+$', keyword):
        # For symbols, use simple pattern without word boundaries
        pattern = re.escape(keyword)
        expanded_query = re.sub(pattern, expansion, query)  # Case-sensitive for symbols
    else:
        # Replace keyword with expansion (case-insensitive) for text keywords
        pattern = r'\b' + re.escape(keyword) + r'\b'
        expanded_query = re.sub(pattern, expansion, query, flags=re.IGNORECASE)

    # Clean up extra whitespace for all expansions
    expanded_query = re.sub(r'\s+', ' ', expanded_query)  # Multiple spaces -> single space
    expanded_query = expanded_query.strip()  # Remove leading/trailing whitespace

    # Additional cleanup for removals (empty expansions)
    if expansion == "":
        expanded_query = re.sub(r':\s+:', ':', expanded_query)  # ": :" -> ":"
        expanded_query = re.sub(r'\s+:', ':', expanded_query)  # "word :" -> "word:"
        expanded_query = re.sub(r':\s*$', '', expanded_query)  # Remove trailing ":"

    return expanded_query


def generate_expanded_queries(original_query: str, detected_keywords: Dict[str, str]) -> List[str]:
    """
    Generate expanded queries by replacing keywords with their expansions.

    Args:
        original_query: Original search query
        detected_keywords: Dictionary of detected keywords and their expansions

    Returns:
        List of expanded query strings
    """
    expanded_queries = []

    # Generate individual transformations (one per keyword)
    for keyword, expansion in detected_keywords.items():
        expanded_query = apply_single_transformation(original_query, keyword, expansion)
        expanded_queries.append(expanded_query)
        action = "Removed" if expansion == "" else "Expanded"
        logger.debug(f"Generated {action.lower()} query: '{expanded_query}' from keyword '{keyword}'")

    # Generate fully transformed query (all keywords applied together) if multiple keywords
    if len(detected_keywords) > 1:
        fully_transformed = original_query
        for keyword, expansion in detected_keywords.items():
            fully_transformed = apply_single_transformation(fully_transformed, keyword, expansion)

        # Only add if it's different from individual transformations (avoid duplicates)
        if fully_transformed not in expanded_queries and fully_transformed != original_query:
            expanded_queries.append(fully_transformed)
            logger.debug(f"Generated fully transformed query: '{fully_transformed}'")

    return expanded_queries


def rank_games_by_fuzzy_match(games: List[GameMetadata], query: str, threshold: float = 0.6) -> List[GameMetadata]:
    """Rank games by fuzzy matching similarity to query."""
    if not games or not query.strip():
        return games

    query.lower().strip()

    # Calculate similarity scores for each game using shared fuzzy matching logic
    from app.utils.fuzzy_match import calculate_fuzzy_confidence
    scored_games = []
    for game in games:
        # Calculate confidence score using sophisticated multi-metric fuzzy matching
        final_score = calculate_fuzzy_confidence(query, game.title)

        # Only include games above threshold
        if final_score >= threshold:
            scored_games.append((game, final_score))

    # Sort by score (descending) and return games
    scored_games.sort(key=lambda x: x[1], reverse=True)

    logger.debug(f"Fuzzy matching results for '{query}': {[(g.title, s) for g, s in scored_games[:5]]}")

    return [game for game, score in scored_games]


def merge_and_deduplicate_results(
    original_results: List[GameMetadata],
    expanded_results: List[List[GameMetadata]],
    limit: int
) -> List[GameMetadata]:
    """
    Merge and deduplicate search results, prioritizing original query results.

    Args:
        original_results: Results from original query
        expanded_results: List of results from expanded queries
        limit: Maximum number of results to return

    Returns:
        Merged and deduplicated results
    """
    seen_ids = set()
    merged = []

    # Add original results first (highest priority)
    for game in original_results:
        if game.igdb_id not in seen_ids:
            merged.append(game)
            seen_ids.add(game.igdb_id)
            if len(merged) >= limit:
                break

    # Add expanded results for unseen games
    for expanded_list in expanded_results:
        for game in expanded_list:
            if game.igdb_id not in seen_ids and len(merged) < limit:
                merged.append(game)
                seen_ids.add(game.igdb_id)
                if len(merged) >= limit:
                    break
        if len(merged) >= limit:
            break

    logger.debug(f"Merged results: {len(merged)} games from {len(original_results)} original + "
                f"{sum(len(r) for r in expanded_results)} expanded results")

    return merged[:limit]
