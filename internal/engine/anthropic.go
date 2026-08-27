package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicTranslator 是 Anthropic Claude 翻译引擎，基于官方 anthropic-sdk-go 实现。
// 配置：APIKey=sk-ant-...，Endpoint=base URL（默认 https://api.anthropic.com），
// Extra=JSON（{"model":"claude-3-5-sonnet-20241022","timeout_sec":30}）；兼容旧版纯模型名字符串。
type anthropicTranslator struct {
	client  anthropic.Client
	model   anthropic.Model
	timeout time.Duration
}

// NewAnthropic 由引擎配置构造 Anthropic 引擎。
// 复用项目全局 HTTP client（由 service 层注入，带自定义 DNS/代理/日志/贡献上报），
// 与 openai/gemini 等引擎保持一致，统一走 network.BuildHTTPClient 网络策略。
func NewAnthropic(cfg *EngineConfig) *anthropicTranslator {
	ex := parseLLMExtra(cfg.Extra)
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	// 注入全局 HTTP client 并克隆为带引擎级超时的独立实例（超时同步到 HTTP 层），
	// 避免直接改共享全局 client 的 Timeout 相互影响；nil 时回退 SDK 默认 client。
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cloneHTTPClientWithTimeout(cfg.HTTPClient, ex.TimeoutSec)))
	}
	if cfg.Endpoint != "" && cfg.Endpoint != AnthropicDefaultBaseURL {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	modelName := anthropic.Model(ex.Model) // nolint:unconvert // 类型转换提供编译期类型安全
	if modelName == "" {
		modelName = "claude-3-5-sonnet-20241022"
	}
	return &anthropicTranslator{
		client:  anthropic.NewClient(opts...),
		model:   modelName,
		timeout: time.Duration(ex.TimeoutSec) * time.Second,
	}
}

// Name 返回引擎标识。
func (e *anthropicTranslator) Name() string { return "anthropic" }

func (e *anthropicTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.anthropic_model_required"))
	}
	// 引擎级请求超时（默认 30s，可由 Extra.timeout_sec 配置）。
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	system := i18n.T("engine.openai_system")
	userPrompt := i18n.T("engine.openai_prompt")
	userContent := fmt.Sprintf(userPrompt, srcName(from), dstName(to), text)

	params := anthropic.MessageNewParams{
		Model:     e.model,
		MaxTokens: int64(8192),
		System: []anthropic.TextBlockParam{
			{Text: system},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userContent)),
		},
	}

	msg, err := e.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf(i18n.T("err.anthropic_api_error"), err.Error())
	}

	var sb strings.Builder
	for _, block := range msg.Content {
		tb := block.AsText()
		if string(tb.Type) == "text" {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String(), nil
}

func (e *anthropicTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	from := string(req.From)
	to := string(req.To)
	if from == "" || from == "auto" {
		from = "auto"
	}
	result, err := e.translate(ctx, req.Text, from, to)
	if err != nil {
		return nil, err
	}
	if result == "" {
		return nil, fmt.Errorf(i18n.T("err.anthropic_empty"))
	}
	return &model.TranslateResult{
		Engine: "anthropic",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(result),
	}, nil
}
