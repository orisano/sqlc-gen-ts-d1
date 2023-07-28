-- name: GetAccount :one
SELECT * FROM account WHERE id = @account_id;

-- name: ListAccounts :many
SELECT sqlc.embed(account) FROM account;

-- name: CreateAccount :exec
INSERT INTO account (id, display_name, email)
VALUES (@id, @display_name, @email);

-- name: UpdateAccountDisplayName :one
UPDATE account
SET display_name = @display_name
WHERE id = @id
RETURNING *;

-- name: GetAccounts :many
SELECT * FROM account WHERE id IN (sqlc.slice(ids));

-- name: GetConnectionId :one
SELECT CAST(json_extract('{"connection_id":"foo"}', '$.connection_id') AS TEXT) AS connection_id;
