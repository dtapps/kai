package historystore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// newStore 打开一个内存库（path 为空 → :memory:），用例结束时自动关闭。
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close 失败: %v", err)
		}
	})
	return s
}

// insertRow 插入一条历史记录，返回自增 ID。
func insertRow(t *testing.T, s *Store, text, result, fromLang, toLang string, engineID, fromOCR, createdAt int64) int64 {
	t.Helper()
	id, err := s.InsertHistory(t.Context(), InsertHistoryParams{
		Text:      text,
		Result:    result,
		FromLang:  fromLang,
		ToLang:    toLang,
		EngineID:  engineID,
		FromOcr:   fromOCR,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("InsertHistory 失败: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertHistory 返回非法 ID: %d", id)
	}
	return id
}

// TestOpenInMemory 内存库应建表成功且可立即读写。
func TestOpenInMemory(t *testing.T) {
	s := newStore(t)
	if s.db == nil || s.Queries == nil {
		t.Fatal("Open 返回的 Store 未初始化")
	}
	var n int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM history").Scan(&n); err != nil {
		t.Fatalf("history 表不可查询: %v", err)
	}
	if n != 0 {
		t.Fatalf("新库应为空, 实际 %d 条", n)
	}
}

// TestOpenFileAndReopenIdempotent 文件库重复 Open 必须幂等：
// migration.sql 里的 ALTER 会在第二次执行时报 "duplicate column"，
// Open 必须吞掉该错误而不是启动失败。
func TestOpenFileAndReopenIdempotent(t *testing.T) {
	// sqlite.BuildDSN 不做建目录，所以只用 t.TempDir() 已建好的目录。
	path := filepath.Join(t.TempDir(), "history.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("首次 Open 失败: %v", err)
	}
	id := insertRow(t, first, "hello", "你好", "en", "zh", 1, 0, 100)
	if err := first.Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}

	// 第二次打开：schema.sql 是 IF NOT EXISTS，migration.sql 的 ALTER 会撞列。
	second, err := Open(path)
	if err != nil {
		t.Fatalf("重复 Open 应为幂等, 实际报错: %v", err)
	}
	defer second.Close()

	rows, err := second.QueryByKeyword(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatalf("重复打开后查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != id || rows[0].Text != "hello" {
		t.Fatalf("重开库数据丢失: %+v", rows)
	}
}

// TestOpenInvalidPath 不可用路径应返回错误而不是 panic。
func TestOpenInvalidPath(t *testing.T) {
	// 目录不存在时 sqlite 无法建库文件。
	path := filepath.Join(t.TempDir(), "no-such-dir", "history.db")
	s, err := Open(path)
	if err == nil {
		_ = s.Close()
		t.Fatal("路径不可创建时应返回错误")
	}
}

// TestInsertAndFindByKey 去重键包含 text/engine/from/to/from_ocr 五个维度，
// 任一维度不同都不应命中去重。
func TestInsertAndFindByKey(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	id := insertRow(t, s, "hello", "你好", "en", "zh", 7, 0, 1000)

	got, err := s.FindByKey(ctx, "hello", "en", "zh", 7, 0)
	if err != nil {
		t.Fatalf("FindByKey 失败: %v", err)
	}
	if got != id {
		t.Fatalf("FindByKey 应命中 id=%d, 实际 %d", id, got)
	}

	cases := []struct {
		name                   string
		text, fromLang, toLang string
		engineID, fromOCR      int64
	}{
		{"文本不同", "hello!", "en", "zh", 7, 0},
		{"源语言不同", "hello", "ja", "zh", 7, 0},
		{"目标语言不同", "hello", "en", "ja", 7, 0},
		{"引擎不同", "hello", "en", "zh", 8, 0},
		{"来源不同", "hello", "en", "zh", 7, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.FindByKey(ctx, c.text, c.fromLang, c.toLang, c.engineID, c.fromOCR)
			if err != nil {
				t.Fatalf("FindByKey 失败: %v", err)
			}
			if got != 0 {
				t.Fatalf("不同维度不应去重命中, 实际 %d", got)
			}
		})
	}
}

// TestFindByKeyNoRowsIsZero 空库查询必须返回 (0, nil)，
// 而不是把 sql.ErrNoRows 透给调用方（调用方靠 0 判断「无重复」）。
func TestFindByKeyNoRowsIsZero(t *testing.T) {
	s := newStore(t)
	got, err := s.FindByKey(t.Context(), "nothing", "en", "zh", 1, 0)
	if err != nil {
		t.Fatalf("空库 FindByKey 不应报错: %v", err)
	}
	if got != 0 {
		t.Fatalf("空库 FindByKey 应返回 0, 实际 %d", got)
	}
}

// TestQueryByKeyword 关键词匹配 text 或 result；空关键词返回全部；
// 结果按 created_at 倒序；limit/offset 生效。
func TestQueryByKeyword(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	a := insertRow(t, s, "apple", "苹果", "en", "zh", 1, 0, 100)
	b := insertRow(t, s, "banana", "香蕉 apple", "en", "zh", 1, 0, 200)
	c := insertRow(t, s, "cherry", "樱桃", "en", "zh", 1, 0, 300)

	t.Run("空关键词返回全部且按时间倒序", func(t *testing.T) {
		rows, err := s.QueryByKeyword(ctx, "", 10, 0)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("应有 3 条, 实际 %d", len(rows))
		}
		if rows[0].ID != c || rows[1].ID != b || rows[2].ID != a {
			t.Fatalf("排序应为 created_at DESC, 实际顺序 %d,%d,%d",
				rows[0].ID, rows[1].ID, rows[2].ID)
		}
	})

	t.Run("命中原文与译文", func(t *testing.T) {
		rows, err := s.QueryByKeyword(ctx, "apple", 10, 0)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("apple 应命中 2 条（原文 + 译文）, 实际 %d", len(rows))
		}
		seen := map[int64]bool{}
		for _, r := range rows {
			seen[r.ID] = true
		}
		if !seen[a] || !seen[b] {
			t.Fatalf("命中结果不正确: %+v", seen)
		}
	})

	t.Run("未命中返回空切片", func(t *testing.T) {
		rows, err := s.QueryByKeyword(ctx, "zzz", 10, 0)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("应无命中, 实际 %d", len(rows))
		}
	})

	t.Run("分页", func(t *testing.T) {
		page1, err := s.QueryByKeyword(ctx, "", 2, 0)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(page1) != 2 {
			t.Fatalf("第一页应 2 条, 实际 %d", len(page1))
		}
		page2, err := s.QueryByKeyword(ctx, "", 2, 2)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(page2) != 1 || page2[0].ID != a {
			t.Fatalf("第二页应只剩 %d, 实际 %+v", a, page2)
		}
	})
}

// TestQueryByKeywordFields 确认所有列都被正确读回（用于前端展示）。
func TestQueryByKeywordFields(t *testing.T) {
	s := newStore(t)
	id := insertRow(t, s, "src", "dst", "ja", "ko", 42, 1, 12345)

	rows, err := s.QueryByKeyword(t.Context(), "", 1, 0)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应有 1 条, 实际 %d", len(rows))
	}
	got := rows[0]
	want := History{
		ID:        id,
		Text:      "src",
		Result:    "dst",
		FromLang:  "ja",
		ToLang:    "ko",
		EngineID:  42,
		FromOcr:   1,
		CreatedAt: 12345,
	}
	if got != want {
		t.Fatalf("字段读回不一致:\n got=%+v\nwant=%+v", got, want)
	}
}

// TestCountByKeyword 计数必须与关键词过滤一致（前端分页依赖它）。
func TestCountByKeyword(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()
	insertRow(t, s, "apple", "苹果", "en", "zh", 1, 0, 100)
	insertRow(t, s, "banana", "香蕉", "en", "zh", 1, 0, 200)

	if n, err := s.CountByKeyword(ctx, ""); err != nil || n != 2 {
		t.Fatalf("空关键词计数应为 2, 实际 %d (err=%v)", n, err)
	}
	if n, err := s.CountByKeyword(ctx, "banana"); err != nil || n != 1 {
		t.Fatalf("banana 计数应为 1, 实际 %d (err=%v)", n, err)
	}
	if n, err := s.CountByKeyword(ctx, "zzz"); err != nil || n != 0 {
		t.Fatalf("未命中计数应为 0, 实际 %d (err=%v)", n, err)
	}
}

// TestDeleteAndClear 单条删除与清空。
func TestDeleteAndClear(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	a := insertRow(t, s, "a", "A", "en", "zh", 1, 0, 100)
	insertRow(t, s, "b", "B", "en", "zh", 1, 0, 200)

	if err := s.DeleteHistory(ctx, a); err != nil {
		t.Fatalf("DeleteHistory 失败: %v", err)
	}
	rows, err := s.QueryByKeyword(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Text != "b" {
		t.Fatalf("删除后应只剩 b, 实际 %+v", rows)
	}

	if err := s.ClearHistory(ctx); err != nil {
		t.Fatalf("ClearHistory 失败: %v", err)
	}
	if n, err := s.CountByKeyword(ctx, ""); err != nil || n != 0 {
		t.Fatalf("清空后计数应为 0, 实际 %d (err=%v)", n, err)
	}
}

// TestClosedStoreReturnsError 关库后所有操作应返回错误而不是崩溃
// （调用方在退出流程中可能仍持有引用）。
func TestClosedStoreReturnsError(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}

	if _, err := s.InsertHistory(t.Context(), InsertHistoryParams{Text: "x", Result: "y"}); err == nil {
		t.Error("关库后 InsertHistory 应返回错误")
	}
	if _, err := s.QueryByKeyword(t.Context(), "", 10, 0); err == nil {
		t.Error("关库后 QueryByKeyword 应返回错误")
	}
	if _, err := s.CountByKeyword(t.Context(), ""); err == nil {
		t.Error("关库后 CountByKeyword 应返回错误")
	}
	if _, err := s.FindByKey(t.Context(), "x", "", "", 0, 0); err == nil {
		t.Error("关库后 FindByKey 应返回错误")
	}
	if err := s.ClearHistory(t.Context()); err == nil {
		t.Error("关库后 ClearHistory 应返回错误")
	}
}

// TestDuplicateColumnTolerated 直接验证迁移容错分支：对已含 engine_id 列
// 的库再次执行 ALTER 必须被忽略。
func TestDuplicateColumnTolerated(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	// 第一次补列成功，第二次必须报 duplicate column（佐证 Open 的容错是必要的）。
	if _, err := db.Exec("ALTER TABLE history ADD COLUMN engine_id INTEGER NOT NULL DEFAULT 0"); err == nil {
		t.Fatal("重复补列应报错，否则 Open 里的容错分支没有存在意义")
	}

	// Open 对同一路径重复调用不应失败（已验证数据可读）。
	path := filepath.Join(t.TempDir(), "h.db")
	for i := range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("第 %d 次 Open 失败: %v", i+1, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close 失败: %v", err)
		}
	}
}

// TestContextCanceled 传入已取消的 context 应立刻失败。
func TestContextCanceled(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.QueryByKeyword(ctx, "", 10, 0); err == nil {
		t.Error("已取消的 context 应导致查询失败")
	}
	if _, err := s.InsertHistory(ctx, InsertHistoryParams{Text: "x", Result: "y"}); err == nil {
		t.Error("已取消的 context 应导致插入失败")
	}
}
