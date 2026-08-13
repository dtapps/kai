-- name: InsertHttpLog :exec
INSERT INTO http_log (
    hostname, method, url, request_headers, request_body,
    status_code, response_headers, response_body,
    elapse_time, process_elapse_time, is_error, created_at,
    go_version, plugin_version
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteOldHttpLog :execrows
DELETE FROM http_log WHERE created_at < ?;
