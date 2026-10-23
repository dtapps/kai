package analytics

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/settings"
)

func TestLenBucket(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{-100, "0"},
		{-1, "0"},
		{0, "0"},
		{1, "1-20"},
		{20, "1-20"},
		{21, "21-100"},
		{100, "21-100"},
		{101, "101-500"},
		{500, "101-500"},
		{501, "501-2000"},
		{2000, "501-2000"},
		{2001, "2000+"},
		{1 << 20, "2000+"},
	}
	for _, c := range cases {
		if got := LenBucket(c.n); got != c.want {
			t.Fatalf("LenBucket(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// 分桶必须覆盖全区间且互不重叠——漏一个区间会让属性基数暴涨。
func TestLenBucketIsExhaustive(t *testing.T) {
	seen := map[string]bool{}
	for n := -5; n <= 3000; n++ {
		seen[LenBucket(n)] = true
	}
	if len(seen) != 6 {
		t.Fatalf("分桶数 = %d, want 6，实际 %v", len(seen), seen)
	}
}

// stateFixture 搭建一个可完全控制门控变量的环境。
type stateFixture struct {
	settings *settings.Service
}

func newStateFixture(t *testing.T) *stateFixture {
	t.Helper()

	oldToken := buildinfo.PosthogToken
	oldDev := buildinfo.Dev
	oldDeviceOK := deviceOK
	t.Cleanup(func() {
		mu.Lock()
		ss = nil
		mu.Unlock()
		buildinfo.PosthogToken = oldToken
		buildinfo.Dev = oldDev
		deviceOK = oldDeviceOK
		devUpload.Store(false)
	})

	svc, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("settings.NewService 失败: %v", err)
	}

	mu.Lock()
	ss = svc
	mu.Unlock()

	return &stateFixture{settings: svc}
}

func (f *stateFixture) set(enabled bool) {
	f.settings.Get().AnalyticsEnabled = enabled
}

func TestAnalyticsStateReasons(t *testing.T) {
	f := newStateFixture(t)

	t.Run("未配置 token", func(t *testing.T) {
		buildinfo.PosthogToken = ""
		buildinfo.Dev = "false"
		deviceOK = true
		f.set(true)

		on, reason := analyticsState()
		if on || reason != "log.analytics_reason_no_token" {
			t.Fatalf("state = (%v, %q), want (false, no_token)", on, reason)
		}
	})

	t.Run("设备 ID 不可用", func(t *testing.T) {
		buildinfo.PosthogToken = "phc_test"
		buildinfo.Dev = "false"
		deviceOK = false

		on, reason := analyticsState()
		if on || reason != "log.analytics_reason_no_device" {
			t.Fatalf("state = (%v, %q), want (false, no_device)", on, reason)
		}
	})

	t.Run("dev 构建拦截", func(t *testing.T) {
		buildinfo.PosthogToken = "phc_test"
		buildinfo.Dev = "true"
		deviceOK = true
		f.set(true)

		on, reason := analyticsState()
		if on || reason != "log.analytics_reason_dev" {
			t.Fatalf("state = (%v, %q), want (false, dev)", on, reason)
		}
	})

	t.Run("dev 构建 + 显式开启 dev 上报", func(t *testing.T) {
		buildinfo.PosthogToken = "phc_test"
		buildinfo.Dev = "true"
		deviceOK = true
		EnableDevUpload()

		on, reason := analyticsState()
		if !on || reason != "log.analytics_reason_on" {
			t.Fatalf("state = (%v, %q), want (true, on)", on, reason)
		}
	})

	t.Run("用户关闭", func(t *testing.T) {
		buildinfo.PosthogToken = "phc_test"
		buildinfo.Dev = "false"
		deviceOK = true
		f.set(false)

		on, reason := analyticsState()
		if on || reason != "log.analytics_reason_user_off" {
			t.Fatalf("state = (%v, %q), want (false, user_off)", on, reason)
		}
	})

	t.Run("全部条件满足", func(t *testing.T) {
		buildinfo.PosthogToken = "phc_test"
		buildinfo.Dev = "false"
		deviceOK = true
		f.set(true)

		on, reason := analyticsState()
		if !on || reason != "log.analytics_reason_on" {
			t.Fatalf("state = (%v, %q), want (true, on)", on, reason)
		}
	})
}

// 四个原因 key 必须各不相同，否则重启日志无法区分关闭原因。
func TestAnalyticsReasonKeysAreDistinct(t *testing.T) {
	f := newStateFixture(t)
	keys := map[string]bool{}

	buildinfo.PosthogToken = ""
	_, k := analyticsState()
	keys[k] = true

	buildinfo.PosthogToken = "phc_test"
	deviceOK = false
	_, k = analyticsState()
	keys[k] = true

	deviceOK = true
	buildinfo.Dev = "true"
	_, k = analyticsState()
	keys[k] = true

	buildinfo.Dev = "false"
	f.set(false)
	_, k = analyticsState()
	keys[k] = true

	if len(keys) != 4 {
		t.Fatalf("原因 key 有重复，实际 %v", keys)
	}
}

func TestEnabledMirrorsState(t *testing.T) {
	f := newStateFixture(t)

	buildinfo.PosthogToken = "phc_test"
	buildinfo.Dev = "false"
	deviceOK = true
	f.set(true)
	if !Enabled() {
		t.Fatal("条件齐全时 Enabled() 应为 true")
	}

	f.set(false)
	if Enabled() {
		t.Fatal("用户关闭后 Enabled() 应为 false")
	}

	f.set(true)
	buildinfo.Dev = "true"
	if Enabled() {
		t.Fatal("dev 构建未开启 dev 上报时 Enabled() 应为 false")
	}
	EnableDevUpload()
	if !Enabled() {
		t.Fatal("开启 dev 上报后 Enabled() 应为 true")
	}
}

func TestEnabledWithoutService(t *testing.T) {
	f := newStateFixture(t)
	_ = f

	mu.Lock()
	ss = nil
	mu.Unlock()

	buildinfo.PosthogToken = "phc_test"
	buildinfo.Dev = "false"
	deviceOK = true

	if Enabled() {
		t.Fatal("未 Init 时 Enabled() 必须为 false（不能默认上报）")
	}
}

func TestIsFirstLaunch(t *testing.T) {
	f := newStateFixture(t)

	if !IsFirstLaunch() {
		t.Fatal("AnalyticsInstalled=false 时应为首装")
	}

	f.settings.Get().AnalyticsInstalled = true
	if IsFirstLaunch() {
		t.Fatal("AnalyticsInstalled=true 时不应是首装")
	}

	mu.Lock()
	ss = nil
	mu.Unlock()
	if IsFirstLaunch() {
		t.Fatal("未 Init 时 IsFirstLaunch 应为 false")
	}
}

// 门控关闭时，所有上报入口都必须是空操作（不建连、不 panic）。
func TestTrackAndErrorAreNoopWhenDisabled(t *testing.T) {
	f := newStateFixture(t)
	f.set(false)
	buildinfo.PosthogToken = ""
	buildinfo.Dev = "false"
	deviceOK = true

	Track(EventAppStarted, map[string]any{"a": 1})
	Identify(map[string]any{"b": 2})
	Error("test_error", map[string]any{"c": 3})
	AppStarted("1.0.0", "zh-CN", "stable", true)

	mu.Lock()
	has := hasClient
	mu.Unlock()
	if has {
		t.Fatal("上报关闭时不应创建 client")
	}
}

func TestSetEnabledWithoutService(t *testing.T) {
	f := newStateFixture(t)
	_ = f
	mu.Lock()
	ss = nil
	mu.Unlock()

	if err := SetEnabled(true); err != nil {
		t.Fatalf("未 Init 时 SetEnabled 应静默返回 nil，实际 %v", err)
	}
}

func TestSetEnabledPersists(t *testing.T) {
	f := newStateFixture(t)
	buildinfo.PosthogToken = ""
	buildinfo.Dev = "false"
	deviceOK = true

	if err := SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true) 失败: %v", err)
	}
	if !f.settings.Get().AnalyticsEnabled {
		t.Fatal("开关应写回 settings")
	}

	if err := SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false) 失败: %v", err)
	}
	if f.settings.Get().AnalyticsEnabled {
		t.Fatal("关闭后 settings 应同步为 false")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	newStateFixture(t)
	Close()
	Close()
}

// 事件名是对外契约（PostHog 面板靠它出图），改名等于丢历史数据。
func TestEventNamesAreStable(t *testing.T) {
	want := map[string]string{
		"app_started":          EventAppStarted,
		"app_installed":        EventAppInstalled,
		"translate_input":      EventTranslateInput,
		"translate_screenshot": EventTranslateScreenshot,
		"settings_opened":      EventSettingsOpened,
		"feature_toggled":      EventFeatureToggled,
		"engine_configured":    EventEngineConfigured,
		"update_installed":     EventUpdateInstalled,
		"error_occurred":       EventError,
	}
	for name, got := range want {
		if got != name {
			t.Fatalf("事件名被改动: %q（应保持 %q）", got, name)
		}
	}
}
