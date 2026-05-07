-- name: ListPlatforms :many
SELECT * FROM platforms ORDER BY display_name;

-- name: GetPlatform :one
SELECT * FROM platforms WHERE name = $1;
