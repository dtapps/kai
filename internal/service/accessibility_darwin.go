//go:build darwin

package service

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// isAccessibilityEnabled 检查 macOS 辅助功能是否已授权当前二进制。
func (s *AppService) isAccessibilityEnabled() bool {
	enabled := swiftbridge.KaiAccessibilityEnabled() != 0
	s.log.Info(i18n.T("log.accessibility_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// openAccessibilitySettings 通过系统弹窗请求辅助功能授权（仅 darwin 生效）。
func (s *AppService) openAccessibilitySettings() {
	s.log.Info(i18n.T("log.accessibility_request"))
	swiftbridge.KaiAccessibilityRequest()
}

// isScreenRecordingEnabled 检查 macOS 屏幕录制是否已授权当前二进制（截图 OCR 依赖）。
func (s *AppService) isScreenRecordingEnabled() bool {
	enabled := swiftbridge.KaiScreenRecordingEnabled() != 0
	s.log.Info(i18n.T("log.screenrecording_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// openScreenRecordingSettings 弹系统「屏幕录制」授权框（仅 darwin 生效）。
func (s *AppService) openScreenRecordingSettings() {
	s.log.Info(i18n.T("log.screenrecording_request"))
	swiftbridge.KaiScreenRecordingRequest()
}

// TODO: 输入监控相关（isInputMonitoringEnabled / openInputMonitoringSettings）当前未使用，已注释。需 robotgo 模拟复制键时恢复。
// // isInputMonitoringEnabled 检查 macOS「输入监控」是否已授权当前二进制。
// func (s *AppService) isInputMonitoringEnabled() bool {
// 	enabled := C.kai_input_monitoring_enabled() != 0
// 	s.log.Info(i18n.T("log.input_monitoring_query"), slog.Bool("结果", enabled))
// 	return enabled
// }
//
// // openInputMonitoringSettings 打开系统「安全性与隐私 > 输入监控」设置面板（仅 darwin 生效）。
// func (s *AppService) openInputMonitoringSettings() {
// 	s.log.Info("[Kai-Bridge-Cgo] 输入监控授权 打开系统设置面板")
// 	C.kai_input_monitoring_request()
// }
