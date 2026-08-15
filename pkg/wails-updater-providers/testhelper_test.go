package wails_updater_providers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// 通用占位仓库，不写死任何真实业务信息。
const testRepo = "example-org/example-repo"

// redirectClient 把对 api.cnb.cool / github.com 的请求重定向到 mock server。
func redirectClient(srv *httptest.Server) *http.Client {
	host := strings.TrimPrefix(srv.URL, "http://")
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		u := *req.URL
		u.Scheme = "http"
		u.Host = host
		r := req.Clone(req.Context())
		r.URL = &u
		return http.DefaultClient.Do(r)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// discardLogger 返回一个静默 logger，避免测试刷屏。
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mustServe 启动 mock server 并在测试结束时关闭。
func mustServe(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// safeVersion 安全读取候选版本的版本号，rel 为 nil 时返回 "<无候选>"。
func safeVersion(rel *updater.Release) string {
	if rel == nil {
		return "<无候选>"
	}
	return rel.Version
}
