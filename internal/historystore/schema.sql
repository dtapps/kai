-- engine_id 引用 config.db 的 engines.id（跨库，无 FK 约束）。
CREATE TABLE IF NOT EXISTS history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    text       TEXT    NOT NULL,
    result     TEXT    NOT NULL,
    from_lang  TEXT    NOT NULL DEFAULT '',
    to_lang    TEXT    NOT NULL DEFAULT '',
    engine_id  INTEGER NOT NULL DEFAULT 0,
    from_ocr   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_history_created ON history (created_at DESC);
