# 测试与覆盖率

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

本文讲本地怎么跑、CI 怎么算、当前基线是多少、红线为什么没开。CI 平台细节见 `ci/README.md`；覆盖率专题见 `codecov.md` / `coveralls.md` / `coverage-go.md` / `coverage-node.md`。

## 1. 测试分层

### 1.1 Go

当前 **26 个 `_test.go`，222 个 `TestXxx` 函数**（`find internal pkg -name '*_test.go'`）。集中在纯逻辑与存储层：

| 包                                                                                 | 测什么                                     | 当前包覆盖率（本地实测） |
|------------------------------------------------------------------------------------|--------------------------------------------|--------------------------|
| `internal/configstore`                                                             | schema、CRUD、加密/解密、迁移              | 86.0%                    |
| `internal/historystore`                                                            | 增删查、去重、迁移                         | 85.7%                    |
| `internal/httplogstore`                                                            | 写入、清理、Unicode 解码                   | 86.7%                    |
| `internal/settings`                                                                | 默认值、读写、废弃 key 清理                | 91.1%                    |
| `internal/i18n`                                                                    | locale、回退、错误信封                     | 96.8%                    |
| `internal/logutil`                                                                 | 滚动、压缩、清理、前端日志                 | 90.0%                    |
| `internal/engine`                                                                  | 语言映射、schema、HTTP 成功/错误/签名      | 53.7%                    |
| `internal/network`                                                                 | 代理、DNS、transport 解包                  | 73.3%                    |
| `internal/translate`                                                               | 编排、多引擎、历史写入                     | 65.4%                    |
| `internal/sqlite` / `useragent` / `buildinfo`                                      | 小工具                                     | 100%                     |
| `internal/analytics`                                                               | 事件名、长度分桶、开关门控                 | 55.0%                    |
| `pkg/wails-updater-providers`                                                      | GitHub/CNB stable/prerelease 判定、matcher | 50.1%                    |
| `internal/service` / `selection` / `execkey` / `model` / `webview` / `swiftbridge` | 无测试或仅平台桩                           | 0%                       |

> 覆盖率是 2026-10-27 本机 `go test -vet=off -coverprofile ... ./internal/... ./pkg/...` 的 `go tool cover -func` 结果，环境 macOS/arm64，会随代码变化。

### 1.2 前端

当前 **11 个测试文件，67 个用例**（`frontend/src/**/*.test.ts`）。覆盖纯函数、store、i18n、runtime、utils：

- `frontend/src/constants/constants.test.ts`、`frontend/src/i18n/index.test.ts`
- `frontend/src/runtime/frontendLog.test.ts`、`index.test.ts`、`platform.test.ts`
- `frontend/src/stores/persisted.test.ts`、`theme.test.ts`、`ui.test.ts`
- `frontend/src/utils/analytics.test.ts`、`events.test.ts`、`style.test.ts`

Svelte 组件（`TranslateWindow` / `ScreenshotWindow` / `Settings` / `Settings` 子 Tab）目前没有 DOM 测试，覆盖率为 0%。`vitest.config.ts` 用 `environment: 'node'`，需要 DOM 的模块在用例内自行打桩；`setupFiles` 补了 `localStorage` 与 `navigator.language`。

## 2. 本地命令

### 2.1 Go

```bash
# Makefile 目标：只跑 internal
make test-go

# 与 CI 一致：internal + pkg，关闭 vet
go test -vet=off ./internal/... ./pkg/...

# 覆盖率（与 CNB / Codecov 相同的范围）
go test -vet=off \
  -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) \
  -coverprofile=coverage.out \
  ./internal/... ./pkg/...
go tool cover -func=coverage.out | tail -1

# 单个包 / 单个用例
go test -vet=off -run TestDeepLLang ./internal/engine/ -v
```

### 2.2 前端

```bash
cd frontend
pnpm test                 # vitest run
pnpm test:coverage        # v8 覆盖率，输出 lcov + text
pnpm test -- --reporter=verbose
```

或者从仓库根：

```bash
pnpm --dir frontend test
pnpm --dir frontend test:coverage
```

`vitest.config.ts` 不 merge `vite.config.ts`（后者注入 Wails 插件，依赖构建期 bindings），只保留 `@bindings` 别名。跑测试前确保 `frontend/bindings` 存在（`make bindings`）。

## 3. 为什么 `-vet=off`

CI 与 Makefile 的 Go 测试命令都带 `-vet=off`。原因写在 `.cnb/workflows/push-coverage.yml`：

> `swiftbridge/selection/engine` 存在 `fmt.Errorf` 非常量格式串，vet 会挂。

`.golangci.yml` 也对 `govet` 的 `non-constant format string` 做了排除，并排除 `staticcheck SA1006`。这是项目约定（`i18n.T()` 返回值直接作为 format 参数），不是忽略所有 vet 检查；`golangci-lint` 仍会跑其他 linter。

## 4. Fuzz

```bash
make fuzz-go
make fuzz-go FUZZ=FuzzXxx TIME=30s
```

当前仓库**没有 `FuzzXxx` 函数**，所以 `make fuzz-go` 实际只是重复跑普通测试，不会做模糊测试。要真正 fuzz：

1. 在目标包加 `func FuzzXxx(f *testing.F) { ... }`；
2. `make fuzz-go FUZZ=FuzzXxx TIME=30s`；
3. 失败用例会写到 `testdata/fuzz/FuzzXxx/`，提交前确认是否要入库。

## 5. 当前基线

| 指标                 | 数值                 | 命令                                |
|----------------------|----------------------|-------------------------------------|
| Go 语句覆盖率        | **52.8%**            | 见 §2.1 的 `-coverpkg` 命令         |
| 前端语句覆盖率       | **14.38%**           | `pnpm --dir frontend test:coverage` |
| 前端分支 / 函数 / 行 | 9.71% / 25% / 14.38% | 同上                                |
| Go 测试文件 / 用例   | 26 / 222             | `find` + `grep`                     |
| 前端测试文件 / 用例  | 11 / 67              | `pnpm test`                         |

这些数字只用于判断趋势，不是门禁。

## 6. 覆盖率门禁现状

CNB 的 `testing:coverage` 配置在 `.cnb/workflows/{push,pull}-coverage.yml`：

- Go：`lines` / `diffLines` 都是注释状态，**红线未启用**；`breakIfNoCoverage: false`。
- 前端：`lines` / `diffLines` 也是注释状态；`breakIfNoCoverage: true`（报告缺失会失败）。
- 注释里的数值是「启用时怎么填」的示例（Go `lines: 10`，前端 `lines: 90`），不是当前生效值。

Codecov 侧 `.github/codecov.yml` 的所有 status 都是 `informational: true`，只做可视化，不阻塞 PR。Coveralls 的阈值在 Coveralls 后台配置，仓库里没有 `.coveralls.yml`。

## 7. CI 产物与消费方

| 产物                          | 生成方                  | 消费方                                                   |
|-------------------------------|-------------------------|----------------------------------------------------------|
| `coverage.out`                | Go 测试                 | Codecov（`flags: go`）、Coveralls（`format: golang`）    |
| `frontend/coverage/lcov.info` | Vitest v8               | Codecov（`flags: node`）、Coveralls（`flag-name: node`） |
| `go-test-report.xml`          | `gotestsum --junitfile` | Codecov Test Analytics                                   |
| `frontend/test-results.json`  | Vitest `json` reporter  | 经 `frontend/scripts/vitest-json-to-junit.mjs` 转 JUnit  |
| `frontend/test-report.xml`    | 上述转换脚本            | Codecov Test Analytics                                   |

当前仓库**没有 Cobertura** 产物；GitLab 覆盖率方案已删除（见 `gitlab-ci.md`）。不要照搬 `coverage-go.md` / `coverage-node.md` 里带 Cobertura 的历史方案。

## 8. 加测试的约定

- Go：测试文件与被测包同目录，包名一致；存储测试用 `t.TempDir()`，不要写用户目录；HTTP 引擎用 `httptest.Server`，不要打真实 API。
- 引擎新增/改语言映射：在 `engine_core_test.go` 加表驱动用例；改请求/响应：在 `engine_http_test.go` 加成功、API 错误、解码错误、缺凭证四类。
- 前端：纯函数用 `environment: node`；需要 store 时先 `set` 再断言；不要依赖真实 `localStorage` / `navigator`，用 `setup.ts` 的桩。
- 改事件名/统计事件名：现有稳定性测试会拦住（`internal/analytics/analytics_test.go` 的 `TestEventNamesAreStable`）。
- 改了生成代码（bindings/sqlc）不需要写测试，但要在 PR 里说明重新生成了哪些文件。
