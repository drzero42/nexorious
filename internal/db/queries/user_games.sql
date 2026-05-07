-- name: GetUserGame :one
SELECT * FROM user_games WHERE id = $1 AND user_id = $2;

-- name: GetUserGameByGameID :one
SELECT * FROM user_games WHERE user_id = $1 AND game_id = $2;

-- name: CreateUserGame :one
INSERT INTO user_games (
    id, user_id, game_id, play_status, personal_rating,
    is_loved, hours_played, personal_notes, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, now(), now()
)
RETURNING *;

-- name: UpdateUserGame :one
UPDATE user_games SET
    play_status     = $2,
    personal_rating = $3,
    is_loved        = $4,
    hours_played    = $5,
    personal_notes  = $6,
    updated_at      = now()
WHERE id = $1 AND user_id = $7
RETURNING *;

-- name: DeleteUserGame :exec
DELETE FROM user_games WHERE id = $1 AND user_id = $2;

-- name: CountUserGamesByGameID :one
-- Used by unreferenced-game cleanup: after deleting a user_game, the handler
-- checks this count; if zero, the games row and its cover art file are deleted.
SELECT COUNT(*) FROM user_games WHERE game_id = $1;
