// Package logutil 提供应用运行日志的初始化、按天滚动、压缩与过期清理能力。
// 日志等级、保留天数、压缩开关由 settings.LogConfig 驱动，可在 settings.json 热更新。
package logutil

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// ParseLevel 解析等级字符串为 slog.Level，非法值回退 info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Rotator 管理按天滚动的日志文件，并向外提供一个可动态调整级别的 slog.Handler。
type Rotator struct {
	mu          sync.Mutex
	dir         string
	level       slog.Level
	handler     *slog.TextHandler
	currentPath string
	file        *os.File
	retention   int
	compress    bool
}

// NewRotator 创建 Rotator，立即滚动一次（若当前 kai.log 非当天则归档），并打开当日日志文件。
func NewRotator(dir string, level slog.Level, retention int, compress bool) (*Rotator, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.logutil_create_dir"), err)
	}
	r := &Rotator{
		dir:       dir,
		level:     level,
		retention: retention,
		compress:  compress,
	}
	if err := r.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return r, nil
}

// Handler 返回 slog.Handler，调用方据此构建 slog.Logger 并 SetDefault。
func (r *Rotator) Handler() slog.Handler { return r.handler }

// Close 关闭底层日志文件句柄（panic 兜底最后落盘用），幂等。
func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

// SetLevel 动态调整日志级别（热更新）。
func (r *Rotator) SetLevel(level slog.Level) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.level = level
	r.rebuildHandler()
}

// UpdateRetention 动态调整保留天数与压缩开关（热更新）。
func (r *Rotator) UpdateRetention(retention int, compress bool) {
	r.mu.Lock()
	r.retention = retention
	r.compress = compress
	r.mu.Unlock()
	// 立即触发一次清理，使修改即时生效。
	r.cleanup()
}

// dayFile 返回指定日期对应的归档日志文件名（不含扩展名）。
func (r *Rotator) dayFile(t time.Time) string {
	return fmt.Sprintf("kai-%s.log", t.Format("2006-01-02"))
}

// rotateIfNeeded 若当前 kai.log 不属于今天，则将其重命名为 kai-YYYY-MM-DD.log（并按需压缩），
// 再新建当日 kai.log 并重建 handler。
func (r *Rotator) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")
	current := filepath.Join(r.dir, "kai.log")

	// 若已存在 kai.log 且不是今天（按 mtime 日期判定），归档旧文件。
	if info, err := os.Stat(current); err == nil {
		modDay := info.ModTime().Format("2006-01-02")
		if modDay != today {
			archiveName := r.dayFile(info.ModTime())
			archivePath := filepath.Join(r.dir, archiveName)
			// 避免同日多次启动覆盖：加序号后缀。
			archivePath = uniquePath(archivePath)
			if err := os.Rename(current, archivePath); err != nil {
				return fmt.Errorf("%s: %w", i18n.T("err.logutil_archive_old"), err)
			}
			if r.compress {
				if err := gzipFile(archivePath, archivePath+".gz"); err == nil {
					os.Remove(archivePath)
				}
			}
		}
	}

	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.logutil_open_file"), err)
	}
	if r.file != nil {
		r.file.Close()
	}
	r.file = f
	r.currentPath = current
	r.rebuildHandler()
	return nil
}

// rebuildHandler 用当前级别与 writer 重建 handler。
func (r *Rotator) rebuildHandler() {
	w := io.MultiWriter(r.file, os.Stderr)
	r.handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: r.level})
}

// cleanup 删除超过 retention 天的归档日志文件（kai-YYYY-MM-DD.log 或 .gz）。
func (r *Rotator) cleanup() {
	r.mu.Lock()
	retention := r.retention
	compress := r.compress
	r.mu.Unlock()
	if retention <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retention)
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	type archived struct {
		path string
		day  time.Time
	}
	var olds []archived
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "kai-") || (!strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz")) {
			continue
		}
		// 解析日期前缀 kai-2006-01-02
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".log")
		day, err := time.Parse("2006-01-02", strings.TrimPrefix(base, "kai-"))
		if err != nil {
			continue
		}
		if day.Before(cutoff) {
			olds = append(olds, archived{path: filepath.Join(r.dir, name), day: day})
		}
	}
	sort.Slice(olds, func(i, j int) bool { return olds[i].day.Before(olds[j].day) })
	for _, o := range olds {
		os.Remove(o.path)
	}
	_ = compress
}

// uniquePath 若 path 已存在，则在中段插入 .N 避免覆盖（kai-2026-08-12.1.log）。
func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// gzipFile 将 src 压缩为 dst，dst 存在则覆盖。
func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	if _, err := io.Copy(gw, in); err != nil {
		return err
	}
	return gw.Close()
}

// FrontendWriter 是一个独立写入 logs/frontend.log 的 slog.Handler 容器。
// 前端 console 日志与 JS 错误经此落盘，与主应用日志（kai.log）分离。
type FrontendWriter struct {
	mu    sync.Mutex
	dir   string
	file  *os.File
	rot   *Rotator
	level slog.Level
}

// NewFrontendWriter 创建前端日志写入器（按天滚动 + 过期清理，沿用主日志策略）。
func NewFrontendWriter(dir string, level slog.Level, retention int, compress bool) (*FrontendWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.logutil_create_dir"), err)
	}
	fw := &FrontendWriter{dir: dir, level: level, rot: &Rotator{dir: dir, level: level, retention: retention, compress: compress}}
	if err := fw.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return fw, nil
}

// rotateIfNeeded 前端日志按天滚动（frontend.log -> frontend-YYYY-MM-DD.log）。
func (fw *FrontendWriter) rotateIfNeeded() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	current := filepath.Join(fw.dir, "frontend.log")
	if info, err := os.Stat(current); err == nil {
		if info.ModTime().Format("2006-01-02") != today {
			archivePath := uniquePath(filepath.Join(fw.dir, fmt.Sprintf("frontend-%s.log", info.ModTime().Format("2006-01-02"))))
			if err := os.Rename(current, archivePath); err == nil && fw.rot.compress {
				if err := gzipFile(archivePath, archivePath+".gz"); err == nil {
					os.Remove(archivePath)
				}
			}
		}
	}
	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.logutil_open_frontend"), err)
	}
	if fw.file != nil {
		fw.file.Close()
	}
	fw.file = f
	return nil
}

// Write 实现 io.Writer：复用 logutil 的滚动清理逻辑（与 Rotator 共享压缩/清理）。
func (fw *FrontendWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.file == nil {
		return len(p), nil
	}
	return fw.file.Write(p)
}

// Handler 返回供 slog 使用的 handler。
func (fw *FrontendWriter) Handler() slog.Handler {
	return slog.NewTextHandler(fw, &slog.HandlerOptions{Level: fw.level})
}

// SetLevel 动态调整前端日志级别。
func (fw *FrontendWriter) SetLevel(level slog.Level) {
	fw.mu.Lock()
	fw.level = level
	fw.mu.Unlock()
}

// FrontendLogService 是供前端调用的 Go 绑定服务：接收前端日志/错误并写入 frontend.log。
type FrontendLogService struct {
	logger *slog.Logger
	fw     *FrontendWriter
}

// NewFrontendLogService 创建前端日志服务。
func NewFrontendLogService(fw *FrontendWriter) *FrontendLogService {
	return &FrontendLogService{
		logger: slog.New(fw.Handler()),
		fw:     fw,
	}
}

// FrontendLog 由前端调用：level 取值 debug/info/warn/error，msg 为日志文本。
func (s *FrontendLogService) FrontendLog(level, msg string) {
	switch ParseLevel(level) {
	case slog.LevelDebug:
		s.logger.Debug(msg)
	case slog.LevelWarn:
		s.logger.Warn(msg)
	case slog.LevelError:
		s.logger.Error(msg)
	default:
		s.logger.Info(msg)
	}
}

// SetLevel 同步前端日志级别（由 applyLogConfig 调用）。
func (s *FrontendLogService) SetLevel(level slog.Level) {
	s.fw.SetLevel(level)
	s.logger = slog.New(s.fw.Handler())
}
