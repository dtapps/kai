//go:build darwin

package service

import (
	"cnb.cool/dtapp/kai/internal/engine"
)

// SystemLanguages 返回 macOS 系统翻译（Translation.framework）已安装的语言包列表。
// 仅 darwin 可用；非 darwin 平台见 system_langs_other.go。
func (w *ConfigWrapper) SystemLanguages() []string {
	langs, err := engine.AvailableLanguages()
	if err != nil {
		return nil
	}
	return langs
}
