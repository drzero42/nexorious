-- Add a foreign key from external_game_platforms.platform to platforms(name) so
-- the database itself enforces that only canonical platform slugs are stored.
-- This replaces the previous code-level allowlist (PlatformToSlug); sync adapters
-- now emit canonical slugs directly.

-- Remove any orphaned rows that reference a platform not present in the platforms
-- table before adding the constraint. Under the prior code path everything passed
-- through PlatformToSlug and is already canonical, so this is expected to be a
-- no-op; it guards against a stray value blocking the ALTER. Such rows match no
-- known platform and are already invisible in the UI.
DELETE FROM external_game_platforms
WHERE platform NOT IN (SELECT name FROM platforms);

ALTER TABLE external_game_platforms
    ADD CONSTRAINT external_game_platforms_platform_fkey
    FOREIGN KEY (platform) REFERENCES platforms(name);
