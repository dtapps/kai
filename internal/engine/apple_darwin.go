//go:build darwin

package engine

/*
#cgo darwin CFLAGS: -I${SRCDIR}/../swiftbridge
#cgo darwin LDFLAGS: -L${SRCDIR}/../swiftbridge -lkai_bridge -framework Translation -framework Vision -framework CoreGraphics
#include <stdlib.h>

// 由 swiftbridge/libkai_bridge.a 提供的 C 接口（Swift @_cdecl 暴露）。
int kai_translate(const char* src, const char* dst, const char* text,
                  char* out, int out_cap);
int kai_available_languages(char* out, int out_cap);
void kai_set_log_config(const char* dir, const char* level, int retention_days, int compress);
*/
import "C"

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
)

// appleTranslator 调用 macOS 系统自带翻译（Translation.framework）。
// 通过 cgo 链接 Swift 桥接静态库（internal/swiftbridge），免 API Key、无需辅助功能授权，
// 是开箱即用的本地离线翻译后端。静态库需用 bridge/build.sh 预先生成（见 Taskfile darwin 构建）。
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
// boolToInt 将 Go bool 转为 C int（1/0），供 cgo 的 int 参数使用。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func SetLogConfig(dir, level string, retentionDays int, compress bool) {
	if dir == "" {
		return
	}
	cDir := C.CString(dir)
	cLevel := C.CString(level)
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cLevel))
	C.kai_set_log_config(cDir, cLevel, C.int(retentionDays), C.int(boolToInt(compress)))
}

// SupportsAutoSource 系统翻译（Translation.framework）支持自动检测源语言：
// from=auto 时 Go 传空串给 Swift，Swift 侧用 NaturalLanguage 自动识别并约束到已安装列表。
func (s *appleTranslator) SupportsAutoSource() bool { return true }

// translateResult 对应 Swift 端返回的 JSON 结构。
type translateResult struct {
	Result string `json:"result"`
	From   string `json:"from"`
	Error  string `json:"error"`
}

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
	cSrc := C.CString(sl)
	cDst := C.CString(tl)
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cSrc))
	defer C.free(unsafe.Pointer(cDst))
	defer C.free(unsafe.Pointer(cText))

	n := C.kai_translate(cSrc, cDst, cText, (*C.char)(unsafe.Pointer(&outBuf[0])), C.int(len(outBuf)))
	if n < 0 {
		slog.Error(i18n.T("err.apple_translate_buffer"), "from", sl, "to", tl, "text_len", len(text))
		return nil, fmt.Errorf(i18n.T("err.apple_translate_buffer"))
	}

	// 裁剪 Swift 写入时可能附带的一个结尾 \0（C 字符串习惯），避免 JSON 解析报 \x00 错误。
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var tr translateResult
	if err := json.Unmarshal(payload, &tr); err != nil {
		slog.Error(i18n.T("err.apple_translate_parse"), "from", sl, "to", tl, "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.apple_translate_parse"), err)
	}
	if tr.Error != "" {
		slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "engine_error", tr.Error)
		return nil, fmt.Errorf("%s: %s", i18n.T("err.apple_translate_engine"), tr.Error)
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
	n := C.kai_available_languages((*C.char)(unsafe.Pointer(&outBuf[0])), C.int(len(outBuf)))
	if n < 0 {
		slog.Error(i18n.T("err.apple_lang_buffer"))
		return nil, fmt.Errorf(i18n.T("err.apple_lang_buffer"))
	}
	// 裁剪 Swift 写入时可能附带的一个结尾 \0（C 字符串习惯），避免 JSON 解析报 \x00 错误。
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	// Swift 返回 {"langs":[...]}，langs 为本机已安装（已下载、可离线翻译）的语言标识符。
	var resp struct {
		Langs []string `json:"langs"`
	}
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
