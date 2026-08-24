package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAI 兼容聊天接口翻译引擎（Chat Completions），基于官方 openai-go SDK。
// 配置：APIKey=sk-...，Endpoint=Base URL（默认 https://api.openai.com/v1），Extra=模型名（如 gpt-4o-mini）。
// Endpoint 填 base URL 即可（与 DeepSeek/硅基流动等兼容平台一致），SDK 自动拼 /chat/completions。
type openaiTranslator struct {
	apiKey string
	model  string
	client openai.Client
}

// normalizeOpenAIBaseURL 将用户填写的 endpoint 规范化为纯 Base URL。
// v3 SDK 的 WithBaseURL 会在底层自动追加 /chat/completions，
// 因此这里必须把用户可能填的完整 chat/completions 地址剥回 base，否则会出现重复路径。
// 兼容：填 base（.../v1）、填完整地址（.../v1/chat/completions）、或带/不带末尾斜杠。
func normalizeOpenAIBaseURL(raw string) string {
	ep := strings.TrimSpace(raw)
	if ep == "" {
		return ""
	}
	ep = strings.TrimRight(ep, "/")
	// 已含 /chat/completions 则剥掉，保留纯 base
	ep = strings.TrimSuffix(ep, "/chat/completions")
	ep = strings.TrimRight(ep, "/")
	return ep
}

// NewOpenAI 创建 OpenAI 兼容翻译引擎。
func NewOpenAI(cfg *EngineConfig, client *http.Client) Translator {
	modelName := cfg.Extra
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	// endpoint 归一化为纯 Base URL 后传给 SDK（v3 SDK 的 WithBaseURL 会自动追加 /chat/completions）。
	// 兼容两种填法：填 base（.../v1）或填完整地址（.../v1/chat/completions）都归一到底层 base，
	// 避免 SDK 再拼一次导致 /chat/completions/chat/completions 重复路径。
	if ep := normalizeOpenAIBaseURL(cfg.Endpoint); ep != "" {
		opts = append(opts, option.WithBaseURL(ep))
	}
	// 复用项目统一 http.Client（含超时、贡献日志），保证与全局网络策略一致
	if client != nil {
		opts = append(opts, option.WithHTTPClient(client))
	}

	return &openaiTranslator{
		apiKey: cfg.APIKey,
		model:  modelName,
		client: openai.NewClient(opts...),
	}
}

// Name 返回引擎标识。
func (o *openaiTranslator) Name() string { return "openai" }

func (o *openaiTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if o.apiKey == "" {
		return nil, ErrAPIKey
	}
	src := string(req.From)
	dst := string(req.To)
	if src == "" || src == "auto" {
		src = srcName("auto")
	}

	prompt := fmt.Sprintf(
		i18n.T("engine.openai_prompt"),
		srcName(src), dstName(dst), req.Text,
	)

	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(i18n.T("engine.openai_system")),
			openai.UserMessage(prompt),
		},
	}

	completion, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		// SDK 错误类型可给出状态码与 message，给出更清晰的报错
		if apiErr, ok := err.(*openai.Error); ok {
			// 410 Gone：OpenAI 官方对「已下线/废弃模型」的标准响应；但自托管兼容服务
			// （vLLM / ollama 等）也可能返回 410，语义不固定。故不再武断说「模型下线」，
			// 而是把接口真实返回的 message 透传，并提示检查 endpoint/模型是否匹配。
			if apiErr.StatusCode == http.StatusGone {
				return nil, fmt.Errorf(i18n.T("err.openai_model_gone"), o.model, apiErr.Message)
			}
			return nil, fmt.Errorf(i18n.T("err.openai_api_error"), apiErr.Message)
		}
		return nil, fmt.Errorf(i18n.T("err.openai_do"), err, err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf(i18n.T("err.openai_api_status"), "no choices")
	}

	msg := completion.Choices[0].Message
	result := strings.TrimSpace(msg.Content)
	// 模型拒绝生成内容（安全策略等）时 Content 为空但 Refusal 有值，
	// 将其作为结果返回，否则前端表现为「请求成功但无结果」。
	if result == "" && msg.Refusal != "" {
		result = strings.TrimSpace(msg.Refusal)
	}

	return &model.TranslateResult{
		Engine: "openai",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: result,
	}, nil
}

// srcName/dstName 把内部语言码转成 LLM 更易理解的自然语言名（走 i18n，随界面语言切换）。
func srcName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return i18n.T("lang.zh")
	case "en":
		return i18n.T("lang.en")
	case "ja":
		return i18n.T("lang.ja")
	case "ko":
		return i18n.T("lang.ko")
	default:
		return i18n.T("lang.auto")
	}
}

func dstName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return i18n.T("lang.zh")
	case "en":
		return i18n.T("lang.en")
	case "ja":
		return i18n.T("lang.ja")
	case "ko":
		return i18n.T("lang.ko")
	default:
		return code
	}
}
