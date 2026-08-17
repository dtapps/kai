// Package translate 提供翻译 / OCR 的核心编排能力（纯业务逻辑，不依赖 wails 生命周期）。
// 负责：单引擎翻译、多引擎并行翻译、图片 OCR、截图 OCR，以及翻译成功后写入历史。
// 引擎注册与选择走 engine.Registry；历史持久化走 historystore；用户配置走 settings.Service。
package translate

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Service 翻译 / OCR 编排领域服务。
// 所有依赖通过 NewService 显式注入，不持有共享容器，便于单独替换与测试。
type Service struct {
	registry    *engine.Registry
	history     *historystore.Store
	configStore *configstore.Store
	settings    *settings.Service
	app         *application.App
	log         *slog.Logger
}

// NewService 构造翻译编排服务。app 允许在构造后通过 SetApp 注入（启动编排期 app 才就绪）。
func NewService(reg *engine.Registry, hist *historystore.Store, st *settings.Service, app *application.App, log *slog.Logger) *Service {
	return &Service{registry: reg, history: hist, settings: st, app: app, log: log}
}

// SetApp 在 app 就绪后注入（启动编排阶段）。
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// SetConfigStore 注入引擎配置库，供历史写入时按引擎名解析 ID。
func (s *Service) SetConfigStore(cs *configstore.Store) {
	s.configStore = cs
}

// Translate 单引擎翻译：先按引擎名取已注册 translator，失败回退默认引擎；
// 成功写入历史（失败仅记录日志，不影响返回）。
func (s *Service) Translate(req model.TranslateRequest) (*model.TranslateResult, error) {
	engineName := req.EngineName
	reg, ok := s.registry.GetTranslator(engineName)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.translate_engine_not_registered"), engineName)
	}
	res, err := s.translateWithEngine(reg, engineName, req)
	if err != nil {
		return nil, err
	}
	s.saveHistory(res)
	return res, nil
}

// translateWithEngine 执行单个翻译引擎调用并组装结果。
func (s *Service) translateWithEngine(reg engine.Translator, engineName string, req model.TranslateRequest) (*model.TranslateResult, error) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resCh := make(chan *model.TranslateResult, 1)
	errCh := make(chan error, 1)
	// 翻译引擎调用放在 goroutine，通过带缓冲 channel 回收结果，便于超时与并发编排。
	go func() {
		res, err := reg.Translate(ctx, req)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- &model.TranslateResult{
			Engine:   engineName,
			From:     req.From,
			To:       req.To,
			Text:     req.Text,
			Result:   res.Result,
			Phonetic: res.Phonetic,
			Dict:     res.Dict,
		}
	}()

	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_failed"), engineName, err)
	case <-ctx.Done():
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_timeout"), engineName, ctx.Err())
	}
}

// TranslateMulti 并行启动所有「已开启的翻译引擎」的翻译，结果逐个流式推送到前端（EventTranslateResult）。
// registry 中仅包含用户在设置页启用并成功注册的引擎（OCR 引擎不纳入翻译并行）。
// 返回已启动的引擎数；实际结果通过事件异步到达，前端按 engine 字段聚合展示。
func (s *Service) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	all := s.registry.AllEngines()
	started := 0
	for _, meta := range all {
		// 仅并行已开启的「翻译」引擎，跳过 OCR 引擎。
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		started++
		// 每个引擎独立 goroutine，互不阻塞；完成后通过应用级事件推给前端。
		go func(reg engine.Translator, name string) {
			res, err := s.translateWithEngine(reg, name, req)
			if err != nil {
				s.log.Error(i18n.T("log.translate_multi_engine_failed"), slog.String("engine", name), slog.Any("error", err))
				return
			}
			s.saveHistory(res)
			if s.app != nil {
				s.app.Event.Emit(events.EventTranslateResult, *res)
			}
		}(reg, meta.Name)
	}
	return &model.TranslateMultiResult{Count: started}, nil
}

// Ocr 图片 OCR：直接对传入的图片数据执行 OCR（图片已由调用方截好/选好）。
func (s *Service) Ocr(req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, fmt.Errorf(i18n.T("err.ocr_empty_image"))
	}
	ocr, ok := s.registry.GetOcr(req.Engine)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.ocr_engine_not_registered"), req.Engine)
	}
	return s.ocrWithEngine(ocr, req)
}

// ScreenshotOCR 截图 OCR：先截图，再对截到的图片执行 OCR，结果推给前端（EventTranslateResult）。
func (s *Service) ScreenshotOCR(engineName string) (*model.OcrResult, error) {
	img, err := engine.CaptureScreenshot()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.screenshot_failed"), err)
	}
	return s.TriggerOcr(engineName, img)
}

// ScreenshotTranslate 截图翻译主流程（分阶段流式）：
//  1. 捕获区域截图 → 系统 OCR 识别文字
//  2. 立即 Emit EventScreenshotOCR（image+text，translations 空），前端先显示截图与原文
//  3. 逐引擎翻译，每完成一条再 Emit 一次（累积 translations），前端增量追加译文卡片
//     翻译失败的引擎也以「失败占位」形式追加，避免静默丢失。
//
// 返回完整结果供调用方直接使用（如需要同步回应）。
func (s *Service) ScreenshotTranslate() (*model.ScreenshotResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.log.Debug(i18n.T("log.screenshot_start"), slog.String("step", "capture_region"))
	img, err := engine.CaptureRegion(ctx)
	if err != nil {
		s.log.Error(i18n.T("log.screenshot_capture_region_failed"), slog.Any("error", err))
		return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
	}
	s.log.Debug(i18n.T("log.screenshot_capture_region_done"), slog.Int("image_bytes", len(img)))

	// 截图一完成（用户框选松手）立即把图片推给前端并呼出窗口，
	// 让用户第一时间看到截图，不必等 OCR 与翻译。
	imageURL := "data:image/png;base64," + encodeImage(img)
	if s.app != nil {
		s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
			Image:        imageURL,
			Text:         "",
			Translations: nil,
			To:           model.ZH,
		})
	}
	s.log.Debug(i18n.T("log.screenshot_pushed_image"), slog.String("step", "image_pushed"), slog.Int("image_len", len(imageURL)))

	ocrName := s.registry.DefaultOCREngineName()
	if ocrName == "" {
		s.log.Error(i18n.T("log.screenshot_no_ocr_engine"))
		return nil, fmt.Errorf(i18n.T("err.no_ocr"))
	}
	s.log.Debug(i18n.T("log.screenshot_ocr_start"), slog.String("ocr_engine", ocrName))
	ocrRes, err := s.TriggerOcr(ocrName, img)
	if err != nil {
		s.log.Error(i18n.T("log.screenshot_ocr_failed"), slog.String("ocr_engine", ocrName), slog.Any("error", err))
		// OCR 失败（含超时）必须把错误投递前端，否则页面会一直停在"正在识别文字…"转圈。
		if s.app != nil {
			s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
				Image:        imageURL,
				Text:         "",
				Translations: nil,
				To:           model.ZH,
				Error:        err.Error(),
			})
		}
		return nil, err
	}
	text := ocrRes.Text
	s.log.Debug(i18n.T("log.screenshot_ocr_done"), slog.Int("text_len", len(text)))
	if text == "" {
		s.log.Warn(i18n.T("log.screenshot_ocr_empty"))
		return nil, fmt.Errorf(i18n.T("err.ocr_no_text"))
	}

	to := model.ZH
	if s.settings != nil && s.settings.Get() != nil && s.settings.Get().DefaultTo != "" {
		to = model.Language(s.settings.Get().DefaultTo)
	}
	from := model.Auto

	imageURL = "data:image/png;base64," + encodeImage(img)

	// 阶段一：先推送截图 + 原文，让前端立刻显示（识别到内容即展示，不必等翻译）。
	first := model.ScreenshotResult{Image: imageURL, Text: text, Translations: nil, To: to}
	if s.app != nil {
		s.app.Event.Emit(events.EventScreenshotOCR, first)
	}

	req := model.TranslateRequest{Text: text, From: from, To: to}
	s.log.Debug(i18n.T("log.screenshot_translate_start"), slog.String("target", string(to)), slog.Int("text_len", len(text)))
	translations := s.translateAllStream(req, imageURL, to)

	result := model.ScreenshotResult{
		Image:        imageURL,
		Text:         text,
		Translations: translations,
		To:           to,
	}
	s.log.Debug(i18n.T("log.screenshot_assemble_done"), slog.Int("image_len", len(result.Image)), slog.Int("text_len", len(result.Text)), slog.Int("translations", len(result.Translations)))
	return &result, nil
}

// translateAllStream 串行调用所有已开启的翻译引擎；每完成一条（成功或失败占位）即 Emit
// 一次 EventScreenshotOCR（携带已累积的 translations），实现译文逐条增量追加到截图窗口。
// 返回最终累积的翻译结果列表。
func (s *Service) translateAllStream(req model.TranslateRequest, imageURL string, to model.Language) []model.TranslateResult {
	out := make([]model.TranslateResult, 0)
	for _, meta := range s.registry.AllEngines() {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		s.log.Debug(i18n.T("log.screenshot_engine_start"), slog.String("engine", meta.Name))
		res, err := s.translateWithEngine(reg, meta.Name, req)
		if err != nil {
			s.log.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", err))
			s.log.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", err))
			// 失败也追加占位卡片，让用户看到哪个引擎没翻出来。
			out = append(out, model.TranslateResult{
				Engine: meta.Name,
				From:   req.From,
				To:     req.To,
				Text:   req.Text,
				Result: "",
			})
		} else {
			s.saveHistory(res)
			out = append(out, *res)
		}
		// 每完成一条就增量推送，前端按 engine 去重追加。
		if s.app != nil {
			s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
				Image:        imageURL,
				Text:         req.Text,
				Translations: append([]model.TranslateResult{}, out...),
				To:           to,
			})
		}
	}
	return out
}

// TriggerOcr 对给定图片执行 OCR，返回识别结果。
// 注意：OCR 结果通过返回值返回给调用方（ScreenshotTranslate 统一经 EventScreenshotOCR 推送前端），
// 不可在此用 EventTranslateResult 发出——该事件注册类型为 TranslateResult，发 OcrResult 会触发
// "data of type model.OcrResult ... does not match registered data type model.TranslateResult" 的 ERR。
func (s *Service) TriggerOcr(engineName string, img []byte) (*model.OcrResult, error) {
	ocr, ok := s.registry.GetOcr(engineName)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.ocr_engine_not_registered"), engineName)
	}
	res, err := s.ocrWithEngine(ocr, model.OcrRequest{ImageData: img, Engine: engineName})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ocrWithEngine 执行单个 OCR 引擎调用并组装结果。
// req 携带 Engine/CorrectText/TimeoutSec；Go 侧 ctx 超时取 Swift 超时 + 余量，
// 保证 Swift 内部超时先返回 "ocr timeout"，Go 不会被过早的 ctx.Done() 假触发。
func (s *Service) ocrWithEngine(ocr engine.OcrEngine, req model.OcrRequest) (*model.OcrResult, error) {
	swiftTimeout := req.TimeoutSec
	if swiftTimeout <= 0 {
		swiftTimeout = 60
	}
	timeout := time.Duration(swiftTimeout+10) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resCh := make(chan *model.OcrResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := ocr.Recognize(ctx, req)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- res
	}()

	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.ocr_failed"), req.Engine, err)
	case <-ctx.Done():
		s.log.Error(i18n.T("log.screenshot_ocr_timeout"), slog.String("ocr_engine", req.Engine), slog.Any("error", ctx.Err()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.ocr_timeout"), req.Engine, ctx.Err())
	}
}

// saveHistory 翻译成功后写入历史库（失败仅记录日志，不影响返回）。
func (s *Service) saveHistory(res *model.TranslateResult) {
	if s.history == nil || res == nil {
		return
	}
	fromOCR := int64(0)
	if res.FromOCR {
		fromOCR = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	engineRow, err := s.configStore.GetEngineByName(ctx, res.Engine)
	if err != nil {
		s.log.Warn(i18n.T("log.engine_query_id_failed"), slog.String("engine", res.Engine), slog.Any("error", err))
	}
	engineID := int64(0)
	if engineRow != nil {
		engineID = engineRow.ID
	}
	// 去重：内容完全相同的翻译不重复入库
	if dup, _ := s.history.FindByKey(ctx, res.Text, string(res.From), string(res.To), engineID, fromOCR); dup > 0 {
		return
	}
	if _, err := s.history.InsertHistory(ctx, historystore.InsertHistoryParams{
		Text:      res.Text,
		Result:    res.Result,
		FromLang:  string(res.From),
		ToLang:    string(res.To),
		EngineID:  engineID,
		FromOcr:   fromOCR,
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		s.log.Error(i18n.T("log.translate_save_history_failed"), slog.Any("error", err))
	}
}

// encodeImage 把图片字节编码为 base64 字符串（不含 data URL 前缀）。
func encodeImage(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
