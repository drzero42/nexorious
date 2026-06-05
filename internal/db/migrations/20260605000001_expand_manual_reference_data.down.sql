-- Reverse 20260605000001_expand_manual_reference_data.

-- Part B: remove the associations added by the up migration. (humble-bundle was
-- never touched here; it is owned by 20260604000003.)
DELETE FROM platform_storefronts
WHERE (platform, storefront) IN (
    ('pc-windows', 'uplay'),
    ('mac',        'itch-io'),
    ('pc-linux',   'itch-io'),
    ('android',    'itch-io')
);

-- Part C: drop the Amazon Games storefront. Its pc-windows platform_storefronts
-- row cascades via the storefront FK (ON DELETE CASCADE).
DELETE FROM storefronts WHERE name = 'amazon-games';

-- Part A: drop the 12 platforms. Their physical platform_storefronts rows cascade
-- via the platform FK (ON DELETE CASCADE).
DELETE FROM platforms WHERE name IN (
    'xbox',
    'nintendo-nes',
    'nintendo-snes',
    'nintendo-64',
    'nintendo-gamecube',
    'nintendo-game-boy',
    'nintendo-game-boy-color',
    'nintendo-game-boy-advance',
    'nintendo-ds',
    'nintendo-3ds',
    'sega-genesis',
    'sega-dreamcast'
);
