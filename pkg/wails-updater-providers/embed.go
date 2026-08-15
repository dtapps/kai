package wails_updater_providers

import (
	_ "embed"
	"strings"
	"text/template"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

//go:embed updater_window.html
var updaterWindowHTMLRaw string

// Window 返回完整的内置更新窗口配置（HTML 已按当前包全局 locale/theme 注入文案）。
// 调用方直接把它赋给 updater.Config.Window，无需自行拼装 BuiltinWindow。
// Options/CSS 留零值，由框架回退到默认外观（小、居中、可缩放）。
func (m *MirrorProvider) Window() *updater.BuiltinWindow {
	return &updater.BuiltinWindow{
		HTML: renderWindowHTML(nil),
		Options: updater.WindowOptions{
			Title: T("window_title_check"),
		},
	}
}

// renderWindowHTML 把内嵌的 updater_window.html 模板按当前包全局 locale/theme/当前版本注入文案后返回。
// theme 控制更新窗口配色：ThemeDark 强制深色（注入 class="dark"），ThemeLight 强制浅色，
// ThemeLight 不注入 class、由 CSS 的 prefers-color-scheme 跟随系统。
// locale/theme 已由 T()/GetTheme() 内部读全局，无需入参（每次渲染实时读，热更新语言/配色）。
// CurrentVersion 从 app.Updater.CurrentVersion() 读取；app 为 nil 时注入空串。
// 模板解析或执行失败时回退为原始内嵌 HTML（降级不致命）。
func renderWindowHTML(app *application.App) string {
	theme := GetTheme()
	tmpl, err := template.New("updaterWindow").Parse(updaterWindowHTMLRaw)
	if err != nil {
		return updaterWindowHTMLRaw
	}
	currentVersion := ""
	if app != nil && app.Updater != nil {
		currentVersion = app.Updater.CurrentVersion()
	}
	data := map[string]string{
		"Theme":          themeHTMLTheme(theme),
		"Lang":           string(GetLocale()),
		"CurrentVersion": currentVersion,
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return updaterWindowHTMLRaw
	}
	return sb.String()
}

// themeHTMLTheme 将 Theme 转为 <body data-theme="..."> 的取值。
// ThemeDark → "dark"；ThemeLight → "light"。
func themeHTMLTheme(theme Theme) string {
	if theme == ThemeDark {
		return "dark"
	}
	return "light"
}
