//go:build !sqlite_mattn

package sqlite

import (
	"database/sql"
	"fmt"

	"modernc.org/sqlite"
)

func init() {
	// 注册为 ent 的 dialect.SQLite 所用的驱动名 "sqlite3"，
	// 同时供 httplog、测试等所有 sqlite 调用复用。
	sql.Register("sqlite3", &sqlite.Driver{})
}

// BuildDSN 构造 modernc sqlite（纯 Go，无 CGO）的连接串。
// 使用 _pragma 语法声明外键约束、WAL 日志模式与忙等待超时。
func BuildDSN(path string) string {
	return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000", path)
}
