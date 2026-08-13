// Package execkey 实现"执行键"能力：模拟复制键把选区/剪贴板内容喂给翻译或输入回填。
// 依赖 selection 包读取剪贴板，依赖 settings 读执行键配置，依赖 app 推送事件。
// 平台相关的"模拟按键复制"（copySelection）与"快捷键解析"（parseHotkey）见同包平台文件。
package execkey

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/settings"
)

// ExecKeyController 执行键控制器：承载所有"程序主动模拟按下"的执行键逻辑（与 RegisteredHotkeyConfig
// 这种"被监听的注册键"严格区分）。执行键不参与 mgr.Register 拦截，而是由注册键回调按需调用，
// 通过 robotgo 替用户模拟按下配置的组合键（如 Cmd+C），再把选区写入剪贴板。
type ExecKeyController struct {
	settingsSvc *settings.Service
	app         *application.App
	selection   *selection.Service
	log         *slog.Logger
}

// NewExecKeyController 构造执行键控制器。
func NewExecKeyController(
	st *settings.Service,
	app *application.App,
	sel *selection.Service,
) *ExecKeyController {
	cfg := st.Get()
	e := &ExecKeyController{
		settingsSvc: st,
		app:         app,
		selection:   sel,
		log:         slog.Default(),
	}
	e.log.Info("执行键配置",
		slog.String("复制键", cfg.ExecKeys.Copy.Key),
		slog.Bool("启用", cfg.ExecKeys.Copy.Enabled),
	)
	return e
}

// SetApp 在 app 就绪后注入（启动编排阶段）。同时把 app 传导给其持有的 selection.Service，
// 否则 selection.Service.app 始终为 nil，readClipboardText(nil) 会永远返回空串（复制键读到空）。
func (e *ExecKeyController) SetApp(app *application.App) {
	e.app = app
	if e.selection != nil {
		e.selection.SetApp(app)
	}
}

// ExecuteCopyKey 执行「复制键」（ExecKeyConfig.Copy）——这是执行键，由程序主动 robotgo 模拟按下，
// 把当前选中内容写入剪贴板，再读取并回填主窗口翻译。
// 该函数只会被其他"注册键"的回调按需调用，绝不被 mgr.Register 直接挂接。
func (e *ExecKeyController) ExecuteCopyKey() {
	e.log.Info("执行快捷键 开始", slog.String("功能", "复制键"), slog.String("按键", e.settingsSvc.Get().ExecKeys.Copy.Key))
	e.CopyAndTranslate()
}

// CopyAndTranslate 执行 ExecKeyConfig.Copy 并把结果回填主窗口输入框。
func (e *ExecKeyController) CopyAndTranslate() {
	cfg := e.settingsSvc.Get()
	// 边界保护：config 或复制键未就绪/未启用时，仅告警返回，避免空指针。
	if cfg == nil || cfg.ExecKeys.Copy.Key == "" || !cfg.ExecKeys.Copy.Enabled {
		e.log.Warn("执行快捷键 跳过", slog.String("原因", "复制键未配置"))
		return
	}
	before := e.selection.ReadClipboardText()
	e.log.Info("执行快捷键 模拟前剪贴板", slog.Int("长度", len(before)), slog.String("内容", before))

	text := e.copySelection()
	e.log.Info("执行快捷键 完成", slog.String("按键", cfg.ExecKeys.Copy.Key), slog.Int("长度", len(text)))

	if text == "" {
		e.log.Warn("执行快捷键 剪贴板为空", slog.String("按键", cfg.ExecKeys.Copy.Key),
			slog.String("原因", "模拟复制可能未生效（未授权辅助功能/选区为空）"))
		return
	}
	if e.app != nil {
		e.app.Event.Emit(events.EventInputFill, text)
	}
}

// CopySelection 暴露平台相关的 copySelection（模拟复制键并读剪贴板），
// 供 HotkeyManager 在"唤起主窗口"回调里取选区。
func (e *ExecKeyController) CopySelection() string {
	return e.copySelection()
}

// CopySelectionOrRestore 专供"唤起主窗口"快捷键使用：模拟复制取选区，但保护用户剪贴板。
// 逻辑：
//  1. 执行模拟复制【前】先把剪贴板清空（WriteToClipboard("")）——用户明确要求；
//  2. 再模拟 Cmd+C 取词；
//  3. 取到的内容非空 → 返回该文本（有选区，调用方回填窗口）；
//  4. 取到空 → 说明没选中任何文字，剪贴板已是空（上一步清空所得），无需还原，
//     返回空串（调用方据此只开窗口、不回填）。
//
// 这样"没选中文字"场景下：执行前已清空剪贴板，模拟 Cmd+C 落到空剪贴板取不到内容，
// 窗口照常打开、绝不回填、且不会把你原有剪贴板内容弄丢/弄乱——
// 正是用户要求的"没选中就只是窗口，不要碰剪贴板内容"。
func (e *ExecKeyController) CopySelectionOrRestore() string {
	// 关键：执行复制键前先清空剪贴板，避免无选区时把用户原有内容误改动/误回填。
	_ = e.selection.WriteToClipboard("")
	text := e.copySelection()
	if text == "" {
		// 无选区：剪贴板已是空（上一步清空所得），不回填，只开窗口。
		return ""
	}
	return text
}
