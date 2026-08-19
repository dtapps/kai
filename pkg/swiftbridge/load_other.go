//go:build !darwin

// Package swiftbridge 是 Kai 的 Swift 桥接层（纯 Go 动态加载器 + 双端契约类型）。
//
// 本文件为非 macOS 平台（windows/linux）编译桩：purego.Dlopen/RTLD_* 为 Unix 专有，
// 无法在 Windows 下链接，故将真实加载逻辑隔离在 load.go（//go:build darwin）中。
// 这里仅保留与 load.go 一致的包级函数指针声明与 Init 空实现，保证：
//  1. 包在非 macOS 下可正常编译（make check-cross 的 GOOS=windows 不再报 undefined）；
//  2. main.go 无条件调用的 swiftbridge.Init("") 在非 macOS 下为空操作（返回 nil）；
//  3. 包 API 签名跨平台一致，避免调用方在 darwin 下引用到 undefined 符号。
//
// 非 macOS 平台本就不加载 Swift 桥接（所有 kai_* 调用方均在 *_darwin.go 内），
// 这里的函数指针保持 nil，不会被执行。
package swiftbridge

import "unsafe"

var (
	// handle 在非 macOS 下恒为 0（见 Available），所有 KaiXxx 函数指针恒为 nil。
	handle uintptr

	// kai_* 函数指针（非 macOS 下恒为 nil，仅供签名一致）。
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

// Init 非 macOS 平台空实现：不加载任何 dylib，直接返回 nil。
// Swift 桥接仅 macOS 可用，非 macOS 下所有功能由对应平台的实现（或禁用）兜底。
func Init(dylibPath string) error {
	return nil
}

// Available 非 macOS 平台恒返回 false：无 Swift 桥接，调用方应安全降级。
func Available() bool {
	return handle != 0
}
