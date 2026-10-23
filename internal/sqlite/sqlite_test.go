package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildDSNContainsRequiredPragmas DSN 必须携带三条硬性约束：
// 外键（否则删除引擎时历史会留下悬空引用）、WAL、忙等待超时。
// modernc 与 mattn 两种驱动的写法不同，因此只断言语义关键字。
func TestBuildDSNContainsRequiredPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kai.db")
	dsn := BuildDSN(path)

	for _, want := range []string{path, "foreign_keys", "WAL", "5000"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN 缺少 %q: %s", want, dsn)
		}
	}
	if !strings.HasPrefix(dsn, "file:") {
		t.Fatalf("DSN 应以 file: 开头: %s", dsn)
	}
}

// TestBuildDSNOpenable 用生成的 DSN 真实建库，确认驱动已注册且 WAL 生效。
func TestBuildDSNOpenable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kai.db")

	db, err := sql.Open("sqlite3", BuildDSN(path))
	if err != nil {
		t.Fatalf("sql.Open 失败（驱动可能未注册）: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (v) VALUES ('x')"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	var got string
	if err := db.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&got); err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got != "x" {
		t.Fatalf("读出 %q, 期望 x", got)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("读取 journal_mode 失败: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode 应为 wal, 实际 %q", mode)
	}

	// 外键约束必须真正生效（_pragma 拼错时这里会静默通过）。
	if _, err := db.Exec("CREATE TABLE p (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE c (id INTEGER PRIMARY KEY, pid INTEGER REFERENCES p(id))"); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if _, err := db.Exec("INSERT INTO c (pid) VALUES (999)"); err == nil {
		t.Fatal("外键未生效：引用不存在的父行应被拒绝")
	}
}
