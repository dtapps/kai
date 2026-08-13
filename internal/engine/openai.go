package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/dtapp/kai/internal/model"
)

// OpenAI 兼容聊天接口翻译引擎（Chat Completions）。
// 配置：APIKey=sk-...，Secret=base_url（如 https://api.openai.com/v1），
// Extra=模型名（如 gpt-4o-mini），Endpoint 可覆盖完整 chat/completions 路径。
type openaiTranslator struct {
	apiKey   string
	baseURL  string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAI 创建 OpenAI 兼容翻译引擎。
func NewOpenAI(cfg *EngineConfig, client *http.Client) Translator {
	base := cfg.Secret
	if base == "" {
		base = OpenAIDefaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	modelName := cfg.Extra
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}
	ep := cfg.Endpoint
	if ep == "" {
		ep = base + "/chat/completions"
	}
	return &openaiTranslator{
		apiKey:   cfg.APIKey,
		baseURL:  base,
		model:    modelName,
		endpoint: ep,
		client:   client,
	}
}

// Name 返回引擎标识。
func (o *openaiTranslator) Name() string { return "openai" }

type openaiChatRequest struct {
	Model    string              `json:"model"`
	Messages []openaiChatMessage `json:"messages"`
}

type openaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *openaiTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if o.apiKey == "" {
		return nil, ErrAPIKey
	}
	src := string(req.From)
	dst := string(req.To)
	if src == "" || src == "auto" {
		src = "自动检测"
	}

	prompt := fmt.Sprintf(
		"你是一个翻译引擎。请将下面的文本从 %s 翻译成 %s。"+
			"只输出翻译结果，不要解释，不要附加原文。\n\n%s",
		srcName(src), dstName(dst), req.Text,
	)

	body, err := json.Marshal(openaiChatRequest{
		Model: o.model,
		Messages: []openaiChatMessage{
			{Role: "system", Content: "你是一个精确的翻译引擎，只输出译文。"},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai do: %w", err)
	}
	defer resp.Body.Close()

	var or openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("openai decode: %w", err)
	}
	if or.Error != nil && or.Error.Message != "" {
		return nil, fmt.Errorf("openai error: %s", or.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(or.Choices) == 0 {
		return nil, fmt.Errorf("openai error: status %s", resp.Status)
	}

	return &model.TranslateResult{
		Engine: "openai",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(or.Choices[0].Message.Content),
	}, nil
}

// srcName/dstName 把内部语言码转成 LLM 更易理解的自然语言名。
func srcName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return "中文"
	case "en":
		return "英语"
	case "ja":
		return "日语"
	case "ko":
		return "韩语"
	default:
		return "自动检测"
	}
}

func dstName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return "中文"
	case "en":
		return "英语"
	case "ja":
		return "日语"
	case "ko":
		return "韩语"
	default:
		return code
	}
}
