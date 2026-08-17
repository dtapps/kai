//go:build darwin

package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// currentSelectionPoint 通过 Swift 桥接（AX）获取前台 app 窗口锚点，用于浮窗定位。
func currentSelectionPoint() *application.Point {
	if !isAccessibilityEnabled() {
		return nil
	}
	x, y := selectionPointViaBridge()
	if x == 0 && y == 0 {
		return nil
	}
	return &application.Point{X: x, Y: y}
}

// primaryScreenSize 返回主屏幕分辨率（经 Swift 桥接）。
func primaryScreenSize() (float64, float64) {
	return screenSizeViaBridge()
}

// TODO(2026-08-11): currentSelectionOSA 已禁用 Swift 取词。原实现 selectedTextViaBridge →
// Swift kai_selected_text 因用户反馈引发电脑异常被一并注释禁用。现恒返回空串，使 darwin 上
// currentSelection() 自然回退到剪贴板兜底（与输入翻译「优先复制键」逻辑对齐）。
// 恢复方式：取消注释下方 selectedTextViaBridge 调用，并确保 bridge_darwin.go 与 Swift 端已恢复。
//
// currentSelectionOSA 通过 Swift 桥接层（AXUIElement）读取当前应用选区文本（macOS）。
// 不再依赖 AppleScript / System Events。
//
//	func currentSelectionOSA() string {
//		return selectedTextViaBridge(nil, 0)
//	}
func currentSelectionOSA() string {
	return ""
}
