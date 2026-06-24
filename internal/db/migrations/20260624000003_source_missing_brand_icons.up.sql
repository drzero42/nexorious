-- Wire up the platform & storefront icons sourced in issue #1173.
-- Each icon column was NULL (first-letter fallback in the UI); flip it to the
-- light-variant filename. The app derives the dark variant by filename swap
-- (see #1172), so only the light filename is stored. Assets live under
-- ui/frontend/public/logos/{platforms,storefronts}/<name>/.

-- Storefronts (2)
UPDATE public.storefronts SET icon = 'gamersgate-icon-light.svg'   WHERE name = 'gamersgate';
UPDATE public.storefronts SET icon = 'amazon-games-icon-light.svg' WHERE name = 'amazon-games';

-- Platforms (14)
UPDATE public.platforms SET icon = 'playstation-vita-icon-light.svg'          WHERE name = 'playstation-vita';
UPDATE public.platforms SET icon = 'playstation-psp-icon-light.svg'           WHERE name = 'playstation-psp';
UPDATE public.platforms SET icon = 'xbox-icon-light.svg'                      WHERE name = 'xbox';
UPDATE public.platforms SET icon = 'nintendo-nes-icon-light.svg'              WHERE name = 'nintendo-nes';
UPDATE public.platforms SET icon = 'nintendo-snes-icon-light.svg'             WHERE name = 'nintendo-snes';
UPDATE public.platforms SET icon = 'nintendo-64-icon-light.svg'              WHERE name = 'nintendo-64';
UPDATE public.platforms SET icon = 'nintendo-gamecube-icon-light.svg'         WHERE name = 'nintendo-gamecube';
UPDATE public.platforms SET icon = 'nintendo-game-boy-icon-light.svg'         WHERE name = 'nintendo-game-boy';
UPDATE public.platforms SET icon = 'nintendo-game-boy-color-icon-light.svg'   WHERE name = 'nintendo-game-boy-color';
UPDATE public.platforms SET icon = 'nintendo-game-boy-advance-icon-light.svg' WHERE name = 'nintendo-game-boy-advance';
UPDATE public.platforms SET icon = 'nintendo-ds-icon-light.svg'               WHERE name = 'nintendo-ds';
UPDATE public.platforms SET icon = 'nintendo-3ds-icon-light.svg'              WHERE name = 'nintendo-3ds';
UPDATE public.platforms SET icon = 'sega-genesis-icon-light.svg'              WHERE name = 'sega-genesis';
UPDATE public.platforms SET icon = 'sega-dreamcast-icon-light.svg'            WHERE name = 'sega-dreamcast';
