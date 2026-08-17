// Package httplogstore 提供 HTTP 请求日志的持久化存储。
// 职责：独立的 SQLite 日志库（httplog.db）的全生命周期管理——
// 建库/迁移、Transport 包裹（http_log RoundTripper）、异步入库、
// 定时清理过期日志、关闭。
//
// 约束：httplog.db 为 append-only，仅 INSERT 走常驻连接；DELETE
// 由 Cleanup 用临时连接执行，不阻塞写入。
//
// 设计：包级自包含——Init/Close/WrapTransport 均无需外部传 *sql.DB，
// 调用方只需关心启停，不接触数据库细节。
package httplogstore

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/sqlite"
	"cnb.cool/dtapp/kai/internal/useragent"
	"go.dtapp.net/library/contrib/http_log"
)

// ---------------------------------------------------------------------------
// 包级状态（自包含，不暴露给调用方）
// ---------------------------------------------------------------------------

var (
	conn        *sql.DB
	connDSN     string
	mu          sync.RWMutex
	once        sync.Once
	cleanupDone chan struct{}
)

// ---------------------------------------------------------------------------
// Unicode 解码
// ---------------------------------------------------------------------------

// decodeUnicodeEscapes 检测字节序列中是否包含 \uXXXX 转义序列，如有则解码为正常文本。
// 支持普通 BMP 字符和代理对（如 \uD83D\uDE00 → 😀）。
// 若不含转义序列则原样返回，避免无谓的替换开销。
func decodeUnicodeEscapes(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u`)) {
		return b
	}
	result := make([]byte, 0, len(b))
	remaining := b
	for len(remaining) > 0 {
		idx := bytes.Index(remaining, []byte(`\u`))
		if idx < 0 {
			result = append(result, remaining...)
			break
		}
		result = append(result, remaining[:idx]...)
		if idx+6 > len(remaining) {
			result = append(result, remaining[idx:]...)
			break
		}
		code, err := strconv.ParseUint(string(remaining[idx+2:idx+6]), 16, 16)
		if err != nil {
			result = append(result, remaining[idx:idx+6]...)
			remaining = remaining[idx+6:]
			continue
		}
		// 高代理 (0xD800-0xDBFF)：检查是否为代理对
		if code >= 0xD800 && code <= 0xDBFF && idx+12 <= len(remaining) &&
			bytes.Equal(remaining[idx+6:idx+8], []byte(`\u`)) {
			code2, err2 := strconv.ParseUint(string(remaining[idx+8:idx+12]), 16, 16)
			if err2 == nil && code2 >= 0xDC00 && code2 <= 0xDFFF {
				r := utf16.DecodeRune(rune(code), rune(code2))
				result = append(result, []byte(string(r))...)
				remaining = remaining[idx+12:]
				continue
			}
		}
		result = append(result, []byte(string(rune(code)))...)
		remaining = remaining[idx+6:]
	}
	return result
}

// ---------------------------------------------------------------------------
// 嵌入资源
// ---------------------------------------------------------------------------

//go:embed schema.sql
var schemaSQL string

//go:embed migration.sql
var migrationSQL string

// ---------------------------------------------------------------------------
// Init / Close（包级自包含）
// ---------------------------------------------------------------------------

// Init 初始化 HTTP 请求日志存储。仅 httpLogEnabled 为 true 时打开 DB、
// 建表/迁移、并全局接管 http.DefaultTransport。
// 重复调用安全（sync.Once）；httpLogEnabled=false 时为 no-op。
func Init(dataDir string, httpLogEnabled bool) error {
	var initErr error
	once.Do(func() {
		if !httpLogEnabled {
			return
		}
		connDSN = filepath.Join(dataDir, "httplog.db")
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			initErr = fmt.Errorf(i18n.T("err.httplog_create_dir"), err, err)
			return
		}
		c, err := sql.Open("sqlite3", sqlite.BuildDSN(connDSN))
		if err != nil {
			initErr = fmt.Errorf(i18n.T("err.httplog_open_db"), err, err)
			return
		}
		if _, err := c.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL"); err != nil {
			c.Close()
			initErr = fmt.Errorf(i18n.T("err.httplog_pragma"), err, err)
			return
		}
		// 执行建表 DDL
		if _, err := c.ExecContext(context.Background(), schemaSQL); err != nil {
			c.Close()
			initErr = fmt.Errorf(i18n.T("err.httplog_create_schema"), err, err)
			return
		}
		// 执行迁移脚本
		for stmt := range strings.SplitSeq(migrationSQL, ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := c.Exec(stmt); err != nil {
				if !strings.Contains(err.Error(), "duplicate column") {
					c.Close()
					initErr = fmt.Errorf(i18n.T("err.httplog_migrate"), err, err)
					return
				}
			}
		}
		mu.Lock()
		conn = c
		mu.Unlock()
		// 全局接管 http.DefaultTransport（使第三方库的裸 http.Get 等也记录日志）
		http.DefaultTransport = WrapTransport(http.DefaultTransport)
		http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	})
	return initErr
}

// Close 关闭日志数据库连接并停止定时清理 goroutine。
func Close() error {
	// 先停止清理 goroutine
	mu.Lock()
	if cleanupDone != nil {
		close(cleanupDone)
		cleanupDone = nil
	}
	c := conn
	conn = nil
	mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Transport 包裹
// ---------------------------------------------------------------------------

// WrapTransport 将 base RoundTripper 包裹为带 HTTP 请求日志记录的 RoundTripper，最外层注入全局 User-Agent。
// base 为 nil 时回退 http.DefaultTransport；httplog 未启用时直接返回 useragent 包裹的 base。
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		return useragent.Wrap(base)
	}
	return useragent.Wrap(http_log.NewLoggingRoundTripper(base, &entLogSaver{}, nil))
}

// WrapClient 用 WrapTransport 包裹 client 的 Transport，返回新的 *http.Client。
func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport:     WrapTransport(base),
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}
}

// ---------------------------------------------------------------------------
// 日志处理器（实现 http_log.LogHandler）
// ---------------------------------------------------------------------------

// entLogSaver 实现 http_log.LogHandler 接口，将 HTTP 请求日志写入 httplog.db。
// 无状态——通过包级 conn 读写，不存 db 字段。
type entLogSaver struct{}

// HandleLog 将 HTTP 请求日志数据写入 http_log 表。
func (s *entLogSaver) HandleLog(ctx context.Context, data *http_log.LogData) error {
	if data == nil {
		return nil
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		return nil
	}

	params := InsertHttpLogParams{
		Hostname:          nullableStr(data.Hostname),
		Method:            nullableStr(data.Method),
		Url:               nullableStr(data.URL),
		StatusCode:        nullableInt64(int64(data.StatusCode)),
		ElapseTime:        nullableInt64(data.ElapseTime),
		ProcessElapseTime: nullableInt64(data.ProcessElapseTime),
		IsError:           data.IsError,
		CreatedAt:         time.Now(),
		GoVersion:         nullableStr(data.GoVersion),
		PluginVersion:     nullableStr(data.PluginVersion),
	}

	// 请求/响应头序列化为 JSON 文本
	if data.RequestHeaders != nil {
		if b, err := json.Marshal(data.RequestHeaders); err == nil {
			s := string(b)
			params.RequestHeaders = &s
		}
	}
	if data.ResponseHeaders != nil {
		if b, err := json.Marshal(data.ResponseHeaders); err == nil {
			s := string(b)
			params.ResponseHeaders = &s
		}
	}
	// 请求/响应体仅在非空时写入
	if len(data.RequestBody) > 0 {
		params.RequestBody = decodeUnicodeEscapes(data.RequestBody)
	}
	if len(data.ResponseBody) > 0 {
		params.ResponseBody = decodeUnicodeEscapes(data.ResponseBody)
	}

	if err := New(c).InsertHttpLog(ctx, params); err != nil {
		return fmt.Errorf(i18n.T("err.httplog_insert"), err, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// nullableStr 将 Go 字符串转为 *string（空字符串视为 NULL）。
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableInt64 将 int64 转为 *int64（零值视为 NULL）。
func nullableInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ---------------------------------------------------------------------------
// 清理
// ---------------------------------------------------------------------------

// Cleanup 删除早于 retentionDays 天前的 HTTP 请求日志（基于 created_at）。
// retentionDays <= 0 时表示不清理，直接返回。
// 使用临时建立的独立连接执行 DELETE，不影响常驻 append-only 连接。
func Cleanup(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	mu.RLock()
	dsn := connDSN
	mu.RUnlock()
	if dsn == "" {
		return 0, nil
	}
	cleanupDB, err := sql.Open("sqlite3", sqlite.BuildDSN(dsn))
	if err != nil {
		return 0, fmt.Errorf(i18n.T("err.httplog_cleanup_open"), err, err)
	}
	defer cleanupDB.Close()
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	n, err := New(cleanupDB).DeleteOldHttpLog(context.Background(), cutoff)
	if err != nil {
		return 0, fmt.Errorf(i18n.T("err.httplog_cleanup"), err, err)
	}
	return int(n), nil
}

// StartCleanup 启动定时清理 goroutine，每 1 小时清理一次过期日志。
// goroutine 在 Close() 时自动停止。
func StartCleanup(retentionDays int, logger *slog.Logger) {
	if retentionDays <= 0 {
		return
	}
	mu.RLock()
	dsn := connDSN
	mu.RUnlock()
	if dsn == "" {
		return
	}
	mu.Lock()
	if cleanupDone != nil {
		mu.Unlock()
		return
	}
	cleanupDone = make(chan struct{})
	mu.Unlock()
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n, err := Cleanup(retentionDays)
				if err != nil {
					logger.Error(i18n.T("log.httplog_cleanup_failed"), slog.Any("error", err))
				} else if n > 0 {
					logger.Info(i18n.T("log.httplog_cleaned"), slog.Int("count", n))
				}
			case <-cleanupDone:
				return
			}
		}
	}()
}
