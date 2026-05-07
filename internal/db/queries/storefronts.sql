-- name: ListStorefronts :many
SELECT * FROM storefronts ORDER BY display_name;

-- name: GetStorefront :one
SELECT * FROM storefronts WHERE name = $1;
