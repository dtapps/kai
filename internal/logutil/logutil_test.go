package logutil

import (
	"compress/gzip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":        slog.LevelDebug,
		"DEBUG":        slog.LevelDebug,
		"  Debug  ":    slog.LevelDebug,
		"warn":         slog.LevelWarn,
		"warning":      slog.LevelWarn,
		"error":        slog.LevelError,
		"info":         slog.LevelInfo,
		"":             slog.LevelInfo,
		"trace":        slog.LevelInfo, // 不支持的值回退 info
		"not-a-level!": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := ParseLevel(in); got != want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewRotatorCreatesFileAndHandler(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	r, err := NewRotator(dir, slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	if r.Handler() == nil {
		t.Fatal("Handler() 不应为 nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "kai.log")); err != nil {
		t.Fatalf("kai.log 未创建: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	r, err := NewRotator(t.TempDir(), slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("首次 Close 失败: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("重复 Close 应无错误，实际 %v", err)
	}
}

// 昨日遗留的 kai.log 应被归档为 kai-YYYY-MM-DD.log，并重新打开当日文件。
func TestRotateArchivesStaleLog(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "kai.log")

	if err := os.WriteFile(current, []byte("昨天的日志\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	if err := os.Chtimes(current, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}

	r, err := NewRotator(dir, slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	archive := filepath.Join(dir, "kai-"+yesterday.Format("2006-01-02")+".log")
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("归档文件未生成: %v", err)
	}
	if !strings.Contains(string(data), "昨天的日志") {
		t.Fatalf("归档内容不符: %q", string(data))
	}

	// 当日 kai.log 应当被重新创建（且是空的）。
	if info, err := os.Stat(current); err != nil {
		t.Fatalf("当日 kai.log 未重建: %v", err)
	} else if info.Size() != 0 {
		t.Fatalf("重建的 kai.log 应为空，实际 %d 字节", info.Size())
	}
}

func TestRotateCompressesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "kai.log")

	if err := os.WriteFile(current, []byte("需要压缩的内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -3)
	if err := os.Chtimes(current, old, old); err != nil {
		t.Fatal(err)
	}

	r, err := NewRotator(dir, slog.LevelInfo, 7, true)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	gz := filepath.Join(dir, "kai-"+old.Format("2006-01-02")+".log.gz")
	f, err := os.Open(gz)
	if err != nil {
		t.Fatalf("未生成 .gz 归档: %v", err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("归档不是合法 gzip: %v", err)
	}
	defer zr.Close()

	content, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("读取 gzip 失败: %v", err)
	}
	if !strings.Contains(string(content), "需要压缩的内容") {
		t.Fatalf("解压内容不符: %q", string(content))
	}

	// 压缩成功后源文件应被删除。
	plain := filepath.Join(dir, "kai-"+old.Format("2006-01-02")+".log")
	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Fatalf("压缩后原始 .log 应被删除，实际仍存在（err=%v）", err)
	}
}

// 同一天多次启动不能互相覆盖：第二个归档要加序号。
func TestRotateAvoidsOverwrite(t *testing.T) {
	dir := t.TempDir()
	yesterday := time.Now().AddDate(0, 0, -1)
	day := yesterday.Format("2006-01-02")

	// 已存在同名归档。
	if err := os.WriteFile(filepath.Join(dir, "kai-"+day+".log"), []byte("旧归档"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "kai.log")
	if err := os.WriteFile(current, []byte("新的一天"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(current, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}

	r, err := NewRotator(dir, slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	numbered := filepath.Join(dir, "kai-"+day+".1.log")
	if _, err := os.Stat(numbered); err != nil {
		t.Fatalf("应生成带序号的归档 %s: %v", filepath.Base(numbered), err)
	}

	// 原归档内容不能被覆盖。
	oldData, err := os.ReadFile(filepath.Join(dir, "kai-"+day+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(oldData) != "旧归档" {
		t.Fatalf("已有归档被覆盖，实际 %q", string(oldData))
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "kai-2026-01-01.log")

	if got := uniquePath(base); got != base {
		t.Fatalf("不存在时应原样返回，got %q", got)
	}

	if err := os.WriteFile(base, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := uniquePath(base), filepath.Join(dir, "kai-2026-01-01.1.log"); got != want {
		t.Fatalf("uniquePath() = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(dir, "kai-2026-01-01.1.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := uniquePath(base), filepath.Join(dir, "kai-2026-01-01.2.log"); got != want {
		t.Fatalf("序号应递增: got %q, want %q", got, want)
	}
}

func TestCleanupRemovesExpiredArchives(t *testing.T) {
	dir := t.TempDir()

	mustWrite := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("kai-2000-01-01.log", "太老了")
	mustWrite("kai-2000-01-02.log.gz", "太老了")
	mustWrite("kai-"+time.Now().Format("2006-01-02")+".log", "今天的")
	mustWrite("unrelated.log", "无关文件")
	mustWrite("kai-not-a-date.log", "文件名非法日期")

	r, err := NewRotator(dir, slog.LevelInfo, 3, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	r.UpdateRetention(3, false)

	for _, gone := range []string{"kai-2000-01-01.log", "kai-2000-01-02.log.gz"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("过期归档 %s 应被删除（err=%v）", gone, err)
		}
	}
	for _, kept := range []string{"kai-" + time.Now().Format("2006-01-02") + ".log", "unrelated.log", "kai-not-a-date.log"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("文件 %s 不应被删除: %v", kept, err)
		}
	}
}

func TestCleanupSkippedWhenRetentionNonPositive(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "kai-2000-01-01.log")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := NewRotator(dir, slog.LevelInfo, 0, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	r.UpdateRetention(0, false)
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("retention<=0 表示不清理，文件却没了: %v", err)
	}
}

func TestSetLevelRebuildsHandler(t *testing.T) {
	r, err := NewRotator(t.TempDir(), slog.LevelError, 7, false)
	if err != nil {
		t.Fatalf("NewRotator 失败: %v", err)
	}
	defer r.Close()

	before := r.Handler()
	r.SetLevel(slog.LevelDebug)
	after := r.Handler()

	if before == after {
		t.Fatal("SetLevel 后 handler 应被重建")
	}
	if !after.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("切到 debug 后应放行 debug 日志")
	}
}

func TestDayFile(t *testing.T) {
	r := &Rotator{dir: "x"}
	day := time.Date(2026, 8, 12, 15, 4, 5, 0, time.UTC)
	if got, want := r.dayFile(day), "kai-2026-08-12.log"; got != want {
		t.Fatalf("dayFile() = %q, want %q", got, want)
	}
}

func TestFrontendWriterBasics(t *testing.T) {
	dir := t.TempDir()

	fw, err := NewFrontendWriter(dir, slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewFrontendWriter 失败: %v", err)
	}
	if fw.Handler() == nil {
		t.Fatal("Handler() 不应为 nil")
	}

	n, err := fw.Write([]byte("前端日志一行\n"))
	if err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if want := len("前端日志一行\n"); n != want {
		t.Fatalf("Write 返回 %d 字节，want %d", n, want)
	}

	data, err := os.ReadFile(filepath.Join(dir, "frontend.log"))
	if err != nil {
		t.Fatalf("frontend.log 未生成: %v", err)
	}
	if !strings.Contains(string(data), "前端日志一行") {
		t.Fatalf("写入内容未落盘: %q", string(data))
	}

	fw.SetLevel(slog.LevelDebug)
}

// file 为 nil 时写入应静默丢弃，不 panic、不计错。
func TestFrontendWriterNilFile(t *testing.T) {
	fw := &FrontendWriter{}
	n, err := fw.Write([]byte("abc"))
	if err != nil {
		t.Fatalf("nil file 写入不应报错: %v", err)
	}
	if n != 3 {
		t.Fatalf("应报告全部写入，got %d", n)
	}
}

func TestFrontendWriterRotatesStaleLog(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "frontend.log")
	if err := os.WriteFile(current, []byte("昨天的前端日志"), 0o644); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	if err := os.Chtimes(current, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFrontendWriter(dir, slog.LevelInfo, 7, false)
	if err != nil {
		t.Fatalf("NewFrontendWriter 失败: %v", err)
	}

	if fw.Handler() == nil {
		t.Fatal("滚动后 handler 不应为 nil")
	}
	archive := filepath.Join(dir, "frontend-"+yesterday.Format("2006-01-02")+".log")
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("前端日志未归档: %v", err)
	}
}

func TestFrontendLogServiceLevels(t *testing.T) {
	dir := t.TempDir()
	fw, err := NewFrontendWriter(dir, slog.LevelDebug, 7, false)
	if err != nil {
		t.Fatalf("NewFrontendWriter 失败: %v", err)
	}
	svc := NewFrontendLogService(fw)

	// 四种等级都要能落盘（debug 依赖级别放行）。
	svc.FrontendLog("debug", "调试消息")
	svc.FrontendLog("warn", "警告消息")
	svc.FrontendLog("error", "错误消息")
	svc.FrontendLog("info", "信息消息")
	svc.FrontendLog("unknown-level", "未知等级消息")

	data, err := os.ReadFile(filepath.Join(dir, "frontend.log"))
	if err != nil {
		t.Fatalf("frontend.log 读取失败: %v", err)
	}
	text := string(data)
	for _, want := range []string{"调试消息", "警告消息", "错误消息", "信息消息", "未知等级消息"} {
		if !strings.Contains(text, want) {
			t.Fatalf("日志中缺少 %q，实际内容：\n%s", want, text)
		}
	}

	// SetLevel 后 debug 应被过滤掉。
	svc.SetLevel(slog.LevelError)
	svc.FrontendLog("debug", "切换等级后的调试消息")

	after, err := os.ReadFile(filepath.Join(dir, "frontend.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "切换等级后的调试消息") {
		t.Fatalf("error 级别不应写入 debug 日志：\n%s", string(after))
	}
}

func TestGzipFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.log")
	dst := filepath.Join(dir, "a.log.gz")
	if err := os.WriteFile(src, []byte("压缩我"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := gzipFile(src, dst); err != nil {
		t.Fatalf("gzipFile 失败: %v", err)
	}

	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	content, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "压缩我" {
		t.Fatalf("解压内容 = %q", string(content))
	}

	if err := gzipFile(filepath.Join(dir, "missing.log"), dst); err == nil {
		t.Fatal("源文件不存在时应返回错误")
	}
}
