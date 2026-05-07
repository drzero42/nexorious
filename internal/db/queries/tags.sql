-- name: ListUserTags :many
SELECT * FROM tags WHERE user_id = $1 ORDER BY name;

-- name: GetUserTag :one
SELECT * FROM tags WHERE id = $1 AND user_id = $2;

-- name: CreateTag :one
INSERT INTO tags (id, user_id, name, color, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
RETURNING *;

-- name: UpdateTag :one
UPDATE tags SET
    name       = $2,
    color      = $3,
    updated_at = now()
WHERE id = $1 AND user_id = $4
RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1 AND user_id = $2;
