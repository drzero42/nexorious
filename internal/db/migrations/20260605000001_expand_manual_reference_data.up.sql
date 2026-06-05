-- Expand manual-workflow reference data (issue #818). Reference-data only: no
-- sync adapters, no service/worker changes. The platforms/storefronts APIs and
-- the manual-add flow read these tables generically, so no code changes accompany
-- this migration.
--
-- Part A: 12 commonly-owned retro/older platforms (Xbox original + retro
--         Nintendo/Sega). All physical-only: default_storefront='physical' plus a
--         single platform_storefronts row -> physical. Icons seed NULL (precedent:
--         Vita, PSP, GamersGate). IGDB platform IDs verified against IGDB's list.
-- Part B: fix missing platform<->storefront associations that are clear gaps
--         (uplay had zero associations; itch-io was pc-windows only). The
--         humble-bundle associations from the original Part B were already added by
--         20260604000003_humble_platform_storefronts, so they are intentionally
--         omitted here.
-- Part C: Amazon Games storefront (manual-only label; no sync) + its pc-windows
--         association.

-- Part A: platforms
INSERT INTO platforms (name, display_name, icon, igdb_platform_id, default_storefront) VALUES
    ('xbox',                     'Xbox',                            NULL, 11, 'physical'),
    ('nintendo-nes',             'Nintendo Entertainment System',   NULL, 18, 'physical'),
    ('nintendo-snes',            'Super Nintendo (SNES)',           NULL, 19, 'physical'),
    ('nintendo-64',              'Nintendo 64',                     NULL, 4,  'physical'),
    ('nintendo-gamecube',        'Nintendo GameCube',               NULL, 21, 'physical'),
    ('nintendo-game-boy',        'Game Boy',                        NULL, 33, 'physical'),
    ('nintendo-game-boy-color',  'Game Boy Color',                  NULL, 22, 'physical'),
    ('nintendo-game-boy-advance','Game Boy Advance',                NULL, 24, 'physical'),
    ('nintendo-ds',              'Nintendo DS',                     NULL, 20, 'physical'),
    ('nintendo-3ds',             'Nintendo 3DS',                    NULL, 37, 'physical'),
    ('sega-genesis',             'Sega Genesis / Mega Drive',       NULL, 29, 'physical'),
    ('sega-dreamcast',           'Sega Dreamcast',                  NULL, 23, 'physical');

-- Part A: physical association for each new platform
INSERT INTO platform_storefronts (platform, storefront) VALUES
    ('xbox',                     'physical'),
    ('nintendo-nes',             'physical'),
    ('nintendo-snes',            'physical'),
    ('nintendo-64',              'physical'),
    ('nintendo-gamecube',        'physical'),
    ('nintendo-game-boy',        'physical'),
    ('nintendo-game-boy-color',  'physical'),
    ('nintendo-game-boy-advance','physical'),
    ('nintendo-ds',              'physical'),
    ('nintendo-3ds',             'physical'),
    ('sega-genesis',             'physical'),
    ('sega-dreamcast',           'physical');

-- Part C: Amazon Games storefront (manual-only)
INSERT INTO storefronts (name, display_name, icon, base_url) VALUES
    ('amazon-games', 'Amazon Games', NULL, 'https://gaming.amazon.com');

-- Part B + C: fix/add associations
INSERT INTO platform_storefronts (platform, storefront) VALUES
    ('pc-windows', 'uplay'),         -- Part B: uplay had zero associations
    ('mac',        'itch-io'),       -- Part B: itch-io sells Mac/Linux/Android
    ('pc-linux',   'itch-io'),
    ('android',    'itch-io'),
    ('pc-windows', 'amazon-games')   -- Part C
ON CONFLICT (platform, storefront) DO NOTHING;
