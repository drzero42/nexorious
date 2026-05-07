-- name: ListUserGameTags :many
SELECT * FROM user_game_tags WHERE user_game_id = $1 ORDER BY created_at;

-- name: AddUserGameTag :one
INSERT INTO user_game_tags (id, user_game_id, tag_id, created_at)
VALUES ($1, $2, $3, now())
RETURNING *;

-- name: RemoveUserGameTag :exec
DELETE FROM user_game_tags WHERE user_game_id = $1 AND tag_id = $2;
