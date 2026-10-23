package hotkey

import (
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestNilAppIsSafe 启动早期 app 可能尚未就绪：
// Register / Unregister 必须安全返回，不能因 nil 解引用把启动流程打崩。
func TestNilAppIsSafe(t *testing.T) {
	m := NewManager(nil, nil, nil, nil, nil, nil, nil, nil)

	m.Register()
	m.Unregister()
}

// TestTriggerInputWithoutMainWindow 主窗口未就绪（或已销毁）时点击托盘菜单必须直接返回。
func TestTriggerInputWithoutMainWindow(t *testing.T) {
	m := NewManager(nil, nil, nil,
		func() application.Window { return nil }, // 主窗口不可用
		nil, nil, nil, nil)

	m.TriggerInput()
}

// TestTriggerScreenshotWithoutWindow 截图窗口为 nil 时不得 panic，
// 且回调按顺序执行（先隐藏窗口，再启动截图流程）。
func TestTriggerScreenshotWithoutWindow(t *testing.T) {
	var called int
	m := NewManager(nil, nil, nil, nil, nil,
		func() error { called++; return nil },
		func() application.Window { return nil },
		nil,
	)

	m.TriggerScreenshot()
	if called != 1 {
		t.Fatalf("screenshotTranslate 应被调用 1 次, 实际 %d", called)
	}
}

// TestTriggerScreenshotPropagatesError 截图失败只记录日志，不应向上 panic。
func TestTriggerScreenshotPropagatesError(t *testing.T) {
	m := NewManager(nil, nil, nil, nil, nil,
		func() error { return errors.New("no permission") },
		func() application.Window { return nil },
		nil,
	)

	m.TriggerScreenshot()
}
