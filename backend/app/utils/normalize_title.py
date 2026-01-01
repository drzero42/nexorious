"""
Title normalization utilities for improved matching.

Normalizes game titles to a canonical form for comparison during sync matching.
This normalization is internal only - never displayed to users or stored in the database.
"""
import re


def normalize_title(title: str) -> str:
    """
    Normalize a game title for matching purposes.

    Applies the following transformations in order:
    1. Expand GOTY -> Game of the Year (case-insensitive)
    2. Remove trademark symbols (TM, R)
    3. Remove apostrophes (straight and curly)
    4. Remove colons
    5. Remove standalone dashes ( - ) but preserve hyphens (Spider-Man)
    6. Remove year in parentheses, e.g., (2023)
    7. Collapse whitespace
    8. Lowercase and trim

    Args:
        title: The game title to normalize

    Returns:
        Normalized title string for comparison
    """
    result = title

    # 1. Expand GOTY -> Game of the Year (case-insensitive)
    result = re.sub(r'\bGOTY\b', 'Game of the Year', result, flags=re.IGNORECASE)

    # 2. Remove trademark symbols
    result = re.sub(r'[™®]', '', result)

    # 3. Remove apostrophes (straight and curly)
    result = re.sub(r"[''']", '', result)

    # 4. Remove colons
    result = result.replace(':', '')

    # 5. Remove standalone dashes (separators, not hyphens)
    result = re.sub(r' - ', ' ', result)

    # 6. Remove year in parentheses, e.g., (2023)
    result = re.sub(r'\(\d{4}\)', '', result)

    # 7. Collapse whitespace
    result = re.sub(r'\s+', ' ', result)

    # 8. Lowercase and trim
    result = result.lower().strip()

    return result
