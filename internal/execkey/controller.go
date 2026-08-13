// Package execkey 实现"执行键"能力：模拟复制键把选区/剪贴板内容喂给翻译或输入回填。
// 依赖 selection 包读取剪贴板，依赖 settings 读执行键配置，依赖 app 推送事件。
// 平台相关的"模拟按键复制"（copySelection）与"快捷键解析"（parseHotkey）见同包平台文件。
package execkey

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
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

// CopySelection 取当前选区文本（供"唤起主窗口"快捷键使用）：模拟复制键取选区，
// 但全程保护用户剪贴板——备份 → 清空 → 复制 → 先清空再写回 backup（双保险），
// 系统剪贴板始终还原成用户原内容，不残留选区。复制失败/空选区则返回空串。
func (e *ExecKeyController) CopySelection() string {
	cfg := e.settingsSvc.Get()
	// 1. 备份原剪贴板，避免任何改动。
	backup := e.selection.ReadClipboardText()
	e.log.Debug("[复制键] 唤起主窗口 备份剪贴板", slog.Int("长度", len(backup)))

	// 2. 清空剪贴板，避免无选区时残留旧内容被误回填。
	_ = e.selection.WriteToClipboard("")

	// 3. 执行复制（尊重 fallback 配置）：自定义键没拿到内容时回退系统默认复制键重试。
	text := e.copySelection(cfg.ExecKeys.Copy.Fallback)

	// 4. 还原用户原剪贴板内容：先清空（挤掉复制残留的选区内容）再写回 backup，双保险不残留。
	if err := e.selection.WriteToClipboard(""); err != nil {
		e.log.Warn("[复制键] 唤起主窗口 清空剪贴板失败", slog.String("错误", err.Error()))
	}
	if err := e.selection.WriteToClipboard(backup); err != nil {
		e.log.Warn("[复制键] 唤起主窗口 还原剪贴板失败", slog.String("错误", err.Error()))
	}
	return text
}
