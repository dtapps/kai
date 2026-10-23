package engine

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// recorder 记录服务端收到的最后一个请求，便于断言参数与头部。
type recorder struct {
	mu   sync.Mutex
	reqs []*http.Request
	form url.Values
}

func (r *recorder) add(req *http.Request) {
	_ = req.ParseForm()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
	r.form = req.Form
}

func (r *recorder) value(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.form == nil {
		return ""
	}
	return r.form.Get(key)
}

func (r *recorder) header(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		return ""
	}
	return r.reqs[len(r.reqs)-1].Header.Get(key)
}

// ---------- Google ----------

func TestGoogleTranslateSuccess(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Content-Type", "application/json")
		// gtx 不规则响应：多段需要拼接
		_, _ = w.Write([]byte(`[[["Hello ","Hello",null,null,1],["world","world",null,null,1]],null,"en",null,null,null,1.0]`))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{
		Text: "你好世界", From: model.Auto, To: model.EN,
	})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}

	if res.Result != "Hello world" {
		t.Fatalf("多段译文未拼接，实际 %q", res.Result)
	}
	if res.From != "en" {
		t.Fatalf("检测语言 = %q, want en", res.From)
	}
	if res.Engine != "google" || res.Text != "你好世界" {
		t.Fatalf("结果字段不符: %+v", res)
	}

	// 请求参数：gtx 端点约定 + 中文码归一化。
	if got := rec.value("client"); got != "gtx" {
		t.Fatalf("client = %q, want gtx", got)
	}
	if got := rec.value("sl"); got != "auto" {
		t.Fatalf("sl = %q, want auto", got)
	}
	if got := rec.value("tl"); got != "en" {
		t.Fatalf("tl = %q, want en", got)
	}
	if got := rec.value("q"); got != "你好世界" {
		t.Fatalf("q = %q（URL 编码可能有问题）", got)
	}
}

func TestGoogleTranslateEmptyText(t *testing.T) {
	tr := NewGoogle("http://127.0.0.1:1/should-not-be-called", http.DefaultClient)
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{To: model.ZH}); err == nil {
		t.Fatal("空文本应直接报错（不应发起请求）")
	}
}

func TestGoogleTranslateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("非 200 应报错")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("错误信息应含状态码，实际 %q", err.Error())
	}
}

func TestGoogleTranslateParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>不是 JSON</html>`))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// 响应合法但译文为空（例如只有检测语言）时应报错，而不是返回空串结果。
func TestGoogleTranslateEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[[],null,"en"]`))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("空译文应报错")
	}
}

// target=auto/空 时应回退中文，避免发出 tl=auto 这种无效请求。
func TestGoogleTranslateAutoTargetFallsBackToZH(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`[[["你好","hi",null,null,1]],null,"en"]`))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", From: model.EN, To: model.Auto}); err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if got := rec.value("tl"); got != "zh-CN" {
		t.Fatalf("tl = %q, want zh-CN", got)
	}
}

func TestGoogleResponseUnmarshal(t *testing.T) {
	t.Run("空数组", func(t *testing.T) {
		var r googleResponse
		if err := json.Unmarshal([]byte(`[]`), &r); err != nil {
			t.Fatalf("空数组不应报错: %v", err)
		}
		if r.Translated != "" || r.DetectedLang != "" {
			t.Fatalf("空数组应得到零值，实际 %+v", r)
		}
	})

	t.Run("跳过空段与非字符串", func(t *testing.T) {
		var r googleResponse
		data := `[[["A",1],["",2],[null],["B"]]]`
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if r.Translated != "AB" {
			t.Fatalf("应只拼接非空字符串段，实际 %q", r.Translated)
		}
	})

	t.Run("根不是数组", func(t *testing.T) {
		var r googleResponse
		if err := json.Unmarshal([]byte(`{"a":1}`), &r); err == nil {
			t.Fatal("根不是数组时应报错")
		}
	})
}

// ---------- DeepL ----------

func TestDeepLTranslateSuccess(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"translations":[{"detected_source_language":"EN","text":"你好"}]}`))
	}))
	defer srv.Close()

	tr := NewDeepL(&EngineConfig{APIKey: "secret-key", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.Auto, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}

	if res.Result != "你好" {
		t.Fatalf("译文 = %q", res.Result)
	}
	if res.From != "en" {
		t.Fatalf("检测语言应小写，实际 %q", res.From)
	}
	if got := rec.header("Authorization"); got != "DeepL-Auth-Key secret-key" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := rec.value("target_lang"); got != "ZH" {
		t.Fatalf("target_lang = %q, want ZH", got)
	}
	// from=auto 时不发送 source_lang，交给 DeepL 自动检测。
	if got := rec.value("source_lang"); got != "" {
		t.Fatalf("auto 源语言不应发送 source_lang，实际 %q", got)
	}
}

func TestDeepLMissingAPIKey(t *testing.T) {
	tr := NewDeepL(&EngineConfig{}, http.DefaultClient)
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("缺 Key 应报错")
	}
}

func TestDeepLAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Wrong endpoint. Use api-free.deepl.com"}`))
	}))
	defer srv.Close()

	tr := NewDeepL(&EngineConfig{APIKey: "k", Endpoint: srv.URL}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("403 应报错")
	}
	if !strings.Contains(err.Error(), "Wrong endpoint") {
		t.Fatalf("应透传上游 message，实际 %q", err.Error())
	}
}

// 非 200 且没有 message 时用状态行兜底，不能返回空错误。
func TestDeepLAPIErrorWithoutMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"translations":[]}`))
	}))
	defer srv.Close()

	tr := NewDeepL(&EngineConfig{APIKey: "k", Endpoint: srv.URL}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("空 translations 应报错")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("应含状态码，实际 %q", err.Error())
	}
}

func TestDeepLDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	tr := NewDeepL(&EngineConfig{APIKey: "k", Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// 缺 detected_source_language 时回退请求里的 from。
func TestDeepLFallbackSourceLang(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"translations":[{"text":"你好"}]}`))
	}))
	defer srv.Close()

	tr := NewDeepL(&EngineConfig{APIKey: "k", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.From != "en" {
		t.Fatalf("应回退 req.From，实际 %q", res.From)
	}
}

// ---------- 百度 ----------

func TestBaiduTranslateSuccess(t *testing.T) {
	rec := &recorder{}
	const appID, appKey = "20240101000000001", "sign-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"from":"en","to":"zh","trans_result":[{"src":"hello","dst":"你好"}]}`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{APIKey: appID, Secret: appKey, Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Result != "你好" || res.From != "en" {
		t.Fatalf("结果不符: %+v", res)
	}

	// 签名必须是 md5(appid + q + salt + key)，且 salt 与请求一致。
	salt := rec.value("salt")
	if salt == "" {
		t.Fatal("缺少 salt 参数")
	}
	sum := md5.Sum([]byte(appID + "hello" + salt + appKey))
	if want, got := hex.EncodeToString(sum[:]), rec.value("sign"); got != want {
		t.Fatalf("签名不符: got %q, want %q", got, want)
	}
	if got := rec.value("from"); got != "en" {
		t.Fatalf("from = %q, want en", got)
	}
	if got := rec.value("to"); got != "zh" {
		t.Fatalf("to = %q, want zh", got)
	}
}

// salt 每次都必须不同，否则同一文本会产生完全相同的签名（可被重放/缓存）。
func TestBaiduSaltIsUnique(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"from":"en","to":"zh","trans_result":[{"dst":"你好"}]}`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{APIKey: "id", Secret: "k", Endpoint: srv.URL}, srv.Client())
	salts := map[string]bool{}
	for i := range 3 {
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH}); err != nil {
			t.Fatalf("第 %d 次失败: %v", i, err)
		}
		s := rec.value("salt")
		if salts[s] {
			t.Fatalf("salt 重复: %q", s)
		}
		salts[s] = true
	}
}

func TestBaiduMissingCredentials(t *testing.T) {
	for _, cfg := range []*EngineConfig{
		{Engine: "baidu"},
		{Engine: "baidu", APIKey: "id"},
		{Engine: "baidu", Secret: "key"},
	} {
		tr := NewBaidu(cfg, http.DefaultClient)
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatalf("配置 %+v 应校验失败", cfg)
		}
	}
}

func TestBaiduAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":"54003","error_msg":"Invalid Access Limit"}`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{APIKey: "id", Secret: "k", Endpoint: srv.URL}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("error_code 非空应报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "54003") || !strings.Contains(msg, "Invalid Access Limit") {
		t.Fatalf("错误信息应含错误码与原因，实际 %q", msg)
	}
}

func TestBaiduEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"from":"en","to":"zh","trans_result":[]}`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{APIKey: "id", Secret: "k", Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("trans_result 为空应报错")
	}
}

func TestBaiduDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`oops`))
	}))
	defer srv.Close()

	tr := NewBaidu(&EngineConfig{APIKey: "id", Secret: "k", Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// ---------- OpenAI 兼容 ----------

func TestOpenAITranslateSuccess(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("请求路径 = %q, want /v1/chat/completions（base URL 归一化有问题）", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"  你好  "},"finish_reason":"stop"}]
		}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{
		APIKey:   "sk-test",
		Endpoint: srv.URL + "/v1",
		Extra:    `{"model":"gpt-4o-mini"}`,
	}, srv.Client())

	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Result != "你好" {
		t.Fatalf("译文应被 TrimSpace，实际 %q", res.Result)
	}
	if res.Engine != "openai" {
		t.Fatalf("Engine = %q", res.Engine)
	}
	if got := rec.header("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", got)
	}
}

// 用户把完整 chat/completions 地址填进 endpoint 时，不能拼成两段。
func TestOpenAIEndpointWithFullPath(t *testing.T) {
	paths := make([]string, 0, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "sk", Endpoint: srv.URL + "/v1/chat/completions"}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/v1/chat/completions" {
		t.Fatalf("请求路径 = %v, want [/v1/chat/completions]", paths)
	}
}

func TestOpenAIMissingAPIKey(t *testing.T) {
	tr := NewOpenAI(&EngineConfig{}, http.DefaultClient)
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("缺 Key 应报错")
	}
}

// 410 是「模型下线」的典型响应，错误信息里要带上模型名，方便用户改配置。
func TestOpenAIModelGone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"message":"The model has been deprecated","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "sk", Endpoint: srv.URL + "/v1", Extra: `{"model":"gpt-3.5-turbo"}`}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("410 应报错")
	}
	if !strings.Contains(err.Error(), "gpt-3.5-turbo") {
		t.Fatalf("错误信息应含模型名，实际 %q", err.Error())
	}
}

func TestOpenAIAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "bad", Endpoint: srv.URL + "/v1"}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("401 应报错")
	}
	if !strings.Contains(err.Error(), "Incorrect API key") {
		t.Fatalf("应透传上游 message，实际 %q", err.Error())
	}
}

// 模型拒答时 Content 为空、Refusal 有值：必须把 Refusal 作为结果，否则前端「成功但空白」。
func TestOpenAIRefusalFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"","refusal":"I cannot help with that"}}]}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "sk", Endpoint: srv.URL + "/v1"}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Result != "I cannot help with that" {
		t.Fatalf("refusal 应作为结果返回，实际 %q", res.Result)
	}
}

func TestOpenAINoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[]}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "sk", Endpoint: srv.URL + "/v1"}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("choices 为空应报错")
	}
}

// ---------- 通用：ctx 取消 ----------

func TestEngineRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // 挂住，等客户端取消
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	tr := NewGoogle(srv.URL, srv.Client())
	if _, err := tr.Translate(ctx, model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("context 已取消时应报错")
	}
}
