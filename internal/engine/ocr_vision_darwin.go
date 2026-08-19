//go:build darwin

package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// VisionOCR 调用 macOS 系统 Vision.framework 做离线 OCR（零安装、无需本机 tesseract）。
// 通过 purego 运行时动态加载 Swift 桥接动态库（pkg/swiftbridge）。改 Swift 后只需重编
// internal/swift/build.sh（产 .dylib 并自动复制到 pkg/swiftbridge），运行时 Dlopen 加载即最新，无需重链接。
type VisionOCR struct {
	name   string
	config *EngineConfig // 持有所属引擎配置，从 Extra(JSON) 读取 OCR 专属参数
}

// NewVisionOCR 构造系统 OCR 引擎。cfg 为 vision 引擎的 EngineConfig（含 Extra 中的 OCR 参数）。
func NewVisionOCR(cfg *EngineConfig) *VisionOCR {
	return &VisionOCR{name: "vision", config: cfg}
}

// Name 引擎名
func (v *VisionOCR) Name() string { return v.name }

// ocrOptions 解析当前配置与本次请求的参数，得到最终生效的 correct / timeout / retry。
// 优先级：req 显式覆盖 > 引擎 Extra 配置 > 内置默认(true / 60s / 2)。
// 复用统一 parseOCRExtra 解析 Extra(JSON)，与 tesseract 保持 extra 格式一致。
// retry 为「额外重试次数（不含首次尝试）」：Extra 显式设 0 => 关闭重试（仅首次尝试）；
// Extra 未含该字段(nil) => 回落默认 2; req.RetryCount>0 可显式覆盖。
func (v *VisionOCR) ocrOptions(req model.OcrRequest) (correct bool, timeoutSec int, retryCount int) {
	correct = true
	timeoutSec = DefaultOCRTimeoutSec
	retryCount = DefaultOCRRetryCount
	e := parseOCRExtra(v.name, optExtra(v.config))
	if e.TimeoutSec > 0 {
		timeoutSec = e.TimeoutSec
	}
	if e.RetryCount != nil {
		retryCount = *e.RetryCount // 允许 0（关闭重试）
	}
	if e.Correct != nil {
		correct = *e.Correct
	}
	if req.CorrectText != nil {
		correct = *req.CorrectText
	}
	if req.TimeoutSec > 0 {
		timeoutSec = req.TimeoutSec
	}
	if req.RetryCount > 0 {
		retryCount = req.RetryCount
	}
	return
}

// Recognize 对图片字节做 Vision OCR。
func (v *VisionOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, ErrEmptyImage
	}

	correct, timeoutSec, retryCount := v.ocrOptions(req)

	b64 := base64.StdEncoding.EncodeToString(req.ImageData)

	// dylib 未加载（非 macOS / 缺失 / 路径错）时安全降级，避免 nil 函数指针 panic。
	if !swiftbridge.Available() {
		return nil, fmt.Errorf(i18n.T("err.swiftbridge_unavailable"))
	}
	outBuf := make([]byte, 1<<20) // 1MB 输出缓冲，足以容纳大图 OCR 的 region 明细
	n := swiftbridge.KaiOCR(b64, unsafe.Pointer(&outBuf[0]), int32(len(outBuf)), boolToInt32(correct), int32(timeoutSec), int32(retryCount))
	slog.Debug(i18n.T("log.vision_ocr_call"), "n", int(n), "out_cap", len(outBuf), "correct", correct, "timeout", timeoutSec, "retry", retryCount)
	if n < 0 {
		slog.Error(i18n.T("err.vision_ocr_buffer"), "n", int(n))
		return nil, fmt.Errorf(i18n.T("err.vision_ocr_buffer"))
	}

	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var resp swiftbridge.OCRSuccess
	if err := json.Unmarshal(payload, &resp); err != nil {
		slog.Error(i18n.T("err.vision_ocr_parse"), "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.vision_ocr_parse"), err)
	}
	if resp.Code != "" {
		// Swift 自定义错误：按错误码走 Go 侧 i18n 渲染用户可见文案，detail 作技术细节。
		// 已知错误码映射到 err.apple_<code>；未知 code 回退到通用 OCR 引擎错误文案。
		var msg string
		switch resp.Code {
		case swiftbridge.BridgeErrNullImage:
			msg = i18n.T("err.apple_null_image")
		case swiftbridge.BridgeErrDecodeFailed:
			msg = i18n.T("err.apple_decode_failed")
		case swiftbridge.BridgeErrEmptyImage:
			msg = i18n.T("err.apple_empty_image")
		case swiftbridge.BridgeErrBitmapCtxFailed:
			msg = i18n.T("err.apple_bitmap_ctx_failed")
		case swiftbridge.BridgeErrBitmapRedrawFailed:
			msg = i18n.T("err.apple_bitmap_redraw_failed")
		case swiftbridge.BridgeErrOcrTimeout:
			msg = i18n.T("err.apple_ocr_timeout")
		case swiftbridge.BridgeErrAppleOcr:
			slog.Error(i18n.T("err.vision_ocr_engine"), "detail", resp.Detail)
			msg = i18n.T("err.vision_ocr_engine")
		default:
			slog.Error(i18n.T("err.vision_ocr_engine"), "code", resp.Code, "detail", resp.Detail)
			msg = i18n.T("err.vision_ocr_engine")
		}
		if resp.Detail != "" {
			msg = msg + " (" + resp.Detail + ")"
		}
		return nil, fmt.Errorf("%s", msg)
	}

	regions := make([]model.OcrRegion, 0, len(resp.Regions))
	for _, r := range resp.Regions {
		// Swift 回传的 box 为 [x, y, w, h]，模型 OcrRegion.Box 约定 [x1,y1,x2,y2]。
		box := r.Box
		if len(box) == 4 {
			box = []int{box[0], box[1], box[0] + box[2], box[1] + box[3]}
		}
		regions = append(regions, model.OcrRegion{Text: r.Text, Conf: r.Conf, Box: box})
	}

	slog.Debug(i18n.T("log.vision_ocr_done"), "text_len", len(resp.Text), "regions", len(regions), "correct", correct, "timeout_sec", timeoutSec)
	return &model.OcrResult{
		Engine:  v.name,
		Text:    resp.Text,
		Regions: regions,
	}, nil
}

// boolToInt32 将 Go bool 转为 C int（1/0），供 purego 的 int32 参数使用。
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
