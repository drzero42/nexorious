-- Revert the icons sourced in issue #1173 back to NULL (first-letter fallback).

UPDATE public.storefronts SET icon = NULL WHERE name IN ('gamersgate', 'amazon-games');

UPDATE public.platforms SET icon = NULL WHERE name IN (
    'playstation-vita',
    'playstation-psp',
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
