"""
Platform resolution models and constants.

This module contains constants for explicit platform and storefront mappings
used when fuzzy matching fails.
"""

import re
from typing import Optional


# Explicit mappings for cases where fuzzy matching fails
EXPLICIT_PLATFORM_MAPPINGS = {
    # Short forms that are too different for fuzzy matching
    'PC': 'PC (Windows)',
    'Linux': 'PC (Linux)',
    'PS3': 'PlayStation 3',
    'PS4': 'PlayStation 4',
    'PS5': 'PlayStation 5',
    'Wii': 'Nintendo Wii',

    # Special cases with very different names
    'PlayStation Network (PS3)': 'PlayStation 3',
    'PlayStation Network (Vita)': 'PlayStation Vita',
    'PlayStation Network (PSP)': 'PlayStation Portable (PSP)',
    'Xbox 360 Games Store': 'Xbox 360',
}

# Minimal explicit storefront mappings
EXPLICIT_STOREFRONT_MAPPINGS = {
    # Short forms and abbreviations
    'PSN': 'PlayStation Store',
    'HB': 'Humble Bundle',
    'Epic': 'Epic Games Store',
    'Origin': 'Origin/EA App',

    # Special cases
    'Other': 'Physical',
    'Sony Entertainment Network': 'PlayStation Store',
}


def sanitize_platform_name(name: Optional[str]) -> str:
    """
    Sanitize platform name input to prevent injection attacks.
    Based on validate_platform_name from the spec but for suggestions only.
    """
    if not name:
        return ""

    # Strip whitespace and limit length
    sanitized = str(name).strip()[:200]

    # Remove null bytes and control characters except basic whitespace
    sanitized = ''.join(char for char in sanitized if ord(char) >= 32 or char in '\n\r\t')

    # Remove potential script injection patterns
    script_patterns = [
        re.compile(r'<script.*?</script>', re.IGNORECASE | re.DOTALL),
        re.compile(r'javascript:', re.IGNORECASE),
        re.compile(r'vbscript:', re.IGNORECASE),
    ]

    for pattern in script_patterns:
        sanitized = pattern.sub('', sanitized)

    return sanitized
