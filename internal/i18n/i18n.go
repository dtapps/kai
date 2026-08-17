package i18n

import (
	"embed"
	"encoding/json"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

type Locale string

const (
	ZH_CN Locale = "zh-CN"
	EN_US Locale = "en-US"
)

var (
	mu     sync.RWMutex
	locale Locale = ZH_CN

	bundle *i18n.Bundle
)

func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	loadMessages()
}

func loadMessages() {
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := localesFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			continue
		}
		bundle.ParseMessageFileBytes(data, entry.Name())
	}
}

func localizer() *i18n.Localizer {
	mu.RLock()
	defer mu.RUnlock()
	return i18n.NewLocalizer(bundle, string(locale))
}

// SetLocale 设置当前语言环境
func SetLocale(l string) {
	mu.Lock()
	defer mu.Unlock()
	switch Locale(l) {
	case EN_US:
		locale = EN_US
	default:
		locale = ZH_CN
	}
}

// GetLocale 返回当前语言环境字符串
func GetLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(locale)
}

// T translates a key with named template parameters.
// Usage: i18n.T("err.empty_text")
//
//	i18n.T("err.configstore_get_engine_id", "id", 123)
//	i18n.T("notification.update_available_subtitle", "version", "1.2.3")
func T(key string, templateData ...any) string {
	l := localizer()

	data := make(map[string]any)
	for i := 0; i < len(templateData)-1; i += 2 {
		if k, ok := templateData[i].(string); ok {
			// 防止传入 nil 接口：go-i18n 渲染模板时会对值做 reflect.Value.Type，
			// 零值 interface{} 会得到 zero Value 并触发 "reflect.Value.Type on zero Value" panic。
			// 用空串兜底，既避免 panic，也保证模板 {{.k}} 渲染为占位而非崩溃。
			if templateData[i+1] == nil {
				data[k] = ""
				continue
			}
			data[k] = templateData[i+1]
		}
	}

	msg, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		defaultLocalizer := i18n.NewLocalizer(bundle, string(ZH_CN))
		msg, err = defaultLocalizer.Localize(&i18n.LocalizeConfig{
			MessageID:    key,
			TemplateData: data,
		})
		if err != nil {
			return key
		}
	}
	return msg
}

// TWithLocale 使用指定语言环境翻译 key
func TWithLocale(loc string, key string, templateData ...any) string {
	mu.Lock()
	saved := locale
	locale = Locale(loc)
	if locale != EN_US {
		locale = ZH_CN
	}
	mu.Unlock()

	result := T(key, templateData...)

	mu.Lock()
	locale = saved
	mu.Unlock()

	return result
}

// ResolveLocale 将前端语言环境转换为后端语言环境
func ResolveLocale(loc string) string {
	if loc == string(EN_US) {
		return string(EN_US)
	}
	return string(ZH_CN)
}

// SupportedLocales 返回所有支持的语言环境代码
func SupportedLocales() []string {
	return []string{string(ZH_CN), string(EN_US)}
}

// GetCurrentLocale 返回当前语言环境字符串
func GetCurrentLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(locale)
}
