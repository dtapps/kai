//go:build !darwin

package engine

import (
	"context"
	"fmt"
	"log/slog"

	"cnb.cool/dtapp/kai/internal/model"
)

// systemTranslator 非 macOS 平台的占位实现（系统翻译仅在 macOS 可用）。
type systemTranslator struct{}

// NewSystem 在非 macOS 平台返回不支持的系统翻译引擎。
func NewSystem() Translator {
	return &systemTranslator{}
}

func (s *systemTranslator) Name() string { return "system" }

// Translate 非 macOS 平台不支持系统翻译。
func (s *systemTranslator) Translate(_ context.Context, _ model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, fmt.Errorf("系统翻译仅支持 macOS")
}

// SetLogConfig 配置引擎层日志输出。非 darwin 平台无 Swift 桥接日志系统，
// 直接走 Go 标准 slog（默认输出到 stderr）。签名与 darwin 版一致以便调用方统一。
func SetLogConfig(dir, level string, _ int, _ bool) {
	if dir != "" {
		// 非 darwin 平台暂不重定向到文件，仅记录意图，避免引入 CGO/平台特定文件锁。
		slog.Info("[Kai-Engine] 非 macOS 平台 SetLogConfig 走标准 slog（不重定向文件）",
			"dir", dir, "level", level)
	}
}
