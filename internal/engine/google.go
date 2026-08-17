package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Google 免 API Key 的公开翻译端点（网页端 gtx 接口）。
const googleEndpoint = "https://translate.googleapis.com/translate_a/single"

// googleLang 把内部语言码映射为 Google gtx 端点接受的码。
// gtx 对 "zh" 也能工作，但 "zh-CN" 更稳，故统一转换。
func googleLang(code string) string {
	switch code {
	case "zh", "zh-CN", "zh_CN":
		return "zh-CN"
	case "auto", "":
		return "auto"
	default:
		return code
	}
}

// googleTranslator Google 翻译引擎（免 Key）。
type googleTranslator struct {
	endpoint string
	client   *http.Client
}

// NewGoogle 创建 Google 翻译引擎。endpoint 为空时使用默认公开端点。
func NewGoogle(endpoint string, client *http.Client) Translator {
	if endpoint == "" {
		endpoint = googleEndpoint
	}
	return &googleTranslator{
		endpoint: endpoint,
		client:   client,
	}
}

func (g *googleTranslator) Name() string { return "google" }

// Translate 调用 Google 公开端点完成翻译。
func (g *googleTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if req.Text == "" {
		return nil, fmt.Errorf(i18n.T("err.empty_text"))
	}
	sl := googleLang(string(req.From))
	tl := googleLang(string(req.To))
	if tl == "auto" || tl == "" {
		tl = "zh-CN"
	}

	u := fmt.Sprintf("%s?client=gtx&sl=%s&tl=%s&dt=t&q=%s",
		g.endpoint, url.QueryEscape(sl), url.QueryEscape(tl), url.QueryEscape(req.Text))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(i18n.T("err.google_http"), resp.StatusCode, string(body), resp.StatusCode, string(body))
	}

	// 解析 Google gtx 响应：[[["dst","src",...],...], "detected_lang", ...]
	var gresp googleResponse
	if err := json.Unmarshal(body, &gresp); err != nil {
		return nil, fmt.Errorf(i18n.T("err.google_parse"), err, err)
	}

	if gresp.Translated == "" {
		return nil, fmt.Errorf(i18n.T("err.google_empty_result"))
	}

	return &model.TranslateResult{
		Engine: "google",
		From:   model.Language(gresp.DetectedLang),
		To:     req.To,
		Text:   req.Text,
		Result: gresp.Translated,
	}, nil
}

// googleResponse 表示 Google gtx (dt=t) 的响应。
// 根结构：[ 翻译段数组, ... , 检测源语言, ... ]
// 翻译段数组：[[dst, src, ...], ...]，其中 dst(译文)在 [0]、src(原文)在 [1]。
// 由于响应是不规则嵌套数组，用自定义 UnmarshalJSON 把位置语义收敛到具名字段。
type googleResponse struct {
	Translated   string
	DetectedLang string
}

func (r *googleResponse) UnmarshalJSON(data []byte) error {
	// 根层级：第 0 元素是翻译段数组，第 2 元素是检测源语言
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var segs []json.RawMessage
	if err := json.Unmarshal(raw[0], &segs); err != nil {
		return err
	}
	var sb strings.Builder
	for _, seg := range segs {
		// 每段格式 [dst, src, null, null, N, ...]，仅取 dst(索引0)字符串字段
		var pair []json.RawMessage
		if err := json.Unmarshal(seg, &pair); err != nil || len(pair) == 0 {
			continue
		}
		var s string
		if err := json.Unmarshal(pair[0], &s); err == nil && s != "" {
			sb.WriteString(s)
		}
	}
	r.Translated = sb.String()
	if len(raw) > 2 {
		var d string
		if err := json.Unmarshal(raw[2], &d); err == nil {
			r.DetectedLang = d
		}
	}
	return nil
}
