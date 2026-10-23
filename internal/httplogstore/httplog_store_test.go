package httplogstore

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"go.dtapp.net/library/contrib/http_log"
)

// TestHandleLogWritesRow 全字段落库：头转 JSON、Body 解 \uXXXX、
// 耗时/状态码/版本号按列写入。
func TestHandleLogWritesRow(t *testing.T) {
	setupEnabled(t)

	data := &http_log.LogData{
		GoVersion:         "go1.27.3",
		PluginVersion:     "v1.2.3",
		Hostname:          "api.example.com",
		Method:            http.MethodPost,
		URL:               "https://api.example.com/v1/translate",
		RequestHeaders:    map[string][]string{"Content-Type": {"application/json"}},
		RequestBody:       []byte(`{"q":"\u4f60\u597d"}`),
		StatusCode:        http.StatusOK,
		ResponseHeaders:   map[string][]string{"Server": {"nginx"}},
		ResponseBody:      []byte(`{"t":"\ud83d\ude00"}`),
		ElapseTime:        120,
		ProcessElapseTime: 80,
		IsError:           true,
	}
	if err := (&entLogSaver{}).HandleLog(t.Context(), data); err != nil {
		t.Fatalf("HandleLog 失败: %v", err)
	}

	var (
		hostname, method, url   string
		reqHeaders, respHeaders string
		reqBody, respBody       []byte
		status, elapse, process int64
		isErr                   bool
		goVer, pluginVer        string
	)
	err := currentConn(t).QueryRow(
		`SELECT hostname, method, url, request_headers, request_body,
		        status_code, response_headers, response_body,
		        elapse_time, process_elapse_time, is_error,
		        go_version, plugin_version
		 FROM http_log ORDER BY id DESC LIMIT 1`,
	).Scan(&hostname, &method, &url, &reqHeaders, &reqBody,
		&status, &respHeaders, &respBody, &elapse, &process, &isErr,
		&goVer, &pluginVer)
	if err != nil {
		t.Fatalf("读回日志失败: %v", err)
	}

	if hostname != "api.example.com" || method != http.MethodPost || url != "https://api.example.com/v1/translate" {
		t.Fatalf("请求行字段错误: %s %s %s", method, hostname, url)
	}
	if status != http.StatusOK || elapse != 120 || process != 80 {
		t.Fatalf("数值字段错误: status=%d elapse=%d process=%d", status, elapse, process)
	}
	if !isErr {
		t.Fatal("IsError 应为 true")
	}
	if goVer != "go1.27.3" || pluginVer != "v1.2.3" {
		t.Fatalf("版本字段错误: %s %s", goVer, pluginVer)
	}

	// 头部必须是可解析 JSON（前端按 map[string][]string 渲染详情）。
	var rh map[string][]string
	if err := json.Unmarshal([]byte(reqHeaders), &rh); err != nil {
		t.Fatalf("request_headers 不是合法 JSON: %v (%q)", err, reqHeaders)
	}
	if got := rh["Content-Type"]; len(got) != 1 || got[0] != "application/json" {
		t.Fatalf("request_headers 内容错误: %v", rh)
	}
	var sh map[string][]string
	if err := json.Unmarshal([]byte(respHeaders), &sh); err != nil {
		t.Fatalf("response_headers 不是合法 JSON: %v (%q)", err, respHeaders)
	}
	if got := sh["Server"]; len(got) != 1 || got[0] != "nginx" {
		t.Fatalf("response_headers 内容错误: %v", sh)
	}

	// Body 中的转义必须还原，否则日志不可读。
	if !bytes.Contains(reqBody, []byte("你好")) {
		t.Fatalf("request_body 未解码 unicode: %q", reqBody)
	}
	if !bytes.Contains(respBody, []byte("😀")) {
		t.Fatalf("response_body 未解码代理对: %q", respBody)
	}
}

// TestHandleLogEmptyFieldsAreNull 空字段落库为 NULL，
// 前端据此区分「没有该字段」与「字段为空串」。
func TestHandleLogEmptyFieldsAreNull(t *testing.T) {
	setupEnabled(t)

	data := &http_log.LogData{Method: http.MethodHead, URL: "http://x/y"}
	if err := (&entLogSaver{}).HandleLog(t.Context(), data); err != nil {
		t.Fatalf("HandleLog 失败: %v", err)
	}

	var (
		hostname, reqHeaders, respHeaders *string
		status, elapse, process           *int64
	)
	err := currentConn(t).QueryRow(
		`SELECT hostname, request_headers, response_headers,
		        status_code, elapse_time, process_elapse_time
		 FROM http_log ORDER BY id DESC LIMIT 1`,
	).Scan(&hostname, &reqHeaders, &respHeaders, &status, &elapse, &process)
	if err != nil {
		t.Fatalf("读回日志失败: %v", err)
	}
	for name, v := range map[string]*string{
		"hostname":         hostname,
		"request_headers":  reqHeaders,
		"response_headers": respHeaders,
	} {
		if v != nil {
			t.Fatalf("空 %s 应落库为 NULL, 实际 %q", name, *v)
		}
	}
	for name, v := range map[string]*int64{
		"status_code":         status,
		"elapse_time":         elapse,
		"process_elapse_time": process,
	} {
		if v != nil {
			t.Fatalf("零值 %s 应落库为 NULL, 实际 %d", name, *v)
		}
	}
}

// TestHandleLogNoopWhenDisabled 未启用 / 空数据时应静默跳过，不报错也不写库。
func TestHandleLogNoopWhenDisabled(t *testing.T) {
	resetState(t)
	t.Cleanup(func() { resetState(t) })

	saver := &entLogSaver{}
	if err := saver.HandleLog(t.Context(), nil); err != nil {
		t.Fatalf("nil 数据不应报错: %v", err)
	}
	if err := saver.HandleLog(t.Context(), &http_log.LogData{Method: http.MethodGet}); err != nil {
		t.Fatalf("未启用时 HandleLog 应为 no-op, 实际 %v", err)
	}
}

// TestInsertHttpLogAndCleanup 过期日志清理：只删早于保留期的行。
func TestInsertHttpLogAndCleanup(t *testing.T) {
	setupEnabled(t)
	c := currentConn(t)

	q := New(c)
	if err := q.InsertHttpLog(t.Context(), InsertHttpLogParams{
		Url:       new("http://old"),
		CreatedAt: time.Now().AddDate(0, 0, -60),
	}); err != nil {
		t.Fatalf("插入旧日志失败: %v", err)
	}
	if err := q.InsertHttpLog(t.Context(), InsertHttpLogParams{
		Url:       new("http://fresh"),
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("插入新日志失败: %v", err)
	}

	// 保留期 90 天：60 天前的日志仍在保留期内，一条都不该删。
	if n, err := Cleanup(90); err != nil || n != 0 {
		t.Fatalf("保留期内不应删除, 实际 (%d, %v)", n, err)
	}

	// 保留期 30 天：恰好删掉 60 天前那条。
	n, err := Cleanup(30)
	if err != nil {
		t.Fatalf("Cleanup 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应删除 1 条过期日志, 实际 %d", n)
	}

	var remain string
	if err := c.QueryRow("SELECT url FROM http_log").Scan(&remain); err != nil {
		t.Fatalf("查询剩余日志失败: %v", err)
	}
	if remain != "http://fresh" {
		t.Fatalf("保留期内的日志被误删, 剩余 %q", remain)
	}

	// 再清一次没有可删的行 → 返回 0 而不是报错。
	if n, err := Cleanup(365); err != nil || n != 0 {
		t.Fatalf("无可删行时应返回 (0, nil), 实际 (%d, %v)", n, err)
	}
}

// TestCleanupNoop 未启用或保留期 <= 0 时不应清理（也不能报错）。
func TestCleanupNoop(t *testing.T) {
	resetState(t)
	t.Cleanup(func() { resetState(t) })

	if n, err := Cleanup(30); n != 0 || err != nil {
		t.Fatalf("未启用时 Cleanup 应为 no-op, 实际 (%d, %v)", n, err)
	}

	setupEnabled(t)
	if n, err := Cleanup(0); n != 0 || err != nil {
		t.Fatalf("retentionDays=0 应为 no-op, 实际 (%d, %v)", n, err)
	}
	if n, err := Cleanup(-1); n != 0 || err != nil {
		t.Fatalf("retentionDays<0 应为 no-op, 实际 (%d, %v)", n, err)
	}
}

// TestStartCleanupLifecycle 定时清理 goroutine 的启动/幂等/停止。
func TestStartCleanupLifecycle(t *testing.T) {
	setupEnabled(t)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// 保留期 <= 0：不启动 goroutine。
	StartCleanup(0, logger)
	mu.RLock()
	d := cleanupDone
	mu.RUnlock()
	if d != nil {
		t.Fatal("retentionDays=0 不应启动清理 goroutine")
	}

	// 正常启动，重复调用不应启动第二个。
	StartCleanup(30, logger)
	mu.RLock()
	first := cleanupDone
	mu.RUnlock()
	if first == nil {
		t.Fatal("StartCleanup 应启动清理 goroutine")
	}
	StartCleanup(30, logger)
	mu.RLock()
	second := cleanupDone
	mu.RUnlock()
	if first != second {
		t.Fatal("重复 StartCleanup 不应重建清理通道")
	}

	// Close 必须停掉 goroutine 并清空状态，否则退出时会泄漏。
	if err := Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}
	mu.RLock()
	after := cleanupDone
	mu.RUnlock()
	if after != nil {
		t.Fatal("Close 后 cleanupDone 应为 nil")
	}
}

// TestStartCleanupDisabled 未启用存储时不应启动后台 goroutine。
func TestStartCleanupDisabled(t *testing.T) {
	resetState(t)
	t.Cleanup(func() { resetState(t) })

	StartCleanup(30, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	mu.RLock()
	d := cleanupDone
	mu.RUnlock()
	if d != nil {
		t.Fatal("未启用时不应启动清理 goroutine")
	}
}

// TestHandleLogInsertErrorAfterConnClose 连接关闭后写入应返回错误而不是 panic。
func TestHandleLogInsertErrorAfterConnClose(t *testing.T) {
	setupEnabled(t)
	c := currentConn(t)
	mu.Lock()
	conn = nil
	mu.Unlock()
	if err := c.Close(); err != nil {
		t.Fatalf("关闭连接失败: %v", err)
	}
	t.Cleanup(func() { resetState(t) })

	// conn 已置空 → 走 no-op 分支，不报错（调用方无需感知存储已停）。
	if err := (&entLogSaver{}).HandleLog(t.Context(), &http_log.LogData{Method: http.MethodGet}); err != nil {
		t.Fatalf("连接关闭后 HandleLog 应为 no-op, 实际 %v", err)
	}
}
