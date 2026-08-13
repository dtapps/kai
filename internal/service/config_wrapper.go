package service

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"

	"cnb.cool/dtapp/kai/internal/hotkey"
)

// ConfigWrapper 负责 UI 配置（语言/主题/默认引擎/热键/TTS/执行键）的持久化与读取，
// 以及语言/主题的解析与对外查询。引擎配置（Engines）由 EngineWrapper 独立管理。
// 仅暴露 RPC，不实现 wails 生命周期三件套。
type ConfigWrapper struct {
	settingsSvc *settings.Service
	app         *application.App
	hotkeyMgr   *hotkey.Manager
}

// NewConfigWrapper 构造配置 Wrapper。app 与 hotkeyMgr 允许在启动编排后注入。
func NewConfigWrapper(st *settings.Service, app *application.App, hm *hotkey.Manager) *ConfigWrapper {
	return &ConfigWrapper{settingsSvc: st, app: app, hotkeyMgr: hm}
}

// SetApp 在 app 就绪后注入。
func (w *ConfigWrapper) SetApp(app *application.App) {
	w.app = app
}

// Theme 返回用户配置的主题（auto/light/dark，未配置回退 auto）。
func (w *ConfigWrapper) Theme() string {
	if w.settingsSvc.Get() == nil {
		return string(model.ThemeAuto)
	}
	return w.settingsSvc.Get().Theme
}

// GetTheme 返回当前主题配置（可能含 auto）。
func (w *ConfigWrapper) GetTheme() string {
	if w.settingsSvc.Get() == nil || w.settingsSvc.Get().Theme == "" {
		return string(model.ThemeAuto)
	}
	return w.settingsSvc.Get().Theme
}

// GetSystemTheme 返回当前系统外观解析后的实际主题（dark/light）。
// Wails webview 内的 matchMedia('prefers-color-scheme') 在 macOS 上可能不可靠，
// 因此由后端通过 application.Env.IsDarkMode() 提供唯一可信来源。
func (w *ConfigWrapper) GetSystemTheme() string {
	if w.app != nil && w.app.Env.IsDarkMode() {
		return string(model.ThemeDark)
	}
	return string(model.ThemeLight)
}

// SetTheme 持久化主题配置。
func (w *ConfigWrapper) SetTheme(theme string) error {
	cfg := w.settingsSvc.Get()
	if cfg == nil {
		return nil
	}
	cfg.Theme = theme
	if err := w.settingsSvc.Save(); err != nil {
		return err
	}
	if w.app != nil {
		w.app.Event.Emit(events.EventThemeChanged, events.ThemeChangedPayload{
			Mode:  cfg.Theme,
			Theme: w.GetSystemTheme(),
		})
	}
	return nil
}

// SaveConfig 持久化 UI 配置到 settings.json；语言/主题变更会全局同步。
func (w *ConfigWrapper) SaveConfig(cfg *settings.Settings) error {
	cur := w.settingsSvc.Get()
	if cur == nil {
		return nil
	}
	langChanged := cur.Language != cfg.Language
	themeChanged := cur.Theme != cfg.Theme
	cur.Language = cfg.Language
	cur.Theme = cfg.Theme
	cur.DefaultTo = cfg.DefaultTo
	cur.DefaultFrom = cfg.DefaultFrom
	cur.Hotkeys = cfg.Hotkeys
	cur.TTS = cfg.TTS
	cur.ExecKeys = cfg.ExecKeys
	if err := w.settingsSvc.Save(); err != nil {
		return err
	}
	// 热键配置同步后直接重注册，保证保存即生效
	if w.hotkeyMgr != nil {
		w.hotkeyMgr.Register()
	}
	if langChanged {
		i18n.SetLocale(cur.Language)
	}
	if w.app != nil {
		if langChanged {
			w.app.Event.Emit(events.EventLocaleChanged, events.LocaleChangedPayload{
				Mode:     cur.Language,
				Language: cur.Language,
			})
		}
		if themeChanged {
			w.app.Event.Emit(events.EventThemeChanged, events.ThemeChangedPayload{
				Mode:  cur.Theme,
				Theme: w.GetSystemTheme(),
			})
		}
	}
	return nil
}

// NamedItem 带展示名的条目（供前端下拉/列表使用）。
type NamedItem struct {
	Value string `json:"value"` // 选项值（标识）
	Name  string `json:"name"`  // 选项展示名
}

// GetLanguages 返回支持的语言 (value=code, name=展示名)。
func (w *ConfigWrapper) GetLanguages(lang string) []NamedItem {
	codes := model.AllLanguages()
	items := make([]NamedItem, 0, len(codes))
	for _, c := range codes {
		items = append(items, NamedItem{Value: string(c), Name: string(c)})
	}
	return items
}

// GetConfig 返回当前配置指针。
func (w *ConfigWrapper) GetConfig() *settings.Settings {
	return w.settingsSvc.Get()
}
