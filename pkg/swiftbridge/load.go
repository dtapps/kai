//go:build darwin

// Package swiftbridge 是 Kai 的 Swift 桥接层（纯 Go 动态加载器 + 双端契约类型）。
//
// 通过 github.com/ebitengine/purego 在运行时 Dlopen 加载 libkai_bridge.dylib，
// 注册全部 kai_* 函数指针（零 cgo，桥接库不静态链接进主二进制）。改 Swift 后只需重编
// internal/swift/build.sh（产 .dylib 并自动复制到本目录），运行时 Dlopen 加载即最新，
// 规避「重编后 app 仍是旧代码」的疑虑。
//
// 类型（OCRSuccess / TranslateSuccess / BridgeErr* / SelectionPoint / ScreenSize 等）与
// Swift 端 Codable / BRIDGE_ERR_* 字面量一一对应，详见 bridge_errors.go 与 internal/swift/。
//
// 类型映射遵循 purego 约定：
//   - C 的 char*（仅输入字符串） -> Go 的 string（purego 自动 C 字符串化，调用结束释放）
//   - C 的 char*（输出缓冲区）   -> Go 的 unsafe.Pointer（调用方用 unsafe.Pointer(&buf[0])）
//   - C 的 int / Int32           -> Go 的 int32（macOS LP64 下 C int 为 32 位）
//   - C 的 Bool                  -> Go 的 bool（1 字节 _Bool）
//
// 本文件仅 macOS 编译（purego.Dlopen/RTLD_* 为 Unix 专有）。非 macOS 平台由 load_other.go
// 提供同签名空实现（Init 直接返回 nil，函数指针保持 nil，调用方均在 darwin 下、非 darwin
// 不触发；main.go 无条件调用的 Init 在非 darwin 下为空操作）。
package swiftbridge

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/ebitengine/purego"
)

var (
	loadOnce sync.Once
	loadErr  error
	handle   uintptr

	// kai_* 函数指针（由 purego 注册，零 cgo）。
	KaiOCR                    func(base64 string, out unsafe.Pointer, outCap int32, correct int32, timeout int32, retry int32) int32
	KaiAccessibilityEnabled   func() int32
	KaiAccessibilityRequest   func() int32
	KaiScreenRecordingEnabled func() int32
	KaiScreenRecordingRequest func() int32
	KaiSelectionPoint         func(out unsafe.Pointer, outCap int32) int32
	KaiScreenSize             func(out unsafe.Pointer, outCap int32) int32
	KaiInputMonitoringEnabled func() int32
	KaiAvailableLanguages     func(out unsafe.Pointer, outCap int32) int32
	KaiSetLogConfig           func(dir string, level string, retentionDays int32, compress bool)
	KaiSetLocale              func(locale string)
	KaiTranslate              func(src string, dst string, text string, out unsafe.Pointer, outCap int32) int32
)

// dylib 默认与本 .go 源文件同目录（build.sh 会把最新 libkai_bridge.dylib 复制到这里）。
const dylibName = "libkai_bridge.dylib"

// Init 加载 dylib 并注册全部 kai_* 函数。可重复调用（仅首次真正执行）。
// dylibPath 可选：传空则用默认（与本包源文件同目录的 libkai_bridge.dylib）；
// 打包进 Kai.app 后，可传 app bundle 内的绝对路径（如 Frameworks/libkai_bridge.dylib）。
func Init(dylibPath string) error {
	loadOnce.Do(func() {
		if dylibPath == "" {
			_, thisFile, _, ok := runtime.Caller(0)
			if !ok {
				loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_resolve_path"))
				return
			}
			dylibPath = filepath.Join(filepath.Dir(thisFile), dylibName)
		}
		handle, loadErr = purego.Dlopen(dylibPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if loadErr != nil {
			loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_dlopen", "path", dylibPath, "detail", loadErr.Error()))
			return
		}
		// 逐个注册；符号缺失不致命（记录后跳过），保证其余函数仍可用。
		register := func(fptr any, name string) {
			if err := registerSafe(fptr, handle, name); err != nil {
				loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_register", "name", name, "detail", err.Error()))
			}
		}
		register(&KaiOCR, "kai_ocr")
		register(&KaiAccessibilityEnabled, "kai_accessibility_enabled")
		register(&KaiAccessibilityRequest, "kai_accessibility_request")
		register(&KaiScreenRecordingEnabled, "kai_screenrecording_enabled")
		register(&KaiScreenRecordingRequest, "kai_screenrecording_request")
		register(&KaiSelectionPoint, "kai_selection_point")
		register(&KaiScreenSize, "kai_screen_size")
		register(&KaiInputMonitoringEnabled, "kai_input_monitoring_enabled")
		register(&KaiAvailableLanguages, "kai_available_languages")
		register(&KaiSetLogConfig, "kai_set_log_config")
		register(&KaiSetLocale, "kai_set_locale")
		register(&KaiTranslate, "kai_translate")
	})
	return loadErr
}

// registerSafe 包装 RegisterLibFunc，把 panic/error 转为 error 返回（符号缺失时不直接 panic）。
func registerSafe(fptr any, h uintptr, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(i18n.T("err.swiftbridge_purego_panic", "detail", fmt.Sprintf("%v", r)))
		}
	}()
	purego.RegisterLibFunc(fptr, h, name)
	return nil
}
