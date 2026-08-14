//go:build darwin

package engine

/*
#cgo darwin CFLAGS: -I${SRCDIR}/../swiftbridge
#cgo darwin LDFLAGS: -L${SRCDIR}/../swiftbridge -lkai_bridge -framework Vision -framework CoreGraphics
#include <stdlib.h>

// 由 swiftbridge/libkai_bridge.a 提供的 C 接口（Swift @_cdecl 暴露）。
// correct: 1=开启语言校正(usesLanguageCorrection)，0=关闭；timeout_sec: OCR 超时秒数(<=0 用 Swift 默认)。
int kai_ocr(const char* img, char* out, int out_cap, int correct, int timeout_sec);
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/model"
)

// VisionOCR 调用 macOS 系统 Vision.framework 做离线 OCR（零安装、无需本机 tesseract）。
// 通过 cgo 链接 Swift 桥接静态库（internal/swiftbridge）。静态库需用 build.sh 预先生成。
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

// ocrOptions 解析当前配置与本次请求的参数，得到最终生效的 correct / timeout。
// 优先级：req 显式覆盖 > 引擎 Extra 配置 > 内置默认(true / 60s)。
// 复用统一 parseOCRExtra 解析 Extra(JSON)，与 tesseract 保持 extra 格式一致。
func (v *VisionOCR) ocrOptions(req model.OcrRequest) (correct bool, timeoutSec int) {
	correct = true
	timeoutSec = DefaultOCRTimeoutSec
	e := parseOCRExtra(v.name, optExtra(v.config))
	if e.TimeoutSec > 0 {
		timeoutSec = e.TimeoutSec
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
	return
}

// ocrResponse 对应 Swift 端 kai_ocr 返回的 JSON 结构。
type ocrResponse struct {
	Text    string `json:"text"`
	Regions []struct {
		Text string  `json:"text"`
		Conf float64 `json:"conf"`
		Box  []int   `json:"box"`
	} `json:"regions"`
	Error string `json:"error"`
}

// Recognize 对图片字节做 Vision OCR。
func (v *VisionOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, ErrEmptyImage
	}

	correct, timeoutSec := v.ocrOptions(req)

	b64 := base64.StdEncoding.EncodeToString(req.ImageData)
	cImg := C.CString(b64)
	defer C.free(unsafe.Pointer(cImg))

	outBuf := make([]byte, 1<<20) // 1MB 输出缓冲，足以容纳大图 OCR 的 region 明细
	n := C.kai_ocr(cImg, (*C.char)(unsafe.Pointer(&outBuf[0])), C.int(len(outBuf)), boolToCInt(correct), C.int(timeoutSec))
	if n < 0 {
		slog.Error("[Kai-Vision-OCR] 系统 OCR 失败: 输出缓冲区不足")
		return nil, fmt.Errorf("系统 OCR 失败: 输出缓冲区不足")
	}

	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var resp ocrResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		slog.Error("[Kai-Vision-OCR] 系统 OCR 失败: 解析结果异常", "raw", string(payload), "error", err)
		return nil, fmt.Errorf("系统 OCR 失败: %w", err)
	}
	if resp.Error != "" {
		slog.Error("[Kai-Vision-OCR] 系统 OCR 失败: 引擎返回错误", "engine_error", resp.Error)
		return nil, fmt.Errorf("系统 OCR 失败: %s", resp.Error)
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

	slog.Debug("[Kai-Vision-OCR] 系统 OCR 完成", "text_len", len(resp.Text), "regions", len(regions), "correct", correct, "timeout_sec", timeoutSec)
	return &model.OcrResult{
		Engine:  v.name,
		Text:    resp.Text,
		Regions: regions,
	}, nil
}

// boolToCInt 将 Go bool 转为 C int（1/0）。
func boolToCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
