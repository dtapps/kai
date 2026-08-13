//go:build !darwin

package service

// SystemLanguages 非 macOS 平台系统翻译不可用，返回空列表。
func (w *ConfigWrapper) SystemLanguages() []string {
	return nil
}
