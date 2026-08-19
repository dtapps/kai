//go:build darwin

package selection

import (
	"encoding/json"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// TODO(2026-08-11): selectedTextViaBridge 已禁用。它仅被 point_darwin.go 的 currentSelectionOSA
// 使用，而 currentSelectionOSA 已改为返回空串（见 point_darwin.go）。其下游 Swift 符号
// kai_selected_text 已同步注释禁用。若日后恢复 Swift 取词，取消本函数注释、恢复 cgo 声明
// 与 Swift 端 @_cdecl 即可。
//
// selectedTextViaBridge 通过 Swift 桥接层（AXUIElement）读取前台 app 当前选区文本。
// 失败或无选区返回空串；bufSize 为接收缓冲区容量。
// func selectedTextViaBridge(bufSize int) string {
// 	if bufSize <= 0 {
// 		bufSize = 4096
// 	}
// 	buf := make([]byte, bufSize)
// 	n := C.kai_selected_text((*C.char)(unsafe.Pointer(&buf[0])), C.int(bufSize))
// 	if n < 0 {
// 		slog.Warn(i18n.T("log.selection_read"))
// 		return ""
// 	}
// 	text := string(buf[:n])
// 	slog.Info(i18n.T("log.selection_read"), slog.Int("长度", len([]rune(text))), slog.Bool("有内容", text != ""))
// 	return text
// }

// accessibilityEnabledViaBridge 仅查询辅助功能授权状态（供坐标定位前探活），不读取选区。
func accessibilityEnabledViaBridge() bool {
	// dylib 未加载时安全降级：视为未授权（不 panic）。
	if !swiftbridge.Available() {
		slog.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	enabled := swiftbridge.KaiAccessibilityEnabled() != 0
	slog.Info(i18n.T("log.selection_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// isAccessibilityEnabled 检查 macOS 辅助功能是否已授权当前二进制（经 Swift 桥接）。
func isAccessibilityEnabled() bool {
	return accessibilityEnabledViaBridge()
}

// selectionPointViaBridge 通过 Swift 桥接读取前台 app 窗口锚点（JSON {x,y}）。
func selectionPointViaBridge() (x, y int) {
	if !swiftbridge.Available() {
		return 0, 0
	}
	buf := make([]byte, 128)
	n := swiftbridge.KaiSelectionPoint(unsafe.Pointer(&buf[0]), int32(len(buf)))
	if n <= 0 {
		return 0, 0
	}
	var p swiftbridge.SelectionPoint
	if err := json.Unmarshal(buf[:n], &p); err == nil {
		slog.Info(i18n.T("log.selection_read"), slog.Float64("x", p.X), slog.Float64("y", p.Y))
		return int(p.X), int(p.Y)
	}
	return 0, 0
}

// screenSizeViaBridge 通过 Swift 桥接读取主屏分辨率（JSON {w,h}）。
func screenSizeViaBridge() (w, h float64) {
	if !swiftbridge.Available() {
		return 0, 0
	}
	buf := make([]byte, 128)
	n := swiftbridge.KaiScreenSize(unsafe.Pointer(&buf[0]), int32(len(buf)))
	if n <= 0 {
		return 0, 0
	}
	var s swiftbridge.ScreenSize
	if err := json.Unmarshal(buf[:n], &s); err == nil {
		return s.W, s.H
	}
	return 0, 0
}
