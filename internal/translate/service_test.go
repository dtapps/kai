package translate

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/model"
)

// ---------------------------------------------------------------------------
// 测试替身
// ---------------------------------------------------------------------------

// fakeTranslator 可控的翻译引擎：返回固定结果或固定错误。
type fakeTranslator struct {
	name     string
	result   string
	phonetic string
	dict     []model.DictItem
	err      error
}

func (f *fakeTranslator) Name() string { return f.name }

func (f *fakeTranslator) Translate(_ context.Context, _ model.TranslateRequest) (*model.TranslateResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &model.TranslateResult{
		Engine:   f.name,
		Result:   f.result,
		Phonetic: f.phonetic,
		Dict:     f.dict,
	}, nil
}

// fakeOCR 可控的 OCR 引擎。
type fakeOCR struct {
	name string
	text string
	err  error
}

func (f *fakeOCR) Name() string { return f.name }

func (f *fakeOCR) Recognize(_ context.Context, _ model.OcrRequest) (*model.OcrResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &model.OcrResult{Engine: f.name, Text: f.text}, nil
}

// newTestService 构造带真实内存历史库 / 配置库的编排服务。
// app 传 nil：所有 Event.Emit 都有 nil 判断，可在无 Wails 运行时下验证业务逻辑。
func newTestService(t *testing.T) (*Service, *engine.Registry, *historystore.Store, *configstore.Store) {
	t.Helper()

	hist, err := historystore.Open("")
	if err != nil {
		t.Fatalf("打开历史库失败: %v", err)
	}
	t.Cleanup(func() { _ = hist.Close() })

	cfg, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("打开配置库失败: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })

	reg := engine.NewRegistry()
	svc := NewService(reg, hist, nil, nil)
	svc.SetConfigStore(cfg)
	return svc, reg, hist, cfg
}

// registerEngineConfig 在配置库登记引擎行，使 saveHistory 能解析出 engine_id。
// 注意：历史去重键是 (text, engine_id, from, to, from_ocr)——不含引擎名，
// 未登记配置的引擎 engine_id 恒为 0，会导致不同引擎的相同原文互相误判为重复。
func registerEngineConfig(t *testing.T, cfg *configstore.Store, name string) int64 {
	t.Helper()
	id, err := cfg.InsertEngineConfig(t.Context(), &engine.EngineConfig{Engine: name, Enabled: true})
	if err != nil {
		t.Fatalf("登记引擎 %s 失败: %v", name, err)
	}
	row, err := cfg.GetEngineByName(t.Context(), name)
	if err != nil || row == nil {
		t.Fatalf("回查引擎 %s 失败: row=%v err=%v", name, row, err)
	}
	if row.ID != id {
		t.Fatalf("引擎 %s 的 ID 不一致: 插入 %d, 回查 %d", name, id, row.ID)
	}
	return id
}

// waitHistoryCount 轮询等待历史条数（多引擎并行是异步写库的）。
func waitHistoryCount(t *testing.T, hist *historystore.Store, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		n, err := hist.CountByKeyword(t.Context(), "")
		if err == nil && n == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待历史条数 = %d 超时, 实际 %d (err=%v)", want, n, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Translate（单引擎）
// ---------------------------------------------------------------------------

// TestTranslateSuccess 成功路径：结果字段回填 + 落一条历史。
func TestTranslateSuccess(t *testing.T) {
	svc, reg, hist, cfg := newTestService(t)
	engineID := registerEngineConfig(t, cfg, "google")
	reg.RegisterTranslator(&fakeTranslator{
		name:     "google",
		result:   "你好",
		phonetic: "ni hao",
		dict:     []model.DictItem{{Word: "hello", Pos: "int.", Explain: "你好"}},
	})

	res, err := svc.Translate(model.TranslateRequest{
		Text:       "hello",
		From:       model.Auto,
		To:         model.ZH,
		EngineName: "google",
	})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Engine != "google" || res.Result != "你好" {
		t.Fatalf("结果不正确: %+v", res)
	}
	if res.Text != "hello" || res.From != model.Auto || res.To != model.ZH {
		t.Fatalf("请求字段未回填: %+v", res)
	}
	if res.Phonetic != "ni hao" || len(res.Dict) != 1 {
		t.Fatalf("音标/词典丢失: %+v", res)
	}

	waitHistoryCount(t, hist, 1)
	rows, err := hist.QueryByKeyword(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if rows[0].Text != "hello" || rows[0].Result != "你好" ||
		rows[0].FromLang != string(model.Auto) || rows[0].ToLang != string(model.ZH) {
		t.Fatalf("历史字段不正确: %+v", rows[0])
	}
	if rows[0].EngineID != engineID {
		t.Fatalf("历史 engine_id 应为配置库中的 %d, 实际 %d", engineID, rows[0].EngineID)
	}
}

// TestTranslateEngineNotRegistered 未注册引擎必须显式报错（含引擎名，便于前端定位）。
func TestTranslateEngineNotRegistered(t *testing.T) {
	svc, _, _, _ := newTestService(t)

	_, err := svc.Translate(model.TranslateRequest{Text: "hi", To: model.ZH, EngineName: "nope"})
	if err == nil {
		t.Fatal("未注册引擎应返回错误")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("错误信息应包含引擎名, 实际: %v", err)
	}
}

// TestTranslateEngineError 引擎报错时错误必须可被 errors.Is 追溯，
// 且不得写入历史。
func TestTranslateEngineError(t *testing.T) {
	svc, reg, hist, _ := newTestService(t)
	boom := errors.New("engine boom")
	reg.RegisterTranslator(&fakeTranslator{name: "deepl", err: boom})

	_, err := svc.Translate(model.TranslateRequest{Text: "hi", To: model.ZH, EngineName: "deepl"})
	if err == nil {
		t.Fatal("引擎报错应向上返回")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("错误应包裹原始错误, 实际: %v", err)
	}
	if n, _ := hist.CountByKeyword(t.Context(), ""); n != 0 {
		t.Fatalf("失败不应写历史, 实际 %d 条", n)
	}
}

// TestTranslateDedupesIdenticalHistory 相同内容重复翻译只入库一次。
func TestTranslateDedupesIdenticalHistory(t *testing.T) {
	svc, reg, hist, _ := newTestService(t)
	reg.RegisterTranslator(&fakeTranslator{name: "google", result: "你好"})

	req := model.TranslateRequest{Text: "hello", From: model.Auto, To: model.ZH, EngineName: "google"}
	for range 3 {
		if _, err := svc.Translate(req); err != nil {
			t.Fatalf("Translate 失败: %v", err)
		}
	}
	if n, err := hist.CountByKeyword(t.Context(), ""); err != nil || n != 1 {
		t.Fatalf("相同内容应只入库 1 条, 实际 %d (err=%v)", n, err)
	}

	// 换目标语言属于不同翻译，应新增一条。
	req.To = model.JA
	if _, err := svc.Translate(req); err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if n, err := hist.CountByKeyword(t.Context(), ""); err != nil || n != 2 {
		t.Fatalf("不同目标语言应新增, 实际 %d (err=%v)", n, err)
	}
}

// ---------------------------------------------------------------------------
// TranslateMulti（多引擎并行）
// ---------------------------------------------------------------------------

// TestTranslateMultiOnlyStartedTranslators 只启动翻译类引擎，OCR 引擎不参与并行翻译。
func TestTranslateMultiOnlyStartedTranslators(t *testing.T) {
	svc, reg, hist, cfg := newTestService(t)
	// 必须先登记配置：去重键含 engine_id，未登记时两个引擎的 engine_id 都是 0，
	// 相同原文的第二次写入会被误判为重复。
	registerEngineConfig(t, cfg, "google")
	registerEngineConfig(t, cfg, "deepl")
	reg.RegisterTranslator(&fakeTranslator{name: "google", result: "你好"})
	reg.RegisterTranslator(&fakeTranslator{name: "deepl", result: "您好"})
	reg.RegisterOcr(&fakeOCR{name: "vision", text: "ocr"})

	res, err := svc.TranslateMulti(model.TranslateRequest{Text: "hello", To: model.ZH})
	if err != nil {
		t.Fatalf("TranslateMulti 失败: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("应启动 2 个翻译引擎（OCR 不算）, 实际 %d", res.Count)
	}
	// 结果事件异步发出，历史共 2 条（每个引擎各 1 条）。
	waitHistoryCount(t, hist, 2)

	rows, err := hist.QueryByKeyword(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Result] = true
	}
	if !got["你好"] || !got["您好"] {
		t.Fatalf("两个引擎的结果都应入库: %+v", rows)
	}
}

// TestTranslateMultiAllFail 全部引擎失败时 Count 仍为已启动数，且不写历史。
func TestTranslateMultiAllFail(t *testing.T) {
	svc, reg, hist, _ := newTestService(t)
	reg.RegisterTranslator(&fakeTranslator{name: "google", err: errors.New("fail 1")})
	reg.RegisterTranslator(&fakeTranslator{name: "deepl", err: errors.New("fail 2")})

	res, err := svc.TranslateMulti(model.TranslateRequest{Text: "hello", To: model.ZH})
	if err != nil {
		t.Fatalf("TranslateMulti 不应因引擎失败报错: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("Count 应为 2, 实际 %d", res.Count)
	}
	// 给异步 goroutine 一点时间确认「没有写入」。
	time.Sleep(200 * time.Millisecond)
	if n, _ := hist.CountByKeyword(t.Context(), ""); n != 0 {
		t.Fatalf("全部失败不应写历史, 实际 %d 条", n)
	}
}

// TestTranslateMultiEmptyRegistry 没有可用引擎时 Count 为 0，不 panic。
func TestTranslateMultiEmptyRegistry(t *testing.T) {
	svc, _, _, _ := newTestService(t)

	res, err := svc.TranslateMulti(model.TranslateRequest{Text: "hello", To: model.ZH})
	if err != nil {
		t.Fatalf("空注册表不应报错: %v", err)
	}
	if res.Count != 0 {
		t.Fatalf("Count 应为 0, 实际 %d", res.Count)
	}
}

// ---------------------------------------------------------------------------
// OCR
// ---------------------------------------------------------------------------

// TestOcr 图片为空 / 引擎未注册 / 正常识别三条路径。
func TestOcr(t *testing.T) {
	svc, reg, _, _ := newTestService(t)
	reg.RegisterOcr(&fakeOCR{name: "vision", text: "识别结果"})

	t.Run("图片为空", func(t *testing.T) {
		if _, err := svc.Ocr(model.OcrRequest{Engine: "vision"}); err == nil {
			t.Fatal("空图片应报错")
		}
	})
	t.Run("引擎未注册", func(t *testing.T) {
		_, err := svc.Ocr(model.OcrRequest{ImageData: []byte("png"), Engine: "nope"})
		if err == nil || !strings.Contains(err.Error(), "nope") {
			t.Fatalf("未注册引擎应报错并含引擎名, 实际 %v", err)
		}
	})
	t.Run("正常识别", func(t *testing.T) {
		res, err := svc.Ocr(model.OcrRequest{ImageData: []byte("png"), Engine: "vision"})
		if err != nil {
			t.Fatalf("Ocr 失败: %v", err)
		}
		if res.Text != "识别结果" || res.Engine != "vision" {
			t.Fatalf("识别结果不正确: %+v", res)
		}
	})
}

// TestTriggerOcr 供截图流程复用的无 req 版本。
func TestTriggerOcr(t *testing.T) {
	svc, reg, _, _ := newTestService(t)
	reg.RegisterOcr(&fakeOCR{name: "vision", text: "ok"})

	res, err := svc.TriggerOcr("vision", []byte("png"))
	if err != nil {
		t.Fatalf("TriggerOcr 失败: %v", err)
	}
	if res.Text != "ok" {
		t.Fatalf("结果不正确: %+v", res)
	}
	if _, err := svc.TriggerOcr("nope", []byte("png")); err == nil {
		t.Fatal("未注册引擎应报错")
	}

	// OCR 引擎自身报错必须透出（前端据此停止转圈）。
	reg.RegisterOcr(&fakeOCR{name: "broken", err: errors.New("ocr boom")})
	if _, err := svc.TriggerOcr("broken", []byte("png")); err == nil {
		t.Fatal("OCR 引擎报错应向上返回")
	}
}

// ---------------------------------------------------------------------------
// 截图重翻（缓存）
// ---------------------------------------------------------------------------

// TestScreenshotRetranslateNoCache 未截过图时应明确报错，而不是静默成功。
func TestScreenshotRetranslateNoCache(t *testing.T) {
	svc, _, _, _ := newTestService(t)

	if err := svc.ScreenshotRetranslate("screenshot", model.Auto, model.ZH); err == nil {
		t.Fatal("无缓存时应返回错误")
	}
}

// TestScreenshotRetranslateUsesCache 有缓存时跳过截图/OCR，直接用新语言重翻并写历史。
func TestScreenshotRetranslateUsesCache(t *testing.T) {
	svc, reg, hist, _ := newTestService(t)
	reg.RegisterTranslator(&fakeTranslator{name: "google", result: "译文"})

	// 直接注入缓存（等价于上一次 ScreenshotOCR 的结果）。
	svc.screenshotCache["screenshot"] = ocrCache{text: "原文", imageURL: "data:image/png;base64,AAA"}

	if err := svc.ScreenshotRetranslate("screenshot", model.ZH, model.EN); err != nil {
		t.Fatalf("ScreenshotRetranslate 失败: %v", err)
	}
	waitHistoryCount(t, hist, 1)
	rows, _ := hist.QueryByKeyword(t.Context(), "", 10, 0)
	if rows[0].Text != "原文" || rows[0].FromLang != string(model.ZH) || rows[0].ToLang != string(model.EN) {
		t.Fatalf("重翻历史不正确: %+v", rows[0])
	}

	// 不同 session 的缓存互不干扰。
	if err := svc.ScreenshotRetranslate("input", model.Auto, model.ZH); err == nil {
		t.Fatal("未缓存的 session 应报错")
	}
}

// ---------------------------------------------------------------------------
// translateAllStream / 辅助
// ---------------------------------------------------------------------------

// TestTranslateAllStreamKeepsFailurePlaceholder 失败的引擎也要占位，
// 前端才能显示「哪个引擎没翻出来」。
func TestTranslateAllStreamKeepsFailurePlaceholder(t *testing.T) {
	svc, reg, hist, _ := newTestService(t)
	reg.RegisterTranslator(&fakeTranslator{name: "google", result: "你好"})
	reg.RegisterTranslator(&fakeTranslator{name: "deepl", err: errors.New("boom")})

	out := svc.translateAllStream(
		model.TranslateRequest{Text: "hello", From: model.Auto, To: model.ZH},
		"data:image/png;base64,AAA", model.ZH,
	)
	if len(out) != 2 {
		t.Fatalf("两个引擎都应出现在结果中, 实际 %d", len(out))
	}
	byEngine := map[string]model.TranslateResult{}
	for _, r := range out {
		byEngine[r.Engine] = r
	}
	if got := byEngine["google"]; got.Result != "你好" {
		t.Fatalf("成功引擎结果错误: %+v", got)
	}
	if got := byEngine["deepl"]; got.Result != "" || got.Text != "hello" {
		t.Fatalf("失败引擎应保留空结果占位: %+v", got)
	}
	// 只有成功的引擎写历史。
	if n, _ := hist.CountByKeyword(t.Context(), ""); n != 1 {
		t.Fatalf("只有成功引擎应写历史, 实际 %d 条", n)
	}
}

// TestEnabledTranslatorNames 统计上报的引擎名只含翻译类引擎。
func TestEnabledTranslatorNames(t *testing.T) {
	svc, reg, _, _ := newTestService(t)
	reg.RegisterTranslator(&fakeTranslator{name: "google"})
	reg.RegisterOcr(&fakeOCR{name: "vision"})

	names := svc.enabledTranslatorNames()
	if len(names) != 1 || names[0] != "google" {
		t.Fatalf("应只返回翻译引擎, 实际 %v", names)
	}
}

// TestSaveHistoryNilSafety 历史库或结果为空时不得 panic（服务可能未装配）。
func TestSaveHistoryNilSafety(t *testing.T) {
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	svc.saveHistory(nil)
	svc.saveHistory(&model.TranslateResult{Engine: "google", Text: "a", Result: "b"})
}

// TestSaveHistoryMarksFromOCR OCR 来源必须落库为 from_ocr=1（历史页要区分入口）。
func TestSaveHistoryMarksFromOCR(t *testing.T) {
	svc, _, hist, _ := newTestService(t)

	svc.saveHistory(&model.TranslateResult{
		Engine: "google", From: model.Auto, To: model.ZH,
		Text: "截图文字", Result: "译文", FromOCR: true,
	})

	rows, err := hist.QueryByKeyword(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatalf("查询历史失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应写入 1 条, 实际 %d", len(rows))
	}
	if rows[0].FromOcr != 1 {
		t.Fatalf("FromOCR 应落库为 1, 实际 %d", rows[0].FromOcr)
	}
}

// TestEncodeImage base64 编码不带 data URL 前缀（前缀由调用方拼接）。
func TestEncodeImage(t *testing.T) {
	raw := []byte("PNG-BYTES")
	if got, want := encodeImage(raw), base64.StdEncoding.EncodeToString(raw); got != want {
		t.Fatalf("encodeImage = %q, 期望 %q", got, want)
	}
}

// TestScreenshotWindowWithoutApp app 未注入时取窗口必须安全返回 nil。
func TestScreenshotWindowWithoutApp(t *testing.T) {
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	if win := svc.screenshotWindow(); win != nil {
		t.Fatal("app 为 nil 时应返回 nil 窗口")
	}
}
