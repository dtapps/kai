//go:build darwin

package service

import (
	"log/slog"
)

/*
#cgo darwin CFLAGS: -I${SRCDIR}/../swiftbridge
#cgo darwin LDFLAGS: -L${SRCDIR}/../swiftbridge -lkai_bridge -framework ApplicationServices
#include <stdlib.h>

// 由 swiftbridge/libkai_bridge.a 提供的 C 接口（Swift @_cdecl 暴露）。
int kai_accessibility_enabled(void);
void kai_accessibility_request(void);
int kai_screenrecording_enabled(void);
void kai_screenrecording_request(void);
// TODO: 输入监控相关（kai_input_monitoring_enabled/kai_input_monitoring_request）当前未使用，已注释。如需 robotgo 模拟复制键再恢复。
// int kai_input_monitoring_enabled(void);
// void kai_input_monitoring_request(void);
*/
import "C"

// isAccessibilityEnabled 检查 macOS 辅助功能是否已授权当前二进制。
func (s *AppService) isAccessibilityEnabled() bool {
	enabled := C.kai_accessibility_enabled() != 0
	s.log.Info("[Kai-Bridge-Cgo] 辅助功能授权查询", slog.Bool("结果", enabled))
	return enabled
}

// openAccessibilitySettings 通过系统弹窗请求辅助功能授权（仅 darwin 生效）。
func (s *AppService) openAccessibilitySettings() {
	s.log.Info("[Kai-Bridge-Cgo] 辅助功能授权请求 弹出系统授权框")
	C.kai_accessibility_request()
}

// isScreenRecordingEnabled 检查 macOS 屏幕录制是否已授权当前二进制（截图 OCR 依赖）。
func (s *AppService) isScreenRecordingEnabled() bool {
	enabled := C.kai_screenrecording_enabled() != 0
	s.log.Info("[Kai-Bridge-Cgo] 屏幕录制授权查询", slog.Bool("结果", enabled))
	return enabled
}

// openScreenRecordingSettings 弹系统「屏幕录制」授权框（仅 darwin 生效）。
func (s *AppService) openScreenRecordingSettings() {
	s.log.Info("[Kai-Bridge-Cgo] 屏幕录制授权请求 弹出系统授权框")
	C.kai_screenrecording_request()
}

// TODO: 输入监控相关（isInputMonitoringEnabled / openInputMonitoringSettings）当前未使用，已注释。需 robotgo 模拟复制键时恢复。
// // isInputMonitoringEnabled 检查 macOS「输入监控」是否已授权当前二进制。
// func (s *AppService) isInputMonitoringEnabled() bool {
// 	enabled := C.kai_input_monitoring_enabled() != 0
// 	s.log.Info("[Kai-Bridge-Cgo] 输入监控授权查询", slog.Bool("结果", enabled))
// 	return enabled
// }
//
// // openInputMonitoringSettings 打开系统「安全性与隐私 > 输入监控」设置面板（仅 darwin 生效）。
// func (s *AppService) openInputMonitoringSettings() {
// 	s.log.Info("[Kai-Bridge-Cgo] 输入监控授权 打开系统设置面板")
// 	C.kai_input_monitoring_request()
// }
