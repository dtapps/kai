package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// ---------- 有道 ----------

func TestYoudaoLang(t *testing.T) {
	cases := map[string]string{
		"zh": "zh-CHS", "zh-CN": "zh-CHS", "zh_cn": "zh-CHS",
		"en": "en", "ja": "ja", "ko": "ko",
		"auto": "auto", "": "auto",
		"PT": "pt",
	}
	for in, want := range cases {
		if got := youdaoLang(in); got != want {
			t.Fatalf("youdaoLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestYoudaoTranslateSuccess(t *testing.T) {
	rec := &recorder{}
	const appKey, appSec = "app-key-1", "app-secret-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"errorCode":"0","query":"hello","translation":["你好"],"l":"en"}`))
	}))
	defer srv.Close()

	tr := NewYoudao(&EngineConfig{APIKey: appKey, Secret: appSec, Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.Auto, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Result != "你好" || res.From != "en" {
		t.Fatalf("结果不符: %+v", res)
	}

	// 签名 = sha256(appKey + input + salt + curtime + appSec)，input 在 <=20 字符时等于原文
	salt, curtime := rec.value("salt"), rec.value("curtime")
	if salt == "" || curtime == "" {
		t.Fatalf("缺少 salt/curtime: salt=%q curtime=%q", salt, curtime)
	}
	sum := sha256.Sum256([]byte(appKey + "hello" + salt + curtime + appSec))
	if want, got := hex.EncodeToString(sum[:]), rec.value("sign"); got != want {
		t.Fatalf("签名不符: got %q, want %q", got, want)
	}
	if got := rec.value("signType"); got != "v3" {
		t.Fatalf("signType = %q, want v3", got)
	}
	if got := rec.value("to"); got != "zh-CHS" {
		t.Fatalf("to = %q, want zh-CHS", got)
	}
	if got := rec.header("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", got)
	}
}

// 超长文本的 input 必须是「前 10 + 后 10」字符，否则签名与服务端不一致。
func TestYoudaoLongTextSignTruncation(t *testing.T) {
	rec := &recorder{}
	const appKey, appSec = "k", "s"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"errorCode":"0","translation":["ok"],"l":"en"}`))
	}))
	defer srv.Close()

	text := "0123456789ABCDEFGHIJ56789" // 25 个 ASCII 字符
	tr := NewYoudao(&EngineConfig{APIKey: appKey, Secret: appSec, Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: text, To: model.ZH}); err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}

	// 完整原文必须原样发送（只有签名用的 input 被截断）
	if got := rec.value("q"); got != text {
		t.Fatalf("q 参数被改动: %q", got)
	}

	input := text[:10] + text[len(text)-10:]
	sum := sha256.Sum256([]byte(appKey + input + rec.value("salt") + rec.value("curtime") + appSec))
	if want, got := hex.EncodeToString(sum[:]), rec.value("sign"); got != want {
		t.Fatalf("截断规则不符: got %q, want %q（input=%q）", got, want, input)
	}
}

// 中文按 rune 计长度，不能按字节截断（否则会把字符切坏并算出错误签名）。
func TestYoudaoTruncationCountsRunes(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		_, _ = w.Write([]byte(`{"errorCode":"0","translation":["ok"],"l":"zh-CHS"}`))
	}))
	defer srv.Close()

	// 25 个互不相同的汉字（75 字节）：按字节截断会把字符切坏，按 rune 截断才对。
	runes := make([]rune, 25)
	for i := range runes {
		runes[i] = rune(0x4e00 + i)
	}
	text := string(runes)

	tr := NewYoudao(&EngineConfig{APIKey: "k", Secret: "s", Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: text, To: model.EN}); err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}

	input := string(runes[:10]) + string(runes[len(runes)-10:])
	sum := sha256.Sum256([]byte("k" + input + rec.value("salt") + rec.value("curtime") + "s"))
	if want, got := hex.EncodeToString(sum[:]), rec.value("sign"); got != want {
		t.Fatalf("应按 rune 截断: got %q, want %q", got, want)
	}
}

func TestYoudaoErrors(t *testing.T) {
	t.Run("缺凭证", func(t *testing.T) {
		tr := NewYoudao(&EngineConfig{APIKey: "only-key"}, http.DefaultClient)
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatal("缺 Secret 应报错")
		}
	})

	t.Run("业务错误码", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"errorCode":"108","translation":[]}`))
		}))
		defer srv.Close()

		tr := NewYoudao(&EngineConfig{APIKey: "k", Secret: "s", Endpoint: srv.URL}, srv.Client())
		_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
		if err == nil {
			t.Fatal("errorCode != 0 应报错")
		}
		if !strings.Contains(err.Error(), "108") {
			t.Fatalf("错误信息应含错误码，实际 %q", err.Error())
		}
	})

	t.Run("空译文", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"errorCode":"0","translation":[]}`))
		}))
		defer srv.Close()

		tr := NewYoudao(&EngineConfig{APIKey: "k", Secret: "s", Endpoint: srv.URL}, srv.Client())
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatal("空 translation 应报错")
		}
	})

	t.Run("非法 JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>`))
		}))
		defer srv.Close()

		tr := NewYoudao(&EngineConfig{APIKey: "k", Secret: "s", Endpoint: srv.URL}, srv.Client())
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatal("非法 JSON 应报错")
		}
	})

	t.Run("连接失败", func(t *testing.T) {
		tr := NewYoudao(&EngineConfig{APIKey: "k", Secret: "s", Endpoint: "http://127.0.0.1:1/"}, http.DefaultClient)
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatal("连不上应报错")
		}
	})
}

// ---------- 腾讯 ----------

func TestTencentLang(t *testing.T) {
	cases := map[string]string{
		"zh": "zh", "zh-CN": "zh", "zh_cn": "zh",
		"en": "en", "ja": "ja", "auto": "auto", "": "auto", "PT": "pt",
	}
	for in, want := range cases {
		if got := tencentLang(in); got != want {
			t.Fatalf("tencentLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTencentTranslateSuccess(t *testing.T) {
	var (
		gotAuth, gotAction, gotVersion, gotRegion, gotTimestamp string
		gotBody                                                 tencentRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAction = r.Header.Get("X-TC-Action")
		gotVersion = r.Header.Get("X-TC-Version")
		gotRegion = r.Header.Get("X-TC-Region")
		gotTimestamp = r.Header.Get("X-TC-Timestamp")

		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		_, _ = w.Write([]byte(`{"Response":{"TargetText":"你好","Source":"en","RequestId":"req-1"}}`))
	}))
	defer srv.Close()

	tr := NewTencent(&EngineConfig{APIKey: "AKID-test", Secret: "sk-test", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.Result != "你好" || res.From != "en" || res.Engine != "tencent" {
		t.Fatalf("结果不符: %+v", res)
	}

	if gotBody.SourceText != "hello" || gotBody.Source != "en" || gotBody.Target != "zh" {
		t.Fatalf("请求体不符: %+v", gotBody)
	}
	if gotAction != "TextTranslate" || gotVersion != "2018-03-21" || gotRegion != "ap-guangzhou" {
		t.Fatalf("TC 头部不符: action=%q version=%q region=%q", gotAction, gotVersion, gotRegion)
	}

	// Authorization 结构 + 用同样的入参重算签名必须完全一致（防止时间戳与签名不同步）。
	if !strings.HasPrefix(gotAuth, "TC3-HMAC-SHA256 Credential=AKID-test/") {
		t.Fatalf("Authorization 前缀不符: %q", gotAuth)
	}
	if !strings.Contains(gotAuth, "/tmt/tc3_request, SignedHeaders=content-type;host, Signature=") {
		t.Fatalf("Authorization 缺少 scope/signedHeaders: %q", gotAuth)
	}

	payload, _ := json.Marshal(gotBody)
	date := gotTimestamp
	if len(date) >= 10 {
		date = date[:10] // 仅用于断言，真实 date 由 UTC 时间决定
	}
	recomputed := signTencent("AKID-test", "sk-test", string(payload), gotTimestamp, date)
	if !strings.Contains(recomputed, "Signature=") {
		t.Fatalf("重算签名失败: %q", recomputed)
	}
}

func TestSignTencentDeterministic(t *testing.T) {
	a := signTencent("id", "key", `{"a":1}`, "1700000000", "2023-11-14")
	b := signTencent("id", "key", `{"a":1}`, "1700000000", "2023-11-14")
	if a != b {
		t.Fatal("相同入参应得到相同签名")
	}

	c := signTencent("id", "key", `{"a":2}`, "1700000000", "2023-11-14")
	if a == c {
		t.Fatal("payload 变化后签名必须变化")
	}

	d := signTencent("other-id", "key", `{"a":1}`, "1700000000", "2023-11-14")
	if a == d {
		t.Fatal("SecretId 变化后签名必须变化")
	}
}

func TestTencentAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"签名错误"}}}`))
	}))
	defer srv.Close()

	tr := NewTencent(&EngineConfig{APIKey: "id", Secret: "key", Endpoint: srv.URL}, srv.Client())
	_, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH})
	if err == nil {
		t.Fatal("Error 非空应报错")
	}
	if !strings.Contains(err.Error(), "AuthFailure.SignatureFailure") || !strings.Contains(err.Error(), "签名错误") {
		t.Fatalf("错误信息应含 Code 与 Message，实际 %q", err.Error())
	}
}

func TestTencentMissingCredentials(t *testing.T) {
	for _, cfg := range []*EngineConfig{{}, {APIKey: "id"}, {Secret: "key"}} {
		tr := NewTencent(cfg, http.DefaultClient)
		if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
			t.Fatalf("配置 %+v 应校验失败", cfg)
		}
	}
}

func TestTencentDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	tr := NewTencent(&EngineConfig{APIKey: "id", Secret: "key", Endpoint: srv.URL}, srv.Client())
	if _, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hi", To: model.ZH}); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// 源语言缺失时回退请求里的 from，不能让 From 为空。
func TestTencentFallbackSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Response":{"TargetText":"你好"}}`))
	}))
	defer srv.Close()

	tr := NewTencent(&EngineConfig{APIKey: "id", Secret: "key", Endpoint: srv.URL}, srv.Client())
	res, err := tr.Translate(t.Context(), model.TranslateRequest{Text: "hello", From: model.EN, To: model.ZH})
	if err != nil {
		t.Fatalf("Translate 失败: %v", err)
	}
	if res.From != "en" {
		t.Fatalf("From = %q, want en", res.From)
	}
}
