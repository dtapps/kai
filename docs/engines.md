# 翻译与 OCR 引擎

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

引擎分两类：`Translator`（翻译）和 `OcrEngine`（OCR）。注册表、schema、默认值集中在 `internal/engine/engine.go`；运行时注册在 `internal/service/engine_wrapper.go`。配置字段与 `config.db` 结构见 `configuration.md` §3。

## 1. 接口与注册

```go
type Translator interface {
    Name() string
    Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error)
}

type OcrEngine interface {
    Name() string
    Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error)
}
```

可选接口 `SupportsAutoSource() bool`：未实现视为支持 `from=auto`；实现并返回 `false` 的引擎必须显式源语言。当前 `apple` 显式返回 `true`（由 Swift 侧用 NaturalLanguage 检测）。

`Registry` 只是内存注册表：

```go
reg := engine.NewRegistry()
reg.RegisterTranslator(engine.NewGoogle(endpoint, client))
reg.RegisterOcr(engine.NewVisionOCR(cfg))
```

`EngineWrapper.registerEngines()` 从 `config.db` 读取已启用行，按引擎名构造实例；每次增删改启停都会重建注册表并重新注册快捷键。

## 2. 翻译引擎能力矩阵

| 引擎        | 平台   | 鉴权 / 配置                                                    | 默认端点 / Base URL                                   | 服务层 client 超时 | 说明                                                       |
|-------------|--------|----------------------------------------------------------------|-------------------------------------------------------|--------------------|------------------------------------------------------------|
| `apple`     | macOS  | 无                                                             | 系统 Translation.framework                            | —                  | 内置、免 Key；目标语言必填；走 Swift 桥接                  |
| `google`    | 全平台 | 无；可选 `endpoint`                                            | `https://translate.googleapis.com/translate_a/single` | 15s                | 公开 gtx 端点；`zh` 映射为 `zh-CN`                         |
| `deepl`     | 全平台 | `api_key` 必填；`endpoint` 可选                                | `https://api-free.deepl.com/v2/translate`             | 15s                | Pro 版改 `api.deepl.com`；源语言 `auto` 时省略 source_lang |
| `openai`    | 全平台 | `api_key` 必填；`endpoint`（Base URL）、`model`、`timeout_sec` | `https://api.openai.com/v1`                           | 30s                | SDK 自动拼 `/chat/completions`；兼容 OpenAI 协议的第三方   |
| `anthropic` | 全平台 | `api_key` 必填；`endpoint`、`model`、`timeout_sec`             | `https://api.anthropic.com`                           | 60s                | SDK 自动拼 `/v1/messages`                                  |
| `gemini`    | 全平台 | `api_key` 必填；`endpoint`、`model`、`timeout_sec`             | `https://generativelanguage.googleapis.com`           | 60s                | 基于 Google GenAI SDK；构造失败会跳过注册                  |
| `baidu`     | 全平台 | `api_key`=AppID、`secret`=密钥                                 | `https://fanyi-api.baidu.com/api/trans/vip/translate` | 15s                | MD5 签名                                                   |
| `tencent`   | 全平台 | `api_key`=SecretId、`secret`=SecretKey                         | `https://tmt.tencentcloudapi.com`                     | 15s                | TC3-HMAC-SHA256 签名                                       |
| `youdao`    | 全平台 | `api_key`=appKey、`secret`=appSecret                           | `https://openapi.youdao.com/api`                      | 15s                | SHA-256 签名 + 随机 salt                                   |

补充：

- `translate.Service.translateWithEngine` 另有一层 30s 的 context 超时；LLM 的 `timeout_sec` 会在此之上再收紧/放宽（取决于引擎实现）。
- 百度/腾讯/有道是显式源语言引擎；`from=auto` 时按各引擎映射传 `auto` 或空。
- 所有引擎都走 `network.BuildHTTPClient`，即共享代理 / 自定义 DNS / HTTP 日志 / 全局 UA（`internal/network/transport.go`）。
- Gemini 这类 SDK 必须注入 `HTTPClient`，否则会碰到被 `useragent` 包裹的全局 `http.DefaultTransport` 而 panic（`engine.EngineConfig.HTTPClient`）。

## 3. OCR 引擎

| 引擎        | 平台   | 依赖                            | 参数                                                                           |
|-------------|--------|---------------------------------|--------------------------------------------------------------------------------|
| `vision`    | macOS  | 系统 Vision.framework（免安装） | `correct_text`（默认 true）、`timeout_sec`（默认 60）、`retry_count`（默认 2） |
| `tesseract` | 全平台 | 本机 `tesseract` 可执行文件     | `endpoint`（可选二进制路径）、`langs`（`+` 拼接）、`timeout_sec`               |

- OCR 单选：同一时刻只注册一个 OCR 引擎。`registerEngines` 优先 `tesseract`，否则 `vision`；启用一个 OCR 时 `disableOtherOcrs` 会停用其它 OCR。
- `vision` 的参数存 `Extra` JSON，`engine.DefaultEngineConfigs()` 默认写入 `{"correct_text":true,"timeout_sec":60}`；`retry_count` 未设置时用默认 2，显式 0 表示关闭重试。
- tesseract 的语言候选：`chi_sim`、`chi_tra`、`eng`、`jpn`、`kor`、`fra`、`deu`、`spa`、`rus`、`por`、`ita`（`engine.OcrLangsOptions()`）。
- tesseract 路径探测顺序：`exec.LookPath("tesseract")` → `/opt/homebrew/bin` → `/usr/local/bin` → `/usr/bin`（`internal/engine/ocr_tesseract.go`）。
- 截图区域捕获：`CaptureRegion` 调 `screencapture -i -x`（macOS）；非 darwin 返回 `ErrNoScreenshot`。Windows 没有截图翻译入口。

## 4. 配置 schema

前端不写死表单，按 `GetEngineSchema(name)` 返回的字段动态渲染。`EngineFieldSchema` 的关键字段：

| 字段                                         | 含义                                                                                                               |
|----------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| `field`                                      | 目标字段：`api_key` / `secret` / `endpoint` / `extra`                                                              |
| `label_key` / `placeholder_key` / `hint_key` | i18n key                                                                                                           |
| `type`                                       | `string` / `secret`                                                                                                |
| `widget`                                     | 结构化控件：`ocr_langs` / `ocr_timeout` / `ocr_correct` / `ocr_retry` / `ocr_status` / `llm_model` / `llm_timeout` |
| `required`                                   | 启用/保存时是否必填                                                                                                |
| `default`                                    | 真实默认值（endpoint、model 等）                                                                                   |
| `options`                                    | 枚举值（如 OCR 语言码）                                                                                            |

### 4.1 必填与默认

| 引擎        | 必填字段                     | 主要默认值                                                                               |
|-------------|------------------------------|------------------------------------------------------------------------------------------|
| `apple`     | 无                           | 无                                                                                       |
| `google`    | 无                           | `endpoint=DefaultEndpoint`                                                               |
| `deepl`     | `api_key`                    | `endpoint=DeepLFreeEndpoint`                                                             |
| `openai`    | `api_key`                    | `endpoint=OpenAIDefaultBaseURL`、`model=gpt-4o-mini`、`timeout_sec=30`                   |
| `anthropic` | `api_key`                    | `endpoint=AnthropicDefaultBaseURL`、`model=claude-3-5-sonnet-20241022`、`timeout_sec=30` |
| `gemini`    | `api_key`                    | `endpoint=GeminiDefaultEndpoint`、`model=gemini-2.0-flash`、`timeout_sec=30`             |
| `baidu`     | `api_key`、`secret`          | `endpoint=BaiduDefaultEndpoint`                                                          |
| `tencent`   | `api_key`、`secret`          | `endpoint=TencentDefaultEndpoint`                                                        |
| `youdao`    | `api_key`、`secret`          | `endpoint=YoudaoDefaultEndpoint`                                                         |
| `vision`    | 无                           | `correct_text=true`、`timeout_sec=60`、`retry_count=2`                                   |
| `tesseract` | 无（但本机必须装 tesseract） | `langs=chi_sim+eng`、`timeout_sec` 默认 60                                               |

保存/启用时只校验 `Required=true` 的字段；缺项返回 `err.service_missing_field`，前端弹窗提示。

### 4.2 默认启用的引擎

`DefaultEngineConfigs()` 只预置：

- `google`（全平台）
- `apple`（仅 macOS）
- `vision`（仅 macOS）

其余引擎在设置页「可用服务」里默认关闭，由用户添加并填 Key。Windows 上 `apple` / `vision` 因 `EngineSupported` 返回 false 不会被预置。

## 5. 新增一个引擎

以新增翻译引擎 `foo` 为例：

1. **实现接口**：在 `internal/engine/foo.go` 写 `Name() string` 与 `Translate(ctx, req)`；如果需要自定义语言映射，写 `fooLang` 并加表驱动测试。
2. **注册运行时**：在 `internal/service/engine_wrapper.go` 的 `registerEngines()` 加分支，选择合适的 `newClient(timeout)`；需要自定义 HTTP client 的 SDK 记得设置 `e.HTTPClient`。
3. **声明目录**：在 `internal/engine/engine.go` 的 `KnownEngines()` 名字列表加 `foo`；OCR 引擎还要在 `KindOfEngine()` 返回 `KindOCR`。
4. **声明 schema**：在 `engineSchemas` 加 `foo` 的字段、`Required`、`Default`、`Widget`。前端会自动渲染，不需要改 Vue/Svelte 表单。
5. **平台限制**：若只在某平台可用，在 `EngineSupported()` 加判断；需要默认启用时再改 `defaultEngineNames` / `DefaultEngineConfigs()`。
6. **i18n**：在 `internal/i18n/locales/split/{zh-CN,en-US}/engine.json`（标签/占位符）与 `errors.json`（错误）加 key，然后 `make i18n-go`。
7. **测试**：`engine_core_test.go` 加语言映射/schema 存在性；`engine_http_test.go` 用 `httptest` 模拟成功、API 错误、解码错误、缺 Key。
8. **文档**：更新本文 §2、`user-guide.md` 引擎表、根 `README.md` 的引擎列表（如果对用户可见）。
9. **不需要**改 `config.db` schema：`engines` 表是通用列，新增引擎只是多一行。

新增 OCR 引擎同理，但要注意：

- `Recognize` 的输入是 `model.OcrRequest.ImageData`，输出 `OcrResult{Text, Regions}`。
- 新 OCR 会被 `disableOtherOcrs` 纳入单选逻辑；确认 `KindOfEngine` 返回 `KindOCR`。
- 截图路径只依赖 `CaptureRegion` 与默认 OCR 名，不需要改截图窗口。

## 6. 常见错误

| 现象                       | 可能原因                                    | 排查                                                           |
|----------------------------|---------------------------------------------|----------------------------------------------------------------|
| 启用时报「缺少字段」       | `Required` 字段为空                         | `GetEngineSchema` 对应字段；`ValidateRequired`                 |
| 翻译返回鉴权错误           | Key 错、端点错、账号未开通                  | `~/.kai/data/httplog.db` 看请求/响应；`kai.log` 有引擎耗时日志 |
| DeepL 报 403/456           | 免费 Key 用了 Pro 端点，或额度耗尽          | 改 `endpoint` 或检查账号                                       |
| Gemini 启动即崩            | 未注入 `HTTPClient`，SDK 触碰全局 Transport | 确认注册分支设置了 `e.HTTPClient`                              |
| OCR 报「未找到 tesseract」 | 未安装或 GUI 启动 PATH 不含 Homebrew        | 在引擎配置里填绝对路径                                         |
| Windows 截图无反应         | 平台不支持                                  | 用输入翻译或复制键；截图仅 macOS                               |
