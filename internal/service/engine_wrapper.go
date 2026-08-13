package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/network"
	"cnb.cool/dtapp/kai/internal/settings"
)

var (
	errEngineStoreNotReady = errors.New("引擎存储未就绪")
	log                    = slog.Default()
)

// EngineItem 返回给前端的引擎条目（带启用状态与展示名）。
type EngineItem struct {
	ID      int64  `json:"id"`      // 引擎自增主键 ID
	Name    string `json:"name"`    // 引擎展示名
	Enabled bool   `json:"enabled"` // 是否已启用
}

// EngineWrapper 引擎配置的薄适配层：持有 registry + configstore + settings + hotkey，
// 负责引擎启停的运行时注册与持久化、schema 下发。仅暴露 RPC，不实现 wails 生命周期三件套。
type EngineWrapper struct {
	registry         *engine.Registry
	configStore      *configstore.Store
	settingsSvc      *settings.Service
	settingsProvider func() settings.Settings
	app              *application.App
	hotkeyMgr        *hotkey.Manager
}

// NewEngineWrapper 构造引擎 Wrapper。app 与 hotkeyMgr 允许在启动编排后注入。
func NewEngineWrapper(
	reg *engine.Registry,
	store *configstore.Store,
	st *settings.Service,
	app *application.App,
	hm *hotkey.Manager,
) *EngineWrapper {
	return &EngineWrapper{
		registry:         reg,
		configStore:      store,
		settingsSvc:      st,
		settingsProvider: func() settings.Settings { return *st.Get() },
		app:              app,
		hotkeyMgr:        hm,
	}
}

// SetApp 在 app 就绪后注入。
func (w *EngineWrapper) SetApp(app *application.App) {
	w.app = app
}

// registerEngines 根据配置注册启用的翻译/OCR 引擎。
// 引擎配置从 config.db 读取（独立数据源，不混入 settings）；
// settingsProvider 仅用于构建带自定义 DNS + 代理的 HTTP 客户端。
func (w *EngineWrapper) registerEngines() {
	cfg := w.settingsProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		slog.Default().Error("加载引擎配置失败", slog.Any("error", err))
		return
	}
	engines := engine.EngineMap(configstore.EnginesToConfig(rows))

	// 设置自定义 HTTP 客户端（自定义 DNS + 代理）
	newClient := func(timeout time.Duration) *http.Client {
		c := network.BuildHTTPClient(cfg)
		c.Timeout = timeout
		return c
	}

	// system：macOS 系统翻译（/usr/bin/translate），免 Key、无需网络客户端。
	if e, ok := engines["system"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewSystem())
	}
	if e, ok := engines["google"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewGoogle(e.Endpoint, newClient(15*time.Second)))
	}
	if e, ok := engines["deepl"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewDeepL(e, newClient(15*time.Second)))
	}
	if e, ok := engines["openai"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewOpenAI(e, newClient(30*time.Second)))
	}
	if e, ok := engines["baidu"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewBaidu(e, newClient(15*time.Second)))
	}
	if e, ok := engines["tencent"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewTencent(e, newClient(15*time.Second)))
	}
	if e, ok := engines["youdao"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewYoudao(e, newClient(15*time.Second)))
	}
	if e, ok := engines["tesseract"]; ok && e.Enabled {
		w.registry.RegisterOcr(engine.NewTesseractOCR(e.Extra))
	}
	// vision：macOS 系统 Vision.framework 离线 OCR，零安装、开箱即用，darwin 下始终注册并作为默认。
	if runtime.GOOS == "darwin" {
		w.registry.RegisterOcr(engine.NewVisionOCR())
	}
}

// reregisterHotkeys 重新注册全局热键（引擎配置变更后调用）。
func (w *EngineWrapper) reregisterHotkeys() {
	if w.hotkeyMgr == nil {
		return
	}
	w.hotkeyMgr.Register()
}

// GetEngines 返回所有已注册引擎（翻译 + OCR）的 (value, 类型名, 类型) 列表，
// 顺序严格按 config.db 的 engines 表自增 id 顺序（而非代码注册目录顺序）。
func (w *EngineWrapper) GetEngines() []EngineListItem {
	metas := w.registry.AllEngines()
	metaMap := make(map[string]engine.EngineMeta, len(metas))
	for _, m := range metas {
		metaMap[m.Name] = m
	}
	ctx := context.Background()
	dbEngs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		log.Error("读取引擎列表失败，回退到注册表顺序", slog.Any("error", err))
		return registryEngineListItems(metas)
	}
	items := make([]EngineListItem, 0, len(dbEngs))
	for _, e := range dbEngs {
		m, ok := metaMap[e.Engine]
		if !ok {
			continue
		}
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// registryEngineListItems 按注册表原始顺序构造前端引擎条目（仅作失败兜底）。
func registryEngineListItems(metas []engine.EngineMeta) []EngineListItem {
	items := make([]EngineListItem, 0, len(metas))
	for _, m := range metas {
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// GetAllEngines 返回 config.db 的 engines 表中「全部」引擎及其启用状态。
// 列表数据直接来自数据库（configStore.LoadEngines），不经由 settings 副本、也不与
// 代码里的 KnownEngines 合并——引擎配置是 config.db 的独立数据源，与界面偏好 settings 无关。
// 新增/启用/删除均通过 form 与各自的 DB 操作完成。
// kind 取自 KnownEngines 目录（引擎类型由代码注册，不在数据库内）。
func (w *EngineWrapper) GetAllEngines() []AllEngineItem {
	kindMap := make(map[string]string, len(engine.KnownEngines()))
	for _, m := range engine.KnownEngines() {
		kindMap[m.Name] = string(m.Kind)
	}
	ctx := context.Background()
	engs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		slog.Default().Error("读取引擎列表失败", slog.Any("error", err))
		return nil
	}
	// 注意：此处【不再】做「库空→重置默认引擎」的兜底。
	// 该兜底只在启动编排入口 loadEngines() 中执行一次，避免在列表刷新 /
	// toggle 回拉时把用户已禁用（或校验未通过的）引擎错误地重置为启用。
	items := make([]AllEngineItem, 0, len(engs))
	for _, e := range engs {
		kind := kindMap[e.Engine]
		if kind == "" {
			kind = string(engine.KindTranslator)
		}
		items = append(items, AllEngineItem{
			ID:        e.ID,
			Value:     e.Engine,
			Name:      e.Engine,
			Kind:      kind,
			Enabled:   e.Enabled != 0,
			Supported: engine.EngineSupported(e.Engine),
		})
	}
	return items
}

// GetKnownEngines 返回「可添加」的引擎目录（来自代码 KnownEngines），
// 供设置页新增表单的「引擎类型」下拉使用。列表只渲染数据库已有项，
// 下拉项必须来自代码目录以保证 name 能被 Registry 识别、翻译可工作。
func (w *EngineWrapper) GetKnownEngines() []EngineListItem {
	metas := engine.KnownEngines()
	items := make([]EngineListItem, 0, len(metas))
	for _, m := range metas {
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// GetEngineSchema 返回指定引擎的「真实」配置字段 schema，供前端动态渲染右列表单。
func (w *EngineWrapper) GetEngineSchema(engineName string) engine.EngineSchema {
	return engine.GetEngineSchema(engineName)
}

// GetEngineConfig 按 ID 返回单个引擎的完整配置（含已持久化的 api_key/endpoint 等），
// 供设置页右列表单回填已存值。未找到返回 nil。
func (w *EngineWrapper) GetEngineConfig(id int64) *engine.EngineConfig {
	if w.configStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := w.configStore.GetEngineByID(ctx, id)
	if err != nil {
		slog.Default().Error("读取引擎配置失败", slog.Int64("id", id), slog.Any("error", err))
		return nil
	}
	return cfg
}

// AddEngine 新增单个引擎配置到 config.db，返回 DB 分配的 ID。
func (w *EngineWrapper) AddEngine(cfg *engine.EngineConfig) (int64, error) {
	if w.configStore == nil {
		return 0, errEngineStoreNotReady
	}
	if miss := engine.ValidateRequired(cfg); miss != nil {
		return 0, fmt.Errorf("缺少必填项：%s", miss.LabelKey)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := w.configStore.InsertEngineConfig(ctx, cfg)
	if err != nil {
		return 0, err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	return id, nil
}

// UpdateEngineConfig 按 ID 更新单个引擎的全部配置字段。
func (w *EngineWrapper) UpdateEngineConfig(cfg *engine.EngineConfig) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	if miss := engine.ValidateRequired(cfg); miss != nil {
		return fmt.Errorf("缺少必填项：%s", miss.LabelKey)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.configStore.UpdateEngineConfig(ctx, cfg); err != nil {
		return err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	return nil
}

// ToggleEngineEnabled 按 ID 切换单个引擎的启用/禁用状态。
// 启用（enabled=true）时校验必填参数，缺项则拒绝开启并提示。
func (w *EngineWrapper) ToggleEngineEnabled(id int64, enabled bool) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	if enabled {
		ctx0, cancel0 := context.WithTimeout(context.Background(), 5*time.Second)
		cfg, err := w.configStore.GetEngineByID(ctx0, id)
		cancel0()
		if err != nil {
			return err
		}
		if miss := engine.ValidateRequired(cfg); miss != nil {
			return fmt.Errorf("缺少必填项：%s", miss.LabelKey)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.configStore.SetEngineEnabled(ctx, id, enabled); err != nil {
		return err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	return nil
}

// RemoveEngine 按 ID 删除单个引擎配置。
func (w *EngineWrapper) RemoveEngine(id int64) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.configStore.DeleteEngineByID(ctx, id); err != nil {
		return err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	return nil
}

// loadEngines 从 config.db 加载引擎配置到 settings（作为唯一真相源），并注册到 registry。
func (w *EngineWrapper) loadEngines() error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		return err
	}

	if len(engs) == 0 {
		log.Info("config.db 引擎表为空，使用默认引擎初始化")
		def := engine.DefaultEngineConfigs()
		if err := w.configStore.InitDefaultEngines(ctx, def); err != nil {
			return err
		}
		if _, err := w.configStore.LoadEngines(ctx); err != nil {
			return err
		}
	}
	w.registerEngines()
	return nil
}

// EngineChangedPayload 引擎配置变更广播负载。
type EngineChangedPayload struct {
	ID      int64 `json:"id"`      // 发生变更的引擎 ID
	Enabled bool  `json:"enabled"` // 变更后的启用状态
}

// EngineListItem 设置页「引擎」分组：左列服务名、右列配置用。
// Kind 区分 translate / ocr，前端据此分组并展示不同标记。
type EngineListItem struct {
	Value     string `json:"value"`     // 引擎标识
	Name      string `json:"name"`      // 展示名
	Kind      string `json:"kind"`      // translate | ocr
	Supported bool   `json:"supported"` // 当前平台是否支持（如 system 仅 darwin）
}

// AllEngineItem 设置页「全部可配置服务」条目（无论是否已启用）
type AllEngineItem struct {
	ID        int64  `json:"id"`        // 引擎自增主键 ID
	Value     string `json:"value"`     // 引擎标识
	Name      string `json:"name"`      // 展示名
	Kind      string `json:"kind"`      // translate | ocr
	Enabled   bool   `json:"enabled"`   // 是否已启用
	Supported bool   `json:"supported"` // 当前平台是否支持
}
