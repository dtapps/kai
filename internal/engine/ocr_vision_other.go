//go:build !darwin

package engine

import (
	"context"

	"cnb.cool/dtapp/kai/internal/model"
)

// VisionOCR 非 macOS 平台的占位类型。系统 OCR（Vision.framework）仅 macOS 可用，
// 其它平台应使用 tesseract（NewTesseractOCR，需本机 tesseract 依赖）。
type VisionOCR struct {
	name string
}

// NewVisionOCR 构造系统 OCR 引擎。仅 darwin 平台有真实实现，
// 此占位供 engine_wrapper.go 在 darwin 块内引用时提供编译符号（Windows/Linux 不会注册）。
func NewVisionOCR() *VisionOCR {
	return nil
}

// Name 引擎名
func (v *VisionOCR) Name() string { return "vision" }

// Recognize 非 macOS 平台的占位实现。Vision.framework 仅 macOS 可用，
// 且 engine_wrapper.go 仅在 darwin 下注册 VisionOCR，故本方法在 Windows/Linux
// 上永远不会被调用。返回空结果而非错误，仅用于满足 OcrEngine 接口签名。
func (v *VisionOCR) Recognize(_ context.Context, _ model.OcrRequest) (*model.OcrResult, error) {
	return &model.OcrResult{}, nil
}
