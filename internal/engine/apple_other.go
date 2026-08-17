//go:build !darwin

package engine

import (
	"context"
	"fmt"
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// appleTranslator 非 macOS 平台的占位实现（系统翻译仅在 macOS 可用）。
type appleTranslator struct{}

// NewApple 在非 macOS 平台返回不支持的系统翻译引擎。
func NewApple() Translator {
	return &appleTranslator{}
}

func (s *appleTranslator) Name() string { return "apple" }

// Translate 非 macOS 平台不支持系统翻译。
func (s *appleTranslator) Translate(_ context.Context, _ model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, fmt.Errorf(i18n.T("err.apple_unsupported_platform"))
}

// SetLogConfig 配置引擎层日志输出。非 darwin 平台无 Swift 桥接日志系统，
// 直接走 Go 标准 slog（默认输出到 stderr）。签名与 darwin 版一致以便调用方统一。
func SetLogConfig(dir, level string, _ int, _ bool) {
	if dir != "" {
		// 非 darwin 平台暂不重定向到文件，仅记录意图，避免引入 CGO/平台特定文件锁。
		slog.Info(i18n.T("log.apple_setlog_std"),
			"dir", dir, "level", level)
	}
}

// SetBridgeLocale 非 darwin 平台无 Swift 桥接层，无需同步语言，空实现以保持签名一致。
func SetBridgeLocale(_ string) {}
