package network

import (
	"net/http"
	"net/url"
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/useragent"
)

func TestBuildProxyURL(t *testing.T) {
	t.Run("无认证", func(t *testing.T) {
		proxy := settings.ProxyConfig{Protocol: "http", Host: "127.0.0.1", Port: 7890}
		u := buildProxyURL(proxy)
		if u == nil {
			t.Fatal("buildProxyURL 返回 nil")
		}
		if got, want := u.String(), "http://127.0.0.1:7890"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if u.User != nil {
			t.Fatalf("无认证时不应带 User: %v", u.User)
		}
	})

	t.Run("带认证", func(t *testing.T) {
		proxy := settings.ProxyConfig{
			Protocol: "socks5", Host: "proxy.example.com", Port: 1080,
			Username: "user", Password: "p@ss:word",
		}
		u := buildProxyURL(proxy)
		if got, want := u.Scheme, "socks5"; got != want {
			t.Fatalf("Scheme = %q, want %q", got, want)
		}
		if got, want := u.Host, "proxy.example.com:1080"; got != want {
			t.Fatalf("Host = %q, want %q", got, want)
		}
		if u.User == nil {
			t.Fatal("带认证时应有 User")
		}
		pass, _ := u.User.Password()
		if u.User.Username() != "user" || pass != "p@ss:word" {
			t.Fatalf("认证信息不符: %v", u.User)
		}
	})
}

func TestUnwrapHTTPTransport(t *testing.T) {
	t.Run("裸 Transport", func(t *testing.T) {
		base := &http.Transport{}
		got, ok := unwrapHTTPTransport(base)
		if !ok || got != base {
			t.Fatalf("应直接返回原 Transport，ok=%v", ok)
		}
	})

	t.Run("被 useragent 包裹", func(t *testing.T) {
		base := &http.Transport{}
		got, ok := unwrapHTTPTransport(useragent.Wrap(base))
		if !ok || got != base {
			t.Fatalf("应解包出原 Transport，ok=%v got=%T", ok, got)
		}
	})

	t.Run("无法解包", func(t *testing.T) {
		if _, ok := unwrapHTTPTransport(nil); ok {
			t.Fatal("nil 不应解包成功")
		}
		// 未知实现（例如自定义 RoundTripper）应放弃而非 panic。
		if _, ok := unwrapHTTPTransport(&customRT{}); ok {
			t.Fatal("未知 RoundTripper 不应解包成功")
		}
	})
}

type customRT struct{}

func (c *customRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestBuildHTTPClientDefaults(t *testing.T) {
	c := BuildHTTPClient(*settings.DefaultSettings())
	if c == nil {
		t.Fatal("BuildHTTPClient 返回 nil")
	}
	if c.Timeout == 0 {
		t.Fatal("应设置整体超时")
	}
	if c.Transport == nil {
		t.Fatal("应包裹 Transport（含 UA 与 HTTP 日志）")
	}

	// 未启用代理时，底层 Transport 的 Proxy 应为 nil。
	base, ok := unwrapHTTPTransport(c.Transport)
	if !ok {
		t.Fatalf("无法解包底层 Transport: %T", c.Transport)
	}
	if base.Proxy != nil {
		t.Fatalf("未启用代理时 Proxy 应为 nil，实际非 nil")
	}
	if base.TLSHandshakeTimeout == 0 || base.ResponseHeaderTimeout == 0 {
		t.Fatal("应设置 TLS/响应头超时")
	}
}

// 代理配置必须真正落到可解包出的底层 *http.Transport 上（直接断言外层类型一定会失败）。
func TestBuildHTTPClientAppliesProxy(t *testing.T) {
	s := *settings.DefaultSettings()
	s.Proxy = settings.ProxyConfig{Enabled: true, Protocol: "http", Host: "127.0.0.1", Port: 7890}

	c := BuildHTTPClient(s)
	base, ok := unwrapHTTPTransport(c.Transport)
	if !ok {
		t.Fatalf("无法解包底层 Transport: %T", c.Transport)
	}
	if base.Proxy == nil {
		t.Fatal("启用代理后底层 Transport.Proxy 不应为 nil")
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := base.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy 回调报错: %v", err)
	}
	if u == nil {
		t.Fatal("Proxy 回调返回 nil")
	}
	if got, want := u.String(), "http://127.0.0.1:7890"; got != want {
		t.Fatalf("代理地址 = %q, want %q", got, want)
	}
}

// 开关打开但 Host 为空时不应设置代理（否则所有请求都会失败）。
func TestBuildHTTPClientProxyWithoutHost(t *testing.T) {
	s := *settings.DefaultSettings()
	s.Proxy = settings.ProxyConfig{Enabled: true, Port: 7890}

	base, ok := unwrapHTTPTransport(BuildHTTPClient(s).Transport)
	if !ok {
		t.Fatal("无法解包底层 Transport")
	}
	if base.Proxy != nil {
		t.Fatalf("Host 为空时不应设置代理，实际非 nil")
	}
}

// 未启用自定义 DNS 时走系统解析；localhost 不依赖外网。
func TestResolveHostFallsBackToSystem(t *testing.T) {
	s := *settings.DefaultSettings()
	ips, err := resolveHost(s, "localhost")
	if err != nil {
		t.Fatalf("resolveHost 失败: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("localhost 应解析出至少一个 IP")
	}
}

// 已禁用（Enabled=false）的 DNS 配置必须被忽略，不参与解析。
func TestResolveHostIgnoresDisabledDNS(t *testing.T) {
	s := *settings.DefaultSettings()
	s.DNSConfigs = []settings.DNSConfig{
		{Enabled: false, Servers: []string{"127.0.0.1"}},
		{Enabled: true, Servers: nil}, // 没有服务器地址，也应跳过
	}

	ips, err := resolveHost(s, "localhost")
	if err != nil {
		t.Fatalf("resolveHost 失败: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("应回退系统解析并拿到 IP")
	}
}

// 自定义 DNS 全部不可用时，要能回退系统解析而不是直接失败。
func TestResolveHostUnreachableDNSServerFallsBack(t *testing.T) {
	s := *settings.DefaultSettings()
	// 127.0.0.1:1 上没有 DNS 服务，查询会立即被拒。
	s.DNSConfigs = []settings.DNSConfig{{Enabled: true, Servers: []string{"127.0.0.1:1"}}}

	ips, err := resolveHost(s, "localhost")
	if err != nil {
		t.Fatalf("自定义 DNS 失败后应回退系统解析，实际报错: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("应拿到系统解析结果")
	}
}

func TestQueryDNSServerInvalidHost(t *testing.T) {
	// 非法主机名必然解析失败，用来覆盖错误分支（不依赖外网）。
	if _, err := queryDNSServer("127.0.0.1:1", "invalid..host..name"); err == nil {
		t.Fatal("非法域名应返回错误")
	}
}

func TestBuildProxyURLWithDefaultSettings(t *testing.T) {
	// 默认配置（未启用）也应构造出合法 URL，供上层判断用。
	u := buildProxyURL(settings.DefaultSettings().Proxy)
	if _, err := url.Parse(u.String()); err != nil {
		t.Fatalf("构造出的代理 URL 非法: %v", err)
	}
}
