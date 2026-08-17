// Package selection 提供选区读取 / 选区坐标能力（纯业务逻辑，不依赖 wails 生命周期）。
// 负责：读取当前选区文本（跨平台，含剪贴板兜底）、按选区坐标定位。
// 跨平台选区坐标（currentSelectionPoint / primaryScreenSize）与各系统取词（currentSelectionOSA /
// GetSelectedText）实现见同包平台文件。
package selection

import (
	"fmt"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Service 选区领域服务。
type Service struct {
	app         *application.App
	settingsSvc *settings.Service
	log         *slog.Logger
}

// NewService 构造选区服务。app 用于剪贴板读写，settingsSvc 用于平台相关配置。
func NewService(app *application.App, st *settings.Service, log *slog.Logger) *Service {
	return &Service{app: app, settingsSvc: st, log: log}
}

// SetApp 在 app 就绪后注入（启动编排阶段）。
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// ReadClipboardText 跨平台读取系统剪贴板文本（通用实现，平台差异见 copy 平台文件）。
func (s *Service) ReadClipboardText() string {
	return readClipboardText(s.app)
}

// WriteToClipboard 跨平台写入系统剪贴板文本。
func (s *Service) WriteToClipboard(text string) error {
	if s.app == nil {
		return nil
	}
	if !s.app.Clipboard.SetText(text) {
		return fmt.Errorf(i18n.T("err.selection_write_clipboard"))
	}
	return nil
}

// TODO(2026-08-11): SelectedTextViaSystem 已禁用。它是"唤起主窗口"快捷键（manager.go 内
// case h.selSvc != nil 分支）专用的系统取词入口；该分支因疑似引发电脑异常已被注释禁用，
// 故本方法当前无活跃调用者。实现保留，恢复 manager.go 分支时一并恢复本方法即可。
// 注意：划词浮窗/选区翻译仍走 ReadSelection → currentSelectionOSA（Swift kai_selected_text），
// 与本条路径相互独立，不受影响。
//
// SelectedTextViaSystem 直接经系统取词读取当前前台应用选区（不依赖复制键/剪贴板）。
// macOS 走 Swift 桥接（AXUIElement, kai_selected_text），Windows 走 UI Automation，
// 其它平台返回空串。供"唤起主窗口"快捷键在未配置复制键时取选区。
// func (s *Service) SelectedTextViaSystem() string {
// 	return currentSelectionOSA()
// }

// ReadSelection 选区优先（OSA / 系统取词），失败回退剪贴板读取。
func (s *Service) ReadSelection() string {
	text := currentSelection()
	if text != "" {
		return text
	}
	return s.ReadClipboardText()
}

// ScreenSize 返回主屏幕分辨率。
func (s *Service) ScreenSize() (float64, float64) {
	return primaryScreenSize()
}

// SelectionPoint 返回当前选区锚点屏幕坐标（选区为空时返回 nil）。
// 供执行键（execkey）在推送选区时附带坐标，与 emitSelection 保持同一坐标来源。
func (s *Service) SelectionPoint() *application.Point {
	return currentSelectionPoint()
}

// currentSelection 选区优先，失败回退剪贴板（跨平台通用，平台实现见平台文件）。
func currentSelection() string {
	if text := currentSelectionOSA(); text != "" {
		return text
	}
	return readClipboardText(nil)
}
