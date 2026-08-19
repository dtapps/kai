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
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/buildinfo"
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
// dylibPath 可选：传空则用默认（与本包源文件同目录的 libkai_bridge.dylib，开发态）；
// 若默认路径加载失败，自动回退尝试打包后的 app bundle 标准位置
// Contents/Frameworks/libkai_bridge.dylib（由 build 脚本拷入）。
// 所有候选都失败才返回错误，且调用方（经 Available()）仍应安全降级，不致命。
func Init(dylibPath string) error {
	loadOnce.Do(func() {
		candidates := make([]string, 0, 6)
		if dylibPath != "" {
			candidates = append(candidates, dylibPath)
		}
		if buildinfo.IsDev() {
			// 开发态：优先本地 pkg/swiftbridge/libkai_bridge.dylib（build.sh 产出），
			// 改 Swift 重编后无需重编 Go 二进制即生效。仅保留两条可靠路径：
			//  - 与本 .go 源文件同目录（go build 把真实源路径编进二进制，直接命中 pkg/swiftbridge/）
			//  - 可执行文件所在目录（项目根或 bin/）上溯到项目根后的 pkg/swiftbridge/
			if _, thisFile, _, ok := runtime.Caller(0); ok {
				candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), dylibName))
			}
			if exe, err := os.Executable(); err == nil {
				exeDir := filepath.Dir(exe)
				candidates = append(candidates,
					filepath.Join(exeDir, "pkg", "swiftbridge", dylibName),
					filepath.Join(exeDir, "..", "pkg", "swiftbridge", dylibName),
				)
			}
		}
		// 非开发态（打包产物）从 go:embed 内嵌字节落地临时文件加载，不依赖外部文件；
		// 开发态本地文件缺失时也以 embed 兜底，确保任何情况都能加载、绝不 panic。
		if embedded, err := writeEmbeddedDylib(); err == nil {
			candidates = append(candidates, embedded)
		}

		var lastErr error
		for _, p := range candidates {
			if p == "" {
				continue
			}
			h, err := purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				lastErr = err
				continue
			}
			slog.Info(i18n.T("log.swiftbridge_loaded"), "path", p)
			handle = h
			loadErr = nil
			registerAll(handle)
			return
		}
		if lastErr != nil {
			loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_dlopen", "path", strings.Join(candidates, ", "), "detail", lastErr.Error()))
			slog.Warn(i18n.T("log.swiftbridge_unavailable"), "candidates", strings.Join(candidates, ", "))
		}
	})
	return loadErr
}

// writeEmbeddedDylib 把 go:embed 内嵌的 dylib 字节落地到临时文件并返回其路径。
// purego.Dlopen 仅支持路径加载，故内嵌字节必须先写出文件。文件权限 0o755 以便 dlopen 执行。
func writeEmbeddedDylib() (string, error) {
	if len(dylibEmbed) == 0 {
		return "", fmt.Errorf("embedded dylib is empty")
	}
	dir, err := os.MkdirTemp("", "kai-bridge-")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, dylibName)
	if err := os.WriteFile(p, dylibEmbed, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// registerAll 逐个注册 kai_* 函数；符号缺失不致命（记录后跳过），保证其余函数仍可用。
func registerAll(h uintptr) {
	register := func(fptr any, name string) {
		if err := registerSafe(fptr, h, name); err != nil {
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
}

// Available 报告 Swift 桥接层是否成功加载（dylib 已 Dlopen 且 handle 有效）。
// 调用方在调用任何 KaiXxx 函数指针前应先判断；加载失败（非 macOS、dylib 缺失/路径错、
// 或损坏）时返回 false，调用点应安全降级而不得直接调用 nil 函数指针（否则 panic）。
func Available() bool {
	return handle != 0
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
