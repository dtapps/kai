CREATE TABLE IF NOT EXISTS http_log (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname            TEXT,
    method              TEXT,
    url                 TEXT,
    request_headers     TEXT,
    request_body        BLOB,
    status_code         INTEGER,
    response_headers    TEXT,
    response_body       BLOB,
    elapse_time         BIGINT,
    process_elapse_time BIGINT,
    is_error            BOOLEAN NOT NULL DEFAULT 0,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    go_version          TEXT,
    plugin_version      TEXT
);

CREATE INDEX IF NOT EXISTS http_log_hostname ON http_log (hostname);
CREATE INDEX IF NOT EXISTS http_log_method ON http_log (method);
CREATE INDEX IF NOT EXISTS http_log_url ON http_log (url);
CREATE INDEX IF NOT EXISTS http_log_status_code ON http_log (status_code);
CREATE INDEX IF NOT EXISTS http_log_created_at ON http_log (created_at DESC);
