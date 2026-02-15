-- name: CreateUser :one
INSERT INTO users (id, credit)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1 AND is_deleted = false
LIMIT 1;

-- name: GetAllUsers :many
SELECT *
FROM users;

-- name: DeleteUser :exec
UPDATE users
SET is_deleted = true
WHERE id = $1;

-- name: UpdateUserByID :one
UPDATE users
SET
    is_banned   = $1,
    is_premium  = $2,
    is_verified = $3,
    total_links = $4
WHERE id = $5
RETURNING *;

-- name: GetTotalActiveUsersCount :one
SELECT COUNT(*)
FROM users
WHERE is_deleted = false;

-- name: IncrementCreditWithDate :one
UPDATE users
SET credit = credit + $2,
    last_credit_update = now()
WHERE id = $1
RETURNING *;

-- name: IncrementCredit :one
UPDATE users
SET credit = credit + $2
WHERE id = $1
RETURNING *;

-- name: DecrementCredit :one
UPDATE users
SET credit = credit - $2
WHERE id = $1
RETURNING *;

-- name: IncrementTotalLinks :one
UPDATE users SET total_links = total_links + 1 WHERE id = $1 RETURNING *;
