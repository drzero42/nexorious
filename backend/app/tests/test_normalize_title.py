"""Tests for title normalization utilities."""
from app.utils.normalize_title import normalize_title


class TestNormalizeTitle:
    """Test cases for normalize_title function."""

    def test_removes_trademark_symbol(self):
        assert normalize_title("Doom™") == "doom"

    def test_removes_registered_symbol(self):
        assert normalize_title("The Witcher®") == "the witcher"

    def test_removes_colons(self):
        assert normalize_title("The Witcher 3: Wild Hunt") == "the witcher 3 wild hunt"

    def test_removes_apostrophes_straight(self):
        assert normalize_title("Assassin's Creed") == "assassins creed"

    def test_removes_apostrophes_curly(self):
        assert normalize_title("Assassin's Creed") == "assassins creed"

    def test_removes_year_in_parentheses(self):
        assert normalize_title("Resident Evil 4 (2023)") == "resident evil 4"

    def test_expands_goty(self):
        assert normalize_title("Fallout 3 GOTY") == "fallout 3 game of the year"

    def test_expands_goty_case_insensitive(self):
        assert normalize_title("Fallout 3 goty") == "fallout 3 game of the year"

    def test_removes_standalone_dashes(self):
        result = normalize_title("Wild Hunt - Game of the Year Edition")
        assert result == "wild hunt game of the year edition"

    def test_preserves_hyphens_in_words(self):
        assert normalize_title("Spider-Man") == "spider-man"

    def test_preserves_hyphens_in_words_with_numbers(self):
        assert normalize_title("Half-Life 2") == "half-life 2"

    def test_collapses_whitespace(self):
        assert normalize_title("Doom   Eternal") == "doom eternal"

    def test_trims_whitespace(self):
        assert normalize_title("  DOOM  ") == "doom"

    def test_lowercases(self):
        assert normalize_title("DOOM ETERNAL") == "doom eternal"

    def test_combined_normalizations(self):
        result = normalize_title("The Witcher® 3: Wild Hunt - GOTY Edition")
        assert result == "the witcher 3 wild hunt game of the year edition"

    def test_empty_string(self):
        assert normalize_title("") == ""

    def test_only_whitespace(self):
        assert normalize_title("   ") == ""

    def test_multiple_trademark_symbols(self):
        assert normalize_title("EA™ Sports™ FC™") == "ea sports fc"

    def test_year_at_end(self):
        assert normalize_title("FIFA (2023)") == "fifa"

    def test_year_in_middle(self):
        assert normalize_title("NBA 2K (2023) Edition") == "nba 2k edition"
