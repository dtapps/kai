-- engines.id 是主键，history.engine_id 跨库引用此 id（跨库无 FK 约束）。
CREATE TABLE IF NOT EXISTS engines (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    engine   TEXT    NOT NULL UNIQUE,
    enabled  INTEGER NOT NULL DEFAULT 0,
    api_key  TEXT    NOT NULL DEFAULT '',
    secret   TEXT    NOT NULL DEFAULT '',
    extra    TEXT    NOT NULL DEFAULT '',
    endpoint TEXT    NOT NULL DEFAULT ''
);
