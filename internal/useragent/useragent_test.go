package useragent

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetUA 清空全局 UA，避免用例间互相污染。
func resetUA(t *testing.T, v string) {
	t.Helper()
	Set(v)
	t.Cleanup(func() { Set("") })
}

func TestSetGet(t *testing.T) {
	resetUA(t, "")

	if got := Get(); got != "" {
		t.Fatalf("初始 Get() = %q, want 空串", got)
	}

	Set("Mozilla/5.0 (Macintosh) Kai/1.0")
	if got, want := Get(), "Mozilla/5.0 (Macintosh) Kai/1.0"; got != want {
		t.Fatalf("Get() = %q, want %q", got, want)
	}

	Set("")
	if got := Get(); got != "" {
		t.Fatalf("清空后 Get() = %q, want 空串", got)
	}
}

// recordingTransport 记录最终发出的请求，用于断言 UA 是否被注入。
type recordingTransport struct {
	mu  sync.Mutex
	got []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.got = append(r.got, req.Header.Get("User-Agent"))
	r.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func (r *recordingTransport) first() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.got) == 0 {
		return ""
	}
	return r.got[0]
}

func TestTransportInjectsGlobalUA(t *testing.T) {
	resetUA(t, "Kai-Test-UA")
	rec := &recordingTransport{}
	tr := &Transport{Base: rec}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Del("User-Agent") // httptest 默认不带，显式清一遍防止环境差异

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip 失败: %v", err)
	}
	if got, want := rec.first(), "Kai-Test-UA"; got != want {
		t.Fatalf("注入后 UA = %q, want %q", got, want)
	}
}

func TestTransportKeepsExplicitUA(t *testing.T) {
	resetUA(t, "Kai-Test-UA")
	rec := &recordingTransport{}
	tr := &Transport{Base: rec}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("User-Agent", "CertFlow/1.0")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip 失败: %v", err)
	}
	if got, want := rec.first(), "CertFlow/1.0"; got != want {
		t.Fatalf("显式 UA 被覆盖: got %q, want %q", got, want)
	}
}

func TestTransportNoopWhenGlobalUAEmpty(t *testing.T) {
	resetUA(t, "")
	rec := &recordingTransport{}
	tr := &Transport{Base: rec}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Del("User-Agent")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip 失败: %v", err)
	}
	if got := rec.first(); got != "" {
		t.Fatalf("全局 UA 为空时不应注入，实际 UA = %q", got)
	}
}

// 注入时必须 Clone 请求，不能就地修改调用方的 Header。
func TestTransportDoesNotMutateOriginalRequest(t *testing.T) {
	resetUA(t, "Kai-Test-UA")
	tr := &Transport{Base: &recordingTransport{}}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Del("User-Agent")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip 失败: %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("原请求被就地修改，UA = %q", got)
	}
}

func TestTransportNilBaseFallsBackToDefault(t *testing.T) {
	resetUA(t, "")
	tr := &Transport{}

	// 指向必然连不上的地址：只要不是 panic（nil RoundTripper）就算通过。
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("预期连接失败，却拿到 nil error")
	}
}

func TestWrapIdempotent(t *testing.T) {
	inner := Wrap(nil)
	if _, ok := inner.(*Transport); !ok {
		t.Fatalf("Wrap(nil) 应返回 *Transport，实际 %T", inner)
	}

	again := Wrap(inner)
	first, ok1 := inner.(*Transport)
	second, ok2 := again.(*Transport)
	if !ok1 || !ok2 {
		t.Fatal("类型断言失败")
	}
	if first != second {
		t.Fatal("重复 Wrap 应原样返回同一个 *Transport")
	}
}
