package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/model"
)

// ---------- 语言码映射 ----------

func TestGoogleLang(t *testing.T) {
	cases := map[string]string{
		"zh": "zh-CN", "zh-CN": "zh-CN", "zh_CN": "zh-CN",
		"auto": "auto", "": "auto",
		"en": "en", "ja": "ja", "fr": "fr",
	}
	for in, want := range cases {
		if got := googleLang(in); got != want {
			t.Fatalf("googleLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeepLLang(t *testing.T) {
	cases := map[string]string{
		"zh": "ZH", "zh-CN": "ZH", "zh_cn": "ZH",
		"en": "EN", "ja": "JA", "ko": "KO", "fr": "FR", "de": "DE", "es": "ES", "ru": "RU",
		"auto": "", "": "",
		"pt":    "PT", // 未列出的语言原样大写
		"zh-TW": "ZH-TW",
		"ZH":    "ZH",
	}
	for in, want := range cases {
		if got := deeplLang(in); got != want {
			t.Fatalf("deeplLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBaiduLang(t *testing.T) {
	cases := map[string]string{
		"zh": "zh", "zh-CN": "zh", "zh_cn": "zh",
		"en": "en", "ja": "jp", "ko": "kor", "fr": "fra", "de": "de", "es": "spa", "ru": "ru",
		"auto": "auto", "": "auto",
		"PT": "pt",
	}
	for in, want := range cases {
		if got := baiduLang(in); got != want {
			t.Fatalf("baiduLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------- 引擎元信息 / 注册表 ----------

func TestKindOfEngine(t *testing.T) {
	ocr := []string{"tesseract", "vision"}
	for _, n := range ocr {
		if got := KindOfEngine(n); got != KindOCR {
			t.Fatalf("KindOfEngine(%q) = %q, want ocr", n, got)
		}
	}
	for _, n := range []string{"apple", "google", "deepl", "openai", "anthropic", "gemini", "baidu", "tencent", "youdao"} {
		if got := KindOfEngine(n); got != KindTranslator {
			t.Fatalf("KindOfEngine(%q) = %q, want translate", n, got)
		}
	}
}

func TestEngineSupported(t *testing.T) {
	for _, n := range []string{"apple", "vision"} {
		want := runtime.GOOS == "darwin"
		if got := EngineSupported(n); got != want {
			t.Fatalf("EngineSupported(%q) = %v, want %v（GOOS=%s）", n, got, want, runtime.GOOS)
		}
	}
	for _, n := range []string{"google", "deepl", "openai", "baidu", "tesseract"} {
		if !EngineSupported(n) {
			t.Fatalf("%q 应跨平台可用", n)
		}
	}
	// 未知名默认可用（插件式引擎不会因未登记而被禁用）。
	if !EngineSupported("some-future-engine") {
		t.Fatal("未知引擎应默认可用")
	}
}

func TestKnownEnginesCoversAllBuiltins(t *testing.T) {
	metas := KnownEngines()
	if len(metas) < 10 {
		t.Fatalf("KnownEngines 数量 = %d，明显偏少", len(metas))
	}

	byName := map[string]EngineMeta{}
	for _, m := range metas {
		if m.Name == "" {
			t.Fatal("存在空名引擎")
		}
		if _, dup := byName[m.Name]; dup {
			t.Fatalf("引擎 %q 重复", m.Name)
		}
		if m.Kind != KindTranslator && m.Kind != KindOCR {
			t.Fatalf("引擎 %q 的 Kind = %q 非法", m.Name, m.Kind)
		}
		byName[m.Name] = m
	}

	for _, want := range []string{"apple", "google", "deepl", "openai", "baidu", "tencent", "youdao", "tesseract", "vision"} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("KnownEngines 缺少 %q", want)
		}
	}
}

func TestEngineMap(t *testing.T) {
	in := []*EngineConfig{
		{Engine: "google"},
		nil, // 必须跳过而不是 panic
		{Engine: "deepl"},
	}
	m := EngineMap(in)
	if len(m) != 2 {
		t.Fatalf("EngineMap 长度 = %d, want 2", len(m))
	}
	if _, ok := m["google"]; !ok {
		t.Fatal("缺少 google")
	}
	if _, ok := m["deepl"]; !ok {
		t.Fatal("缺少 deepl")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if len(r.TranslatorNames()) != 0 || len(r.OcrNames()) != 0 {
		t.Fatal("新注册表应为空")
	}
	if got := r.DefaultOCREngineName(); got != "" {
		t.Fatalf("无 OCR 引擎时应返回空串，实际 %q", got)
	}
	if _, ok := r.GetTranslator("google"); ok {
		t.Fatal("空注册表不应查到 google")
	}

	tr := NewGoogle("", http.DefaultClient)
	r.RegisterTranslator(tr)
	r.RegisterOcr(NewTesseractOCR(&EngineConfig{}))

	if got, ok := r.GetTranslator("google"); !ok || got.Name() != "google" {
		t.Fatalf("GetTranslator(google) = %v, %v", got, ok)
	}
	if _, ok := r.GetOcr("tesseract"); !ok {
		t.Fatal("GetOcr(tesseract) 未命中")
	}
	if got := r.DefaultOCREngineName(); got != "tesseract" {
		t.Fatalf("DefaultOCREngineName() = %q, want tesseract", got)
	}

	names := r.TranslatorNames()
	if len(names) != 1 || names[0] != "google" {
		t.Fatalf("TranslatorNames() = %v", names)
	}

	all := r.AllEngines()
	if len(all) != 2 {
		t.Fatalf("AllEngines 数量 = %d, want 2", len(all))
	}
}

// ---------- schema / 必填校验 ----------

func TestGetEngineSchema(t *testing.T) {
	for _, name := range []string{"google", "deepl", "openai", "baidu", "tencent", "youdao", "tesseract", "vision"} {
		s := GetEngineSchema(name)
		if len(s.Fields) == 0 {
			t.Fatalf("引擎 %q 的 schema 为空", name)
		}
		for _, f := range s.Fields {
			if f.Field == "" || f.LabelKey == "" {
				t.Fatalf("引擎 %q 有字段缺 field/labelKey: %+v", name, f)
			}
		}
	}

	if s := GetEngineSchema("no-such-engine"); len(s.Fields) != 0 {
		t.Fatalf("未知引擎应返回空 schema，实际 %d 个字段", len(s.Fields))
	}

	// 免 Key 引擎不应把 api_key 标为必填，否则默认配置无法启用。
	for _, name := range []string{"google", "apple"} {
		for _, f := range GetEngineSchema(name).Fields {
			if f.Field == "api_key" && f.Required {
				t.Fatalf("免 Key 引擎 %q 不应要求 api_key", name)
			}
		}
	}
}

func TestValidateRequired(t *testing.T) {
	if got := ValidateRequired(nil); got != nil {
		t.Fatalf("nil 配置应返回 nil，实际 %+v", got)
	}

	// 缺 api_key 的引擎必须报错，并指向 api_key 字段。
	missing := ValidateRequired(&EngineConfig{Engine: "deepl"})
	if missing == nil {
		t.Fatal("deepl 缺少 APIKey 时应校验失败")
	}
	if missing.Field != "api_key" {
		t.Fatalf("应指向 api_key，实际 %q", missing.Field)
	}
	if missing.LabelKey == "" {
		t.Fatal("返回的字段应带 LabelKey 供前端提示")
	}

	// 填齐后通过。
	if got := ValidateRequired(&EngineConfig{Engine: "deepl", APIKey: "k"}); got != nil {
		t.Fatalf("填齐后不应报错，实际缺 %q", got.Field)
	}

	// 百度需要 secret。
	if got := ValidateRequired(&EngineConfig{Engine: "baidu", APIKey: "id"}); got == nil || got.Field != "secret" {
		t.Fatalf("baidu 缺 secret 应报错，实际 %+v", got)
	}

	// 免 Key 引擎永远通过。
	if got := ValidateRequired(&EngineConfig{Engine: "google"}); got != nil {
		t.Fatalf("google 无必填项，实际要求 %q", got.Field)
	}

	// 未知引擎没有 schema，应直接通过而不是 panic。
	if got := ValidateRequired(&EngineConfig{Engine: "unknown"}); got != nil {
		t.Fatalf("未知引擎应通过，实际 %+v", got)
	}
}

func TestDefaultEngineConfigs(t *testing.T) {
	cfgs := DefaultEngineConfigs()
	if len(cfgs) == 0 {
		t.Fatal("默认配置不应为空")
	}

	seen := map[string]bool{}
	for _, c := range cfgs {
		if c.Engine == "" {
			t.Fatal("默认配置里有空引擎名")
		}
		if seen[c.Engine] {
			t.Fatalf("默认引擎 %q 重复", c.Engine)
		}
		seen[c.Engine] = true
		if !c.Enabled {
			t.Fatalf("内置引擎 %q 应默认启用", c.Engine)
		}
		if !EngineSupported(c.Engine) {
			t.Fatalf("不可用平台引擎 %q 不应预置", c.Engine)
		}
		// 预置的必然能通过必填校验，否则启动即报错。
		if f := ValidateRequired(c); f != nil {
			t.Fatalf("默认引擎 %q 缺必填字段 %q", c.Engine, f.Field)
		}
	}

	// vision 的 OCR 参数必须是合法 JSON。
	for _, c := range cfgs {
		if c.Engine == "vision" && c.Extra != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(c.Extra), &m); err != nil {
				t.Fatalf("vision 的 Extra 不是合法 JSON: %q", c.Extra)
			}
			if _, ok := m["timeout_sec"]; !ok {
				t.Fatalf("vision 的 Extra 应带 timeout_sec: %q", c.Extra)
			}
		}
	}
}

// ---------- Extra 解析 ----------

func TestParseLLMExtra(t *testing.T) {
	t.Run("空", func(t *testing.T) {
		ex := parseLLMExtra("")
		if ex.Model != "" {
			t.Fatalf("空 Extra 不应有模型名，实际 %q", ex.Model)
		}
		if ex.TimeoutSec != DefaultLLMTimeoutSec {
			t.Fatalf("超时应回落默认 %d，实际 %d", DefaultLLMTimeoutSec, ex.TimeoutSec)
		}
	})

	t.Run("JSON", func(t *testing.T) {
		ex := parseLLMExtra(`{"model":"gpt-4o","timeout_sec":90}`)
		if ex.Model != "gpt-4o" || ex.TimeoutSec != 90 {
			t.Fatalf("解析结果 = %+v", ex)
		}
	})

	t.Run("旧版纯模型名", func(t *testing.T) {
		ex := parseLLMExtra("claude-3-5-sonnet")
		if ex.Model != "claude-3-5-sonnet" {
			t.Fatalf("兼容旧格式失败: %q", ex.Model)
		}
		if ex.TimeoutSec != DefaultLLMTimeoutSec {
			t.Fatalf("超时应回落默认，实际 %d", ex.TimeoutSec)
		}
	})

	t.Run("负超时回落默认", func(t *testing.T) {
		ex := parseLLMExtra(`{"timeout_sec":-1}`)
		if ex.TimeoutSec != DefaultLLMTimeoutSec {
			t.Fatalf("<=0 的超时应回落默认，实际 %d", ex.TimeoutSec)
		}
	})
}

func TestParseOCRExtra(t *testing.T) {
	t.Run("空", func(t *testing.T) {
		ex := parseOCRExtra("vision", "")
		if ex.Langs != DefaultOCRLangs["vision"] {
			t.Fatalf("语言码 = %q, want %q", ex.Langs, DefaultOCRLangs["vision"])
		}
		if ex.TimeoutSec != DefaultOCRTimeoutSec {
			t.Fatalf("超时 = %d, want %d", ex.TimeoutSec, DefaultOCRTimeoutSec)
		}
		if ex.Correct != nil || ex.RetryCount != nil {
			t.Fatalf("缺省时指针应为 nil: %+v", ex)
		}
	})

	t.Run("JSON 全字段", func(t *testing.T) {
		ex := parseOCRExtra("tesseract", `{"langs":"eng","timeout_sec":5,"correct_text":false,"retry_count":0}`)
		if ex.Langs != "eng" || ex.TimeoutSec != 5 {
			t.Fatalf("解析结果 = %+v", ex)
		}
		if ex.Correct == nil || *ex.Correct {
			t.Fatal("correct_text=false 应被保留为 false")
		}
		if ex.RetryCount == nil || *ex.RetryCount != 0 {
			t.Fatal("retry_count=0 应被保留为 0（表示关闭重试）")
		}
	})

	t.Run("旧版纯语言码", func(t *testing.T) {
		ex := parseOCRExtra("vision", "chi_sim+eng")
		if ex.Langs != "chi_sim+eng" {
			t.Fatalf("兼容旧格式失败: %q", ex.Langs)
		}
		if ex.TimeoutSec != DefaultOCRTimeoutSec {
			t.Fatalf("超时应回落默认，实际 %d", ex.TimeoutSec)
		}
	})

	t.Run("未知引擎用后仍不 panic", func(t *testing.T) {
		if ex := parseOCRExtra("nope", ""); ex.TimeoutSec != DefaultOCRTimeoutSec {
			t.Fatalf("超时 = %d", ex.TimeoutSec)
		}
	})
}

func TestOcrLangsOptionsReturnsCopy(t *testing.T) {
	a := OcrLangsOptions()
	if len(a) == 0 {
		t.Fatal("候选项不应为空")
	}
	a[0] = "被篡改"
	if b := OcrLangsOptions(); b[0] == "被篡改" {
		t.Fatal("OcrLangsOptions 必须返回副本，调用方不应影响内部状态")
	}
}

func TestCloneHTTPClientWithTimeout(t *testing.T) {
	base := &http.Client{Timeout: 99 * time.Second}
	c := cloneHTTPClientWithTimeout(base, 5)
	if c.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v, want 5s", c.Timeout)
	}
	if c == base {
		t.Fatal("应返回新实例，避免污染共享 client")
	}
	if base.Timeout != 99*time.Second {
		t.Fatalf("原 client 被修改: %v", base.Timeout)
	}
	if c.Transport != base.Transport {
		t.Fatal("应复用原 Transport（复用连接池）")
	}

	// <=0 回落默认。
	if got := cloneHTTPClientWithTimeout(base, 0); got.Timeout != DefaultLLMTimeoutSec*time.Second {
		t.Fatalf("0 超时应回落默认，实际 %v", got.Timeout)
	}
	if got := cloneHTTPClientWithTimeout(base, -3); got.Timeout != DefaultLLMTimeoutSec*time.Second {
		t.Fatalf("负超时应回落默认，实际 %v", got.Timeout)
	}

	// base 为 nil 时自建，不能 panic。
	fallback := cloneHTTPClientWithTimeout(nil, 3)
	if fallback.Transport == nil || fallback.Timeout != 3*time.Second {
		t.Fatalf("nil base 兜底失败: %+v", fallback)
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"   ":                        "",
		"https://api.openai.com/v1":  "https://api.openai.com/v1",
		"https://api.openai.com/v1/": "https://api.openai.com/v1",
		"https://api.openai.com/v1/chat/completions": "https://api.openai.com/v1",
		"  https://x.dev/v1/chat/completions/  ":     "https://x.dev/v1",
	}
	for in, want := range cases {
		if got := normalizeOpenAIBaseURL(in); got != want {
			t.Fatalf("normalizeOpenAIBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------- 平台可用性 / 自动源语言 ----------

type fakeTranslator struct {
	name string
}

func (f *fakeTranslator) Name() string { return f.name }
func (f *fakeTranslator) Translate(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	return &model.TranslateResult{Text: req.Text}, nil
}

type fakeAutoTranslator struct {
	fakeTranslator
	supports bool
}

func (f *fakeAutoTranslator) SupportsAutoSource() bool { return f.supports }

func TestSupportsAutoSource(t *testing.T) {
	// 未实现可选接口 → 默认支持。
	if !SupportsAutoSource(&fakeTranslator{name: "x"}) {
		t.Fatal("未实现可选接口的引擎应默认支持 auto")
	}
	if SupportsAutoSource(&fakeAutoTranslator{name: "y", supports: false}) {
		t.Fatal("显式声明不支持时应为 false")
	}
	if !SupportsAutoSource(&fakeAutoTranslator{name: "z", supports: true}) {
		t.Fatal("显式声明支持时应为 true")
	}
	// apple 引擎显式实现了该方法并返回 true（Swift 侧用 NaturalLanguage 自动识别）。
	// 注意 engine.go 里 autoSourceSupporter 的注释说「系统翻译必须显式源语言」，
	// 与 apple_darwin.go 的实际实现相反，属于过时注释。
	if runtime.GOOS == "darwin" {
		if !SupportsAutoSource(NewApple()) {
			t.Fatal("apple 引擎应声明支持 auto 源语言（见 apple_darwin.go:65）")
		}
	}
}
