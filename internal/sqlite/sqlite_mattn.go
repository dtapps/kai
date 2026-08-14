//go:build sqlite_mattn

package sqlite

import (
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// mattn/go-sqlite3 在导入时已通过自身 init() 将驱动注册为 "sqlite3"，
// 因此本文件无需手动注册；只需提供与 modernc 同名的 BuildDSN 即可切换。

// BuildDSN 构造 mattn/go-sqlite3（CGO）的连接串。
// mattn 使用 _foreign_keys / _journal_mode / _busy_timeout 查询参数语法，
// 与 modernc 的 _pragma= 语法不互通。
func BuildDSN(path string) string {
	return fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", path)
}
