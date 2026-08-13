-- name: InsertHistory :execlastid
INSERT INTO history (text, result, from_lang, to_lang, engine_id, from_ocr, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: QueryHistory :many
SELECT id, text, result, from_lang, to_lang, engine_id, from_ocr, created_at
FROM history
WHERE (?1 = '' OR text LIKE '%' || ?1 || '%' OR result LIKE '%' || ?1 || '%')
ORDER BY created_at DESC
LIMIT ?2 OFFSET ?3;

-- name: CountHistory :one
SELECT COUNT(*) FROM history
WHERE (?1 = '' OR text LIKE '%' || ?1 || '%' OR result LIKE '%' || ?1 || '%');

-- name: DeleteHistory :exec
DELETE FROM history WHERE id = ?;

-- name: ClearHistory :exec
DELETE FROM history;

-- name: FindDuplicate :one
SELECT id FROM history
WHERE text = ? AND engine_id = ? AND from_lang = ? AND to_lang = ? AND from_ocr = ?
LIMIT 1;
