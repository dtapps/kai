package i18n

import (
	"encoding/json"
	"errors"
	"strings"
)

// sep 是不可见的控制字符（单元分隔符 U+001F），用于在前端可解析的信封中分隔字段，
// 不会直接展示给用户（消息框/日志中均不可见）。
const sep = "\x1f"

// Error 是携带 i18n 键与命名参数的错误类型，便于前端按自身语言自行翻译（与后端语言解耦）。
//
// 实现 error 接口：Error() 返回「已翻译文本 + 信封」，其中信封含 key 与参数 JSON。
// Wails 将 Error() 序列化后传给前端，前端用 ParseError 提取 key 与参数后调用自身 i18n 翻译；
// 若前端缺少对应 key，则回退展示信封中的已翻译文本（由后端按当前语言生成）。
//
// 注意：落库（部署历史、通知）或后端日志应使用 TranslateError 取出不含信封的纯翻译文本，
// 避免把信封写入持久化数据。
type Error struct {
	Key    string
	Params map[string]any
	Err    error
}

// NewError 构造仅含 i18n key 与参数的错误（无底层错误）。
func NewError(key string, params ...any) *Error {
	return &Error{Key: key, Params: toParamMap(params)}
}

// Wrap 将底层错误与 i18n key/参数组合，Error() 会保留底层错误信息。
func Wrap(err error, key string, params ...any) *Error {
	return &Error{Key: key, Params: toParamMap(params), Err: err}
}

func toParamMap(params []any) map[string]any {
	m := make(map[string]any)
	for i := 0; i+1 < len(params); i += 2 {
		if k, ok := params[i].(string); ok {
			m[k] = params[i+1]
		}
	}
	return m
}

func flatten(m map[string]any) []any {
	out := make([]any, 0, len(m)*2)
	for k, v := range m {
		out = append(out, k, v)
	}
	return out
}

// translated 返回不含信封的已翻译文本（用于日志落库、通知等）。
func (e *Error) translated() string {
	msg := T(e.Key, flatten(e.Params)...)
	if e.Err != nil {
		return msg + ": " + e.Err.Error()
	}
	return msg
}

// Error 返回「已翻译文本 + 信封」，信封含 key 与参数 JSON，供前端解析后自行翻译。
func (e *Error) Error() string {
	payload, _ := json.Marshal(e.Params)
	return e.translated() + sep + e.Key + sep + string(payload)
}

// Unwrap 支持 errors.Is/As 与错误链遍历。
func (e *Error) Unwrap() error { return e.Err }

// TranslateError 从任意 error 中提取不含信封的已翻译文本，用于日志/落库。
// 非 *Error 类型直接返回其 Error()；nil 返回空串。
func TranslateError(err error) string {
	if err == nil {
		return ""
	}
	if ie, ok := errors.AsType[*Error](err); ok {
		return ie.translated()
	}
	return err.Error()
}

// ParseError 解析后端传来的错误信息（来自 Error()），提取 i18n key 与参数。
// 若不含信封，返回 ok=false，调用方应回退展示原始文本。
func ParseError(s string) (key string, params map[string]any, translated string, ok bool) {
	before, after, ok := strings.Cut(s, sep)
	if !ok {
		return "", nil, s, false
	}
	translated = before
	rest := after
	before0, after0, ok0 := strings.Cut(rest, sep)
	if !ok0 {
		return "", nil, s, false
	}
	key = before0
	payload := after0
	pm := map[string]any{}
	_ = json.Unmarshal([]byte(payload), &pm)
	return key, pm, translated, true
}
