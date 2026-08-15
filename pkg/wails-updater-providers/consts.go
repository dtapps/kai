package wails_updater_providers

import "strings"

// Locale 语言代码类型。
type Locale string

// 支持的语言（locale）常量：避免调用方散落 "zh-CN" / "en-US" 裸字符串。
const (
	LocaleZhCN Locale = "zh-CN"
	LocaleEnUS Locale = "en-US"
)

// Theme 更新窗口主题类型。
type Theme string

// 更新窗口主题（theme）常量：包级全局，由 SetTheme 设置。
// 调用方可注入应用自身主题，使更新弹窗与宿主应用配色一致，而非依赖系统媒体查询。
const (
	// ThemeDark 强制深色。
	ThemeDark Theme = "dark"
	// ThemeLight 强制浅色。
	ThemeLight Theme = "light"
)

// Source 更新源类型。
type Source string

// 更新源（source）常量：包级全局，由 SetSource 设置；同时用于 Provider.Name()。
const (
	SourceCNB    Source = "cnb"
	SourceGithub Source = "github"
	SourceAuto   Source = "auto"
)

// MetadataKey 是写入 updater.Release.Metadata 的 key 常量，避免散落裸字符串。
const (
	// MetadataReleaseHTMLURL 发布页地址（用于前端跳转/展示）。
	MetadataReleaseHTMLURL = "release.htmlURL"
)

// normalizeLocale 把任意 locale 字符串归一化为受支持的 Locale；
// 空串或未知 locale 经别名表匹配，仍不匹配则回退 zh-CN（默认语言）。
func normalizeLocale(locale string) Locale {
	if _, ok := i18nMessages[Locale(locale)]; ok {
		return Locale(locale)
	}
	if alias, ok := localeAliases[strings.ToLower(strings.ReplaceAll(locale, "-", "_"))]; ok {
		return Locale(alias)
	}
	return (LocaleZhCN)
}

// normalizeTheme 把任意主题归一化为受支持的 Theme；
// 空串或未知主题回退 ThemeDark（默认深色）。
func normalizeTheme(theme Theme) Theme {
	switch theme {
	case ThemeLight, ThemeDark:
		return theme
	default:
		return ThemeDark
	}
}

// normalizeSource 把任意源偏好归一化为受支持的 Source；
// 空串或未知值回退 SourceAuto（按语言选主源）。
func normalizeSource(src Source) Source {
	switch src {
	case SourceCNB, SourceGithub, SourceAuto:
		return src
	default:
		return SourceAuto
	}
}
