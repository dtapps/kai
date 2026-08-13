-- name: LoadEngines :many
SELECT id, engine, enabled, api_key, secret, extra, endpoint FROM engines ORDER BY id;

-- name: UpdateEngineByID :exec
UPDATE engines SET
    engine   = ?,
    enabled  = ?,
    api_key  = ?,
    secret   = ?,
    extra    = ?,
    endpoint = ?
WHERE id = ?;

-- name: UpdateEngineEnabled :exec
UPDATE engines SET enabled = ? WHERE id = ?;

-- name: InsertEngine :one
INSERT INTO engines (engine, enabled, api_key, secret, extra, endpoint)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: DeleteEngineByID :exec
DELETE FROM engines WHERE id = ?;

-- name: GetEngineByName :one
SELECT id, engine, enabled, api_key, secret, extra, endpoint FROM engines WHERE engine = ? LIMIT 1;

-- name: GetEngineByID :one
SELECT id, engine, enabled, api_key, secret, extra, endpoint FROM engines WHERE id = ? LIMIT 1;
