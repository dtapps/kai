package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	genai "google.golang.org/genai"
)

// geminiTranslator 是 Google Gemini 翻译引擎，基于官方 google.golang.org/genai SDK 实现。
type geminiTranslator struct {
	client  *genai.Client
	model   string
	timeout time.Duration
}

// NewGemini 由引擎配置构造 Gemini 引擎。
// 通过 ClientConfig 同时传入 APIKey 与全局 HTTPClient：新版 SDK 会正确把密钥注入到
// 自定义 HTTPClient 的请求中（旧版 SDK 在自定义 HTTPClient 下会丢失密钥，导致
// "API key is required"）。必须注入全局 client（cfg.HTTPClient 由 service 层提供），
// 否则返回错误——不使用裸直连，避免丢失项目的 DNS/代理/日志网络策略。
func NewGemini(cfg *EngineConfig) (*geminiTranslator, error) {
	ex := parseLLMExtra(cfg.Extra)
	if cfg.HTTPClient == nil {
		return nil, fmt.Errorf(i18n.T("err.gemini_uninitialized"))
	}
	// 克隆为带引擎级超时的独立实例（超时同步到 HTTP 层），避免直接改共享全局
	// client 的 Timeout 相互影响。
	httpClient := cloneHTTPClientWithTimeout(cfg.HTTPClient, ex.TimeoutSec)

	cc := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
	}
	// 仅在用户显式配置了非默认 Base URL 时覆盖（Endpoint 存完整 Base URL）。
	if cfg.Endpoint != "" && cfg.Endpoint != GeminiDefaultEndpoint {
		cc.HTTPOptions.BaseURL = cfg.Endpoint
	}

	client, err := genai.NewClient(context.Background(), cc)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.gemini_client"), err)
	}

	model := ex.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}
	return &geminiTranslator{
		client:  client,
		model:   model,
		timeout: time.Duration(ex.TimeoutSec) * time.Second,
	}, nil
}

// Name 返回引擎标识。
func (e *geminiTranslator) Name() string { return "gemini" }

func (e *geminiTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.gemini_model_required"))
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

	contents := []*genai.Content{
		{Role: genai.RoleUser, Parts: []*genai.Part{{Text: userContent}}},
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: system}}},
		MaxOutputTokens:   8192,
	}

	resp, err := e.client.Models.GenerateContent(ctx, e.model, contents, config)
	if err != nil {
		return "", fmt.Errorf(i18n.T("err.gemini_api_error"), err.Error())
	}
	result := resp.Text()
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf(i18n.T("err.gemini_empty"))
	}
	return result, nil
}

func (e *geminiTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	from := string(req.From)
	to := string(req.To)
	if from == "" || from == "auto" {
		from = "auto"
	}
	result, err := e.translate(ctx, req.Text, from, to)
	if err != nil {
		return nil, err
	}
	return &model.TranslateResult{
		Engine: "gemini",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(result),
	}, nil
}
