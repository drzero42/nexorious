"""
Tests for the shared fuzzy matching utility.
"""
from app.utils.fuzzy_match import calculate_fuzzy_confidence


class TestFuzzyMatchUtility:
    """Test fuzzy matching utility functions."""
    
    def test_exact_match_gets_perfect_score(self):
        """Test that exact matches get 100% confidence."""
        query = "The Witcher 3: Wild Hunt"
        title = "The Witcher 3: Wild Hunt"
        
        confidence = calculate_fuzzy_confidence(query, title)
        assert confidence == 1.0
    
    def test_case_insensitive_exact_match(self):
        """Test that case differences don't affect exact matches."""
        query = "the witcher 3"
        title = "The Witcher 3"
        
        confidence = calculate_fuzzy_confidence(query, title)
        assert confidence == 1.0
    
    def test_partial_match_gets_high_score(self):
        """Test that good partial matches get high scores."""
        query = "Batman Arkham City"
        title = "Batman: Arkham City - Game of the Year Edition"
        
        confidence = calculate_fuzzy_confidence(query, title)
        # Should be high due to good partial matching
        assert confidence > 0.7
    
    def test_token_set_matching_handles_reordering(self):
        """Test that token set matching handles word reordering well."""
        query = "Grand Theft Auto V"
        title = "Grand Theft Auto: V"
        
        confidence = calculate_fuzzy_confidence(query, title)
        # Should be very high due to token set similarity
        assert confidence > 0.8
    
    def test_goty_expansion_case(self):
        """Test the specific GOTY case that was failing."""
        query = "Batman: Arkham City GOTY"
        title = "Batman: Arkham City - Game of the Year Edition"
        
        confidence = calculate_fuzzy_confidence(query, title)
        # Should pass the 60% threshold used by manual search
        assert confidence >= 0.6
        # Should be around 72.7% based on our earlier testing
        assert 0.70 <= confidence <= 0.75
    
    def test_completely_different_titles_get_low_score(self):
        """Test that unrelated titles get low scores."""
        query = "The Witcher 3"
        title = "Call of Duty: Modern Warfare"
        
        confidence = calculate_fuzzy_confidence(query, title)
        assert confidence < 0.35
    
    def test_empty_strings_handled_gracefully(self):
        """Test that empty strings don't cause errors."""
        assert calculate_fuzzy_confidence("", "") == 1.0  # Empty exact match
        assert calculate_fuzzy_confidence("test", "") < 0.5
        assert calculate_fuzzy_confidence("", "test") < 0.5
    
    def test_whitespace_normalization(self):
        """Test that extra whitespace is handled correctly."""
        query = "  The Witcher 3  "
        title = "The Witcher 3"
        
        confidence = calculate_fuzzy_confidence(query, title)
        assert confidence == 1.0
    
    def test_sophisticated_scoring_vs_simple_ratio(self):
        """Test that sophisticated scoring performs better than simple ratio."""
        from rapidfuzz import fuzz
        
        query = "Batman: Arkham City GOTY"
        title = "Batman: Arkham City - Game of the Year Edition"
        
        # Simple ratio (old method)
        simple_score = fuzz.ratio(query.lower(), title.lower()) / 100.0
        
        # Sophisticated scoring (new method) 
        sophisticated_score = calculate_fuzzy_confidence(query, title)
        
        # Sophisticated should be higher
        assert sophisticated_score >= simple_score
        # For this specific case, sophisticated should be meaningfully better
        assert sophisticated_score - simple_score > 0.03  # At least 3% improvement