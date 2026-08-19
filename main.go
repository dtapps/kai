package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	kevents "cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/httplogstore"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/logutil"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/network"
	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/service"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/translate"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
	kupdater "cnb.cool/dtapp/kai/pkg/wails-updater-providers"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon.png
var trayIcon []byte

// parseBuildTime 把构建时注入的 RFC3339 字符串解析为 time.Time。
// 本地 dev 构建注入的是 "unknown" 等占位串，解析失败时返回零值，
// 更新器据此（buildTime.IsZero()）跳过 nightly 比较。
func parseBuildTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// resolveUpdaterLocale 把 settings 里的语言（可能含 auto）解析为第三方库接受的真实取值。
// auto 由 i18n 当前生效语言决定（项目内已按系统语言解析），非 auto 直接透传。
func resolveUpdaterLocale(lang string) string {
	if lang == string(model.LocaleAuto) || lang == "" {
		return i18n.GetLocale()
	}
	return lang
}

// resolveUpdaterTheme 把 settings 里的主题（可能含 auto）解析为第三方库接受的真实取值。
// auto 用系统真实外观（IsDarkMode）解析为 light/dark；非 auto 直接透传。
func resolveUpdaterTheme(theme string, app *application.App) string {
	if theme == string(model.ThemeAuto) || theme == "" {
		if app != nil && app.Env.IsDarkMode() {
			return string(model.ThemeDark)
		}
		return string(model.ThemeLight)
	}
	return theme
}

func init() {
	// 注册自定义事件类型，供后端 emit / 前端监听（对齐 certflow 的 RegisterEvent 模式）。
	// 必须 emit 的数据类型与注册类型严格一致，否则 Wails3 validateCustomEvent 会 panic。
	application.RegisterEvent[kevents.LocaleChangedPayload](kevents.EventLocaleChanged)
	application.RegisterEvent[kevents.ThemeChangedPayload](kevents.EventThemeChanged)
	application.RegisterEvent[string](kevents.EventWindowShow)
	application.RegisterEvent[[]string](kevents.EventHotkeysChanged)
	application.RegisterEvent[string](kevents.EventInputFill)
	application.RegisterEvent[model.TranslateResult](kevents.EventTranslateResult)
	application.RegisterEvent[model.ScreenshotResult](kevents.EventScreenshotOCR)
	application.RegisterEvent[struct{}](kevents.EventWindowScreenshot)
	application.RegisterEvent[struct{}](kevents.EventScreenshotRecapture)
	application.RegisterEvent[kevents.ScreenshotRetranslatePayload](kevents.EventScreenshotRetranslate)
}

// formatBuildTime 将构建注入的 UTC RFC3339 时间（如 2006-01-02T15:04:05Z）
// 解析为本地时区可读格式（2006-01-02 15:04:05）；解析失败则原样返回，空字符串返回"-"。
func formatBuildTime(raw string) string {
	if raw == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func main() {
	// ── 阶段一：创建数据目录（最早执行，此时尚不知用户语言，失败文案直接写中文）──
	// 开发构建（buildinfo.IsDev() 为 true，即 `wails3 dev` / 未注入 VERSION）使用独立的
	// .kai.dev 目录，与正式版 .kai 隔离，避免开发调试污染正式数据（对齐 certflow）。
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("获取用户主目录失败: %v", err)
	}
	dataDir := buildinfo.DataDir(homeDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	// 数据库文件（config.db / history.db / httplog.db）统一放在 DataDir 的 data/ 子目录下，
	// 即 ~/.kai/data/（或 dev 的 ~/.kai.dev/data/）。目录规则由 buildinfo.DBDir 提供。
	dbDir := buildinfo.DBDir(homeDir)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		log.Fatalf("创建数据库目录失败: %v", err)
	}

	// ── 阶段一·五：动态加载 Swift 桥接层（purego 运行时 Dlopen）──
	// 必须在任何 kai_* 调用（SetLogConfig/SetBridgeLocale/OCR/Translate 等）之前完成。
	// Init("") 默认加载与本源文件同目录的 libkai_bridge.dylib（开发态位于
	// pkg/swiftbridge/）；打包进 app bundle 时由 build 脚本拷入并传绝对路径。
	// 加载失败不致命（仅记录），缺失符号的函数变量保持 nil，调用时在对应包内报错，
	// 保证非 macOS 或 dylib 缺失环境仍能编译/启动其余功能。
	if err := swiftbridge.Init(""); err != nil {
		log.Printf("WARN: Swift 桥接层动态加载失败（部分 macOS 专属功能不可用）: %v", err)
	}

	// ── 阶段二：获取设置（日志/i18n 依赖它，必须在数据库初始化之前）──
	settingsService, err := settings.NewService(dataDir)
	if err != nil {
		log.Fatalf("加载设置失败: %v", err)
	}
	logCfg := settingsService.Get().Log

	// 更新器 Provider 句柄（初始化段赋值）；运行时语言/主题变更时通过
	// SetLocale/SetTheme 动态同步，使更新弹窗文案与配色实时跟随。
	var updaterProvider *kupdater.MirrorProvider
	// app 提升为函数级变量，使前置注册的 OnChange 闭包（早于 application.New）可引用。
	var app *application.App

	// 主日志：按天滚动写 dataDir/logs/kai.log（可随时 tail -f 查看），
	// 等级/保留天数/压缩全部取自 settings.json 的 log 段，不写死。
	logRotator := initLogging(homeDir, logCfg)

	// 全局兜底：捕获主流程及任何 goroutine 中未 recover 的 panic，
	// 先写 ERROR 日志（确保在 Close 落盘前写入）再关闭日志文件退出，
	// 避免进程静默消失、无任何痕迹。必须在 initLogging 之后注册。
	defer func() {
		if r := recover(); r != nil {
			slog.Error(i18n.T("log.global_panic"), "panic", r)
			slog.Error(i18n.T("log.global_panic_stack"), "stack", string(debug.Stack()))
			_ = logRotator.Close()
		}
	}()
	slog.Info(i18n.T("log.app_starting", "Version", buildinfo.Version, "BuildTime", formatBuildTime(buildinfo.BuildTime), "GitCommit", buildinfo.GitCommit))
	slog.Info(i18n.T("log.data_dir", "Dir", dataDir))

	// 前端日志独立落盘到 dataDir/logs/frontend.log（前端 console / JS 错误经此转发）。
	// 初始等级/保留天数/压缩同样取自 settings.json 的 log 段；后续由 applyLogConfig 实时同步。
	frontendLogFW, err := logutil.NewFrontendWriter(buildinfo.LogDir(homeDir), logutil.ParseLevel(logCfg.Level), logCfg.RetentionDays, logCfg.Compress)
	if err != nil {
		log.Printf(i18n.T("log.frontend_log_init_failed"), err)
	}
	frontendLogSvc := logutil.NewFrontendLogService(frontendLogFW)

	// ── 阶段三：数据库初始化（必须在阶段二「获取设置」之后，日志/i18n 已就绪）──
	// 初始化 HTTP 请求日志存储：必须在引擎注册（BuildHTTPClient→WrapTransport）
	// 之前点火，否则 httplog 未启用、日志层不被包裹，翻译请求不会入库。
	httpLogCfg := settingsService.Get().HttpLog
	slog.Info(i18n.T("log.http_log_status"), "enabled", httpLogCfg.Enabled, "retention_days", httpLogCfg.RetentionDays, "dbDir", dbDir)
	if err := httplogstore.Init(dbDir, httpLogCfg.Enabled); err != nil {
		slog.Warn(i18n.T("log.http_log_init_failed"), "error", err)
	} else {
		slog.Info(i18n.T("log.http_log_initialized"), "enabled", httpLogCfg.Enabled, "db", dbDir+"/httplog.db")
	}
	// 启动过期日志定时清理：仅在 enabled 时 Init 已就绪 connDSN，
	// retention_days<=0 时 StartCleanup 内部直接 return，安全。
	httplogstore.StartCleanup(httpLogCfg.RetentionDays, slog.Default())

	i18n.SetLocale(settingsService.Get().Language)

	// 运行时语言切换：外部/热重载改 settings.json 时，OnChange 同步后端 i18n 语言环境，
	// 使后端返回的错误/日志文案跟随界面语言变化（与 SaveConfig 内的主动同步互补）。
	// 同时同步日志等级 / 清理策略（log 段）。
	settingsService.OnChange(func(cfg *settings.Settings) {
		i18n.SetLocale(cfg.Language)
		applyLogConfig(logRotator, frontendLogSvc, cfg.Log, homeDir)
		// 语言/主题变更同步到更新器（auto 解析为真实语言/系统外观），刷新后续 Check/Download 文案。
		if updaterProvider != nil {
			kupdater.SetLocale(kupdater.Locale(resolveUpdaterLocale(cfg.Language)))
			// 主题变更同步到更新器（auto 解析为系统真实外观 light/dark）。
			kupdater.SetTheme(kupdater.Theme(resolveUpdaterTheme(cfg.Theme, app)))
			// 同步刷新内置更新窗口（库内原地重建窗口以应用最新语言/主题文案）。
			kupdater.SetUpdaterLocaleTheme(app)
		}
	})

	// 启动即应用一次日志配置（等级/清理策略/压缩可被 settings.json 的 log 段覆盖），
	// 同时把同一套 LogConfig 同步给 Swift 桥接层，使 kai-bridge.log 与 kai.log 一致。
	applyLogConfig(logRotator, frontendLogSvc, settingsService.Get().Log, homeDir)

	// 领域包与薄 Wrapper 的显式依赖注入（取代旧 ServiceContext 大容器）。
	// 构造时 app 尚未创建，先传 nil 占位，待 application.New 之后由 AppService.SetApp 统一注入。
	reg := engine.NewRegistry()
	// 历史库 / 引擎配置库（阶段三：数据库）
	histDB, err := historystore.Open(filepath.Join(dbDir, "history.db"))
	if err != nil {
		log.Fatalf(i18n.T("log.open_history_db_failed"), err)
	}
	cfgDB, err := configstore.Open(filepath.Join(dbDir, "config.db"))
	if err != nil {
		log.Fatalf(i18n.T("log.open_config_db_failed"), err)
	}

	trSvc := translate.NewService(reg, histDB, settingsService, nil)
	trSvc.SetConfigStore(cfgDB)
	selSvc := selection.NewService(nil, settingsService)

	// 顶层服务引用（闭包延迟解析，运行时已赋值）
	var appSvc *service.AppService
	var windowSvc *service.WindowWrapper
	// 通知服务：封装授权检查与安全发送，复用 Wails 已注册的单例。
	var notifySvc *service.NotificationService

	// 执行键：复制键触发后把选区回填主窗口
	ekCtrl := execkey.NewExecKeyController(settingsService, nil, selSvc)

	// 快捷键管理器：回调闭包桥接到 domain service
	hm := hotkey.NewManager(nil, settingsService, ekCtrl,
		func() application.Window { return mainWindow },
		func() error { _, err := trSvc.ScreenshotOCR(reg.DefaultOCREngineName()); return err },
		func() error {
			_, err := trSvc.ScreenshotTranslate(kevents.ScreenshotSessionScreenshot)
			return err
		},
		func() application.Window { return screenshotWindow },
		func(active []string) {
			if appSvc != nil {
				appSvc.EmitHotkeysChanged(active)
			}
		},
	)

	windowSvc = service.NewWindowWrapper(nil)
	configSvc := service.NewConfigWrapper(settingsService, nil, hm)
	engineSvc := service.NewEngineWrapper(reg, cfgDB, settingsService, nil, hm)
	historySvc := service.NewHistoryWrapper(histDB, cfgDB)
	translateSvc := service.NewTranslateWrapper(trSvc)
	appSvc = service.NewAppService(settingsService, trSvc, ekCtrl, hm, reg, histDB, cfgDB, nil)
	// 必须先初始化 wails notifications 单例（notifications.New 才会给 NotificationService_ 赋值），
	// 否则它是 nil 指针，传给 application.NewService 后 wails 在反射绑定方法时
	// 会对 nil 指针调 reflect.Value.Type → panic "reflect.Value.Type on zero Value"。
	notifications.New()
	notifySvc = service.NewNotificationService(notifications.NotificationService_)

	app = application.New(application.Options{
		Name:        "Kai",
		Description: i18n.T("app.description"),
		Services: []application.Service{
			// 核心服务（翻译/OCR/辅助功能/生命周期），集中编排启动
			application.NewService(appSvc),
			// 领域薄 Wrapper（前端按领域调用）
			application.NewService(configSvc),
			application.NewService(engineSvc),
			application.NewService(historySvc),
			application.NewService(translateSvc),
			application.NewService(windowSvc),
			// 前端日志桥接：接收前端 console / JS 错误，写入 logs/frontend.log
			application.NewService(frontendLogSvc),
			// 原生桌面通知（封装授权检查的 NotificationService wrapper，macOS 走 UNUserNotificationCenter，
			// 不再经前端 Web Notification 转发）
			application.NewService(notifySvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Accessory：agent app，启动即无 Dock 图标、无菜单栏，只在状态栏托盘。
			// 由 Wails 在 Cocoa 初始化阶段设置，比运行时 HideAppIcon 更早，无闪烁。
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	appSvc.SetApp(app) // 统一注入 app 到 AppService 内部持有的 domain 与 Wrapper
	// main.go 侧直接 Bind 的 wrapper 实例（与 AppService 内部不是同一批），
	// 同样需要 app 才能执行 Event.Emit 广播，故在此逐一注入。
	ekCtrl.SetApp(app) // 传导 app 到 execKeyCtrl 及其持有的 selection.Service（剪贴板读取依赖）
	configSvc.SetApp(app)
	engineSvc.SetApp(app)
	windowSvc.SetApp(app)
	trSvc.SetApp(app) // 传导 app 到 translate.Service，截图 OCR/翻译结果依赖 Event.Emit 广播给前端

	// 前端通过 runtime.EventsEmit('kai:window:show', 'settings'|'translate') 呼出窗口
	app.Event.On(kevents.EventWindowShow, func(e *application.CustomEvent) {
		name, _ := e.Data.(string)
		switch name {
		case "translate":
			windowSvc.ShowTranslateWindow()
		case "settings":
			windowSvc.ShowSettings()
		case "screenshot":
			showScreenshotWindow()
		}
	})

	// 前端「重新截图」按钮：隐藏窗口后重新走一次区域截图→OCR→翻译流程。
	app.Event.On(kevents.EventScreenshotRecapture, func(e *application.CustomEvent) {
		if screenshotWindow != nil {
			screenshotWindow.Hide()
		}
		go func() {
			if _, err := trSvc.ScreenshotTranslate(kevents.ScreenshotSessionScreenshot); err != nil {
				slog.Error(i18n.T("log.screenshot_retake_failed"), slog.Any("error", err))
			}
		}()
	})

	// 前端改语言后触发：复用最近一次 OCR 原文，跳过截图/OCR 直接用新语言重新翻译并增量推送。
	app.Event.On(kevents.EventScreenshotRetranslate, func(e *application.CustomEvent) {
		p, ok := e.Data.(kevents.ScreenshotRetranslatePayload)
		if !ok {
			slog.Error(i18n.T("log.screenshot_retranslate_failed"), slog.String("reason", "invalid payload"))
			return
		}
		go func() {
			if err := trSvc.ScreenshotRetranslate(p.Session, p.From, p.To); err != nil {
				slog.Error(i18n.T("log.screenshot_retranslate_failed"), slog.Any("error", err))
			}
		}()
	})

	// 主翻译窗口（默认隐藏，由快捷键/托盘呼出）
	// frameless + 透明标题栏；InvisibleTitleBarHeight=36 保留原生不可见标题栏区域，
	// 让 macOS 拖拽生效（frameless+透明下前端 -webkit-app-region 不接收拖拽事件），
	// 与前端 TitleBar 组件（h-9=36px, -webkit-app-region:drag）精确对齐。
	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           model.WindowTranslate,
		Title:          i18n.T("window.translate_title"),
		Width:          420,
		Height:         560,
		MinWidth:       420,
		MaxWidth:       420,
		MinHeight:      520,
		MaxHeight:      2000,
		DisableResize:  true,
		URL:            "/translate.html",
		Hidden:         true,
		Frameless:      true,
		BackgroundType: application.BackgroundTypeTransparent,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 36,
			Backdrop:                application.MacBackdropNormal,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
			},
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true, // 窗口不在 Windows 任务栏显示，只在托盘
		},
	})
	// 点红 X = 隐藏窗口（不退出）：用 RegisterHook 在 WindowClosing 的
	// 销毁 listener 之前 Cancel 掉关闭，并改为 Hide。这样红 X 保留、
	// 窗口不被销毁，随时可再次 Show。
	_ = mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		mainWindow.Hide()
	})
	mainWindow.Center()

	// 设置窗口（与 translate 一致：frameless + 透明标题栏 + 自定义 TitleBar 组件，InvisibleTitleBarHeight=36 启用拖拽）
	settingsWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           model.WindowSettings,
		Title:          i18n.T("window.settings_title"),
		Width:          1280,
		Height:         800,
		URL:            "/settings.html",
		Hidden:         true,
		Frameless:      true,
		BackgroundType: application.BackgroundTypeTransparent,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 36,
			Backdrop:                application.MacBackdropNormal,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
			},
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true, // 窗口不在 Windows 任务栏显示，只在托盘
		},
	})
	_ = settingsWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		settingsWindow.Hide()
	})

	// 截图翻译窗口：左图右译。由截图快捷键/EventScreenshotOCR 呼出。
	// frameless + 透明标题栏 + 自定义 TitleBar（InvisibleTitleBarHeight=36 启用拖拽）。
	//
	// 关键（2026-08-18 终极定位，推翻前几轮误判）：
	// 曾用 `Hidden: true` 创建，但 beta.9 对 Hidden 窗口的 impl.run() 走 else 分支——
	// 只注册 WindowDidBecomeKey 监听、从不主动 orderFront；该监听还要等 WebViewDidFinishNavigation
	// 才注册，且回调里调的是 w.parent.Show()（App 级 [NSApp unhide]，非窗口级 orderFront）。
	// 整个 Hidden 状态机对「截图窗口要可靠显示」是 hostile 的：预建/截图时无论单次还是双次 Show，
	// 窗口都停在不显示态（日志 IsVisible=false 实证，且 IsVisible 读的是 AppKit occlusionState
	// 可见位，对 accessory+透明+曾 orderOut 的窗口刷新不可靠，本就是误诊判据）。
	// 正解：不用 Hidden 创建，改为创建后立刻 Hide()——走 !options.Hidden 分支立即真实初始化
	// （app show + setShadow + setAlwaysOnTop + WebView 加载），再 orderOut 隐藏。等价于 mainWindow
	// 经 Center() 完成的「真实 show 初始化」，彻底脱离 Hidden 半成品态。截图时 Show() 走已正常初始化
	// 窗口的 makeKeyAndOrderFront，稳定上屏。窗口为 Frameless+Transparent，创建即 Hide 在同一事件循环内，
	// 启动无可见闪现。
	screenshotWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           model.WindowScreenshot,
		Title:          i18n.T("window.screenshot_title"),
		Width:          900,
		Height:         600,
		MinWidth:       600,
		MinHeight:      400,
		URL:            "/screenshot.html",
		Frameless:      true,
		BackgroundType: application.BackgroundTypeTransparent,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 36,
			Backdrop:                application.MacBackdropNormal,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
			},
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true, // 窗口不在 Windows 任务栏显示，只在托盘
		},
	})
	// 创建即 Hide：完成真实初始化（建 impl + 加载 WebView + 设 Shadow/AlwaysOnTop）后隐藏，
	// 等效于预建。之后截图路径 Show() 走轻量 orderFront，不再卡 Hidden 状态机。
	screenshotWindow.Hide()
	_ = screenshotWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		screenshotWindow.Hide()
	})
	registerTray(app, hm, configSvc, settingsService, mainWindow, settingsWindow)

	// 语言变更后，用最新语言重建托盘菜单文案（托盘为原生，只能后端重建）。
	// 优先用事件 payload 携带的语言（mode 可能含 auto，由 i18n 解析为具体语言），
	// 避免依赖配置落盘时序导致菜单仍显示旧语言。
	app.Event.On(kevents.EventLocaleChanged, func(e *application.CustomEvent) {
		if e != nil && e.Data != nil {
			if p, ok := e.Data.(kevents.LocaleChangedPayload); ok {
				if p.Mode != "" {
					i18n.SetLocale(p.Mode)
				} else if p.Language != "" {
					i18n.SetLocale(p.Language)
				}
				// 同步更新 updater 的库全局语言（包级全局，matcher/窗口直接读取），
				// 使更新检查日志与弹窗文案跟随语言切换。
				kupdater.SetLocale(kupdater.Locale(resolveUpdaterLocale(settingsService.Get().Language)))
				// 同步刷新内置更新窗口（库内原地重建窗口以应用最新语言文案）。
				kupdater.SetUpdaterLocaleTheme(app)
				// 同步当前界面语言给 Swift 桥接层，使 kai-bridge.log 调试日志跟随切换。
				engine.SetBridgeLocale(i18n.GetLocale())
			}
		}
		rebuildTrayMenu(app, hm, configSvc, settingsService)
	})

	// 快捷键启用状态变更（保存后重注册完成会广播）后，重建托盘菜单动态显示对应项。
	app.Event.On(kevents.EventHotkeysChanged, func(e *application.CustomEvent) {
		rebuildTrayMenu(app, hm, configSvc, settingsService)
	})

	// 配置自更新功能
	// https://v3.wails.io/guides/updater/
	// 更新器复用全局 HTTP 客户端（含 UA 注入、代理、自定义 DNS），
	// 而非自建裸 client，避免丢失全局注入与可观测性。
	// 更新器作为独立 package（pkg/wails-updater-providers）使用：slog/client 注入。
	// updater 的 Locale/Theme/Source 不接受 auto（第三方库已移除 auto 取值），
	// 必须把 settings 里的 auto 解析为真实值：语言取 i18n 当前生效语言，
	// 主题用系统真实外观（IsDarkMode）解析为 light/dark。这些值写入库全局
	// （SetLocale/SetTheme/SetSource），库内部（matcher/provider/窗口）直接读取。
	updLocale := resolveUpdaterLocale(settingsService.Get().Language)
	updTheme := resolveUpdaterTheme(settingsService.Get().Theme, app)
	updClient := network.BuildHTTPClient(*settingsService.Get())
	// 库全局配置：语言/主题/主源/日志器/HTTP 客户端（一次设置，运行时亦可经 SetXxx 动态切换）。
	kupdater.SetLogger(slog.Default())
	kupdater.SetClient(updClient)
	kupdater.SetLocale(kupdater.Locale(updLocale))
	kupdater.SetTheme(kupdater.Theme(updTheme))
	kupdater.SetSource(kupdater.Source(settingsService.Get().Updater.Source))
	updOpts := kupdater.Options{
		CnbRepo:     "dtapp/kai",
		GithubRepo:  "dtapps/kai",
		GithubToken: buildinfo.GithubToken,
		CnbToken:    buildinfo.CnbToken,
		BuildTime:   parseBuildTime(buildinfo.BuildTime),
		GitCommit:   buildinfo.GitCommit,
		Prerelease:  settingsService.Get().Updater.Prerelease,
	}
	// 自定义资源匹配：仅匹配 updater- 前缀的升级专用压缩包。
	// matcher 直接读库全局语言，语言切换时只需调用 SetLocale，matcher 闭包自动跟随。
	updOpts.AssetMatcher = kupdater.NewUpdaterAssetMatcher()
	updaterProvider, err = kupdater.NewMirrorProvider(&updOpts)
	if err != nil {
		slog.Error(i18n.T("log.updater_init_failed"), "error", err)
	} else {
		// 自带更新窗口（BYO）：由库（wails-updater-providers）自创建并管理窗口，
		// 在 OpenUpdaterWindow 内监听 wails:updater:resize（HTML 侧 ResizeObserver
		// 触发）后调 SetSize，实现内容自适应（框架 Builtin 模式的写死常量尺寸不随
		// notes 内容变化，且 HTML 侧无法直接调 Window.SetSize）。句柄不实现
		// WindowSizer，框架 transition() 不会用写死常量覆盖我们的尺寸。
		// 窗口默认 Hidden，启动不弹窗；Close 复用为 Hide，点 x 不销毁实例。
		winOpt := kupdater.OpenUpdaterWindow(app)
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: buildinfo.Version,
			Providers:      []updater.Provider{updaterProvider},
			Window:         winOpt,
		}); err != nil {
			slog.Error(i18n.T("log.updater_init_failed"), "error", err)
		} else {
			// 更新就绪：打印日志，由用户在托盘菜单选择重启安装。
			// 运行时回调，语言跟随库全局（已随切换更新），不用构造期快照。
			app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
				slog.Info(i18n.T("log.updater_ready"))
			})
			// 启动时静默检查（dev 构建跳过，避免噪音）。
			if !buildinfo.IsDev() {
				checkUpdateOnStart(app, notifySvc)
			}
		}
	}

	// 应用退出：清理 httplog 资源（停止定时清理 + 关闭数据库）。
	app.OnShutdown(func() {
		appSvc.ServiceShutdown()
	})

	// 系统主题变更（Wails3 官方 events.Common.ThemeChanged，跨平台由原生实现）。
	// 官方文档要求用 app.Event.OnApplicationEvent 监听系统级事件（非普通 Event.On）。
	// 回调里用可信的 application.Env.IsDarkMode() 派生真实外观，通过统一的
	// EventThemeChanged 转发出去（webview 内 matchMedia 在 macOS 不可靠）。
	// payload 同时带用户配置模式 mode 与系统真实外观 theme，前端一个事件即可处理 auto 跟随。
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(event *application.ApplicationEvent) {
		if configSvc.Theme() == string(model.ThemeAuto) {
			tray.SetIcon(selectTrayIcon(app))
		}
		sysTheme := model.ThemeLight
		if app != nil && app.Env.IsDarkMode() {
			sysTheme = model.ThemeDark
		}
		// 系统真实外观变更同步到更新器（auto 模式下跟随系统），刷新弹窗配色。
		if updaterProvider != nil {
			kupdater.SetTheme(kupdater.Theme(resolveUpdaterTheme(configSvc.Theme(), app)))
			// 同步刷新内置更新窗口配色（auto 模式下随系统外观变化）。
			kupdater.SetUpdaterLocaleTheme(app)
		}
		app.Event.Emit(kevents.EventThemeChanged, kevents.ThemeChangedPayload{
			Mode:  configSvc.Theme(),
			Theme: string(sysTheme),
		})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// 包级窗口/tray 引用，便于语言变更时重建菜单
var (
	tray             *application.SystemTray
	mainWindow       application.Window
	settingsWindow   application.Window
	screenshotWindow application.Window
)

// showScreenshotWindow 呼出截图翻译窗口并置于前台（红 X 仅隐藏，不销毁）。
// 与 ShowTranslateWindow 机制一致（service.showAndFocus）：连续两次 Show() 建 impl+真正 show，再 Focus()。
// 整个序列包在 InvokeAsync 主线程闭包内执行，避免后台 goroutine 直调 Wails 原生窗口方法触发线程问题。
func showScreenshotWindow() {
	if screenshotWindow == nil {
		return
	}
	application.InvokeAsync(func() {
		screenshotWindow.Show()
		screenshotWindow.Show()
		screenshotWindow.Focus()
	})
}

func registerTray(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, ss *settings.Service, mainWindow application.Window, settingsWindow application.Window) {
	tray = app.SystemTray.New()
	tray.SetIcon(selectTrayIcon(app))
	// 不要用 AttachWindow：macOS 上点击托盘激活 app 时会连带恢复(settings)等所有窗口。
	// 改为手动 toggle 主窗口，避免一次性打开全部窗口。
	tray.OnClick(func() {
		if mainWindow.IsVisible() {
			mainWindow.Hide()
		} else {
			mainWindow.Show().Focus()
		}
	})
	buildTrayMenu(app, hm, configSvc, ss, mainWindow, settingsWindow)
}

// buildTrayMenu 用当前语言构建托盘菜单（语言/快捷键启用状态变更时重建）。
// 菜单项按配置中对应快捷键的「是否启用」动态显示：仅启用时才加入托盘菜单。
func buildTrayMenu(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, ss *settings.Service, mainWindow application.Window, settingsWindow application.Window) {
	// lang 取空串，由 i18n.T 回退到 SetLocale 设置的全局语言（含 auto 解析），
	// 确保语言广播后即时重建菜单用最新语言，不依赖配置落盘时序。
	trayMenu := app.Menu.New()
	// 第一项：应用名称 + 当前版本号，禁用状态（不可点击）
	trayMenu.Add(i18n.T("app.name_version", "Version", buildinfo.Version)).SetEnabled(false)
	// 分隔符隔开
	trayMenu.AddSeparator()
	// 根据快捷键启用状态动态添加菜单项：输入翻译 / 截图翻译。
	// 两项之间按需补分隔符，保持与「设置」之间始终有分隔。
	cfg := configSvc.GetConfig()
	inputEnabled := cfg != nil && cfg.Hotkeys.Input.Enabled
	screenshotEnabled := cfg != nil && cfg.Hotkeys.Screenshot.Enabled
	// 始终显示「输入翻译」「截图翻译」两项，但按启用状态决定可点（禁用=置灰）。
	// 直接隐藏会让用户看不见功能入口，置灰更直观：知道有此功能，只是当前未启用。
	trayMenu.Add(i18n.T("menu.input_translate")).SetEnabled(inputEnabled).OnClick(func(ctx *application.Context) {
		// 等效于按下「输入翻译」快捷键：走复制键/系统取词分支并投递输入框。
		hm.TriggerInput()
	})
	trayMenu.Add(i18n.T("menu.screenshot_translate")).SetEnabled(screenshotEnabled).OnClick(func(ctx *application.Context) {
		// 等效于按下「截图翻译」快捷键：区域截图→OCR→翻译→呼起截图窗口。
		hm.TriggerScreenshot()
	})
	// 翻译类菜单项与下方「设置」之间补一个分隔符
	trayMenu.AddSeparator()
	// 开机自启开关：紧贴「设置」上方。勾选状态由 Wails Autostart 库当前状态决定，
	// 点击由框架自动翻转 checked，回调中调库 Enable/Disable（库自身管理持久化）。
	if enabled, err := app.Autostart.IsEnabled(); err == nil {
		trayMenu.AddCheckbox(i18n.T("menu.auto_start"), enabled).OnClick(func(ctx *application.Context) {
			if ctx.IsChecked() {
				if e := app.Autostart.Enable(); e != nil {
					slog.Warn(i18n.T("log.autostart_enable_failed"), slog.String("error", e.Error()))
				}
			} else {
				if e := app.Autostart.Disable(); e != nil {
					slog.Warn(i18n.T("log.autostart_disable_failed"), slog.String("error", e.Error()))
				}
			}
		})
	}
	// 设置，打开设置窗口
	trayMenu.Add(i18n.T("menu.settings")).OnClick(func(ctx *application.Context) {
		settingsWindow.Show().Focus()
	})
	// 分隔符隔开
	trayMenu.AddSeparator()
	// 检查更新：弹出内置升级窗口（updater_window.html 模板），用户确认后下载安装，就绪重启生效
	trayMenu.Add(i18n.T("menu.check_update")).OnClick(func(ctx *application.Context) {
		if app.Updater.State() != updater.StateUnconfigured {
			// BYO 模式下框架不负责显示窗口。Wails 关闭窗口会销毁并移出注册表，
			// 故不能靠 app.Window.Get 取其句柄 Show；统一走包内 ShowUpdaterWindow
			// 确保窗口存活/重建后再显示，保证「关闭后再检查更新」仍能弹出。
			kupdater.ShowUpdaterWindow(app)
			_ = app.Updater.CheckAndInstall(context.Background())
		}
	})
	// 预发布版更新通道开关：紧贴「检查更新」，切换后写回设置并持久化。
	// 初始勾选状态读当前配置；点击由 Wails 自动翻转 checked，回调里读新值落盘。
	trayMenu.AddCheckbox(i18n.T("menu.prerelease"), ss.Get().Updater.Prerelease).OnClick(func(ctx *application.Context) {
		ss.Get().Updater.Prerelease = ctx.IsChecked()
		if err := ss.Save(); err != nil {
			slog.Error(i18n.T("log.save_prerelease_failed"), slog.String("error", err.Error()))
		}
	})
	// 分隔符隔开
	trayMenu.AddSeparator()
	trayMenu.Add(i18n.T("menu.quit")).OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	tray.SetMenu(trayMenu)
	tray.SetTooltip(i18n.T("menu.tooltip"))
}

// rebuildTrayMenu 语言/快捷键启用状态变更时重建托盘菜单（动态显示菜单项）
func rebuildTrayMenu(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, ss *settings.Service) {
	if mainWindow == nil || settingsWindow == nil {
		return
	}
	buildTrayMenu(app, hm, configSvc, ss, mainWindow, settingsWindow)
}

// checkUpdateOnStart 启动后异步检查更新，有更新时发通知（对齐 certflow）。
func checkUpdateOnStart(app *application.App, notifySvc *service.NotificationService) {
	go func() {
		// 兜底：同点击检查更新，避免 Updater 原生层 panic 拖死主进程。
		defer func() {
			if r := recover(); r != nil {
				slog.Error(i18n.T("log.check_update_panic"), "panic", r)
			}
		}()
		// 生成 1 到 3 分钟之间的随机延迟，对齐 certflow（math/rand 足够）。
		minDuration := 1 * time.Minute
		maxDuration := 3 * time.Minute
		randomDuration := minDuration + time.Duration(rand.Intn(int(maxDuration-minDuration)))
		time.Sleep(randomDuration)
		rel, err := app.Updater.Check(context.Background())
		if err != nil {
			slog.Warn(i18n.T("log.check_update_failed"), "error", err)
			return
		}
		if rel == nil {
			return // 没有更新
		}
		// 发送原生桌面通知（Wails notifications service，不经前端转发）。
		// 授权检查与降级逻辑统一收敛到 service.NotificationService，调用处只关心发什么。
		notifySvc.Notify(notifications.NotificationOptions{
			ID:       "kai-update-available",
			Title:    i18n.T("notification.update_available_title"),
			Subtitle: i18n.T("notification.update_available_subtitle", "version", rel.Version),
		})
	}()
}

// selectTrayIcon 根据系统暗色状态选择托盘图标。使用 Wails3 Environment API
// 探测当前外观，跨平台可靠。预留暗色图标分支：补 build/darwin/trayicon_dark.png
// 并在下方 embed 后即可自动跟随系统暗色。
func selectTrayIcon(app *application.App) []byte {
	// 预留暗色图标分支：补 build/darwin/trayicon_dark.png 并在上方 embed 后即可跟随系统暗色。
	// if app != nil && app.Env.IsDarkMode() && len(trayIconDark) > 0 {
	// 	return trayIconDark
	// }
	return trayIcon
}

// initLogging 将 slog 默认 logger 输出到 dataDir/logs/kai.log（按天滚动），
// 同时多路写 stderr 方便开发期终端查看。日志等级完全由 settings.json 的 log.level 控制
// （启动后 applyLogConfig 应用，不做任何 dev/正式的区别覆盖）。
// 返回 *logutil.Rotator，供设置热更新时动态调整级别 / 清理策略。
func initLogging(homeDir string, logCfg settings.LogConfig) *logutil.Rotator {
	logLevel := logutil.ParseLevel(logCfg.Level)
	logDir := buildinfo.LogDir(homeDir)
	rotator, err := logutil.NewRotator(logDir, logLevel, logCfg.RetentionDays, logCfg.Compress)
	if err != nil {
		// 目录/文件不可用：降级为仅 stderr，保证应用仍能启动并记录关键日志。
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
		log.Printf(i18n.T("log.rotate_log_init_failed"), err)
		return rotator
	}
	slog.SetDefault(slog.New(rotator.Handler()))
	log.Printf(i18n.T("log.log_initialized"), filepath.Join(logDir, "kai.log"))
	return rotator
}

// applyLogConfig 依据 settings.LogConfig 动态调整运行日志等级与清理策略。
// 等级完全由设置文件（settings.json 的 log.level）控制，不做任何 dev/正式的区别覆盖。
// dataDir 用于把同一套配置（目录 + 等级/保留天数/压缩）同步给 Swift 桥接层，
// 使 kai-bridge.log、frontend.log 与主应用日志 kai.log 使用相同策略（等级过滤、按天滚动、清理、压缩）。
func applyLogConfig(r *logutil.Rotator, fl *logutil.FrontendLogService, cfg settings.LogConfig, homeDir string) {
	if r == nil {
		return
	}
	level := logutil.ParseLevel(cfg.Level)
	r.SetLevel(level)
	r.UpdateRetention(cfg.RetentionDays, cfg.Compress)
	if fl != nil {
		fl.SetLevel(level)
	}
	// 同步给 Swift 桥接层（kai-bridge.log），等级同样来自设置文件。
	engine.SetLogConfig(buildinfo.LogDir(homeDir), cfg.Level, cfg.RetentionDays, cfg.Compress)
	// 同步当前界面语言，使桥接层调试日志随系统语言切换中/英文。
	engine.SetBridgeLocale(i18n.GetLocale())
	slog.Info(i18n.T("log.log_config_applied"), "level", level.String(), "retention_days", cfg.RetentionDays, "compress", cfg.Compress)
}
