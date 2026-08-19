//go:build darwin

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// appleTranslator 调用 macOS 系统自带翻译（Translation.framework）。
// 通过 purego 运行时动态加载 Swift 桥接动态库（pkg/swiftbridge），免 API Key、无需辅助功能授权，
// 是开箱即用的本地离线翻译后端。改 Swift 后只需重编 internal/swift/build.sh（产 .dylib 并自动复制到 pkg/swiftbridge），
// 运行时 Dlopen 加载到的即是最新代码，无需重新链接主二进制。
type appleTranslator struct{}

// NewApple 创建系统翻译引擎。
func NewApple() Translator {
	return &appleTranslator{}
}

func (s *appleTranslator) Name() string { return "apple" }

// SetLogConfig 将日志配置（目录 + LogConfig 的等级/保留天数/压缩）同步给 Swift 桥接层，
// 使其 kai-bridge.log 与主应用日志（kai.log）使用同一套策略（等级过滤、按天滚动、保留天数、压缩）。
// dir 为空则跳过；level 为 debug/info/warn/error（非法值由 Swift 侧回退 info）；
// retentionDays <=0 表示仅按天滚动、不清理；compress 决定过期归档是否压缩为 .gz。
func SetLogConfig(dir, level string, retentionDays int, compress bool) {
	if dir == "" {
		return
	}
	swiftbridge.KaiSetLogConfig(dir, level, int32(retentionDays), compress)
}

// SetBridgeLocale 将当前界面语言同步给 Swift 桥接层，使其 kai-bridge.log 调试日志
// 随系统语言切换中/英文。locale 形如 "zh-CN" / "en-US"，以 "en" 开头视为英文。
// 空串则跳过（保持 Swift 侧默认 zh）。
func SetBridgeLocale(locale string) {
	if locale == "" {
		return
	}
	swiftbridge.KaiSetLocale(locale)
}

// SupportsAutoSource 系统翻译（Translation.framework）支持自动检测源语言：
// from=auto 时 Go 传空串给 Swift，Swift 侧用 NaturalLanguage 自动识别并约束到已安装列表。
func (s *appleTranslator) SupportsAutoSource() bool { return true }

// Translate 通过 Translation.framework 完成翻译。
// src 为 "auto"（或空）时，由 Swift 侧用 NaturalLanguage 自动检测源语言并约束到本机已安装列表；
// 目标语言必须显式指定。
func (s *appleTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, fmt.Errorf(i18n.T("err.empty_text"))
	}
	sl := normalizeLang(string(req.From))
	tl := normalizeLang(string(req.To))
	// sl == "" 表示自动检测源语言，交由 Swift 处理；非空则必须显式指定。
	if tl == "auto" || tl == "" {
		return nil, fmt.Errorf(i18n.T("err.apple_need_target"))
	}

	slog.Debug(i18n.T("log.apple_translate_invoke"), "from", sl, "to", tl, "text_len", len(text))

	outBuf := make([]byte, 1<<16) // 64KB 输出缓冲，足以容纳长文本译文 + JSON 包装
	n := swiftbridge.KaiTranslate(sl, tl, text, unsafe.Pointer(&outBuf[0]), int32(len(outBuf)))
	if n < 0 {
		slog.Error(i18n.T("err.apple_translate_buffer"), "from", sl, "to", tl, "text_len", len(text))
		return nil, fmt.Errorf(i18n.T("err.apple_translate_buffer"))
	}

	// 裁剪 Swift 写入时可能附带的一个结尾 \0（C 字符串习惯），避免 JSON 解析报 \x00 错误。
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var tr swiftbridge.TranslateSuccess
	if err := json.Unmarshal(payload, &tr); err != nil {
		slog.Error(i18n.T("err.apple_translate_parse"), "from", sl, "to", tl, "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.apple_translate_parse"), err)
	}
	if tr.Code != "" {
		// Swift 自定义错误：按错误码走 Go 侧 i18n 渲染用户可见文案，detail 作技术细节。
		// 已知错误码映射到 err.apple_<code>；未知 code 回退到通用引擎错误文案，
		// 避免向用户暴露原始 key 字符串。
		var msg string
		switch tr.Code {
		case swiftbridge.BridgeErrEmptyText:
			msg = i18n.T("err.apple_empty_text")
		case swiftbridge.BridgeErrTargetRequired:
			msg = i18n.T("err.apple_target_required")
		case swiftbridge.BridgeErrNoSourceLang:
			slog.Error(i18n.T("err.apple_no_source_lang"), "from", sl, "to", tl, "detail", tr.Detail)
			msg = i18n.T("err.apple_no_source_lang")
		case swiftbridge.BridgeErrAppleTranslate:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "detail", tr.Detail)
			msg = i18n.T("err.apple_translate_engine")
		default:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "code", tr.Code, "detail", tr.Detail)
			msg = i18n.T("err.apple_translate_engine")
		}
		if tr.Detail != "" {
			msg = msg + " (" + tr.Detail + ")"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if tr.Result == "" {
		slog.Error(i18n.T("err.apple_translate_empty"), "from", sl, "to", tl)
		return nil, fmt.Errorf(i18n.T("err.apple_translate_empty"))
	}
	slog.Debug(i18n.T("log.apple_translate_done"), "from", sl, "to", tl, "detected_from", tr.From, "result_len", len(tr.Result))
	return &model.TranslateResult{
		Engine: "apple",
		From:   model.Language(coalesceLang(tr.From, sl)),
		To:     model.Language(tl),
		Text:   text,
		Result: tr.Result,
	}, nil
}

// AvailableLanguages 返回系统已安装语言包的语言码列表（BCP-47）。
// 供前端语言选择器等场景使用；失败返回错误。
func AvailableLanguages() ([]string, error) {
	slog.Debug(i18n.T("log.apple_query_langs"))
	outBuf := make([]byte, 1<<16)
	n := swiftbridge.KaiAvailableLanguages(unsafe.Pointer(&outBuf[0]), int32(len(outBuf)))
	if n < 0 {
		slog.Error(i18n.T("err.apple_lang_buffer"))
		return nil, fmt.Errorf(i18n.T("err.apple_lang_buffer"))
	}
	// 裁剪 Swift 写入时可能附带的一个结尾 \0（C 字符串习惯），避免 JSON 解析报 \x00 错误。
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	// Swift 返回 {"langs":[...]}，langs 为本机已安装（已下载、可离线翻译）的语言标识符。
	var resp swiftbridge.AvailableLanguages
	if err := json.Unmarshal(payload, &resp); err != nil {
		slog.Error(i18n.T("err.apple_lang_parse"), "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.apple_lang_parse"), err)
	}
	slog.Info(i18n.T("log.apple_query_langs_done"), "count", len(resp.Langs), "langs", resp.Langs)
	return resp.Langs, nil
}

// coalesceLang 当 Swift 未回传源语言时回退到 "auto"（表示自动检测）。
func coalesceLang(got, fallback string) string {
	if got == "" {
		if fallback == "" {
			return "auto"
		}
		return fallback
	}
	return got
}

// normalizeLang 把内部语言码映射为 Translation.framework 接受的 BCP-47 码。
// "auto" / "" 映射为空串，让 Swift 侧走 NaturalLanguage 自动检测分支。
func normalizeLang(code string) string {
	switch code {
	case "zh", "zh-CN", "zh_CN":
		return "zh-Hans"
	case "zh-TW", "zh_Hant", "zh-Hant":
		return "zh-Hant"
	case "auto", "":
		return ""
	default:
		return code
	}
}
