// Package translate 提供翻译 / OCR 的核心编排能力（纯业务逻辑，不依赖 wails 生命周期）。
// 负责：单引擎翻译、多引擎并行翻译、图片 OCR、截图 OCR，以及翻译成功后写入历史。
// 引擎注册与选择走 engine.Registry；历史持久化走 historystore；用户配置走 settings.Service。
package translate

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
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

	// screenshotCacheMu 保护 screenshotCache 的并发读写。
	screenshotCacheMu sync.RWMutex
	// screenshotCache 按 session（截图翻译窗口 / 输入翻译页）分别缓存最近一次
	// OCR 原文与截图，供改语言后 ScreenshotRetranslate 复用（跳过截图/OCR 直接重翻）。
	// 区分 session 避免不同入口的 OCR 结果互相覆盖。
	screenshotCache map[string]ocrCache
}

// ocrCache 单次截图 OCR 的缓存单元。
type ocrCache struct {
	text     string
	imageURL string
}

// NewService 构造翻译编排服务。app 允许在构造后通过 SetApp 注入（启动编排期 app 才就绪）。
func NewService(reg *engine.Registry, hist *historystore.Store, st *settings.Service, app *application.App) *Service {
	return &Service{
		registry:        reg,
		history:         hist,
		settings:        st,
		app:             app,
		screenshotCache: make(map[string]ocrCache),
	}
}

// SetApp 在 app 就绪后注入（启动编排阶段）。
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// screenshotWindow 按名取截图翻译窗口句柄（收口 GetByName，避免业务代码散落裸写）。
// 与 internal/service/window_wrapper.go 的 translateWindow()/settingsWindow() 同款风格。
func (s *Service) screenshotWindow() application.Window {
	if s.app == nil {
		return nil
	}
	win, ok := s.app.Window.GetByName(model.WindowScreenshot)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowScreenshot))
		return nil
	}
	return win
}

// showScreenshotWindow 呼出截图窗口（与 window_wrapper.showAndFocus / TriggerInput 同款范式）。
// 因 translate 包不能反向 import service 包（循环依赖），此处独立实现。
// 连续两次 Show() 的原因：Wails v3 (beta.9) 对 Hidden 窗口首次 Show() 仅同步创建
// webview impl 而不真正 show（webview_window.go:Show 在 impl==nil 时 InvokeSync(Run) 后 return），
// 第二次 Show() 时 impl 已就绪才会真正 show；随后 Focus() 激活前台。
// 与之相对，App.Show()/Hide() 是同步直接 cgo（见 application.go:994），非主线程调用会
// 触发 AppKit 线程断言 → SIGTRAP 崩溃，故严禁在后台 goroutine 直接调 s.app.Show()。
// 整个序列包在 InvokeAsync 主线程闭包内执行，避免跨 goroutine 建不出 impl。
func showScreenshotWindow(win application.Window) {
	if win == nil {
		slog.Error(i18n.T("log.screenshot_window_nil"))
		return
	}
	// 整个"建 impl + 显示 + 激活"序列必须在主线程执行：
	// 若从 hotkey 回调（后台 goroutine）同步调 Show()，首次建 impl 的 Run() 内部嵌套 dispatch_async +
	// 信号量同步等待主线程，极易在主线程忙时建不出 impl（IsVisible 永远 false → 窗口不显示）。
	// 用 InvokeAsync 把序列派发到主线程事件循环执行，闭包内首次 Show 的 InvokeSync(w.Run) 直接在主线程
	// 同步完成建 impl，不再跨 goroutine 等待。任意调用方（hotkey/事件）都安全，不崩。
	application.InvokeAsync(func() {
		slog.Debug(i18n.T("log.screenshot_window_show_enter",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Show()
		slog.Debug(i18n.T("log.screenshot_window_after_show",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Show()
		slog.Debug(i18n.T("log.screenshot_window_after_show",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Focus()
		slog.Debug(i18n.T("log.screenshot_window_after_focus",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))
	})
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

	start := time.Now()
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
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", true, "Err", ""))
		return res, nil
	case err := <-errCh:
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", err.Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_failed"), engineName, err)
	case <-ctx.Done():
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", ctx.Err().Error()))
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
				slog.Error(i18n.T("log.translate_multi_engine_failed"), slog.String("engine", name), slog.Any("error", err))
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
// session 标识缓存来源（events.ScreenshotSessionScreenshot / ScreenshotSessionInput），
// 用于把本次 OCR 原文与截图按入口隔离，避免不同入口互相覆盖重翻缓存。
// 返回完整结果供调用方直接使用（如需要同步回应）。
func (s *Service) ScreenshotTranslate(session string) (*model.ScreenshotResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Debug(i18n.T("log.screenshot_start"), slog.String("step", "capture_region"))
	img, err := engine.CaptureRegion(ctx)
	if err != nil {
		slog.Error(i18n.T("log.screenshot_capture_region_failed"), slog.Any("error", err))
		return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
	}
	slog.Debug(i18n.T("log.screenshot_capture_region_done"), slog.Int("image_bytes", len(img)))

	// 截图一完成（用户框选松手、img 已到手）立即呼出窗口，不必等 OCR 与翻译
	// （识别在页面内自行处理）。本路径与输入翻译窗口 TriggerInput（manager.go:101-102
	// 的 w.Show();w.Focus()）保持**完全一致的安全范式**：Hidden 窗口首次 Show() 时
	// Wails 仅同步创建 webview impl 而不真正 show（见 beta.9 webview_window.go:Show），
	// 需再 Show() 一次 impl 已就绪才真正显示；随后 Focus() 激活前台。
	// 注意：Focus() 内部走 InvokeSync 会派发回主线程，在 hotkey 回调等非主线程调用安全；
	// 而 App.Show()/Hide() 是同步直接 cgo 调用（application.go:994 的 impl.show()），
	// 在非主线程调用会触发 AppKit 线程断言 → SIGTRAP 崩溃（已实测栈证）。故本路径
	// 严禁用 s.app.Show()，只用窗口级的 Show()/Focus()，完全对齐 TriggerInput。
	showScreenshotWindow(s.screenshotWindow())
	imageURL := "data:image/png;base64," + encodeImage(img)
	if s.app != nil {
		s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
			Image:        imageURL,
			Text:         "",
			Translations: nil,
			To:           model.ZH,
		})
	}
	slog.Debug(i18n.T("log.screenshot_pushed_image"), slog.String("step", "image_pushed"), slog.Int("image_len", len(imageURL)))

	ocrName := s.registry.DefaultOCREngineName()
	if ocrName == "" {
		slog.Error(i18n.T("log.screenshot_no_ocr_engine"))
		return nil, fmt.Errorf(i18n.T("err.no_ocr"))
	}
	slog.Debug(i18n.T("log.screenshot_ocr_start"), slog.String("ocr_engine", ocrName))
	ocrRes, err := s.TriggerOcr(ocrName, img)
	if err != nil {
		slog.Error(i18n.T("log.screenshot_ocr_failed"), slog.String("ocr_engine", ocrName), slog.Any("error", err))
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
	slog.Debug(i18n.T("log.screenshot_ocr_done"), slog.Int("text_len", len(text)))
	if text == "" {
		slog.Warn(i18n.T("log.screenshot_ocr_empty"))
		return nil, fmt.Errorf(i18n.T("err.ocr_no_text"))
	}
	// 按 session 缓存本次 OCR 原文与截图，供改语言后 ScreenshotRetranslate 复用（隔离不同入口）。
	s.screenshotCacheMu.Lock()
	s.screenshotCache[session] = ocrCache{text: text, imageURL: imageURL}
	s.screenshotCacheMu.Unlock()

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
	slog.Debug(i18n.T("log.screenshot_translate_start"), slog.String("target", string(to)), slog.Int("text_len", len(text)))
	translations := s.translateAllStream(req, imageURL, to)

	result := model.ScreenshotResult{
		Image:        imageURL,
		Text:         text,
		Translations: translations,
		To:           to,
	}
	slog.Debug(i18n.T("log.screenshot_assemble_done"), slog.Int("image_len", len(result.Image)), slog.Int("text_len", len(result.Text)), slog.Int("translations", len(result.Translations)))
	return &result, nil
}

// ScreenshotRetranslate 改语言后重新翻译：复用指定 session 最近一次截图 OCR 的原文与截图，
// 跳过截图/OCR 阶段，直接用传入的 from/to 重新调用各引擎并增量推送 EventScreenshotOCR。
// 返回累积的翻译结果。若对应 session 尚无 OCR 缓存（未截过图）则返回错误。
func (s *Service) ScreenshotRetranslate(session string, from, to model.Language) error {
	s.screenshotCacheMu.RLock()
	cache, ok := s.screenshotCache[session]
	s.screenshotCacheMu.RUnlock()
	if !ok || cache.text == "" || cache.imageURL == "" {
		return fmt.Errorf(i18n.T("err.screenshot_no_cache"))
	}
	req := model.TranslateRequest{Text: cache.text, From: from, To: to}
	slog.Debug(i18n.T("log.screenshot_retranslate_start"),
		slog.String("session", session),
		slog.String("from", string(from)), slog.String("to", string(to)),
		slog.Int("text_len", len(req.Text)))
	s.translateAllStream(req, cache.imageURL, to)
	return nil
}

// translateAllStream 并发调用所有已开启的翻译引擎（每个引擎独立 goroutine，互不阻塞）；
// 每完成一条（成功或失败占位）即 Emit 一次 EventScreenshotOCR（携带已累积的 translations），
// 前端按 engine 去重增量追加，实现译文逐条到达、google 超时不再拖住 deepl 等其它引擎。
// 返回最终累积的翻译结果列表（顺序按引擎注册顺序，非完成顺序）。
func (s *Service) translateAllStream(req model.TranslateRequest, imageURL string, to model.Language) []model.TranslateResult {
	metas := s.registry.AllEngines()
	type task struct {
		meta engine.EngineMeta
		reg  engine.Translator
	}
	tasks := make([]task, 0, len(metas))
	for _, meta := range metas {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		tasks = append(tasks, task{meta: meta, reg: reg})
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	// out 按引擎注册顺序保留槽位，并发写各自下标，避免追加竞态。
	out := make([]model.TranslateResult, len(tasks))
	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, meta engine.EngineMeta, reg engine.Translator) {
			defer wg.Done()
			slog.Debug(i18n.T("log.screenshot_engine_start"), slog.String("engine", meta.Name))
			res, err := s.translateWithEngine(reg, meta.Name, req)
			var item model.TranslateResult
			if err != nil {
				slog.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", err))
				// 失败也追加占位卡片，让用户看到哪个引擎没翻出来。
				item = model.TranslateResult{
					Engine: meta.Name,
					From:   req.From,
					To:     req.To,
					Text:   req.Text,
					Result: "",
				}
			} else {
				s.saveHistory(res)
				item = *res
			}
			mu.Lock()
			out[idx] = item
			// 每完成一条就增量推送当前已完成的全部结果（未完成引擎的槽位为空，前端按 engine 去重追加，空 Result 当作占位）。
			partial := make([]model.TranslateResult, 0, len(out))
			for _, o := range out {
				if o.Engine != "" {
					partial = append(partial, o)
				}
			}
			if s.app != nil {
				s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
					Image:        imageURL,
					Text:         req.Text,
					Translations: partial,
					To:           to,
				})
			}
			mu.Unlock()
		}(i, t.meta, t.reg)
	}
	wg.Wait()
	// 过滤未完成的空槽（理论上 wg.Wait 后都已填好，保险）。
	final := make([]model.TranslateResult, 0, len(out))
	for _, o := range out {
		if o.Engine != "" {
			final = append(final, o)
		}
	}
	return final
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

	start := time.Now()
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
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", true, "Err", ""))
		return res, nil
	case err := <-errCh:
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", err.Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.ocr_failed"), req.Engine, err)
	case <-ctx.Done():
		slog.Error(i18n.T("log.screenshot_ocr_timeout"), slog.String("ocr_engine", req.Engine), slog.Any("error", ctx.Err()))
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", ctx.Err().Error()))
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
		slog.Warn(i18n.T("log.engine_query_id_failed"), slog.String("engine", res.Engine), slog.Any("error", err))
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
		slog.Error(i18n.T("log.translate_save_history_failed"), slog.Any("error", err))
	}
}

// encodeImage 把图片字节编码为 base64 字符串（不含 data URL 前缀）。
func encodeImage(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
