// Package useragent 维护全局 User-Agent（由前端启动时传入 WebView 的 navigator.userAgent），
// 并提供注入 UA 的 RoundTripper：请求未显式设置 User-Agent 时自动补上全局 UA。
// 已显式设置 UA 的请求（如各云 SDK 自带 UA、monitor/scanner 的 CertFlow/1.0）不受影响。
package useragent

import (
	"net/http"
	"sync"
)

var (
	mu sync.RWMutex
	ua string
)

// Set 设置全局 User-Agent（应用启动时由前端经 MonitorService.SetUserAgent 传入）。
func Set(v string) {
	mu.Lock()
	ua = v
	mu.Unlock()
}

// Get 返回当前全局 User-Agent；未设置时返回空串（此时不注入，走 Go 默认 UA）。
func Get() string {
	mu.RLock()
	defer mu.RUnlock()
	return ua
}

// Transport 在请求未显式设置 User-Agent 时注入全局 UA 的 RoundTripper。
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper。按约定不修改原请求，注入时 Clone 一份。
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if req.Header.Get("User-Agent") == "" {
		if v := Get(); v != "" {
			req = req.Clone(req.Context())
			req.Header.Set("User-Agent", v)
		}
	}
	return base.RoundTrip(req)
}

// Wrap 把 base 包裹为注入全局 UA 的 RoundTripper；base 已是 *Transport 时原样返回避免重复包裹
// （即使重复包裹也无副作用：内层看到 UA 已设置不会覆盖）。
func Wrap(base http.RoundTripper) http.RoundTripper {
	if t, ok := base.(*Transport); ok {
		return t
	}
	return &Transport{Base: base}
}
