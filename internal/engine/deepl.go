package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// DeepL 翻译引擎（需 API Key）。免费版（每月 50 万字符）同样需要注册获取 API Key，
// 端点用 api-free.deepl.com；Pro 版用 api.deepl.com。两者认证方式相同，仅端点不同。
type deeplTranslator struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewDeepL 创建 DeepL 引擎。endpoint 为空时按免费版默认。
func NewDeepL(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = DeepLFreeEndpoint
	}
	return &deeplTranslator{
		endpoint: ep,
		apiKey:   cfg.APIKey,
		client:   client,
	}
}

// Name 返回引擎标识。
func (d *deeplTranslator) Name() string { return "deepl" }

// deeplLang 把内部语言码映射为 DeepL 接受的大写码（ZH/EN/...）。
// DeepL 不支持 auto，返回空串表示让 DeepL 自动检测源语言。
func deeplLang(code string) string {
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "zh_cn":
		return "ZH"
	case "en":
		return "EN"
	case "ja":
		return "JA"
	case "ko":
		return "KO"
	case "fr":
		return "FR"
	case "de":
		return "DE"
	case "es":
		return "ES"
	case "ru":
		return "RU"
	case "auto", "":
		return "" // 自动检测
	default:
		// 已是 DeepL 风格大写码则原样返回
		return strings.ToUpper(code)
	}
}

type deeplResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
	Message string `json:"message"`
}

func (d *deeplTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if d.apiKey == "" {
		return nil, fmt.Errorf(i18n.T("err.deepl_missing_apikey"))
	}
	form := url.Values{}
	form.Set("text", req.Text)
	tl := deeplLang(string(req.To))
	if tl == "" {
		tl = "ZH"
	}
	form.Set("target_lang", tl)
	if sl := deeplLang(string(req.From)); sl != "" {
		form.Set("source_lang", sl)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_request"), err, err)
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+d.apiKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_do"), err, err)
	}
	defer resp.Body.Close()

	var dr deeplResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_decode"), err, err)
	}
	if resp.StatusCode != http.StatusOK || len(dr.Translations) == 0 {
		msg := dr.Message
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf(i18n.T("err.deepl_api_error"), msg, msg)
	}

	src := dr.Translations[0].DetectedSourceLanguage
	if src == "" {
		src = strings.ToLower(string(req.From))
	}
	return &model.TranslateResult{
		Engine: "deepl",
		From:   model.Language(strings.ToLower(src)),
		To:     req.To,
		Text:   req.Text,
		Result: dr.Translations[0].Text,
	}, nil
}
