package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Translator 翻译引擎统一接口
type Translator interface {
	Name() string
	Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error)
}

// autoSourceSupporter 可选接口：声明引擎是否支持「自动检测源语言」（from=auto）。
// 未实现该接口的引擎默认视为支持（云端引擎通常支持）；系统翻译（Translation.framework）
// 因 API 限制必须显式源语言，故显式返回 false。
type autoSourceSupporter interface {
	SupportsAutoSource() bool
}

// SupportsAutoSource 判断引擎是否支持自动检测源语言；未实现可选接口则视为支持。
func SupportsAutoSource(t Translator) bool {
	if s, ok := t.(autoSourceSupporter); ok {
		return s.SupportsAutoSource()
	}
	return true
}

// ValidateRequired 依据引擎的字段 schema，校验启用/保存时 Required=true 的字段是否已填写。
// 返回首个缺失的字段信息（LabelKey 供前端转译提示）；全部满足返回 nil。
// 仅校验 Required 字段；有 default 值的非必填字段即使为空也不报错。
func ValidateRequired(cfg *EngineConfig) *EngineFieldSchema {
	if cfg == nil {
		return nil
	}
	schema := GetEngineSchema(cfg.Engine)
	for i := range schema.Fields {
		f := &schema.Fields[i]
		if !f.Required {
			continue
		}
		if valueOfField(cfg, f.Field) == "" {
			return f
		}
	}
	return nil
}

// valueOfField 按 schema 的 Field 名从 EngineConfig 取对应字段值。
func valueOfField(cfg *EngineConfig, field string) string {
	switch field {
	case "api_key":
		return cfg.APIKey
	case "secret":
		return cfg.Secret
	case "extra":
		return cfg.Extra
	case "endpoint":
		return cfg.Endpoint
	default:
		return ""
	}
}

// EngineConfig 单个引擎的凭证/配置（唯一来源，settings 包复用此类型，
// 避免 engine 与 settings 互相 import 造成循环依赖）。
// ID 为 config.db 主键：0 表示未持久化（前端新增），>0 表示已有行（前端更新/删除以此为依据）。
// 引擎配置已迁移至 config.db 持久化，不再走 settings.json（故无 mapstructure 标签，仅保留 json 供前端 RPC）。
type EngineConfig struct {
	ID       int64  `json:"id"`                 // 引擎配置自增主键 ID
	Engine   string `json:"engine"`             // 引擎标识
	Enabled  bool   `json:"enabled"`            // 是否已启用
	APIKey   string `json:"api_key,omitempty"`  // API 令牌 ID（用作鉴权凭据）
	Secret   string `json:"secret,omitempty"`   // API 令牌密钥（用作签名密钥）
	Extra    string `json:"extra,omitempty"`    // 额外扩展配置（JSON 字符串）
	Endpoint string `json:"endpoint,omitempty"` // 自定义接口地址（可选）
	// HTTPClient 可选注入的全局 HTTP 客户端（带自定义 DNS/代理/日志）。
	// 由 service 层注入；Gemini 等基于 google API 的引擎必须注入，否则 SDK 会触碰
	// 被 useragent 包裹的全局 http.DefaultTransport 而 panic。nil 时引擎自建一个
	// 独立 *http.Transport 的 client 作为兜底。
	HTTPClient *http.Client `json:"-"`
}

// 引擎层公共错误（唯一来源，settings/service 复用，避免 engine 反向 import settings 造成循环依赖）
var (
	ErrAPIKey   = fmt.Errorf(i18n.T("err.no_apikey"))
	ErrNoEngine = fmt.Errorf(i18n.T("err.no_engine"))
	ErrNoOCR    = fmt.Errorf(i18n.T("err.no_ocr"))
)

// defaultEngineNames 默认内置、开箱即用的引擎（其余需用户在设置页「添加」启用）。
// system（系统翻译）/ google 均免 Key，开箱即用；macOS 上额外预置 vision（系统 OCR，
// 零安装离线），保证 mac 开箱即有可用的 OCR 引擎。
// deepl 因需要 API Key，不预置默认注入，避免生成「声明启用但缺凭证」的无效引擎；
// openai / baidu / tencent / youdao / tesseract 同样不预置，供用户在设置页添加。
var defaultEngineNames = func() map[string]bool {
	m := map[string]bool{
		"apple":  true,
		"google": true,
	}
	// 系统 OCR(vision) 仅 macOS 可用，仅在该平台作为默认引擎开箱即用。
	if runtime.GOOS == "darwin" {
		m["vision"] = true
	}
	return m
}()

// DefaultEngineConfigs 返回默认引擎配置（唯一来源）。
// 引擎清单来自 KnownEngines，不再手写；仅取 defaultEngineNames 中的内置引擎，
// 且按 schema 的 Default 预填真实默认值（如 DeepL 免费版 URL，供用户后续自行添加），
// 保证落盘默认配置即带真实值而非空。其余引擎（含 deepl）在设置页「可用服务」列表里
// 默认关闭，供用户添加并填写 Key。
func DefaultEngineConfigs() []*EngineConfig {
	metas := KnownEngines()
	out := make([]*EngineConfig, 0, len(defaultEngineNames))
	for _, m := range metas {
		// 仅内置默认引擎；其余交给用户通过 UI 添加
		if !defaultEngineNames[m.Name] {
			continue
		}
		// 当前平台不可用的引擎（如 Windows/Linux 上的 apple 系统翻译）不预置，
		// 避免默认启用一个根本不能用的引擎。用户可在对应平台支持的引擎上手动添加。
		if !EngineSupported(m.Name) {
			continue
		}
		cfg := &EngineConfig{
			Engine:  m.Name,
			Enabled: true, // 默认内置的免 Key 引擎均启用
		}
		// vision（系统 OCR）的 OCR 专属参数存于 Extra(JSON)：默认开启语言校正 + 60s 超时。
		// 注意：vision 走 macOS 系统 Vision 框架，语言自动识别，不需要 langs 字段
		// （langs 仅 tesseract 使用，两者共用 Extra 结构仅为格式统一，vision 忽略 langs）。
		if m.Name == "vision" {
			cfg.Extra = `{"correct_text":true,"timeout_sec":60}`
		}
		// 预填带 Default 的真实值字段（endpoint / 语言码等）
		for _, f := range GetEngineSchema(m.Name).Fields {
			if f.Default != "" {
				switch f.Field {
				case "endpoint":
					cfg.Endpoint = f.Default
				case "secret":
					cfg.Secret = f.Default
				case "extra":
					cfg.Extra = f.Default
				case "api_key":
					cfg.APIKey = f.Default
				}
			}
		}
		out = append(out, cfg)
	}
	return out
}

// 各引擎默认端点（唯一来源）。构造函数回退逻辑与设置页 schema 的 Default 都引用这里，
// 避免魔法字符串在 deepl.go / openai.go / schema 多处散落、彼此不一致。
const (
	// DeepLFreeEndpoint DeepL 免费版默认端点（Pro 版用户需在设置里改为 api.deepl.com）
	DeepLFreeEndpoint = "https://api-free.deepl.com/v2/translate"
	// OpenAIDefaultBaseURL OpenAI 兼容接口默认 base URL（SDK 自动拼 /chat/completions）
	OpenAIDefaultBaseURL = "https://api.openai.com/v1"
	// DefaultEndpoint Google 默认公开端点
	DefaultEndpoint = "https://translate.googleapis.com/translate_a/single"
	// BaiduDefaultEndpoint 百度翻译开放平台默认端点
	BaiduDefaultEndpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"
	// TencentDefaultEndpoint 腾讯机器翻译默认端点
	TencentDefaultEndpoint = "https://tmt.tencentcloudapi.com"
	// YoudaoDefaultEndpoint 有道智云默认端点
	YoudaoDefaultEndpoint = "https://openapi.youdao.com/api"
	// AnthropicDefaultBaseURL Anthropic Claude API 默认 base URL（SDK 内部拼 /v1/messages）
	AnthropicDefaultBaseURL = "https://api.anthropic.com"
	// GeminiDefaultEndpoint Gemini API 默认 endpoint（完整 scheme+host，SDK 内部拼 /v1beta/models/...）
	GeminiDefaultEndpoint = "https://generativelanguage.googleapis.com"
)

// OcrEngine OCR 引擎统一接口
type OcrEngine interface {
	Name() string
	Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error)
}

// Registry 引擎注册表
type Registry struct {
	translators map[string]Translator
	ocrs        map[string]OcrEngine
}

// NewRegistry 新建注册表
func NewRegistry() *Registry {
	return &Registry{
		translators: make(map[string]Translator),
		ocrs:        make(map[string]OcrEngine),
	}
}

// RegisterTranslator 注册翻译引擎
func (r *Registry) RegisterTranslator(t Translator) {
	r.translators[t.Name()] = t
}

// RegisterOcr 注册 OCR 引擎
func (r *Registry) RegisterOcr(o OcrEngine) {
	r.ocrs[o.Name()] = o
}

// GetTranslator 获取翻译引擎
func (r *Registry) GetTranslator(name string) (Translator, bool) {
	t, ok := r.translators[name]
	return t, ok
}

// GetOcr 获取 OCR 引擎
func (r *Registry) GetOcr(name string) (OcrEngine, bool) {
	o, ok := r.ocrs[name]
	return o, ok
}

// TranslatorNames 已注册翻译引擎名列表
func (r *Registry) TranslatorNames() []string {
	names := make([]string, 0, len(r.translators))
	for n := range r.translators {
		names = append(names, n)
	}
	return names
}

// OcrNames 已注册 OCR 引擎名列表
func (r *Registry) OcrNames() []string {
	names := make([]string, 0, len(r.ocrs))
	for n := range r.ocrs {
		names = append(names, n)
	}
	return names
}

// EngineKind 引擎种类：翻译 or OCR
type EngineKind string

const (
	// KindTranslator 翻译引擎
	KindTranslator EngineKind = "translate"
	// KindOCR OCR 引擎
	KindOCR EngineKind = "ocr"
)

// EngineMeta 引擎元信息（供前端设置页分组展示）
type EngineMeta struct {
	Name      string     `json:"name"`      // 引擎展示名
	Kind      EngineKind `json:"kind"`      // 引擎类型（translate | ocr）
	Supported bool       `json:"supported"` // 当前平台是否支持（如 apple 仅 darwin）
}

// EngineSupported 按当前运行平台判断引擎是否可用（导出供 service 层复用）。
// apple（macOS 系统翻译）与 vision（macOS 系统 OCR）仅 darwin 支持；其余引擎跨平台可用。
func EngineSupported(name string) bool {
	switch name {
	case "apple", "vision":
		return runtime.GOOS == "darwin"
	}
	return true
}

// engineSupported 包内别名，保持函数内调用风格一致。
func engineSupported(name string) bool { return EngineSupported(name) }

// KnownEngines 返回全部「已知」引擎（无论是否已注册/启用）的元信息。
// 用于设置页列出所有可配置的服务，让用户去启用/填写凭证，而不只显示已启用的。
// 顺序即设置页展示顺序。
func KnownEngines() []EngineMeta {
	names := []string{
		"apple",
		"google",
		"deepl",
		"openai",
		"anthropic",
		"gemini",
		"baidu",
		"tencent",
		"youdao",
		"tesseract",
		"vision",
	}
	out := make([]EngineMeta, 0, len(names))
	for _, n := range names {
		out = append(out, EngineMeta{
			Name:      n,
			Kind:      KindOfEngine(n),
			Supported: engineSupported(n),
		})
	}
	return out
}

// KindOfEngine 返回引擎种类（tesseract / vision 为 OCR，其余为翻译）。导出供 service 层复用。
// apple（macOS 系统翻译）归类为翻译。
func KindOfEngine(name string) EngineKind {
	switch name {
	case "tesseract", "vision":
		return KindOCR
	}
	return KindTranslator
}

// EngineMap 把引擎配置数组按 engine 名转成 map，供按名访问（如 EngineMap(cfg.Engines)["google"]）。
func EngineMap(engines []*EngineConfig) map[string]*EngineConfig {
	out := make(map[string]*EngineConfig, len(engines))
	for _, e := range engines {
		if e != nil {
			out[e.Engine] = e
		}
	}
	return out
}

// AllEngines 返回全部已注册引擎（翻译 + OCR）的元信息，按 kind 归类。
// 用于设置页「引擎」分组：左列服务名、右列配置，并区分翻译/OCR。
func (r *Registry) AllEngines() []EngineMeta {
	out := make([]EngineMeta, 0, len(r.translators)+len(r.ocrs))
	for n := range r.translators {
		out = append(out, EngineMeta{Name: n, Kind: KindTranslator, Supported: engineSupported(n)})
	}
	for n := range r.ocrs {
		out = append(out, EngineMeta{Name: n, Kind: KindOCR, Supported: engineSupported(n)})
	}
	return out
}

// DefaultOCREngineName 返回第一个已注册的 OCR 引擎名，供无参 OCR 调用（快捷键/前端）使用。
// 无可用 OCR 引擎时返回空串。
func (r *Registry) DefaultOCREngineName() string {
	for _, m := range r.AllEngines() {
		if m.Kind == KindOCR {
			return m.Name
		}
	}
	return ""
}

// EngineFieldType 配置字段类型（前端据此渲染不同控件）
type EngineFieldType string

const (
	// FieldString 普通文本
	FieldString EngineFieldType = "string"
	// FieldSecret 敏感（密码框）
	FieldSecret EngineFieldType = "secret"
)

// FieldWidget 控件形态覆盖。为空时按 Type 默认渲染（string→文本、secret→密码）；
// 非空时按此值渲染结构化控件，供 OCR 等引擎复用，使「配置项全部声明于 engineSchemas」。
type FieldWidget string

const (
	// WidgetOCRLangs OCR 识别语言多选（候选项取 OcrLangsOptions()），值以 "+" 拼写入 Extra.langs
	WidgetOCRLangs FieldWidget = "ocr_langs"
	// WidgetOCRTimeout OCR 超时（秒），正整数，写入 Extra.timeout_sec
	WidgetOCRTimeout FieldWidget = "ocr_timeout"
	// WidgetOCRCorrect OCR 语言校正开关，写入 Extra.correct_text（仅 vision 语义生效）
	WidgetOCRCorrect FieldWidget = "ocr_correct"
	// WidgetOCRRetry Vision OCR 失败兜底重试次数，正整数，写入 Extra.retry_count（仅 vision 语义生效）
	WidgetOCRRetry FieldWidget = "ocr_retry"
	// WidgetOCRStatus tesseract 安装状态探测卡（含可编辑自定义二进制路径 endpoint），仅 tesseract
	WidgetOCRStatus FieldWidget = "ocr_status"
	// WidgetLLMModel LLM 翻译引擎的模型名，独立文本输入，值合并写入 Extra.model
	WidgetLLMModel FieldWidget = "llm_model"
	// WidgetLLMTimeout LLM 翻译引擎的单次请求超时（秒），数字输入，值合并写入 Extra.timeout_sec
	WidgetLLMTimeout FieldWidget = "llm_timeout"
)

// EngineFieldSchema 描述某个引擎所需的单个配置字段。
// Field 对应 EngineConfig 的真实字段名（api_key / secret / endpoint / extra），
// 这样前端渲染时直接读写对应字段，而不是对所有引擎套用同一套万能表单。
type EngineFieldSchema struct {
	Field          string          `json:"field"`           // 目标字段：api_key / secret / endpoint / extra
	LabelKey       string          `json:"label_key"`       // i18n key（前端取 settings.engine_field.<name>）
	PlaceholderKey string          `json:"placeholder_key"` // i18n key（前端取 settings.engine_ph.<name>，可为空）
	Type           EngineFieldType `json:"type"`            // string / secret
	Widget         FieldWidget     `json:"widget"`          // 结构化控件覆盖（ocr_*），空则按 Type 渲染
	Required       bool            `json:"required"`        // 启用该引擎时是否必填
	Default        string          `json:"default"`         // 可选 URL/地址等字段的真实默认值，前端在字段为空时预填展示
	Options        []string        `json:"options"`         // 可选枚举值（如 tesseract 语言码 chi_sim/eng）。非空时前端渲染为多选，值以 "+" 拼接写入对应字段
	HintKey        string          `json:"hint_key"`        // 可选：字段下方的说明文案 i18n key
}

// EngineSchema 某个引擎的全部配置字段（顺序即渲染顺序）。
type EngineSchema struct {
	// Kind 引擎类型（translate 翻译 / ocr OCR），单一事实源；前端据此区分渲染，
	// 取代此前依赖 AllEngineItem.kind 的散落判断。
	Kind EngineKind `json:"kind"`
	// Builtin 是否为系统内置引擎（如 apple 系统翻译 / vision 系统 OCR），
	// 无需配置、不可移除。前端据此项统一渲染「系统内置」状态卡，取代逐引擎 hardcode 判断。
	Builtin bool `json:"builtin"`
	// Fields 该引擎在前端配置表单中渲染的字段列表（顺序即渲染顺序）。
	// 每一项对应一个输入框（endpoint / api_key / secret / 语言等），前端据此动态生成表单。
	Fields []EngineFieldSchema `json:"fields"`
}

// engineSchemas 中心化定义每个引擎「真实」需要的字段。前端按此动态渲染，
// 从而避免对所有引擎套用同一套万能表单（之前那样会显示一堆用不上的乱配字段，
// 例如 tesseract 也显示 API Key、openai 的 secret/extra 语义不清）。
//
// 依据各引擎 NewXxx 构造函数的实际取值：
//   - apple/google：公开端点免 Key（google 可在设置中自定义 endpoint，留空用默认）
//   - deepl：endpoint 默认免费版端点（可改为 Pro 版）；api_key 必填（免费版也需注册获取）
//   - openai：api_key + endpoint(作为 Base URL，默认 https://api.openai.com/v1，自动拼 /chat/completions) + extra(作为 model)
//   - baidu/tencent/youdao：appkey/appid = api_key，密钥 = secret
//   - tesseract：endpoint(可选 tesseract 二进制路径)；语言码/超时等 OCR 专属参数统一存于 Extra(JSON)
var engineSchemas = map[string]EngineSchema{
	// apple 为 macOS 系统内置翻译引擎（Translation.framework），无需配置、不可移除。
	"apple": {
		Kind:    KindTranslator,
		Builtin: true,
		Fields:  nil,
	},
	// vision 为 macOS 系统内置 OCR 引擎（Vision.framework），无需配置、不可移除。
	// 其 OCR 参数（语言校正 / 超时）统一声明于此，前端按 schema 顺序渲染，与 tesseract 同源。
	"vision": {
		Kind:    KindOCR,
		Builtin: true,
		Fields: []EngineFieldSchema{
			{
				Field:    "extra",
				Widget:   WidgetOCRCorrect,
				LabelKey: "settings.engineOcrCorrect",
				HintKey:  "settings.engineOcrCorrectDesc",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRTimeout,
				LabelKey: "settings.engineOcrTimeout",
				HintKey:  "settings.engineOcrTimeoutDesc",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRRetry,
				LabelKey: "settings.engineOcrRetry",
				HintKey:  "settings.engineOcrRetryDesc",
			},
		},
	},
	"google": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field: "endpoint",

				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.google_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        DefaultEndpoint,
			},
		},
	},
	"deepl": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.deepl_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        DeepLFreeEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.deepl_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"openai": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.openai_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        OpenAIDefaultBaseURL,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.openai_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.openai_model",
				Type:           FieldString,
				Required:       false,
				Default:        "gpt-4o-mini",
			},
			{
				Field:          "llm_timeout",
				Widget:         WidgetLLMTimeout,
				LabelKey:       "settings.engine_field.timeout",
				PlaceholderKey: "settings.engine_ph.llm_timeout",
				HintKey:        "settings.engine_hint.llm_timeout",
				Type:           FieldString,
				Required:       false,
				Default:        "30",
			},
		},
	},
	"baidu": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.baidu_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        BaiduDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.app_id",
				PlaceholderKey: "settings.engine_ph.baidu_app_id",
				Type:           FieldString,
				Required:       true,
			},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.app_secret",
				PlaceholderKey: "settings.engine_ph.baidu_app_secret",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"tencent": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.tencent_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        TencentDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.secret_id",
				PlaceholderKey: "settings.engine_ph.tencent_secret_id",
				Type:           FieldString,
				Required:       true,
			},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.secret_key",
				PlaceholderKey: "settings.engine_ph.tencent_secret_key",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"youdao": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.youdao_endpoint",
				Type:           FieldString, Required: false, Default: YoudaoDefaultEndpoint},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.app_key",
				PlaceholderKey: "settings.engine_ph.youdao_app_key",
				Type:           FieldString, Required: true},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.app_secret",
				PlaceholderKey: "settings.engine_ph.youdao_app_secret",
				Type:           FieldSecret, Required: true},
		},
	},
	"anthropic": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.anthropic_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        AnthropicDefaultBaseURL,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.anthropic_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.anthropic_model",
				Type:           FieldString,
				Required:       false,
				Default:        "claude-3-5-sonnet-20241022",
			},
			{
				Field:          "llm_timeout",
				Widget:         WidgetLLMTimeout,
				LabelKey:       "settings.engine_field.timeout",
				PlaceholderKey: "settings.engine_ph.llm_timeout",
				HintKey:        "settings.engine_hint.llm_timeout",
				Type:           FieldString,
				Required:       false,
				Default:        "30",
			},
		},
	},
	"gemini": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.gemini_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        GeminiDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.gemini_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.gemini_model",
				Type:           FieldString,
				Required:       false,
				Default:        "gemini-2.0-flash",
			},
			{
				Field:          "llm_timeout",
				Widget:         WidgetLLMTimeout,
				LabelKey:       "settings.engine_field.timeout",
				PlaceholderKey: "settings.engine_ph.llm_timeout",
				HintKey:        "settings.engine_hint.llm_timeout",
				Type:           FieldString,
				Required:       false,
				Default:        "30",
			},
		},
	},
	"tesseract": {
		Kind: KindOCR,
		// 配置项全部声明于此，顺序即前端渲染顺序（单一事实源）：
		//   1) ocr_status  安装状态探测卡（含可编辑自定义二进制路径 endpoint）
		//   2) ocr_langs   识别语言多选（写入 Extra.langs）
		//   3) ocr_timeout OCR 超时（写入 Extra.timeout_sec）
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				Widget:         WidgetOCRStatus,
				LabelKey:       "settings.engine_field.binary",
				PlaceholderKey: "settings.engine_ph.tesseract_binary",
				Type:           FieldString,
				Required:       false,
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRLangs,
				LabelKey: "settings.engineOcrLangs",
				HintKey:  "settings.engineLangsHintTesseract",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRTimeout,
				LabelKey: "settings.engineOcrTimeout",
				HintKey:  "settings.engineOcrTimeoutDesc",
			},
		},
	},
}

// ocrLangsOptions OCR 引擎（vision / tesseract）语言码候选项，
// 供前端 OCR 专属 UI 渲染 langs 多选。与 schema 解耦，由代码唯一维护。
var ocrLangsOptions = []string{"chi_sim", "chi_tra", "eng", "jpn", "kor", "fra", "deu", "spa", "rus", "por", "ita"}

// OcrLangsOptions 返回 OCR 引擎语言码候选项（供前端渲染 langs 多选）。
func OcrLangsOptions() []string {
	out := make([]string, len(ocrLangsOptions))
	copy(out, ocrLangsOptions)
	return out
}

// GetEngineSchema 返回指定引擎的配置字段 schema；未定义时返回空 schema。
func GetEngineSchema(name string) EngineSchema {
	if s, ok := engineSchemas[name]; ok {
		return s
	}
	return EngineSchema{}
}

// ocrExtra OCR 引擎 Extra 的统一 JSON 解析结果。
// 所有 OCR 引擎（vision / tesseract）共用同一 Extra(JSON) 结构，保证「统一改 JSON」。
//   - langs:      语言码，如 "chi_sim+eng"（"+" 分隔）。
//   - timeoutSec: OCR 超时秒数；<=0 回落默认 60。
//   - correct:    是否开启语言校正；仅 vision 语义生效，tesseract 忽略。nil/true=开启。
//   - retryCount: Vision OCR 失败兜底重试次数（仅 vision 语义生效，tesseract 忽略）。
//     该值为「额外重试次数」，不含首次尝试；<=0（或字段缺省）回落默认 2。显式 0 表示关闭重试。
type ocrExtra struct {
	Langs      string `json:"langs"`        // 语言码（+ 分隔），缺省回落默认
	TimeoutSec int    `json:"timeout_sec"`  // OCR 超时秒数，<=0 用默认 60
	Correct    *bool  `json:"correct_text"` // 语言校正（nil/true=开启），仅 vision 生效
	RetryCount *int   `json:"retry_count"`  // Vision OCR 失败兜底「额外重试」次数（不含首次）；nil=用默认 2，显式 0=关闭
}

// DefaultOCRLangs 各 OCR 引擎的语言码默认（无 Extra 或 Extra 不含 langs 时回落）。
var DefaultOCRLangs = map[string]string{
	"vision":    "chi_sim+eng",
	"tesseract": "chi_sim+eng",
}

// DefaultOCRTimeoutSec OCR 超时默认值（秒）。
const DefaultOCRTimeoutSec = 60

// DefaultOCRRetryCount Vision OCR 失败兜底重试次数默认值。
const DefaultOCRRetryCount = 2

// parseOCRExtra 统一解析 OCR 引擎 Extra(JSON)。两引擎共用，保证 extra 格式一致。
// 兼容旧数据：Extra 为纯字符串语言码（非 JSON）时，整串作为 langs 兜底，超时回落默认。
// timeoutSec/correct 允许由请求 req 显式覆盖（在各自 Recognize 内处理）。
func parseOCRExtra(engineName, extra string) ocrExtra {
	out := ocrExtra{
		Langs:      DefaultOCRLangs[engineName],
		TimeoutSec: DefaultOCRTimeoutSec,
	}
	if extra == "" {
		return out
	}
	// 优先按 JSON 解析（统一方案）。
	var je ocrExtra
	if err := json.Unmarshal([]byte(extra), &je); err == nil {
		if je.Langs != "" {
			out.Langs = je.Langs
		}
		if je.TimeoutSec > 0 {
			out.TimeoutSec = je.TimeoutSec
		}
		if je.Correct != nil {
			out.Correct = je.Correct
		}
		if je.RetryCount != nil {
			out.RetryCount = je.RetryCount
		}
		return out
	}
	// 兼容旧纯字符串语言码（如 "chi_sim+eng"）。
	out.Langs = extra
	return out
}

// DefaultLLMTimeoutSec LLM 翻译引擎请求超时默认值（秒）。
const DefaultLLMTimeoutSec = 30

// cloneHTTPClientWithTimeout 基于 base 克隆一个独立 *http.Client 并设置单次请求超时。
// 共享的全局 client（network.BuildHTTPClient 返回）不可直接改 Timeout（会相互影响），
// 故克隆独立实例，使「引擎级超时」同时作用到 HTTP 层（Transport 复用，避免重复建连）。
// timeoutSec<=0 时回落 DefaultLLMTimeoutSec；base 为 nil 时自建基础 client 兜底。
func cloneHTTPClientWithTimeout(base *http.Client, timeoutSec int) *http.Client {
	d := time.Duration(timeoutSec) * time.Second
	if d <= 0 {
		d = DefaultLLMTimeoutSec * time.Second
	}
	if base == nil {
		base = &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		}
	}
	return &http.Client{
		Transport:     base.Transport,
		Timeout:       d,
		CheckRedirect: base.CheckRedirect,
	}
}

// llmExtra LLM 翻译引擎（openai / anthropic / gemini）Extra 的统一 JSON 解析结果。
// 所有 LLM 引擎共用同一 Extra(JSON) 结构，保证「统一改 JSON」。
type llmExtra struct {
	Model      string `json:"model"`       // 模型名（如 gpt-4o-mini、claude-3-5-sonnet-20241022、gemini-2.0-flash）
	TimeoutSec int    `json:"timeout_sec"` // 单次请求超时（秒），<=0 回落 DefaultLLMTimeoutSec
}

// parseLLMExtra 统一解析 LLM 引擎 Extra(JSON)。
// 兼容旧数据：Extra 为纯模型名字符串（非 JSON）时，整串作为 model 兜底，超时回落默认。
func parseLLMExtra(extra string) llmExtra {
	out := llmExtra{TimeoutSec: DefaultLLMTimeoutSec}
	if extra == "" {
		return out
	}
	// 优先按 JSON 解析（统一方案）。
	var je llmExtra
	if err := json.Unmarshal([]byte(extra), &je); err == nil {
		if je.Model != "" {
			out.Model = je.Model
		}
		if je.TimeoutSec > 0 {
			out.TimeoutSec = je.TimeoutSec
		}
		return out
	}
	// 兼容旧纯字符串模型名（如 "gpt-4o-mini"）。
	out.Model = extra
	return out
}
