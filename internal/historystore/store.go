// Package historystore 提供翻译历史记录的 SQLite 持久化。
// 数据库查询代码由 sqlc 从 query.sql 自动生成；本文件提供 DB 生命周期管理
// 和便捷封装方法（参数适配、ErrNoRows 规范化等）。
package historystore

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

//go:embed migration.sql
var migrationSQL string

// Store 封装 history.db 的数据库连接与生成查询。
type Store struct {
	db *sql.DB
	*Queries
}

// Open 打开（或创建）SQLite 历史数据库并执行迁移。
// path 为空时使用内存库（:memory:），仅用于测试。
func Open(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("historystore: open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite 单写者
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("historystore: ping: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("historystore: migrate: %w", err)
	}
	// 增量迁移：对已存在但缺列的旧库执行 migration.sql 中的 ALTER。
	// SQLite 不支持 ADD COLUMN IF NOT EXISTS，重复的 ALTER 会报 "duplicate column"，
	// 此处忽略该错误以保证幂等（与 httplogstore 的 Migrate 一致）。
	for stmt := range strings.SplitSeq(migrationSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				db.Close()
				return nil, fmt.Errorf("historystore: migrate: %w", err)
			}
		}
	}
	return &Store{db: db, Queries: New(db)}, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// FindByKey 查找内容完全相同的记录（去重用），未找到返回 0。
// 对生成查询的 ErrNoRows 做规范化处理。
func (s *Store) FindByKey(ctx context.Context, text, fromLang, toLang string, engineID, fromOCR int64) (int64, error) {
	id, err := s.FindDuplicate(ctx, FindDuplicateParams{
		Text:     text,
		EngineID: engineID,
		FromLang: fromLang,
		ToLang:   toLang,
		FromOcr:  fromOCR,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// QueryByKeyword 按关键词检索历史，空关键词返回全部，时间倒序。
func (s *Store) QueryByKeyword(ctx context.Context, keyword string, limit, offset int64) ([]History, error) {
	return s.QueryHistory(ctx, QueryHistoryParams{
		Column1: keyword,
		Limit:   limit,
		Offset:  offset,
	})
}

// CountByKeyword 返回符合关键词的历史条数。
func (s *Store) CountByKeyword(ctx context.Context, keyword string) (int64, error) {
	return s.CountHistory(ctx, keyword)
}
