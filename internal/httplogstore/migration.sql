-- httplog 库迁移脚本（专门迁移文件，与 schema.sql 分离）
-- 仅用于「历史库兼容」：给已存在的 http_log 表追加新列 / 新索引。
-- 建表本身在 schema.sql，本文件只放 ALTER / 额外索引。
-- 注意：SQLite 不支持 `ADD COLUMN IF NOT EXISTS`，故重复的 ALTER 会报
-- "duplicate column" 错误。运行时 (httplog.go 的 Migrate) 会忽略
-- 该错误以保证幂等——二次启动 / 已迁移过的库直接跳过，不会报错。
-- 将来新增列时，在此追加一行 ALTER TABLE 即可，无需改动 Go 代码。

-- 示例（当前 schema 与建表语句一致，暂无缺列；保留格式供后续扩展）：
-- ALTER TABLE http_log ADD COLUMN new_field TEXT DEFAULT '';
