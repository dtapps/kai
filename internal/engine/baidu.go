package engine

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/model"
)

// baiduTranslator 百度翻译引擎（需 appid + key）。
// 配置：APIKey=appid，Secret=密钥。
type baiduTranslator struct {
	appID    string
	appKey   string
	endpoint string
	client   *http.Client
}

// NewBaidu 创建百度翻译引擎。
func NewBaidu(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = BaiduDefaultEndpoint
	}
	return &baiduTranslator{
		appID:    cfg.APIKey,
		appKey:   cfg.Secret,
		endpoint: ep,
		client:   client,
	}
}

func (b *baiduTranslator) Name() string { return "baidu" }

func baiduLang(code string) string {
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "zh_cn":
		return "zh"
	case "en":
		return "en"
	case "ja":
		return "jp"
	case "ko":
		return "kor"
	case "fr":
		return "fra"
	case "de":
		return "de"
	case "es":
		return "spa"
	case "ru":
		return "ru"
	case "auto", "":
		return "auto"
	default:
		return strings.ToLower(code)
	}
}

type baiduResponse struct {
	From        string `json:"from"`
	To          string `json:"to"`
	TransResult []struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	} `json:"trans_result"`
	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

func (b *baiduTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if b.appID == "" || b.appKey == "" {
		return nil, ErrAPIKey
	}
	salt := fmt.Sprintf("%d", time.Now().UnixNano())
	signRaw := b.appID + req.Text + salt + b.appKey
	sum := md5.Sum([]byte(signRaw))
	sign := hex.EncodeToString(sum[:])

	form := url.Values{}
	form.Set("q", req.Text)
	form.Set("from", baiduLang(string(req.From)))
	form.Set("to", baiduLang(string(req.To)))
	form.Set("appid", b.appID)
	form.Set("salt", salt)
	form.Set("sign", sign)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("baidu request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("baidu do: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var br baiduResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("baidu decode: %w", err)
	}
	if br.ErrorCode != "" {
		return nil, fmt.Errorf("baidu error %s: %s", br.ErrorCode, br.ErrorMsg)
	}
	if len(br.TransResult) == 0 {
		return nil, fmt.Errorf("baidu empty result: %s", string(body))
	}
	src := br.From
	if src == "" {
		src = strings.ToLower(string(req.From))
	}
	return &model.TranslateResult{
		Engine: "baidu",
		From:   model.Language(src),
		To:     req.To,
		Text:   req.Text,
		Result: br.TransResult[0].Dst,
	}, nil
}
