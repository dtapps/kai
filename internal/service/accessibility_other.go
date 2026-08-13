//go:build !darwin

package service

// isAccessibilityEnabled 非 darwin 平台无需授权，恒为真。
func (s *AppService) isAccessibilityEnabled() bool {
	return true
}

// openAccessibilitySettings 非 darwin 平台为空实现。
func (s *AppService) openAccessibilitySettings() {}

// isScreenRecordingEnabled 非 darwin 平台无需授权，恒为真。
func (s *AppService) isScreenRecordingEnabled() bool {
	return true
}

// openScreenRecordingSettings 非 darwin 平台为空实现。
func (s *AppService) openScreenRecordingSettings() {}

// TODO: 输入监控相关（isInputMonitoringEnabled / openInputMonitoringSettings 非 darwin 空实现）当前未使用，已注释。
// // isInputMonitoringEnabled 非 darwin 平台无需授权，恒为真。
// func (s *AppService) isInputMonitoringEnabled() bool {
// 	return true
// }
//
// // openInputMonitoringSettings 非 darwin 平台为空实现。
// func (s *AppService) openInputMonitoringSettings() {}
