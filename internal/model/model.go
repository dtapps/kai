package model

// Language 语言代码（ISO 639-1 或引擎自定义）
type Language string

// Locale 界面语言，与翻译语言 Language 是完全不同的场景，使用独立类型。
// 值是 BCP 47 区域码（zh-CN/en-US/auto），仅用于后端 i18n 与前端界面语言切换，
// 不要与翻译语言 Language（zh/en/...）混用或互转。
type Locale string

const (
	LocaleAuto Locale = "auto"  // 界面语言自动跟随系统
	LocaleZHCN Locale = "zh-CN" // 界面中文
	LocaleENUS Locale = "en-US" // 界面英文
)

// Theme 界面主题，仅用于外观切换，值为 auto/light/dark。
type Theme string

const (
	ThemeAuto  Theme = "auto"  // 跟随系统
	ThemeLight Theme = "light" // 浅色
	ThemeDark  Theme = "dark"  // 深色
)

// 翻译语言码（Translate Language）：用于翻译请求/结果、引擎调用、历史记录。
// 值是翻译引擎约定的短码（如 zh/en/ja），与界面语言完全无关，切勿与下方界面 locale 混用。
const (
	Auto Language = "auto" // 自动检测源语言
	ZH   Language = "zh"   // 中文
	EN   Language = "en"   // 英语
	JA   Language = "ja"   // 日语
	KO   Language = "ko"   // 韩语
	FR   Language = "fr"   // 法语
	DE   Language = "de"   // 德语
	ES   Language = "es"   // 西班牙语
	RU   Language = "ru"   // 俄语
)

// allLanguages 全部支持的语言（含 Auto），顺序即前端下拉展示顺序。
var allLanguages = []Language{Auto, ZH, EN, JA, KO, FR, DE, ES, RU}

// AllLanguages 返回全部支持的语言常量切片（含 Auto）。
// 供配置层派生下拉选项，避免在各处硬编码语言码列表。
func AllLanguages() []Language {
	out := make([]Language, len(allLanguages))
	copy(out, allLanguages)
	return out
}

// TranslateRequest 统一翻译请求
type TranslateRequest struct {
	Text       string   `json:"text"`   // 待翻译原文
	From       Language `json:"from"`   // 源语言（auto 为自动检测）
	To         Language `json:"to"`     // 目标语言
	EngineName string   `json:"engine"` // 指定翻译引擎标识
}

// TranslateResult 单条翻译结果
type TranslateResult struct {
	Engine   string     `json:"engine"`   // 翻译引擎标识
	From     Language   `json:"from"`     // 实际识别出的源语言
	To       Language   `json:"to"`       // 目标语言
	Text     string     `json:"text"`     // 原文
	Result   string     `json:"result"`   // 译文
	Phonetic string     `json:"phonetic"` // 发音/音标
	Dict     []DictItem `json:"dict"`     // 词典释义明细
	FromOCR  bool       `json:"from_ocr"` // 是否来自 OCR 识别结果
}

// DictItem 词典条目
type DictItem struct {
	Word    string `json:"word"`    // 单词
	Pos     string `json:"pos"`     // 词性（如 n./v.）
	Explain string `json:"explain"` // 释义
}

// OcrRequest OCR 请求
type OcrRequest struct {
	ImageData []byte `json:"-"`      // 图片二进制数据（不序列化）
	Engine    string `json:"engine"` // 指定 OCR 引擎标识
	// CorrectText Vision 的 usesLanguageCorrection（语言校正）。
	// true=开启（更准确，默认）；false=关闭（更快、偶发卡死概率更低，但准确率略降）。
	// 指针类型以便区分"未设置"与"false"，未设置时引擎用默认值 true。
	CorrectText *bool `json:"correct_text,omitempty"`
	// TimeoutSec Vision OCR 超时秒数。<=0 时引擎用各自默认值（vision 默认 60s）。
	TimeoutSec int `json:"timeout_sec,omitempty"`
	// RetryCount Vision OCR 失败兜底重试次数（针对 CRImageReaderError 类瞬拒）。
	// <=0 时引擎用默认值 2（仅 vision 语义生效）。
	RetryCount int `json:"retry_count,omitempty"`
}

// OcrResult OCR 结果
type OcrResult struct {
	Engine  string      `json:"engine"`  // OCR 引擎标识
	Text    string      `json:"text"`    // 识别出的全部文本
	Regions []OcrRegion `json:"regions"` // 各文字区域明细
}

// OcrRegion 单个文字区域
type OcrRegion struct {
	Text string  `json:"text"` // 区域文本
	Conf float64 `json:"conf"` // 识别置信度
	Box  []int   `json:"box"`  // 区域包围盒坐标 [x1,y1,x2,y2]
}

// TranslateMultiResult 多引擎并行翻译的启动确认，Count 为已启动的引擎数。
// 实际结果通过事件 EventTranslateResult 逐个流式推送到前端。
type TranslateMultiResult struct {
	Count   int               `json:"count"`   // 已启动的引擎数
	Results []TranslateResult `json:"results"` // 初始结果集合（含引擎占位）
}

// HistoryItem 翻译历史条目
type HistoryItem struct {
	ID        int64    `json:"id"`         // 历史记录自增主键 ID
	Text      string   `json:"text"`       // 原文
	Result    string   `json:"result"`     // 译文
	From      Language `json:"from"`       // 源语言
	To        Language `json:"to"`         // 目标语言
	Engine    string   `json:"engine"`     // 使用的引擎标识
	FromOCR   bool     `json:"from_ocr"`   // 是否来自 OCR 识别结果
	CreatedAt int64    `json:"created_at"` // 创建时间（毫秒时间戳）
}

// ScreenshotResult 截图翻译完整结果，通过 EventScreenshotOCR 推送到截图窗口。
type ScreenshotResult struct {
	Image        string            `json:"image"`        // 区域截图 PNG 的 base64 data URL（前端直接 <img>）
	Text         string            `json:"text"`         // OCR 识别出的原文
	Translations []TranslateResult `json:"translations"` // 各引擎译文
	To           Language          `json:"to"`           // 目标语言
	Error        string            `json:"error"`        // 流程失败原因（非空时前端停止转圈并展示错误）
}
