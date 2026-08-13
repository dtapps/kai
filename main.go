package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

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
	kupdater "cnb.cool/dtapp/kai/internal/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon.png
var trayIcon []byte

//go:embed updater_window.html
var updaterWindowHTML string

func init() {
	// 注册自定义事件类型，供后端 emit / 前端监听（对齐 certflow 的 RegisterEvent 模式）。
	application.RegisterEvent[kevents.NotificationPayload](kevents.EventNotification)
}

func main() {
	// 确定数据目录。
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
		log.Fatalf("创建 db 目录失败: %v", err)
	}

	// 初始化设置服务（尽早加载，日志/i18n 依赖它）
	settingsService, err := settings.NewService(dataDir)
	if err != nil {
		log.Fatalf("加载设置失败: %v", err)
	}
	logCfg := settingsService.Get().Log

	// 主日志：按天滚动写 dataDir/logs/kai.log（可随时 tail -f 查看），
	// 等级/保留天数/压缩全部取自 settings.json 的 log 段，不写死。
	logRotator := initLogging(homeDir, logCfg)

	// 前端日志独立落盘到 dataDir/logs/frontend.log（前端 console / JS 错误经此转发）。
	// 初始等级/保留天数/压缩同样取自 settings.json 的 log 段；后续由 applyLogConfig 实时同步。
	frontendLogFW, err := logutil.NewFrontendWriter(buildinfo.LogDir(homeDir), logutil.ParseLevel(logCfg.Level), logCfg.RetentionDays, logCfg.Compress)
	if err != nil {
		log.Printf("初始化前端日志失败，跳过: %v", err)
	}
	frontendLogSvc := logutil.NewFrontendLogService(frontendLogFW)

	// 初始化 HTTP 请求日志存储：必须在引擎注册（BuildHTTPClient→WrapTransport）
	// 之前点火，否则 httplog 未启用、日志层不被包裹，翻译请求不会入库。
	httpLogCfg := settingsService.Get().HttpLog
	slog.Info("HTTP 日志开关状态", "enabled", httpLogCfg.Enabled, "retention_days", httpLogCfg.RetentionDays, "dbDir", dbDir)
	if err := httplogstore.Init(dbDir, httpLogCfg.Enabled); err != nil {
		slog.Warn("初始化 HTTP 日志失败", "error", err)
	} else {
		slog.Info("HTTP 日志存储已初始化", "enabled", httpLogCfg.Enabled, "db", dbDir+"/httplog.db")
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
	})

	// 启动即应用一次日志配置（等级/清理策略/压缩可被 settings.json 的 log 段覆盖），
	// 同时把同一套 LogConfig 同步给 Swift 桥接层，使 kai-bridge.log 与 kai.log 一致。
	applyLogConfig(logRotator, frontendLogSvc, settingsService.Get().Log, homeDir)

	// 领域包与薄 Wrapper 的显式依赖注入（取代旧 ServiceContext 大容器）。
	// 构造时 app 尚未创建，先传 nil 占位，待 application.New 之后由 AppService.SetApp 统一注入。
	reg := engine.NewRegistry()
	histDB, err := historystore.Open(filepath.Join(dbDir, "history.db"))
	if err != nil {
		log.Fatalf("打开历史数据库失败: %v", err)
	}
	cfgDB, err := configstore.Open(filepath.Join(dbDir, "config.db"))
	if err != nil {
		log.Fatalf("打开引擎配置数据库失败: %v", err)
	}

	trSvc := translate.NewService(reg, histDB, settingsService, nil, slog.Default())
	trSvc.SetConfigStore(cfgDB)
	selSvc := selection.NewService(nil, settingsService, slog.Default())

	// 顶层服务引用（闭包延迟解析，运行时已赋值）
	var appSvc *service.AppService
	var windowSvc *service.WindowWrapper

	// 执行键：复制键触发后把选区回填主窗口
	ekCtrl := execkey.NewExecKeyController(settingsService, nil, selSvc)

	// 快捷键管理器：回调闭包桥接到 domain service
	hm := hotkey.NewManager(nil, settingsService, ekCtrl,
		func() application.Window { return mainWindow },
		func() error { _, err := trSvc.ScreenshotOCR(reg.DefaultOCREngineName()); return err },
		func() error {
			_, err := trSvc.ScreenshotTranslate()
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

	app := application.New(application.Options{
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

	// 截图翻译结果投递：后端完成 区域截图→OCR→翻译 后，这里仅负责把窗口拉起到前台展示。
	// 结果数据经 EventScreenshotOCR 在前端截图窗口内接收并渲染（左图右译）。
	app.Event.On(kevents.EventScreenshotOCR, func(e *application.CustomEvent) {
		showScreenshotWindow()
	})

	// 前端「重新截图」按钮：隐藏窗口后重新走一次区域截图→OCR→翻译流程。
	app.Event.On(kevents.EventScreenshotRecapture, func(e *application.CustomEvent) {
		if screenshotWindow != nil {
			screenshotWindow.Hide()
		}
		go func() {
			if _, err := trSvc.ScreenshotTranslate(); err != nil {
				slog.Error("[Kai-截图翻译] 重新截图失败", slog.Any("error", err))
			}
		}()
	})

	// 主翻译窗口（默认隐藏，由快捷键/托盘呼出）
	// frameless + 透明标题栏；InvisibleTitleBarHeight=36 保留原生不可见标题栏区域，
	// 让 macOS 拖拽生效（frameless+透明下前端 -webkit-app-region 不接收拖拽事件），
	// 与前端 TitleBar 组件（h-9=36px, -webkit-app-region:drag）精确对齐。
	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "translate",
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
		Name:           "settings",
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
			HiddenOnTaskbar: true,
		},
	})
	_ = settingsWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		settingsWindow.Hide()
	})

	// 截图翻译窗口：左图右译。默认隐藏，由截图快捷键/EventScreenshotOCR 呼出。
	// frameless + 透明标题栏 + 自定义 TitleBar（InvisibleTitleBarHeight=36 启用拖拽）。
	screenshotWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "screenshot",
		Title:          i18n.T("window.screenshot_title"),
		Width:          900,
		Height:         600,
		MinWidth:       600,
		MinHeight:      400,
		URL:            "/screenshot.html",
		Hidden:         true,
		AlwaysOnTop:    true,
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
			HiddenOnTaskbar: true,
		},
	})
	_ = screenshotWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		screenshotWindow.Hide()
	})

	registerTray(app, configSvc, mainWindow, settingsWindow)

	// 语言变更后，用最新语言重建托盘菜单文案（托盘为原生，只能后端重建）。
	// 优先用事件 payload 携带的语言（mode 可能含 auto，由 i18n 解析为具体语言），
	// 避免依赖配置落盘时序导致菜单仍显示旧语言。
	app.Event.On(kevents.EventLocaleChanged, func(e *application.CustomEvent) {
		if e != nil && e.Data != nil {
			if p, ok := e.Data.(map[string]any); ok {
				if mode, ok := p["mode"].(string); ok && mode != "" {
					i18n.SetLocale(mode)
				} else if lang, ok := p["language"].(string); ok && lang != "" {
					i18n.SetLocale(lang)
				}
			}
		}
		rebuildTrayMenu(app)
	})

	// 自动升级：CNB / GitHub 双源（中文用户走 CNB 镜像，英文用户走 GitHub），
	// 需 CI 注入对应 token。升级窗口复用内置窗口模板（updater_window.html），
	// 由托盘菜单「检查更新」触发，就绪后通过 EventUpdateReady 重启。
	// AssetMatcher 仅匹配 updater- 前缀的升级专用压缩包，规避普通 release 附件。
	matcher := func(req updater.CheckRequest, assets []github.ReleaseAsset) int {
		idx := -1
		for i, a := range assets {
			// 平台 / 架构过滤（对齐 github.DefaultAssetMatcher 的核心逻辑）。
			if !strings.Contains(strings.ToLower(a.Name), strings.ToLower(req.Platform)) {
				continue
			}
			// macOS 专用：仅接受 zip / tar.gz（跳过 dmg / pkg 等非自解压格式）。
			ext := strings.ToLower(filepath.Ext(a.Name))
			if ext != ".zip" && ext != ".tar.gz" {
				continue
			}
			// 仅匹配 updater- 前缀的升级专用资源，规避普通 release 附件。
			if !strings.HasPrefix(a.Name, "updater-") {
				continue
			}
			// 命中首个满足条件的资源即返回（kai 每平台仅产出一个 updater- 包）。
			idx = i
			break
		}
		return idx
	}
	provider, err := kupdater.NewMirrorProvider(github.Config{
		Repository:    "dtapp/kai",
		Token:         buildinfo.GithubToken,
		Prerelease:    false,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  matcher,
	}, network.BuildHTTPClient(*settingsService.Get()), buildinfo.BuildTime, buildinfo.CnbToken)
	if err != nil {
		slog.Error("创建更新源失败", "error", err)
	} else {
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: buildinfo.Version,
			Providers:      []updater.Provider{provider},
			Window:         &updater.BuiltinWindow{HTML: updaterWindowHTML},
		}); err != nil {
			slog.Error("初始化更新器失败", "error", err)
		} else {
			// 更新就绪：打印日志，由用户在托盘菜单选择重启安装。
			app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
				slog.Info("更新已就绪，重启后生效")
			})
			// 启动时静默检查（dev 构建跳过，避免噪音）。
			if !buildinfo.IsDev() {
				checkUpdateOnStart(app)
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
		sysTheme := "light"
		if app != nil && app.Env.IsDarkMode() {
			sysTheme = "dark"
		}
		app.Event.Emit(kevents.EventThemeChanged, kevents.ThemeChangedPayload{
			Mode:  configSvc.Theme(),
			Theme: sysTheme,
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
// 与 ShowTranslateWindow 一致：Wails v3 对 Hidden 窗口首次 Show() 只触发 Run() 创建
// webview 而不真正显示，需连续两次 Show()（第一次建 impl，第二次真正 show）才能稳定出现，
// 否则第一次快捷键呼出时窗口不显示、需再触发一次。最后 Focus() 拉到最前。
func showScreenshotWindow() {
	if screenshotWindow != nil {
		screenshotWindow.Show()
		screenshotWindow.Show()
		screenshotWindow.Focus()
	}
}

func registerTray(app *application.App, configSvc *service.ConfigWrapper, mainWindow application.Window, settingsWindow application.Window) {
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
	buildTrayMenu(app, mainWindow, settingsWindow)
}

// buildTrayMenu 用当前语言构建托盘菜单（语言变更时重建）
func buildTrayMenu(app *application.App, mainWindow application.Window, settingsWindow application.Window) {
	// lang 取空串，由 i18n.T 回退到 SetLocale 设置的全局语言（含 auto 解析），
	// 确保语言广播后即时重建菜单用最新语言，不依赖配置落盘时序。
	trayMenu := app.Menu.New()
	// 第一项：应用名称 + 当前版本号，禁用状态（不可点击）
	trayMenu.Add(i18n.T("app.name_version", "Version", buildinfo.Version)).SetEnabled(false)
	// 分隔符隔开
	trayMenu.AddSeparator()
	// 输入翻译，打开翻译主窗口
	trayMenu.Add(i18n.T("menu.input_translate")).OnClick(func(ctx *application.Context) {
		mainWindow.Show().Focus()
	})
	// 分隔符隔开
	trayMenu.AddSeparator()
	// 设置，打开设置窗口
	trayMenu.Add(i18n.T("menu.settings")).OnClick(func(ctx *application.Context) {
		settingsWindow.Show().Focus()
	})
	// 检查更新：弹出内置升级窗口（updater_window.html 模板），用户确认后下载安装，就绪重启生效
	trayMenu.Add(i18n.T("menu.check_update")).OnClick(func(ctx *application.Context) {
		if app.Updater.State() != updater.StateUnconfigured {
			_ = app.Updater.CheckAndInstall(context.Background())
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

// rebuildTrayMenu 语言变更时重建托盘菜单文案
func rebuildTrayMenu(app *application.App) {
	if mainWindow == nil || settingsWindow == nil {
		return
	}
	buildTrayMenu(app, mainWindow, settingsWindow)
}

// checkUpdateOnStart 启动后异步检查更新，有更新时发通知（对齐 certflow）。
func checkUpdateOnStart(app *application.App) {
	go func() {
		// 兜底：同点击检查更新，避免 Updater 原生层 panic 拖死主进程。
		defer func() {
			if r := recover(); r != nil {
				slog.Error("启动检查更新异常", "panic", r)
			}
		}()
		// 生成 1 到 3 分钟之间的随机延迟，对齐 certflow（math/rand 足够）。
		minDuration := 1 * time.Minute
		maxDuration := 3 * time.Minute
		randomDuration := minDuration + time.Duration(rand.Intn(int(maxDuration-minDuration)))
		time.Sleep(randomDuration)
		rel, err := app.Updater.Check(context.Background())
		if err != nil {
			slog.Warn("启动检查更新失败", "error", err)
			return
		}
		if rel == nil {
			return // 没有更新
		}
		// 发送桌面通知
		if ok := app.Event.Emit(kevents.EventNotification, kevents.NotificationPayload{
			Title:    i18n.T("notification.update_available_title"),
			Subtitle: i18n.T("notification.update_available_subtitle", "version", rel.Version),
			Category: "system",
			Level:    "info",
		}); !ok {
			slog.Warn("发送更新通知失败")
		}
	}()
}

// selectTrayIcon 根据系统暗色状态选择托盘图标。使用 Wails3 Environment API
// 探测当前外观，跨平台可靠。预留暗色图标分支：补 build/darwin/trayicon_dark.png
// 并在下方 embed 后即可自动跟随系统暗色。
func selectTrayIcon(app *application.App) []byte {
	if app != nil && app.Env.IsDarkMode() {
		// 若有暗色图标资源，优先返回；否则沿用默认图标。
		// if len(trayIconDark) > 0 { return trayIconDark }
	}
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
		log.Printf("初始化滚动日志失败，仅使用 stderr: %v", err)
		return rotator
	}
	slog.SetDefault(slog.New(rotator.Handler()))
	log.Printf("日志已初始化，写入: %s", filepath.Join(logDir, "kai.log"))
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
	slog.Info("日志配置已应用", "level", level.String(), "retention_days", cfg.RetentionDays, "compress", cfg.Compress)
}
