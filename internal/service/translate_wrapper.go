package service

import (
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
)

// TranslateWrapper 翻译 / OCR 的薄适配层：持有 translate.Service，仅做 RPC 透传。
// 不实现 wails 生命周期三件套（启动编排统一由 AppService 负责）。
type TranslateWrapper struct {
	svc *translate.Service
}

// NewTranslateWrapper 构造翻译 Wrapper。
func NewTranslateWrapper(svc *translate.Service) *TranslateWrapper {
	return &TranslateWrapper{svc: svc}
}

func (w *TranslateWrapper) Translate(req model.TranslateRequest) (*model.TranslateResult, error) {
	return w.svc.Translate(req)
}

func (w *TranslateWrapper) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	return w.svc.TranslateMulti(req)
}

func (w *TranslateWrapper) Ocr(req model.OcrRequest) (*model.OcrResult, error) {
	return w.svc.Ocr(req)
}

func (w *TranslateWrapper) ScreenshotOCR(engineName string) (*model.OcrResult, error) {
	return w.svc.ScreenshotOCR(engineName)
}

// ScreenshotTranslate 截图翻译主流程：区域截图→系统 OCR→多引擎翻译→投递到截图窗口。
// session 标识缓存来源（events.ScreenshotSessionScreenshot / ScreenshotSessionInput）。
func (w *TranslateWrapper) ScreenshotTranslate(session string) (*model.ScreenshotResult, error) {
	return w.svc.ScreenshotTranslate(session)
}
