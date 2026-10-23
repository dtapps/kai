package i18n

import (
	"strings"
	"testing"
)

func restoreLocale(t *testing.T) {
	t.Helper()
	old := GetLocale()
	t.Cleanup(func() { SetLocale(old) })
}

func TestSetGetLocale(t *testing.T) {
	restoreLocale(t)

	SetLocale("en-US")
	if got, want := GetLocale(), string(EN_US); got != want {
		t.Fatalf("GetLocale() = %q, want %q", got, want)
	}

	SetLocale("zh-CN")
	if got, want := GetLocale(), string(ZH_CN); got != want {
		t.Fatalf("GetLocale() = %q, want %q", got, want)
	}

	// 不支持的语言回退中文。
	SetLocale("fr-FR")
	if got, want := GetLocale(), string(ZH_CN); got != want {
		t.Fatalf("非法语言应回退 zh-CN，实际 %q", got)
	}
}

func TestTTranslatesKnownKey(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	got := T("app.description")
	if got == "" || got == "app.description" {
		t.Fatalf("T(app.description) 未命中翻译，返回 %q", got)
	}
}

func TestTUnknownKeyReturnsKey(t *testing.T) {
	restoreLocale(t)
	got := T("definitely.not.exist.key")
	if got != "definitely.not.exist.key" {
		t.Fatalf("未知 key 应原样返回，实际 %q", got)
	}
}

func TestTWithTemplateData(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	got := T("app.name_version", "Version", "9.9.9")
	if !strings.Contains(got, "9.9.9") {
		t.Fatalf("模板参数未渲染: %q", got)
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("模板占位符残留: %q", got)
	}
}

// 传 nil 参数曾导致 go-i18n 在 reflect 上 panic，这里守住回归。
func TestTNilTemplateDataDoesNotPanic(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	got := T("app.name_version", "Version", nil)
	if strings.Contains(got, "{{") {
		t.Fatalf("nil 参数应渲染为空占位，实际 %q", got)
	}
}

// 参数个数为奇数时最后一个 key 没有值，应被忽略而不是 panic。
func TestTOddParamCount(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	if got := T("app.description", "lonelyKey"); got == "" {
		t.Fatal("奇数个参数不应导致空结果")
	}
}

// 非 string 的 key 位置应被跳过。
func TestTNonStringKeySkipped(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")

	got := T("app.name_version", 42, "值")
	if strings.Contains(got, "{{") {
		t.Fatalf("非字符串 key 应被跳过，实际 %q", got)
	}
}

func TestTWithLocaleRestoresPrevious(t *testing.T) {
	restoreLocale(t)
	SetLocale("zh-CN")
	zh := T("app.description")

	en := TWithLocale("en-US", "app.description")
	if en == zh {
		t.Fatalf("en-US 与 zh-CN 文案相同（%q），无法验证语言切换", en)
	}
	if got := GetLocale(); got != string(ZH_CN) {
		t.Fatalf("TWithLocale 后语言未还原，当前 %q", got)
	}

	// 非法语言回退中文。
	if got := TWithLocale("ja-JP", "app.description"); got != zh {
		t.Fatalf("不支持的语言应回退中文，实际 %q (want %q)", got, zh)
	}
}

func TestResolveLocale(t *testing.T) {
	cases := map[string]string{
		"en-US": "en-US",
		"en":    "zh-CN",
		"zh-CN": "zh-CN",
		"":      "zh-CN",
	}
	for in, want := range cases {
		if got := ResolveLocale(in); got != want {
			t.Fatalf("ResolveLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSupportedLocales(t *testing.T) {
	got := SupportedLocales()
	if len(got) != 2 {
		t.Fatalf("SupportedLocales() 长度 = %d, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, l := range got {
		seen[l] = true
	}
	for _, want := range []string{string(ZH_CN), string(EN_US)} {
		if !seen[want] {
			t.Fatalf("SupportedLocales() 缺少 %q，实际 %v", want, got)
		}
	}
}

func TestGetCurrentLocaleMatchesGetLocale(t *testing.T) {
	restoreLocale(t)
	SetLocale("en-US")
	if GetCurrentLocale() != GetLocale() {
		t.Fatalf("GetCurrentLocale() = %q 与 GetLocale() = %q 不一致", GetCurrentLocale(), GetLocale())
	}
}
