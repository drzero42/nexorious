-- name: GetUserGamePlatform :one
SELECT * FROM user_game_platforms WHERE id = $1 AND user_game_id = $2;

-- name: ListUserGamePlatforms :many
SELECT * FROM user_game_platforms
WHERE user_game_id = $1
ORDER BY platform, storefront;

-- name: CreateUserGamePlatform :one
INSERT INTO user_game_platforms (
    id, user_game_id, platform, storefront,
    store_game_id, store_url, is_available, hours_played,
    ownership_status, acquired_date, original_platform_name,
    original_storefront_name, external_game_id, sync_from_source,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14,
    now(), now()
)
RETURNING *;

-- name: UpdateUserGamePlatform :one
UPDATE user_game_platforms SET
    store_game_id    = $3,
    store_url        = $4,
    is_available     = $5,
    hours_played     = $6,
    ownership_status = $7,
    acquired_date    = $8,
    updated_at       = now()
WHERE id = $1 AND user_game_id = $2
RETURNING *;

-- name: DeleteUserGamePlatform :exec
DELETE FROM user_game_platforms WHERE id = $1 AND user_game_id = $2;
