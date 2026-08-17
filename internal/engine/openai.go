package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// OpenAI 兼容聊天接口翻译引擎（Chat Completions）。
// 配置：APIKey=sk-...，Endpoint=完整接口地址（默认 https://api.openai.com/v1/chat/completions），
// Extra=模型名（如 gpt-4o-mini）。Endpoint 即为地址，无需再单独配置 base_url。
type openaiTranslator struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAI 创建 OpenAI 兼容翻译引擎。
func NewOpenAI(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = OpenAIDefaultChatEndpoint
	}
	modelName := cfg.Extra
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}
	return &openaiTranslator{
		apiKey:   cfg.APIKey,
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
		src = srcName("auto")
	}

	prompt := fmt.Sprintf(
		i18n.T("engine.openai_prompt"),
		srcName(src), dstName(dst), req.Text,
	)

	body, err := json.Marshal(openaiChatRequest{
		Model: o.model,
		Messages: []openaiChatMessage{
			{Role: "system", Content: i18n.T("engine.openai_system")},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.openai_marshal"), err, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.openai_request"), err, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.openai_do"), err, err)
	}
	defer resp.Body.Close()

	var or openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf(i18n.T("err.openai_decode"), err, err)
	}
	if or.Error != nil && or.Error.Message != "" {
		return nil, fmt.Errorf(i18n.T("err.openai_api_error"), or.Error.Message, or.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(or.Choices) == 0 {
		return nil, fmt.Errorf(i18n.T("err.openai_api_status"), resp.Status, resp.Status)
	}

	return &model.TranslateResult{
		Engine: "openai",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(or.Choices[0].Message.Content),
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
