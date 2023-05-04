-- name: GetAccount :one
SELECT * FROM account WHERE id = @account_id;

-- name: ListAccounts :many
SELECT * FROM account;

-- name: CreateAccount :exec
INSERT INTO account (id, display_name, email)
VALUES (@id, @display_name, sqlc_narg(email));

-- name: UpdateAccountDisplayName :one
UPDATE account
SET display_name = @display_name
WHERE id = @id
RETURNING *;
