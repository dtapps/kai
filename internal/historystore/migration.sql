-- history 库迁移脚本（专门迁移文件，与 schema.sql 分离）
-- 仅用于「历史库兼容」：给已存在的 history 表追加新列 / 新索引。
-- 建表本身在 schema.sql，本文件只放 ALTER / 额外索引。
-- 注意：SQLite 不支持 `ADD COLUMN IF NOT EXISTS`，故重复的 ALTER 会报
-- "duplicate column" 错误。运行时 (store.go 的 Open) 会忽略
-- 该错误以保证幂等——二次启动 / 已迁移过的库直接跳过，不会报错。
-- 将来新增列时，在此追加一行 ALTER TABLE 即可，无需改动 Go 代码。

-- 兼容旧版本：history 表最初创建时无 engine_id 列，需补列以写入引擎来源。
ALTER TABLE history ADD COLUMN engine_id INTEGER NOT NULL DEFAULT 0;
