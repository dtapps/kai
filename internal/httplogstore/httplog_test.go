package httplogstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------------------

// resetState 重置包级状态。
//
// Init 内部用 sync.Once 保证「一个进程只接管一次 http.DefaultTransport」，
// 但测试需要分别覆盖「未启用」「已启用」「初始化失败」三条路径，
// 因此每个用例开头显式重置一次（用例串行执行，无并发风险）。
func resetState(t *testing.T) {
	t.Helper()
	mu.Lock()
	once = sync.Once{}
	connDSN = ""
	if cleanupDone != nil {
		close(cleanupDone)
		cleanupDone = nil
	}
	c := conn
	conn = nil
	mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// setupEnabled 用临时目录把存储切到「已启用」，用例结束自动收尾
// （关闭连接、还原全局 http.DefaultTransport / DefaultClient）。
func setupEnabled(t *testing.T) string {
	t.Helper()
	origTransport, origClient := http.DefaultTransport, http.DefaultClient
	t.Cleanup(func() {
		http.DefaultTransport = origTransport
		http.DefaultClient = origClient
	})
	resetState(t)
	dir := t.TempDir()
	if err := Init(dir, true); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	t.Cleanup(func() {
		if err := Close(); err != nil {
			t.Errorf("Close 失败: %v", err)
		}
		resetState(t)
	})
	return dir
}

// currentConn 取当前包级连接（断言非 nil）。
func currentConn(t *testing.T) *sql.DB {
	t.Helper()
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		t.Fatal("包级 conn 未初始化")
	}
	return c
}

// currentDSN 取当前数据源路径。
func currentDSN() string {
	mu.RLock()
	defer mu.RUnlock()
	return connDSN
}

// ---------------------------------------------------------------------------
// decodeUnicodeEscapes
// ---------------------------------------------------------------------------

// TestDecodeUnicodeEscapes 覆盖 \uXXXX 解码的关键分支：
// 无转义快速返回、BMP 字符、UTF-16 代理对、截断序列、非法十六进制。
func TestDecodeUnicodeEscapes(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"无转义原样返回", `{"text":"hello 世界"}`, `{"text":"hello 世界"}`},
		{"单个 BMP 转义", `\u4f60`, "你"},
		{"连续 BMP 转义", `\u4f60\u597d`, "你好"},
		{"代理对解码为单码点", `\ud83d\ude00`, "😀"},
		{"混合文本", `a\u4e2db`, "a中b"},
		{"末尾截断的转义原样保留", `x\u12`, `x\u12`},
		{"非法十六进制原样保留", `\uZZZZ`, `\uZZZZ`},
		{"空输入", ``, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(decodeUnicodeEscapes([]byte(c.in))); got != c.want {
				t.Fatalf("decodeUnicodeEscapes(%q) = %q, 期望 %q", c.in, got, c.want)
			}
		})
	}

	// 孤立代理不能 panic，输出必须是合法 UTF-8（退化字符可接受）。
	for _, in := range []string{`\ud83d`, `\ud83d\u4e2d`, `\ud83dz`} {
		if out := decodeUnicodeEscapes([]byte(in)); !utf8.Valid(out) {
			t.Fatalf("孤立代理输入 %q 输出非法 UTF-8: %q", in, out)
		}
	}

	// 无转义时走零拷贝快速路径（返回同一底层数组）。
	same := []byte("plain ascii")
	if got := decodeUnicodeEscapes(same); &got[0] != &same[0] {
		t.Fatal("无转义输入不应重新分配内存")
	}

	// 解码后 JSON 仍须合法（否则前端解析日志会全线失败）。
	if !json.Valid(decodeUnicodeEscapes([]byte(`{"q":"\u4f60\u597d"}`))) {
		t.Fatal("解码 \\uXXXX 后 JSON 应仍然合法")
	}
}

// ---------------------------------------------------------------------------
// nullable 辅助
// ---------------------------------------------------------------------------

// TestNullableStr 空串必须落库为 NULL，非空串保留原值。
func TestNullableStr(t *testing.T) {
	if got := nullableStr(""); got != nil {
		t.Fatalf("空串应转 nil, 实际 %q", *got)
	}
	if got := nullableStr("https://example.com"); got == nil || *got != "https://example.com" {
		t.Fatalf("非空串转换错误: %v", got)
	}
}

// TestNullableInt64 零值必须落库为 NULL（区分「耗时 0ms」与「无耗时」）。
func TestNullableInt64(t *testing.T) {
	if got := nullableInt64(0); got != nil {
		t.Fatalf("0 应转 nil, 实际 %d", *got)
	}
	if got := nullableInt64(120); got == nil || *got != 120 {
		t.Fatalf("非零值转换错误: %v", got)
	}
	if got := nullableInt64(-1); got == nil || *got != -1 {
		t.Fatalf("负值不应被当作零值丢弃: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Init / Close
// ---------------------------------------------------------------------------

// TestInitDisabledIsNoop 未启用时不得建库、不得接管 DefaultTransport。
func TestInitDisabledIsNoop(t *testing.T) {
	origTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = origTransport })
	resetState(t)
	t.Cleanup(func() { resetState(t) })

	dir := t.TempDir()
	if err := Init(dir, false); err != nil {
		t.Fatalf("未启用时 Init 不应报错: %v", err)
	}
	mu.RLock()
	c, dsn := conn, connDSN
	mu.RUnlock()
	if c != nil || dsn != "" {
		t.Fatalf("未启用时不应持有连接, conn=%v dsn=%q", c, dsn)
	}
	if _, err := os.Stat(filepath.Join(dir, "httplog.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("未启用时不应创建数据库文件, err=%v", err)
	}
	if http.DefaultTransport != origTransport {
		t.Fatal("未启用时不应接管 http.DefaultTransport")
	}
	if WrapTransport(nil) == nil {
		t.Fatal("WrapTransport(nil) 不应返回 nil")
	}
	// 重复 Init 必须安全：sync.Once 已消费，后一次调用无副作用。
	if err := Init(dir, true); err != nil {
		t.Fatalf("重复 Init 不应报错: %v", err)
	}
	if currentDSN() != "" {
		t.Fatal("重复 Init 应被 sync.Once 忽略")
	}
}

// TestInitEnabledCreatesDatabase 启用后必须建库并接管默认 Transport。
func TestInitEnabledCreatesDatabase(t *testing.T) {
	origTransport := http.DefaultTransport
	dir := setupEnabled(t)

	dbPath := filepath.Join(dir, "httplog.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("启用后应创建 %s: %v", dbPath, err)
	}
	if currentDSN() != dbPath {
		t.Fatalf("connDSN 应为 %q, 实际 %q", dbPath, currentDSN())
	}
	var n int64
	if err := currentConn(t).QueryRow("SELECT COUNT(*) FROM http_log").Scan(&n); err != nil {
		t.Fatalf("http_log 表不可查询: %v", err)
	}
	if n != 0 {
		t.Fatalf("新库应为空, 实际 %d 条", n)
	}
	if http.DefaultTransport == origTransport {
		t.Fatal("启用后应接管 http.DefaultTransport")
	}
	if http.DefaultClient == nil || http.DefaultClient.Transport != http.DefaultTransport {
		t.Fatal("启用后 DefaultClient 应使用新的 DefaultTransport")
	}
}

// TestInitCreateDirFailure 无法建目录时必须返回错误，且不留半初始化状态。
func TestInitCreateDirFailure(t *testing.T) {
	resetState(t)
	t.Cleanup(func() { resetState(t) })

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	if err := Init(blocker, true); err == nil {
		t.Fatal("dataDir 不可创建时应返回错误")
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c != nil {
		t.Fatal("初始化失败后不应留下连接")
	}
}

// TestCloseIdempotent Close 重复调用必须安全（退出流程可能多处调用）。
func TestCloseIdempotent(t *testing.T) {
	setupEnabled(t)
	if err := Close(); err != nil {
		t.Fatalf("首次 Close 失败: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("重复 Close 应返回 nil, 实际 %v", err)
	}
}

// ---------------------------------------------------------------------------
// WrapTransport / WrapClient
// ---------------------------------------------------------------------------

// TestWrapClientPreservesOptions 包裹后必须保留超时 / Cookie / 重定向策略，
// 否则调用方的手工配置会被静默丢弃。
func TestWrapClientPreservesOptions(t *testing.T) {
	setupEnabled(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("创建 cookie jar 失败: %v", err)
	}
	orig := &http.Client{Timeout: 3 * time.Second, Jar: jar}
	got := WrapClient(orig)
	if got == orig {
		t.Fatal("WrapClient 应返回新的 *http.Client")
	}
	if got.Timeout != orig.Timeout {
		t.Fatalf("Timeout 未保留: %v", got.Timeout)
	}
	if got.Jar != jar {
		t.Fatal("Jar 未保留")
	}
	if got.CheckRedirect != nil {
		t.Fatal("原 CheckRedirect 为 nil 时不应被替换")
	}
	if got.Transport == nil {
		t.Fatal("包裹后 Transport 不应为 nil")
	}
	if nc := WrapClient(nil); nc == nil || nc.Transport == nil {
		t.Fatal("WrapClient(nil) 应返回可用的 client")
	}
}

// TestWrapTransportLogsEndToEnd 真实发一次请求，日志应异步落到 sqlite。
func TestWrapTransportLogsEndToEnd(t *testing.T) {
	setupEnabled(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	resp, err := WrapClient(&http.Client{Timeout: 5 * time.Second}).Get(srv.URL + "/ping?q=1")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("状态码应为 201, 实际 %d", resp.StatusCode)
	}

	// http_log 的 Handler 由 goroutine 异步调用，需要轮询等待落库。
	c := currentConn(t)
	deadline := time.Now().Add(5 * time.Second)
	var (
		method, url string
		statusCode  int64
		isErr       bool
	)
	for {
		err := c.QueryRow(
			`SELECT method, url, COALESCE(status_code,0), is_error FROM http_log ORDER BY id DESC LIMIT 1`,
		).Scan(&method, &url, &statusCode, &isErr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待日志落库超时: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if method != http.MethodGet {
		t.Fatalf("method 应为 GET, 实际 %q", method)
	}
	if !strings.HasSuffix(url, "/ping?q=1") {
		t.Fatalf("url 记录不正确: %q", url)
	}
	if statusCode != http.StatusCreated {
		t.Fatalf("status_code 应为 201, 实际 %d", statusCode)
	}
	if isErr {
		t.Fatal("2xx 响应不应标记为 error")
	}
}
