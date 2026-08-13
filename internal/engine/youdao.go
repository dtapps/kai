package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/model"
)

// youdaoTranslator 有道智云翻译引擎（需 appKey + appSecret）。
// 配置：APIKey=appKey，Secret=appSecret。
type youdaoTranslator struct {
	appKey   string
	appSec   string
	endpoint string
	client   *http.Client
}

// NewYoudao 创建有道翻译引擎。
func NewYoudao(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = "https://openapi.youdao.com/api"
	}
	return &youdaoTranslator{
		appKey:   cfg.APIKey,
		appSec:   cfg.Secret,
		endpoint: ep,
		client:   client,
	}
}

func (y *youdaoTranslator) Name() string { return "youdao" }

func youdaoLang(code string) string {
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "zh_cn":
		return "zh-CHS"
	case "en":
		return "en"
	case "ja":
		return "ja"
	case "ko":
		return "ko"
	case "fr":
		return "fr"
	case "de":
		return "de"
	case "es":
		return "es"
	case "ru":
		return "ru"
	case "auto", "":
		return "auto"
	default:
		return strings.ToLower(code)
	}
}

type youdaoResponse struct {
	ErrorCode   string   `json:"errorCode"`
	Query       string   `json:"query"`
	Translation []string `json:"translation"`
	L           string   `json:"l"`
}

func (y *youdaoTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if y.appKey == "" || y.appSec == "" {
		return nil, ErrAPIKey
	}
	salt := strconv.Itoa(rand.Intn(1<<31) + 1)
	curtime := strconv.FormatInt(time.Now().Unix(), 10)
	q := req.Text
	// 输入超过 20 字符时截断（有道要求 input=q 前 10 + 后 10）
	input := q
	if len([]rune(q)) > 20 {
		r := []rune(q)
		input = string(r[:10]) + string(r[len(r)-10:])
	}
	signRaw := y.appKey + input + salt + curtime + y.appSec
	sum := sha256.Sum256([]byte(signRaw))
	sign := hex.EncodeToString(sum[:])

	form := url.Values{}
	form.Set("q", q)
	form.Set("from", youdaoLang(string(req.From)))
	form.Set("to", youdaoLang(string(req.To)))
	form.Set("appKey", y.appKey)
	form.Set("salt", salt)
	form.Set("sign", sign)
	form.Set("signType", "v3")
	form.Set("curtime", curtime)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, y.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("youdao request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := y.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("youdao do: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var yr youdaoResponse
	if err := json.Unmarshal(body, &yr); err != nil {
		return nil, fmt.Errorf("youdao decode: %w", err)
	}
	if yr.ErrorCode != "0" {
		return nil, fmt.Errorf("youdao error %s", yr.ErrorCode)
	}
	if len(yr.Translation) == 0 {
		return nil, fmt.Errorf("youdao empty result")
	}
	src := yr.L
	if src == "" {
		src = strings.ToLower(string(req.From))
	}
	return &model.TranslateResult{
		Engine: "youdao",
		From:   model.Language(src),
		To:     req.To,
		Text:   req.Text,
		Result: yr.Translation[0],
	}, nil
}
