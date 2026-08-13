package service

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/translate"
	"cnb.cool/dtapp/kai/internal/useragent"
)

// version 应用版本（由构建注入）
var version = "dev"

// AppService 应用核心门面（薄 Wrapper）：
//   - 唯一实现 wails 生命周期三件套（ServiceStartup / ServiceShutdown / ServiceName），
//     作为全局启动编排的唯一入口（引擎加载、快捷键注册、语言/辅助功能初始化）。
//   - 持有各 domain 服务与 Wrapper 的引用，做依赖注入的中枢。
//   - 仅暴露核心 RPC（版本、辅助功能、启动编排）；翻译/历史/配置/窗口等 RPC 由各 Wrapper 负责。
type AppService struct {
	app          *application.App
	settingsSvc  *settings.Service
	translateSvc *translate.Service
	execKeyCtrl  *execkey.ExecKeyController
	hotkeyMgr    *hotkey.Manager
	engineSvc    *EngineWrapper
	historySvc   *HistoryWrapper
	configSvc    *ConfigWrapper
	windowSvc    *WindowWrapper
	registry     *engine.Registry
	log          *slog.Logger
}

// NewAppService 构造应用核心门面。所有依赖显式注入，消除共享容器。
func NewAppService(
	st *settings.Service,
	tr *translate.Service,
	ek *execkey.ExecKeyController,
	hm *hotkey.Manager,
	reg *engine.Registry,
	histDB *historystore.Store,
	cfgStore *configstore.Store,
	app *application.App,
) *AppService {
	windowSvc := NewWindowWrapper(app)
	configSvc := NewConfigWrapper(st, app, hm)
	historySvc := NewHistoryWrapper(histDB, cfgStore)
	engineSvc := NewEngineWrapper(reg, cfgStore, st, app, hm)
	return &AppService{
		app:          app,
		settingsSvc:  st,
		translateSvc: tr,
		execKeyCtrl:  ek,
		hotkeyMgr:    hm,
		engineSvc:    engineSvc,
		historySvc:   historySvc,
		configSvc:    configSvc,
		windowSvc:    windowSvc,
		registry:     reg,
		log:          slog.Default(),
	}
}

// SetApp 在 app 就绪后注入（启动编排阶段）。
func (s *AppService) SetApp(app *application.App) {
	s.app = app
}

// SetUserAgent 由前端在窗口启动时调用，把 WebView 的 navigator.userAgent 传入后端，
// 作为全局 HTTP 请求的默认 User-Agent（经 useragent 包注入到各引擎的 http.Client）。
func (s *AppService) SetUserAgent(ua string) {
	useragent.Set(ua)
}

// GetVersion 返回应用版本。
func (s *AppService) GetVersion() string {
	return version
}

// CheckAccessibility 检查 macOS 辅助功能是否已授权（跨平台：非 darwin 直接返回 true）。
func (s *AppService) CheckAccessibility() bool {
	return s.isAccessibilityEnabled()
}

// OpenAccessibilitySettings 打开系统辅助功能设置面板（仅 darwin 生效）。
func (s *AppService) OpenAccessibilitySettings() {
	s.openAccessibilitySettings()
}

// CheckScreenRecording 检查 macOS 屏幕录制是否已授权（截图翻译依赖，跨平台：非 darwin 直接返回 true）。
func (s *AppService) CheckScreenRecording() bool {
	return s.isScreenRecordingEnabled()
}

// OpenScreenRecordingSettings 弹系统「屏幕录制」授权框（仅 darwin 生效）。
func (s *AppService) OpenScreenRecordingSettings() {
	s.openScreenRecordingSettings()
}

// TODO: 输入监控相关（CheckInputMonitoring / OpenInputMonitoringSettings 导出方法）当前未使用，已注释。
// // CheckInputMonitoring 检查 macOS 输入监控是否已授权（跨平台：非 darwin 直接返回 true）。
// func (s *AppService) CheckInputMonitoring() bool {
// 	return s.isInputMonitoringEnabled()
// }
//
// // OpenInputMonitoringSettings 打开系统输入监控设置面板（仅 darwin 生效）。
// func (s *AppService) OpenInputMonitoringSettings() {
// 	s.openInputMonitoringSettings()
// }

// ServiceName 返回服务名（wails 生命周期三件套之一）。
func (s *AppService) ServiceName() string {
	return "AppService"
}

// ServiceStartup 应用启动编排唯一入口（wails 生命周期三件套之一）。
// app 已由 main.go 在 application.New 之后通过 SetApp 注入，此处不再从 ctx 取。
func (s *AppService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	// 把 app 注入所有依赖 app 的 domain 与 Wrapper
	s.translateSvc.SetApp(s.app)
	s.execKeyCtrl.SetApp(s.app)
	s.configSvc.SetApp(s.app)
	s.engineSvc.SetApp(s.app)
	s.hotkeyMgr.SetApp(s.app)

	// 从 config.db 加载引擎配置并注册到 registry
	if err := s.engineSvc.loadEngines(); err != nil {
		s.log.Error("加载引擎配置失败", slog.Any("error", err))
	}

	// 注册全局快捷键（注册键逻辑集中在 HotkeyManager）
	s.hotkeyMgr.Register()

	// 初始化语言
	i18n.SetLocale(s.settingsSvc.Get().Language)

	// 辅助功能授权提示（darwin 下若未授权，复制键/模拟按键不会生效）
	if !s.isAccessibilityEnabled() {
		s.log.Warn("辅助功能未授权：复制键/模拟按键可能不生效，请在系统设置中授予本应用辅助功能权限")
	}
	return nil
}

// ServiceShutdown 应用关闭时清理（wails 生命周期三件套之一）。
func (s *AppService) ServiceShutdown() error {
	if s.hotkeyMgr != nil {
		s.hotkeyMgr.Unregister()
	}
	return nil
}

// emitHotkeysChanged 把当前生效的快捷键清单广播给前端实时展示。
func (s *AppService) emitHotkeysChanged(active []string) {
	if s.app == nil {
		return
	}
	s.app.Event.Emit(events.EventHotkeysChanged, active)
}

// EmitHotkeysChanged 供外层（快捷键管理器）调用，广播当前生效快捷键清单。
func (s *AppService) EmitHotkeysChanged(active []string) {
	s.emitHotkeysChanged(active)
}

// TranslateMulti 批量翻译（委托 translate.Service，供前端 AppService.TranslateMulti 调用）。
func (s *AppService) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	return s.translateSvc.TranslateMulti(req)
}

// ScreenshotOCR 截图 OCR（无参，使用默认 OCR 引擎，委托 translate.Service）。
func (s *AppService) ScreenshotOCR() (*model.OcrResult, error) {
	return s.translateSvc.ScreenshotOCR(s.registry.DefaultOCREngineName())
}
