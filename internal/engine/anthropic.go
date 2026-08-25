package engine

import (
	"context"
	"fmt"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicTranslator 是 Anthropic Claude 翻译引擎，基于官方 anthropic-sdk-go 实现。
// 配置：APIKey=sk-ant-...，Endpoint=base URL（默认 https://api.anthropic.com），
// Model(Extra)=模型名（如 claude-3-5-sonnet-20241022）。
type anthropicTranslator struct {
	client anthropic.Client
	model  string
}

// NewAnthropic 由引擎配置构造 Anthropic 引擎。
func NewAnthropic(cfg *EngineConfig) *anthropicTranslator {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.Endpoint != "" && cfg.Endpoint != AnthropicDefaultBaseURL {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	return &anthropicTranslator{
		client: anthropic.NewClient(opts...),
		model:  cfg.Extra,
	}
}

// Name 返回引擎标识。
func (e *anthropicTranslator) Name() string { return "anthropic" }

func (e *anthropicTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.anthropic_model_required"))
	}

	system := i18n.T("engine.openai_system")
	userPrompt := i18n.T("engine.openai_prompt")
	userContent := fmt.Sprintf(userPrompt, srcName(from), dstName(to), text)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(e.model),
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
