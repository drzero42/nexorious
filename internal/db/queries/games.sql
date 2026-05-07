-- name: GetGame :one
SELECT * FROM games WHERE id = $1;

-- name: GetGamesByIDs :many
SELECT * FROM games WHERE id = ANY($1::integer[])
ORDER BY title;

-- name: UpsertGame :one
INSERT INTO games (
    id, title, description, genre, developer, publisher,
    release_date, cover_art_url, rating_average, rating_count,
    estimated_playtime_hours, howlongtobeat_main, howlongtobeat_extra,
    howlongtobeat_completionist, igdb_slug, igdb_platform_ids,
    igdb_platform_names, game_modes, themes, player_perspectives,
    game_metadata, last_updated
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16,
    $17, $18, $19, $20,
    $21, $22
)
ON CONFLICT (id) DO UPDATE SET
    title                       = EXCLUDED.title,
    description                 = EXCLUDED.description,
    genre                       = EXCLUDED.genre,
    developer                   = EXCLUDED.developer,
    publisher                   = EXCLUDED.publisher,
    release_date                = EXCLUDED.release_date,
    cover_art_url               = EXCLUDED.cover_art_url,
    rating_average              = EXCLUDED.rating_average,
    rating_count                = EXCLUDED.rating_count,
    estimated_playtime_hours    = EXCLUDED.estimated_playtime_hours,
    howlongtobeat_main          = EXCLUDED.howlongtobeat_main,
    howlongtobeat_extra         = EXCLUDED.howlongtobeat_extra,
    howlongtobeat_completionist = EXCLUDED.howlongtobeat_completionist,
    igdb_slug                   = EXCLUDED.igdb_slug,
    igdb_platform_ids           = EXCLUDED.igdb_platform_ids,
    igdb_platform_names         = EXCLUDED.igdb_platform_names,
    game_modes                  = EXCLUDED.game_modes,
    themes                      = EXCLUDED.themes,
    player_perspectives         = EXCLUDED.player_perspectives,
    game_metadata               = EXCLUDED.game_metadata,
    last_updated                = EXCLUDED.last_updated
RETURNING *;

-- name: UpdateGameMetadata :one
UPDATE games SET
    title                       = $2,
    description                 = $3,
    genre                       = $4,
    developer                   = $5,
    publisher                   = $6,
    release_date                = $7,
    rating_average              = $8,
    rating_count                = $9,
    estimated_playtime_hours    = $10,
    howlongtobeat_main          = $11,
    howlongtobeat_extra         = $12,
    howlongtobeat_completionist = $13,
    game_modes                  = $14,
    themes                      = $15,
    player_perspectives         = $16,
    game_metadata               = $17,
    igdb_slug                   = $18,
    igdb_platform_ids           = $19,
    igdb_platform_names         = $20,
    last_updated                = now()
WHERE id = $1
RETURNING *;

-- name: UpdateGameCoverArtUrl :exec
UPDATE games SET cover_art_url = $2 WHERE id = $1;

-- name: DeleteGame :exec
DELETE FROM games WHERE id = $1;

-- name: SearchGamesByTitle :many
SELECT * FROM games
WHERE title ILIKE '%' || @query || '%'
   OR (description IS NOT NULL AND description ILIKE '%' || @query || '%')
ORDER BY title
LIMIT $1;
