package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestNewErrorFields(t *testing.T) {
	err := NewError("err.empty_text", "name", "明文")
	if err.Key != "err.empty_text" {
		t.Fatalf("Key = %q", err.Key)
	}
	if err.Params["name"] != "明文" {
		t.Fatalf("Params 未解析: %v", err.Params)
	}
	if err.Err != nil {
		t.Fatalf("NewError 不应带底层错误，实际 %v", err.Err)
	}
	if err.Unwrap() != nil {
		t.Fatal("Unwrap() 应为 nil")
	}
}

// 信封格式：已翻译文本 + 分隔符 + key + 分隔符 + 参数 JSON。
func TestErrorEnvelope(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	err := NewError("err.empty_text", "name", "明文")
	s := err.Error()

	if n := strings.Count(s, sep); n != 2 {
		t.Fatalf("信封分隔符数量 = %d, want 2（原文 %q）", n, s)
	}

	key, params, translated, ok := ParseError(s)
	if !ok {
		t.Fatalf("ParseError 未能解析信封: %q", s)
	}
	if key != "err.empty_text" {
		t.Fatalf("解析出的 key = %q", key)
	}
	if params["name"] != "明文" {
		t.Fatalf("解析出的参数 = %v", params)
	}
	if translated == "" {
		t.Fatal("解析出的已翻译文本为空")
	}
}

func TestParseErrorPlainText(t *testing.T) {
	key, params, translated, ok := ParseError("只是普通错误文本")
	if ok {
		t.Fatal("无信封的文本不应解析成功")
	}
	if key != "" || params != nil {
		t.Fatalf("失败时应返回零值，实际 key=%q params=%v", key, params)
	}
	if translated != "只是普通错误文本" {
		t.Fatalf("失败时应回退原文，实际 %q", translated)
	}
}

// 只有一个分隔符属于畸形信封，应整体回退原文。
func TestParseErrorMalformedEnvelope(t *testing.T) {
	in := "部分文本" + sep + "err.some_key"
	key, _, translated, ok := ParseError(in)
	if ok {
		t.Fatal("畸形信封不应解析成功")
	}
	if key != "" {
		t.Fatalf("畸形信封 key 应为空，实际 %q", key)
	}
	if translated != in {
		t.Fatalf("畸形信封应回退原文，实际 %q", translated)
	}
}

func TestParseErrorBadJSON(t *testing.T) {
	in := "已翻译" + sep + "err.key" + sep + "{不是 JSON"
	key, params, _, ok := ParseError(in)
	if !ok {
		t.Fatal("JSON 非法时仍应识别出信封")
	}
	if key != "err.key" {
		t.Fatalf("key = %q", key)
	}
	if len(params) != 0 {
		t.Fatalf("非法 JSON 应得到空参数，实际 %v", params)
	}
}

func TestWrapKeepsCause(t *testing.T) {
	restoreLocale(t)
	sentinel := errors.New("底层失败")
	err := Wrap(sentinel, "err.empty_text")

	if !errors.Is(err, sentinel) {
		t.Fatal("Wrap 后应保留底层错误（errors.Is 应命中）")
	}
	if _, ok := errors.AsType[*Error](err); !ok {
		t.Fatal("errors.As 应能取到 *Error")
	}

	msg := TranslateError(err)
	if strings.Contains(msg, sep) {
		t.Fatalf("TranslateError 结果不应含信封: %q", msg)
	}
	if !strings.Contains(msg, "底层失败") {
		t.Fatalf("TranslateError 应包含底层错误信息，实际 %q", msg)
	}
}

func TestTranslateErrorCases(t *testing.T) {
	restoreLocale(t)

	if got := TranslateError(nil); got != "" {
		t.Fatalf("TranslateError(nil) = %q, want 空串", got)
	}

	plain := errors.New("普通错误")
	if got := TranslateError(plain); got != "普通错误" {
		t.Fatalf("TranslateError(普通 error) = %q", got)
	}

	// 包装成普通 fmt.Errorf 后仍应能透过 %w 链识别出 *Error。
	wrapped := errors.Join(NewError("err.empty_text"))
	if got := TranslateError(wrapped); strings.Contains(got, sep) {
		t.Fatalf("多错误 join 后不应漏出信封: %q", got)
	}
}

func TestToParamMapSkipsNonStringKey(t *testing.T) {
	m := toParamMap([]any{1, "值", "ok", "好"})
	if _, exists := m["1"]; exists {
		t.Fatalf("非字符串 key 不应入表: %v", m)
	}
	if m["ok"] != "好" {
		t.Fatalf("字符串 key 应入表: %v", m)
	}
}

func TestFlattenRoundTrip(t *testing.T) {
	m := map[string]any{"a": 1, "b": "2"}
	flat := flatten(m)
	if len(flat) != 4 {
		t.Fatalf("flatten 长度 = %d, want 4", len(flat))
	}
	back := toParamMap(flat)
	if back["a"] != 1 || back["b"] != "2" {
		t.Fatalf("flatten → toParamMap 往返失真: %v", back)
	}
}
