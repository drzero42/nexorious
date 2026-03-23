"""Tests for Settings DB connection configuration.

All tests that rely on constructed URLs pass _env_file=None and clear
all DB_* env vars to ensure isolation regardless of the developer's
local shell environment or CI configuration.
"""
from app.core.config import Settings

# All DB_* env var names that might leak in from the shell/CI
_DB_ENV_VARS = ("DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DATABASE_URL")


class TestSettingsDatabaseUrl:
    """Test DATABASE_URL priority and individual DB var construction."""

    def test_defaults_produce_dev_url(self, monkeypatch):
        """No env vars set → URL matches the existing dev default."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(_env_file=None)
        assert s.database_url == "postgresql://nexorious:nexorious@localhost:5432/nexorious"

    def test_database_url_takes_priority(self, monkeypatch):
        """DATABASE_URL set → used as-is; individual vars are ignored even if also set."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="postgresql://custom:secret@db.example.com:5433/mydb",
            db_host="should-be-ignored",
            db_port=9999,
            db_user="ignored",
            db_password="ignored",
            db_name="ignored",
        )
        assert s.database_url == "postgresql://custom:secret@db.example.com:5433/mydb"

    def test_empty_database_url_with_defaults_falls_back_to_constructed_url(self, monkeypatch):
        """Empty-string DATABASE_URL, no individual vars → URL built from defaults."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(_env_file=None, database_url="")
        assert s.database_url == "postgresql://nexorious:nexorious@localhost:5432/nexorious"

    def test_all_individual_vars_construct_url(self, monkeypatch):
        """All five individual vars set explicitly → URL constructed from them."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="db.example.com",
            db_port=5433,
            db_user="myuser",
            db_password="mypassword",
            db_name="mydb",
        )
        assert s.database_url == "postgresql://myuser:mypassword@db.example.com:5433/mydb"

    def test_partial_individual_vars_use_defaults_for_missing(self, monkeypatch):
        """Only db_host set → remaining vars fall back to their field defaults."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="custom-host",
        )
        assert s.database_url == "postgresql://nexorious:nexorious@custom-host:5432/nexorious"

    def test_special_characters_in_user_and_password_are_percent_encoded(self, monkeypatch):
        """Special chars in db_user and db_password are percent-encoded in the URL."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="",
            db_user="user@domain",
            db_password="p@ss/word:secret",
            db_host="localhost",
            db_port=5432,
            db_name="nexorious",
        )
        # @ → %40, / → %2F, : → %3A
        assert "user%40domain" in s.database_url
        assert "p%40ss%2Fword%3Asecret" in s.database_url
        # host and port must appear unencoded
        assert "@localhost:5432" in s.database_url

    def test_special_characters_in_db_name_are_percent_encoded(self, monkeypatch):
        """Special chars in db_name are percent-encoded in the URL path segment."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="localhost",
            db_port=5432,
            db_user="nexorious",
            db_password="nexorious",
            db_name="my db/name",
        )
        # space → %20, / → %2F
        assert "my%20db%2Fname" in s.database_url

    def test_env_var_path_reads_db_host_from_environment(self, monkeypatch):
        """Env var DB_HOST is read by pydantic-settings and appears in the URL."""
        # Clear all DB_* vars so only our explicit one is active
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        monkeypatch.setenv("DB_HOST", "env-injected-host")
        s = Settings(_env_file=None)
        assert "env-injected-host" in s.database_url
        assert s.database_url.startswith("postgresql://")
