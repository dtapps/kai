//go:build windows && !server

// Package webview 提供 Windows 平台 WebView2 运行时的启动前自检与浏览器参数兜底。
// 若目标机器未安装/损坏 WebView2 Runtime，在 app.Run() 前提示用户安装并退出，
// 避免崩溃栈。非 Windows 平台由 webview_other.go 提供空实现。
package webview

import (
	"log/slog"
	"os"
	"os/exec"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// regPath 是 WebView2 Runtime（Evergreen 稳定版）的注册表标记键。
// 安装 Runtime 后 HKCU/HKLM 下会出现 Microsoft\EdgeWebView\WebView2 项。
const regPath = `SOFTWARE\Microsoft\EdgeWebView\WebView2`

// IsWebView2RuntimeAvailable 检测系统是否安装了可用的 WebView2 Runtime。
// 任一视图根（HKLM/HKCU，含 32/64 位视图）存在 WebView2 注册项即认为可用。
func IsWebView2RuntimeAvailable() bool {
	roots := []struct {
		root   registry.Key
		access uint32
	}{
		{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_64KEY},
		{registry.CURRENT_USER, registry.READ | registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_32KEY},
		{registry.CURRENT_USER, registry.READ | registry.WOW64_32KEY},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.root, regPath, r.access)
		if err == nil {
			k.Close()
			return true
		}
	}
	return false
}

// WarnAndOpenDownload 在 WebView2 Runtime 不可用时弹出原生 MessageBox 说明问题，
// 并尝试用默认浏览器打开下载页。文案走 i18n（项目红线，禁止硬编码中英文）。
func WarnAndOpenDownload() {
	caption := i18n.T("webview2.missing_title")
	text := i18n.T("webview2.missing_text")

	// 打开下载页（best-effort，失败不影响提示）。
	_ = exec.Command("cmd", "/c", "start", "", "https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/").Start()

	user32, err := windows.LoadDLL("user32.dll")
	if err != nil {
		return
	}
	proc, err := user32.FindProc("MessageBoxW")
	if err != nil {
		return
	}
	captionPtr, _ := windows.UTF16PtrFromString(caption)
	textPtr, _ := windows.UTF16PtrFromString(text)
	// MessageBoxW(hwnd, lpText, lpCaption, uType)
	// MB_OK | MB_ICONINFORMATION
	proc.Call(0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(captionPtr)),
		0x00000040)
}

// EnsureWebView2 在 Windows 启动前调用：检测 Runtime，缺失则提示并退出进程。
// 非 Windows 编译由 _other.go 提供空实现，调用方无需判断平台。
func EnsureWebView2() {
	if IsWebView2RuntimeAvailable() {
		return
	}
	slog.Error("WebView2 Runtime not detected; prompting user to install")
	WarnAndOpenDownload()
	os.Exit(1)
}

// BrowserArgs 返回 Windows 平台下注入 WebView2 的全局浏览器启动参数。
//
// 背景：80010108 (RPC_E_DISCONNECTED) 偶发崩溃的另一高频诱因是 GPU 进程异常
// （显卡驱动 / 硬件加速 GPU 调度 / 系统从睡眠恢复后的 DComp 状态异常），导致
// WebView2 渲染进程在 controller 创建回调前崩溃，回调拿到已断开对象。注入
// --disable-gpu 与 --disable-gpu-compositing 将渲染降级到软件路径，掐断这条
// 偶发崩溃路径。代价是失去硬件加速（对翻译类轻量 UI 影响可忽略）。
// WebView2 共享单一浏览器环境，该参数对所有窗口全局生效。
// 非 Windows 平台（_other.go）返回 nil。
func BrowserArgs() []string {
	return []string{
		"--disable-gpu",
		"--disable-gpu-compositing",
	}
}

// ApplyOptions 在 Windows 上把 WebView2 浏览器兜底参数写入 application.Options。
// 注意：AdditionalBrowserArgs 位于 Options.Windows（WindowsOptions）子结构内，
// 而非顶层 Options（macOS 结构体无 Windows 字段），故只能在 //go:build windows
// 文件中访问。非 Windows 由 _other.go 提供空实现，main.go 可无条件调用。
func ApplyOptions(opts *application.Options) {
	opts.Windows.AdditionalBrowserArgs = BrowserArgs()
}
