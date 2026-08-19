package wails_updater_providers

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// 更新窗口的窗口名/尺寸/resize 事件名统一走包全局变量（在 mirror_provider.go 定义）：
// globalWindowName / globalWindowW / globalWindowH / globalResizeEvent，
// 对外通过 SetWindowName/GetWindowName 等包函数读写，便于运行时覆盖。

// handleMu 保护 updaterHandles，语言切换刷新 HTML 时需串行更新。
// 框架内部 user 动作事件名（用户点窗口按钮触发）。BYO 模式下框架不在这些
// 事件里 show/hide 我们的窗口，所以用户主动关闭窗口由本包监听这些事件自行
// Hide 完成（见 registerCloseHandler）。
const (
	evtUserCancel = "wails:updater:user:cancel"
	evtUserSkip   = "wails:updater:user:skip"
	evtUserRemind = "wails:updater:user:remind"
)

var (
	handleMu                sync.Mutex
	updaterHandles          = map[string]*updaterWindow{}
	resizeHandlerRegistered bool // resize 全局监听只注册一次，避免重复重建窗口时泄漏
	closeHandlerRegistered  bool // user 动作关闭监听只注册一次
)

// updaterWindow 把库自建的 *application.WebviewWindow 适配成
// updater.WindowHandle。注意：这里 *故意不实现* updater.WindowSizer
// （即不暴露 SetSize 方法），否则框架 transition() 会用内置尺寸覆盖我们
// 由 JS 驱动的自适应高度。高度自适应改由本包内部监听 eventResize 后调用
// win.SetSize 完成。
//
// 框架在 Init 时注册此 handle 指针，此后无需变更；语言/主题切换通过 SetHTML
// 刷新内容，窗口实例保持不变。
type updaterWindow struct {
	win *application.WebviewWindow
	app *application.App
}

func (h *updaterWindow) EmitEvent(name string, data ...any) bool {
	return h.win.EmitEvent(name, data...)
}

// Show 由框架在 transition 到展示状态时调用。幂等。
func (h *updaterWindow) Show() {
	h.win.Show()
}

// Close 由框架在两种场景调用：
//  1. 用户点「取消/跳过/稍后提醒」按钮 → 框架 u.closeWindow → handle.Close
//  2. CheckAndInstall 重开 session 时清理旧 session → u.session.close → handle.Close
//
// 注意第 2 种场景：CheckAndInstall 每次执行都会 close 上一次残留的 session，
// 而本 BYO 窗口的显示完全由菜单 ShowUpdaterWindow 掌控、与框架 session 生命周期
// 无关。若此处 Hide，第二次「检查更新」时刚 Show 的窗口会被立刻藏掉（一闪而过）。
// 因此这里做成 no-op——窗口显隐完全由本包控制：
//   - 显示：ShowUpdaterWindow 强制重建可见窗口
//   - 隐藏：用户点 X（WindowClosing → Hide）或点按钮（registerCloseHandler 监听
//     user:cancel/skip/remind 后 Hide）。第 2 种场景的旧 session close 不会再误藏。
func (h *updaterWindow) Close() {}

// createUpdaterWindow 创建一个 *updaterWindow 对象（稳定句柄）并为其创建底层
// WebviewWindow。注意：句柄对象一经创建便保持稳定，后续只重建其内部的 win
// （见 recreateNativeWindow），这样框架 Init 时持有的 WindowHandle 指针在窗口
// 销毁重建后依然有效——否则「关闭后再检查更新」弹出的新窗口与框架持有的旧
// 句柄不是同一个对象，user:cancel 会打到已销毁的旧窗口，表现为「关闭按钮没反应」。
func createUpdaterWindow(app *application.App) *updaterWindow {
	h := &updaterWindow{app: app}
	// Init 阶段创建为隐藏窗口，启动时不弹窗；由 ShowUpdaterWindow 负责显示。
	recreateNativeWindow(app, h, false)
	return h
}

// recreateNativeWindow 仅重建 h 底层的 WebviewWindow（h 对象本身保持稳定）。
// 创建时 Wails 会正确注入原生桥接 window._wails.invoke 与 inline event shim，
// 因此 JS 的 Events.Emit（关闭/安装/跳过按钮、resize 自适应）均可用。
// show=false 时创建为隐藏窗口（Init 阶段不弹窗）；show=true 时创建为可见窗口
// （ShowUpdaterWindow 显示路径使用，配合双 Show + Focus 确保真正显示）。
//
// 注意：Wails 在 WindowClosing 时有一个内部监听器会无条件销毁窗口并从注册表
// 移除（不读 event.Cancelled），因此 e.Cancel() 无法阻止销毁；这里 Hide 仅为
// 「未真正销毁」场景兜底，真正重建由 ensureUpdaterWindow / ShowUpdaterWindow 完成。
func recreateNativeWindow(app *application.App, h *updaterWindow, show bool) {
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                 globalWindowName,
		Title:                T("window_title_check"),
		Width:                globalWindowW,
		Height:               globalWindowH,
		HTML:                 renderWindowHTML(app),
		DisableResize:        false,
		Hidden:               !show, // show=false 时隐藏（启动不弹窗），show=true 时可见
		AllowSimpleEventEmit: true,  // 关键：允许 JS Events.Emit 直接驱动 Go 监听
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
		// 隐藏原生标题栏红绿灯按钮（关闭/最小化/全屏），但保留原生标题栏与标题文字。
		// 用框架原生的 ButtonState=ButtonHidden（applyWindowButtonStates 在 OnCreate 对所有
		// 窗口生效），比 MacTitleBar.Hide（会去掉 Titled 样式、连标题一起消失且红绿灯未必隐藏）更可靠。
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonHidden,
		Mac: application.MacWindow{
			// modalPanel 比普通 AlwaysOnTop(floating) 更高，确保更新窗口始终压在
			// 应用其他窗口（及多数前台窗口）之上，不会被遮挡。
			WindowLevel: application.MacWindowLevelModalPanel,
			// 关键：让原生标题栏透明且内容全尺寸延伸（AppearsTransparent +
			// HideTitle + FullSizeContent），否则 macOS 的 NSVisualEffectView
			// 会绘制系统色背景，在深色主题下顶部露出一条浅色系统色带，与下方
			// body 的 var(--bg) 深色内容不融合（标题栏「没适配主题」）。
			// 透明后内容 var(--bg) 延伸到顶部，自定义 .titlebar 与之同色无缝衔接。
			// 红绿灯仍由上方 MinimiseButtonState/MaximiseButtonState/CloseButtonState=ButtonHidden 隐藏。
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
				HideTitle:          true,
				FullSizeContent:    true,
			},
		},
	})

	win.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		win.Hide()
	})

	h.win = win
}

// getLiveUpdaterWindow 返回存活的更新窗口句柄：若窗口尚未创建、或已被用户
// 关闭销毁（Wails 内部 WindowClosing 监听会销毁并移出注册表），返回 nil。
// 与 ensureUpdaterWindow 区别：本函数不重建窗口，仅做存活探测（语言/主题刷新
// 时若窗口已销毁则无需刷新，留待 ShowUpdaterWindow 重建即可，避免对销毁窗口
// 调 SetHTML/SetTitle/SetSize 导致原生崩溃）。
//
// 关键：仅靠 app.Window.Get(name) 不足够——窗口被销毁时 Go 对象仍会滞留注册表
// 一段时间，且 wails 的 SetTitle/SetSize 仅判 w.impl != nil（销毁后 impl 不会被
// 置空），对已在 teardown 的原生视图发 setTitle: 会抛 NSInvalidArgumentException
// 直接 SIGABRT。因此必须再用窗口自身的 IsVisible() 兜底（其内部对 isDestroyed()
// 返回 false），仅当窗口当前真正可见时才视为可用。
//
// 必须在 handleMu 保护下调用。
func getLiveUpdaterWindow(app *application.App) *updaterWindow {
	h, ok := updaterHandles[globalWindowName]
	if !ok || h == nil || h.win == nil {
		return nil
	}
	// 注册表按 id 管理；窗口被销毁后 app.Window.Get(name) 返回 false。
	if _, live := app.Window.Get(globalWindowName); !live {
		return nil
	}
	// 仅当窗口当前可见才算「可安全刷新」；隐藏（含从未展示的 Hidden 窗口）
	// 或已销毁的窗口跳过——下次 ShowUpdaterWindow 会重建出含最新文案的窗口。
	if !h.win.IsVisible() {
		return nil
	}
	return h
}

// ensureUpdaterWindow 返回可用的更新窗口句柄。句柄对象（*updaterWindow）一旦
// 创建便保持稳定并存入 updaterHandles；仅当底层 WebviewWindow 缺失或已被销毁
// 时重建 h.win。这样框架 Init 持有的 WindowHandle（指向同一个 h 对象）在窗口
// 销毁重建后依然有效——这是「关闭按钮在重开后没反应」的根因修复点。
// resize 全局监听仅注册一次（resizeHandlerRegistered 守卫），避免重复重建泄漏。
//
// 必须在 handleMu 保护下调用。
func ensureUpdaterWindow(app *application.App) *updaterWindow {
	h := updaterHandles[globalWindowName]
	if h == nil {
		h = createUpdaterWindow(app)
		updaterHandles[globalWindowName] = h
		if !resizeHandlerRegistered {
			registerResizeHandler(app)
			resizeHandlerRegistered = true
		}
		if !closeHandlerRegistered {
			registerCloseHandler(app)
			closeHandlerRegistered = true
		}
		return h
	}

	// 句柄对象稳定；仅当底层原生窗口缺失或已销毁时重建 h.win（隐藏态）。
	alive := h.win != nil
	if alive {
		if _, live := app.Window.Get(globalWindowName); !live {
			alive = false
		}
	}
	if !alive {
		recreateNativeWindow(app, h, false)
	}
	return h
}

// OpenUpdaterWindow 在给定 app 上创建（或复用）更新窗口，并把它包装成
// updater.Window 选项返回，供 app.Updater.Init 使用。窗口创建后默认隐藏，
// 不会在启动时弹出；由框架的 Show() 在检查更新时才显示。
//
// 初始语言/配色直接读包全局 GetLocale/GetTheme（注入到 HTML 模板），
// 调用方在 Open 前通过 SetLocale/SetTheme 设定即可。
func OpenUpdaterWindow(app *application.App) updater.WindowOption {
	handleMu.Lock()
	defer handleMu.Unlock()
	return updater.BYOWindow(ensureUpdaterWindow(app))
}

// ShowUpdaterWindow 显示更新窗口（检查更新菜单点击时调用）。
//
// 关键修复（一闪而过根因）：
//  1. 每次显示都强制销毁旧 WebviewWindow 并重建为可见窗口，使新窗口**不隶属于
//     任何旧 session**；随后 CheckAndInstall 清理旧 session 时调 handle.Close
//     （本包已实现为 no-op），不会再误藏当前显示的窗口。
//  2. Wails v3 对 Hidden 窗口的首次 Show() 只触发 webview 创建、不真正显示，
//     需连续两次 Show()（与项目内 showScreenshotWindow 的已知 workaround 一致），
//     并补 Focus() 确保窗口真正置前显示。
func ShowUpdaterWindow(app *application.App) {
	handleMu.Lock()
	defer handleMu.Unlock()
	h := ensureUpdaterWindow(app)
	// 销毁旧窗口（含 Init 阶段创建的隐藏窗口或上一次显示的旧窗口），重建为可见窗口。
	if h.win != nil {
		h.win.Close() // WebviewWindow.Close：框架无条件销毁并移出注册表
	}
	recreateNativeWindow(app, h, true)
	h.win.Show()
	h.win.Show() // 双 Show：规避 Wails Hidden 首次 Show 不显示的 bug
	h.win.Focus()
}

// SetUpdaterLocaleTheme 应用新语言与配色到更新窗口。
// 必须在 OpenUpdaterWindow 之后调用（语言/主题切换场景）。
//
// 关键：可见窗口不能用 SetHTML 刷新——SetHTML 重载后 Wails 不会重新注入原生
// 桥接 window._wails.invoke，导致 JS 的 Events.Emit（关闭/安装/跳过按钮、resize
// 自适应）全部失效，表现为「关闭按钮没反应、窗口不自适应」。因此可见时改为
// 销毁并重建底层 WebviewWindow（recreateNativeWindow，创建路径由 Wails 正确注入
// 桥接与 inline event shim）；h 对象保持稳定，框架持有的 WindowHandle 仍有效。
// 不可见/已销毁的窗口跳过刷新——下次 ShowUpdaterWindow 会重建出含最新文案的窗口。
func SetUpdaterLocaleTheme(app *application.App) {
	handleMu.Lock()
	defer handleMu.Unlock()

	h := getLiveUpdaterWindow(app)
	if h == nil {
		return
	}
	// 重建底层窗口以最新文案重新渲染；重建后重新显示（保持用户当前可见状态）。
	recreateNativeWindow(app, h, true)
	h.win.Show()
	h.win.Show() // 双 Show：与 ShowUpdaterWindow 保持一致
	h.win.Focus()
}

// registerResizeHandler 监听前端 JS 通过 Events.Emit(globalResizeEvent, [w, h])
// 发来的自适应高度请求，转成对窗口的 SetSize 调用。
//
// 注意：必须每次动态解析存活窗口，不能捕获初始 win——resize 监听只注册一次
// （resizeHandlerRegistered 守卫），而窗口可能被关闭销毁并重建；若捕获旧 win，
// 重建后 resize 事件会打到已销毁窗口，导致尺寸不更新甚至崩溃。
func registerResizeHandler(app *application.App) {
	// 前端经内联事件 shim 发起的 Events.Emit 会丢弃 payload（仅发事件名），
	// 因此无法把高度数据回传；窗口高度由 globalWindowW/globalWindowH 固定控制。
	// 这里仅在该事件到达时对存活窗口做一次保底尺寸设定，确保初始/重建后尺寸正确。
	app.Event.On(globalResizeEvent, func(e *application.CustomEvent) {
		h := getLiveUpdaterWindow(app)
		if h == nil {
			return
		}
		h.win.SetSize(globalWindowW, globalWindowH)
	})
}

// registerCloseHandler 监听用户主动关闭窗口的框架事件（取消/跳过/稍后提醒），
// 在回调里真正 Hide 窗口。
//
// 为什么不能依赖框架的 handle.Close() 来隐藏：
//   - BYO 模式下 handle.Close() 已被本包实现为 no-op（见 updaterWindow.Close），
//     目的是避免 CheckAndInstall 重开 session 时清理旧 session 把当前显示的窗口误藏。
//   - 因此用户主动关闭窗口必须由本包显式 Hide。点 X 由 WindowClosing → Hide 兜底；
//     点按钮（user:cancel/skip/remind）经框架 u.closeWindow → handle.Close(no-op)，
//     这里补 Hide 完成关闭。
func registerCloseHandler(app *application.App) {
	hide := func(*application.CustomEvent) {
		h := getLiveUpdaterWindow(app)
		if h == nil {
			return
		}
		h.win.Hide()
	}
	app.Event.On(evtUserCancel, hide)
	app.Event.On(evtUserSkip, hide)
	app.Event.On(evtUserRemind, hide)
}
