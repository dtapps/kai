package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	genai "google.golang.org/genai"
)

// geminiTranslator 是 Google Gemini 翻译引擎，基于官方新版 Gen AI Go SDK
//（google.golang.org/genai，GA 版，替代已弃用的 github.com/google/generative-ai-go）。
// 配置：APIKey=Google AI Studio / Gemini API Key，Endpoint=API Base URL
// （默认官方地址，可在设置中以完整 Base URL 覆盖），
// Model(Extra)=模型名（如 gemini-1.5-flash、gemini-2.0-flash、gemini-2.5-flash）。
type geminiTranslator struct {
	client *genai.Client
	model  string
}

// NewGemini 由引擎配置构造 Gemini 引擎。
// 通过 ClientConfig 同时传入 APIKey 与全局 HTTPClient：新版 SDK 会正确把密钥注入到
// 自定义 HTTPClient 的请求中（旧版 SDK 在自定义 HTTPClient 下会丢失密钥，导致
// "API key is required"）。优先用 service 层注入的全局 client（带自定义 DNS/代理/日志）；
// nil 时回退自建独立 *http.Transport 的 client 兜底。
func NewGemini(cfg *EngineConfig) (*geminiTranslator, error) {
	var httpClient *http.Client
	if cfg.HTTPClient != nil {
		httpClient = cfg.HTTPClient
	} else {
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
			Timeout: 60 * time.Second,
		}
	}
	cc := &genai.ClientConfig{
		APIKey:    cfg.APIKey,
		Backend:   genai.BackendGeminiAPI,
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
	return &geminiTranslator{
		client: client,
		model:  cfg.Extra,
	}, nil
}

// Name 返回引擎标识。
func (e *geminiTranslator) Name() string { return "gemini" }

func (e *geminiTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.gemini_model_required"))
	}
	if e.client == nil {
		return "", fmt.Errorf(i18n.T("err.gemini_uninitialized"))
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
