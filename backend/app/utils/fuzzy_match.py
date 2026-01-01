"""
Fuzzy matching utilities for consistent scoring across the application.
"""
from rapidfuzz import fuzz


def calculate_fuzzy_confidence(query: str, title: str) -> float:
    """
    Calculate sophisticated fuzzy matching confidence score using multiple algorithms.
    
    This function provides consistent scoring logic used by both manual search
    and auto-matching functionality.
    
    Args:
        query: The search query string
        title: The title to compare against
        
    Returns:
        Float confidence score between 0.0 and 1.0
    """
    from app.utils.normalize_title import normalize_title

    query_normalized = normalize_title(query)
    title_normalized = normalize_title(title)

    # Different matching strategies
    exact_score = 1.0 if query_normalized == title_normalized else 0.0
    ratio_score = fuzz.ratio(query_normalized, title_normalized) / 100.0
    partial_score = fuzz.partial_ratio(query_normalized, title_normalized) / 100.0
    token_sort_score = fuzz.token_sort_ratio(query_normalized, title_normalized) / 100.0
    token_set_score = fuzz.token_set_ratio(query_normalized, title_normalized) / 100.0
    
    # Calculate weighted final score - take the maximum of all weighted algorithms
    final_score = max(
        exact_score * 1.0,  # Exact match gets highest priority
        ratio_score * 0.9,  # Overall similarity
        partial_score * 0.8,  # Partial match
        token_sort_score * 0.7,  # Token order similarity
        token_set_score * 0.6  # Token set similarity
    )
    
    return final_score