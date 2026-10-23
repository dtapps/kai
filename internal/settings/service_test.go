package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/model"
)

func TestDefaultSettings(t *testing.T) {
	d := DefaultSettings()

	if d.Language != string(model.LocaleAuto) {
		t.Fatalf("Language = %q, want auto", d.Language)
	}
	if d.Theme != string(model.ThemeAuto) {
		t.Fatalf("Theme = %q, want auto", d.Theme)
	}
	if d.DefaultTo != string(model.ZH) || d.DefaultFrom != string(model.Auto) {
		t.Fatalf("默认语言: to=%q from=%q", d.DefaultTo, d.DefaultFrom)
	}

	// 唤起主窗口是核心功能，默认必须启用；截图默认关闭。
	if !d.Hotkeys.Input.Enabled || d.Hotkeys.Input.Key != "Alt+A" {
		t.Fatalf("Input 快捷键默认值不符: %+v", d.Hotkeys.Input)
	}
	if d.Hotkeys.Screenshot.Enabled {
		t.Fatalf("Screenshot 快捷键应默认关闭: %+v", d.Hotkeys.Screenshot)
	}

	// 复制键按平台区分。
	wantCopy := "Ctrl+C"
	if runtime.GOOS == "darwin" {
		wantCopy = "Cmd+C"
	}
	if d.ExecKeys.Copy.Key != wantCopy || !d.ExecKeys.Copy.Enabled {
		t.Fatalf("复制键默认值不符: %+v (want key %q)", d.ExecKeys.Copy, wantCopy)
	}
	if d.ExecKeys.Copy.Fallback {
		t.Fatal("复制键 Fallback 应默认关闭")
	}

	if d.TTS.Engine != "system" || d.TTS.Speed != 1.0 {
		t.Fatalf("TTS 默认值不符: %+v", d.TTS)
	}
	if !d.HttpLog.Enabled || d.HttpLog.RetentionDays != 30 {
		t.Fatalf("HttpLog 默认值不符: %+v", d.HttpLog)
	}
	if d.Log.Level != "info" || d.Log.RetentionDays != 30 || !d.Log.Compress {
		t.Fatalf("Log 默认值不符: %+v", d.Log)
	}
	if d.Proxy.Enabled || d.Proxy.Protocol != "http" || d.Proxy.Port != 8080 {
		t.Fatalf("Proxy 默认值不符: %+v", d.Proxy)
	}
	if !d.Updater.Prerelease {
		t.Fatal("Updater.Prerelease 应默认开启")
	}
	// 隐私优先：匿名统计必须默认关闭。
	if d.AnalyticsEnabled {
		t.Fatal("AnalyticsEnabled 必须默认 false（opt-in）")
	}
	if d.AnalyticsInstalled {
		t.Fatal("AnalyticsInstalled 应默认 false")
	}
}

func TestNewServiceCreatesFileWithDefaults(t *testing.T) {
	dir := t.TempDir()

	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}

	path := filepath.Join(dir, "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings.json 未创建: %v", err)
	}
	if got := s.Get().Path; got != path {
		t.Fatalf("Get().Path = %q, want %q", got, path)
	}
	if got := s.Get().Log.Level; got != "info" {
		t.Fatalf("默认日志等级 = %q, want info", got)
	}
}

// 首次运行写出的文件必须是合法 JSON，且能被再次读回。
func TestNewServiceFileIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewService(dir); err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json 不是合法 JSON: %v\n%s", err, string(data))
	}
	for _, key := range []string{"language", "theme", "hotkeys", "execkeys", "log", "proxy", "updater"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("settings.json 缺少字段 %q：\n%s", key, string(data))
		}
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()

	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}

	cfg := s.Get()
	cfg.Language = "en-US"
	cfg.Theme = "dark"
	cfg.DefaultTo = string(model.JA)
	cfg.Log.Level = "debug"
	cfg.Log.RetentionDays = 7
	cfg.AutoClipboard = true
	cfg.AnalyticsEnabled = true
	cfg.Hotkeys.Screenshot = HotkeyEntry{Key: "Alt+X", Enabled: true}
	if err := s.Save(); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	again, err := NewService(dir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	got := again.Get()

	if got.Language != "en-US" || got.Theme != "dark" || got.DefaultTo != string(model.JA) {
		t.Fatalf("界面偏好未持久化: %+v", got)
	}
	if got.Log.Level != "debug" || got.Log.RetentionDays != 7 {
		t.Fatalf("日志配置未持久化: %+v", got.Log)
	}
	if !got.AutoClipboard {
		t.Fatal("AutoClipboard 未持久化")
	}
	if !got.AnalyticsEnabled {
		t.Fatal("AnalyticsEnabled 未持久化")
	}
	if got.Hotkeys.Screenshot.Key != "Alt+X" || !got.Hotkeys.Screenshot.Enabled {
		t.Fatalf("快捷键未持久化: %+v", got.Hotkeys.Screenshot)
	}
}

// 只写了部分字段的旧配置文件：缺少的字段要回退默认值，不能被清零。
func TestPartialConfigFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}
	cfg := s.Get()

	if cfg.Theme != "dark" {
		t.Fatalf("已有字段被覆盖: Theme = %q", cfg.Theme)
	}
	if cfg.Language != string(model.LocaleAuto) {
		t.Fatalf("缺失字段未回退默认: Language = %q", cfg.Language)
	}
	if cfg.Proxy.Port != 8080 {
		t.Fatalf("嵌套默认值丢失: Port = %d", cfg.Proxy.Port)
	}
	if cfg.Hotkeys.Input.Key != "Alt+A" || !cfg.Hotkeys.Input.Enabled {
		t.Fatalf("嵌套结构默认值丢失: %+v", cfg.Hotkeys.Input)
	}
}

// 废弃 key 不应被写回文件（writeConfig 用全新 viper 只写已知字段）。
func TestDeprecatedKeysAreDropped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"light","legacy_removed_field":"值"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewService(dir); err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "legacy_removed_field") {
		t.Fatalf("废弃字段应被清除：\n%s", string(data))
	}
}

func TestNewServiceBadJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{不是 JSON`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewService(dir); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
}

func TestNewServiceUnwritableDir(t *testing.T) {
	// 用一个文件路径当目录，MkdirAll 必然失败。
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewService(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("目录不可创建时应返回错误")
	}
}

// 外部修改配置文件后，OnChange 回调应在防抖窗口后被触发。
func TestOnChangeFiresOnExternalEdit(t *testing.T) {
	dir := t.TempDir()
	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}

	got := make(chan string, 4)
	s.OnChange(func(newCfg *Settings) {
		select {
		case got <- newCfg.Theme:
		default:
		}
	})

	// 防抖 500ms，这里等 4s 足够宽松。
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case theme := <-got:
		if theme != "dark" {
			t.Fatalf("回调收到的 Theme = %q, want dark", theme)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("4s 内未收到配置变更回调（fsnotify 未生效？）")
	}
}

// Get 返回的 Path 必须始终指向磁盘上的真实文件。
func TestPathSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	s, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService 失败: %v", err)
	}
	want := filepath.Join(dir, "settings.json")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Path; got != want {
		t.Fatalf("Save 后 Path = %q, want %q", got, want)
	}
}
