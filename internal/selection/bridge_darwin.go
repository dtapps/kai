//go:build darwin

package selection

/*
#cgo darwin CFLAGS: -I${SRCDIR}/../swiftbridge
#cgo darwin LDFLAGS: -L${SRCDIR}/../swiftbridge -lkai_bridge -framework ApplicationServices
#include <stdlib.h>

// 由 swiftbridge/libkai_bridge.a 提供的 C 接口（Swift @_cdecl 暴露）。
// TODO(2026-08-11): kai_selected_text 已禁用（与选中文本 Swift 取词链路一并禁用，因用户反馈
// 启用后电脑偶发异常）。Swift 端对应 @_cdecl 同步注释，故 cgo 声明此处一并注释，避免链接缺失符号。
// int kai_selected_text(char* out, int out_cap);
int kai_accessibility_enabled(void);
int kai_selection_point(char* out, int out_cap);
int kai_screen_size(char* out, int out_cap);
*/
import "C"

import (
	"encoding/json"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// TODO(2026-08-11): selectedTextViaBridge 已禁用。它仅被 point_darwin.go 的 currentSelectionOSA
// 使用，而 currentSelectionOSA 已改为返回空串（见 point_darwin.go）。其下游 Swift 符号
// kai_selected_text 已同步注释禁用。若日后恢复 Swift 取词，取消本函数注释、恢复 cgo 声明
// 与 Swift 端 @_cdecl 即可。
//
// selectedTextViaBridge 通过 Swift 桥接层（AXUIElement）读取前台 app 当前选区文本。
// 失败或无选区返回空串；bufSize 为接收缓冲区容量。
// func selectedTextViaBridge(log *slog.Logger, bufSize int) string {
// 	if bufSize <= 0 {
// 		bufSize = 4096
// 	}
// 	buf := make([]byte, bufSize)
// 	n := C.kai_selected_text((*C.char)(unsafe.Pointer(&buf[0])), C.int(bufSize))
// 	if n < 0 {
// 		if log != nil {
// 			log.Warn("[Kai-Bridge-Cgo] 选区读取失败 桥接返回负数")
// 		}
// 		return ""
// 	}
// 	text := string(buf[:n])
// 	if log != nil {
// 		log.Info("[Kai-Bridge-Cgo] 选区读取", slog.Int("长度", len([]rune(text))), slog.Bool("有内容", text != ""))
// 	}
// 	return text
// }

// accessibilityEnabledViaBridge 仅查询辅助功能授权状态（供坐标定位前探活），不读取选区。
func accessibilityEnabledViaBridge(log *slog.Logger) bool {
	enabled := C.kai_accessibility_enabled() != 0
	if log != nil {
		log.Info(i18n.T("log.selection_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	}
	return enabled
}

// isAccessibilityEnabled 检查 macOS 辅助功能是否已授权当前二进制（经 Swift 桥接）。
func isAccessibilityEnabled() bool {
	return accessibilityEnabledViaBridge(nil)
}

// selectionPointViaBridge 通过 Swift 桥接读取前台 app 窗口锚点（JSON {x,y}）。
func selectionPointViaBridge(log *slog.Logger) (x, y int) {
	buf := make([]byte, 128)
	n := C.kai_selection_point((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if n <= 0 {
		return 0, 0
	}
	var p struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal(buf[:n], &p); err == nil {
		if log != nil {
			log.Info("[Kai-Bridge-Cgo] 选区坐标读取", slog.Float64("x", p.X), slog.Float64("y", p.Y))
		}
		return int(p.X), int(p.Y)
	}
	return 0, 0
}

// screenSizeViaBridge 通过 Swift 桥接读取主屏分辨率（JSON {w,h}）。
func screenSizeViaBridge(log *slog.Logger) (w, h float64) {
	buf := make([]byte, 128)
	n := C.kai_screen_size((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	if n <= 0 {
		return 0, 0
	}
	var s struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal(buf[:n], &s); err == nil {
		return s.W, s.H
	}
	return 0, 0
}
