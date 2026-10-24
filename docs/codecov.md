# Codecov 接入说明（kai）

本仓库的覆盖率 / 打包体积 / 测试结果统一上报到 **Codecov**，由 GitHub Actions 工作流驱动（与 CNB 的硬性红线门禁相互独立）。

## 1. 总览：三个支柱 + 职责分离

| 支柱                     | 内容            | 本仓库实现                                                                                                   | Codecov 里看什么                      |
|--------------------------|-----------------|--------------------------------------------------------------------------------------------------------------|---------------------------------------|
| 覆盖率 Coverage          | 代码行覆盖率    | `codecov/codecov-action` 上传 Go `coverage.out` 与前端 `lcov.info`                                           | Coverage 面板、PR 评论、patch 状态    |
| 打包体积 Bundle Analysis | 前端 chunk 体积 | `@codecov/vite-plugin`（构建时上传）                                                                         | Bundle 面板（体积趋势 / 回归）        |
| 测试结果 Test Analytics  | 单测运行结果    | Go：`gotestsum --junitfile`；Svelte：Vitest `json` → 脚本转 JUnit；合并由 `codecov/test-results-action` 上传 | Tests 面板（时长 / flaky / 失败分析） |

> **职责分离（重要）**：CNB 流水线（`testing:coverage`）负责**硬性红线门禁**（`lines` / `diffLines`），会阻断低覆盖率合并；GitHub Actions 只负责**把数据上报到 Codecov** 做可视化（PR 评论、徽章、patch 状态）。因此 `.github/codecov.yml` 里的 `status` 全部设为 `informational`，避免与 CNB 双重门禁。
>
> Codecov 的上传步骤**不放在 CNB**（CNB 是 CNB，GitHub 是 GitHub）——覆盖率产物在 GitHub Actions 环境内生成并上传。

## 2. 涉及文件清单

| 文件                                        | 作用                                                                                         |
|---------------------------------------------|----------------------------------------------------------------------------------------------|
| `.github/codecov.yml`                       | Codecov 规则：忽略生成代码、`flags`（go / node）、`status: informational`                    |
| `.github/workflows/coverage.yml`            | GitHub Actions 流水线：装依赖 → 跑 Go/前端测试 → 构建前端 → 上传覆盖率 / 测试结果 / 打包体积 |
| `frontend/vite.config.ts`                   | 条件加载 `@codecov/vite-plugin`（仅当 `CODECOV_TOKEN` 存在时实例化）                         |
| `frontend/scripts/vitest-json-to-junit.mjs` | 零依赖脚本：Vitest JSON → JUnit XML                                                          |
| `frontend/.npmrc`                           | pnpm 设置：放行 `@codecov/vite-plugin` 对 Vite 8 的 peer（见第 3.2 节）                      |
| `frontend/package.json`                     | `devDependencies`：`@codecov/vite-plugin`、`@vitest/...` 等                                  |

## 3. 各功能详解

### 3.1 覆盖率（Coverage）

- Go：复用 CNB 的同款口径

  ```
  gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...
  ```
  - 范围 `./internal/... ./pkg/...`：含全部测试（`internal/configstore` + `pkg/wails-updater-providers`）；不含根 main 包——它 `//go:embed all:frontend/dist`，无前端产物会编译失败。
  - `-vet=off`：`swiftbridge`/`selection`/`engine` 存在 `fmt.Errorf` 非常量格式串，vet 会挂。
  - 本项目无 ent 生成代码，无需排除。
  - `gotestsum` 只是 `go test` 的包裹，参数原样透传；`coverage.out` 与 `go-test-report.xml` 同时产出（`go-test-report.xml` 供下方 Tests 面板使用）。
- 前端：`pnpm test:coverage`（Vitest 3.2.7 覆盖率，产出 `frontend/coverage/lcov.info`）。
- 上传：`codecov/codecov-action@v5`，分两步分别带 `flags: go` / `flags: node`。

### 3.2 打包体积（Bundle Analysis）

- 插件 `@codecov/vite-plugin@2.0.1` 在 `frontend/vite.config.ts` 中**条件加载**：

  ```ts
  ...(process.env.CODECOV_TOKEN
    ? [codecovVitePlugin({ enableBundleAnalysis: true, bundleName: "kai-frontend", uploadToken: process.env.CODECOV_TOKEN })]
    : [])
  ```
- 上传发生在 `pnpm build`（= `vite build --mode production`）这一步（coverage.yml 的「构建前端」步骤）。
- ⚠️ **兼容性注意**：该插件 peer 仅声明支持 Vite `4/5/6`，而本仓库为 **Vite 8.3.4**。因此特意用「条件数组」而非无条件 `import`——CNB 生产构建与本地构建都**没有** `CODECOV_TOKEN`，插件根本不会实例化，绝不影响主构建。若日后插件更新支持 Vite 8，逻辑无需改动。
- 构建步骤设了 `continue-on-error: true`：万一插件与 Vite 8 不兼容导致 `vite build` 崩溃，也**不会拖垮**上方的覆盖率 / 测试结果上传。

### 3.3 测试结果（Test Analytics）

Go 与 Svelte 都不原生产出 JUnit，分别用不同方式转换后合并上传到同一个 `codecov/test-results-action@v1`（其 `files` 已合并两份报告：`frontend/test-report.xml,./go-test-report.xml`）：

- **Go**：`go test` 不原生产 JUnit，用 `gotestsum` 包裹（参数原样透传，`coverage.out` 照常产出），额外生成 `go-test-report.xml`：

  ```
  go install gotest.tools/gotestsum@v1.13.0
  export PATH="$(go env GOPATH)/bin:$PATH"
  gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...
  ```
  - ⚠️ `gotestsum` 必须固定为 **v1.13.0**：v1.12.0 依赖的旧版 `golang.org/x/tools@v0.19.0` 无法在 Go 1.27 上编译（`invalid array length`），v1.13.0 升到 x/tools v0.36.0 后正常。
- **Svelte（Vitest 3.2.7）**：无内置 junit reporter（社区 `@vitest/junit` 包在 npm 也已不可得），改用内置 `json` reporter 产出 `test-results.json`，再由零依赖脚本转 JUnit：
  1. 测试步骤额外挂内置 `json` reporter 产出 `test-results.json`：

     ```
     pnpm test:coverage --reporter=default --reporter=json --outputFile=test-results.json
     ```

     （`coverage/lcov.info` 仍正常产出，不受影响。）
  2. 转换步骤用 `node scripts/vitest-json-to-junit.mjs` 把 JSON 转成 `test-report.xml`（JUnit 格式）。
     - ⚠️ 脚本必须把 `classname` 归一化成**相对仓库根 + `/` 分隔**。Codecov Test Analytics 只解析 JUnit，且**只用 `classname` 定位用例归属文件**（文件分组、跨 run 关联 / flaky 判定、失败跳转都靠它）；Vitest `json` reporter 的 `testResults[].name` 是**绝对路径**，原样写入就会变成 `/__w/<repo>/<repo>/frontend/src/...`（GitHub Actions 容器）、`/home/runner/work/<repo>/<repo>/...`（HA 非容器）、`/builds/<group>/<repo>/...`（GitLab）——同名用例在不同 CI 变成不同记录，flaky 关联与跳转全失效。
     - 仓库根用**脚本自身位置**推（`path.resolve(dirname(fileURLToPath(import.meta.url)), '../..')`），**不能用 `process.cwd()`**：CI 里该步骤常设 `working-directory: frontend`，cwd 不是仓库根，会算出 `../src/...`。
     - 归一化分**两层**，结果与绝对前缀是什么无关：
       1. 常规：JSON 与脚本在同一份 checkout 树内，`path.relative(repoRoot, abs)` 直接相减，`/__w`、`/home/runner/work`、`/workspace`、`/builds` 全覆盖。
       2. 兜底：路径不在这棵树里（测试在容器内跑、转换在宿主跑；或本机复现别的 CI 产出的 JSON）。从尾部取最长的「在仓库里真实存在」的后缀；最长为 1 段时放弃——避免误命中仓库根的同名单文件（`package.json` 之类）。取不到就**原样返回**，不编造路径。
     - 兜底探测**必须用 `statSync().isFile()`，不能用 `existsSync()`**：`frontend/src` 这类同名目录会被 `existsSync` 命中，写出一个指向目录的 `classname`。
     - 也别指望 `codecov.yml` 的 `fixes:` 兜底：它属于覆盖率侧路径修补，官方未声明对 Test Analytics 生效；即便生效也只能写死某一个 CI 的前缀，换 runner（容器 ↔ 裸 runner）就失效——这正是同一仓库出现两条记录的原因。
     - 对比：`gotestsum`（Go）的 `classname` 是包导入路径（`cnb.cool/dtapp/kai/internal/...`），天然相对、跨环境一致，无需处理。
  3. 上传步骤用 `codecov/test-results-action@v1` 合并上传 `frontend/test-report.xml,./go-test-report.xml`。
- 最终 Codecov 的 **Tests** 面板同时包含 Go 与 Svelte 的测试结果。

### 3.4 组件（Components）

`.github/codecov.yml` 的 `component_management` 按**职责域**把覆盖率拆成 6 个组件，在 Codecov **Components** 面板各自看趋势 / 设门槛。`name` 为中文展示名，`component_id` 为 ASCII slug：

| component_id       | 展示名     | 覆盖路径                                                                                                                           |
|--------------------|------------|------------------------------------------------------------------------------------------------------------------------------------|
| `backend-core`     | 后端核心   | `internal/service` `internal/engine` `internal/selection` `internal/hotkey` `internal/singleinstance`                              |
| `backend-data`     | 数据与存储 | `internal/configstore` `internal/historystore` `internal/httplogstore` `internal/sqlite` `internal/settings`                       |
| `backend-io`       | 翻译与网络 | `internal/translate` `internal/network` `internal/useragent`                                                                       |
| `backend-platform` | 平台桥接   | `internal/webview` `pkg/swiftbridge` `pkg/wails-updater-providers`                                                                 |
| `backend-infra`    | 基础设施   | `internal/analytics` `internal/buildinfo` `internal/events` `internal/execkey` `internal/i18n` `internal/logutil` `internal/model` |
| `frontend`         | 前端界面   | `frontend/src/**`                                                                                                                  |

拆法说明：按职责域而非每个文件夹拆——2 个组件太粗（看不出具体哪块退化），每个目录一个又太碎（数据稀疏、面板难读），6 个职责域的粒度信息量与可读性平衡最好。组件状态检查沿用 `informational`（不阻塞 PR），`if_ci_failed: false` 表示某次 CI 没上报时该组件不因缺失而变红。

## 4. 必需的密钥

- `CODECOV_TOKEN`：在 GitHub 仓库 **Settings → Secrets → Actions** 中配置（Codecov 项目页获取）。覆盖率、Bundle、测试结果三个动作共用此 token。

## 5. 运行环境

- `runs-on: ubuntu-24.04`（**必须 24.04**，因为只有它有 `libwebkitgtk-6.0-dev`）。
- 工作流开头 `sudo apt-get install pkg-config libgtk-4-dev libwebkitgtk-6.0-dev`：本项目用 Wails，Go 测试会 CGO 链接 GTK4/WebKit 系统库，标准 runner 缺这些库会在 `ld` 阶段失败（`pkg/wails-updater-providers` 间接引入 wails 运行时）。这与 `.cnb/Dockerfile` 里装的库一致。

## 6. 验证与排错

在 Codecov 项目页应能看到三个面板：**Coverage / Bundle / Tests**。

- **Go 测试 `ld` 报错 `cannot find -lwebkitgtk-6.0` 等**：系统库未装，确认 `apt-get install` 步骤在跑且为 `ubuntu-24.04`。
- **Bundle 面板空白**：检查「构建前端」步骤是否真的上传成功；插件与 Vite 8 不兼容时该步骤会 `continue-on-error` 失败但不阻断。
- **Tests 面板空白**：检查 `test-results.json` / `go-test-report.xml` 是否生成、`vitest-json-to-junit.mjs` 是否产出 `test-report.xml`、`codecov/test-results-action` 是否拿到 token。
- **Tests 面板里用例名带 `/__w/...`、`/home/runner/work/...` 等绝对路径，同一条用例出现成两条记录**：`classname` 没归一化。自查： `grep -o 'classname="[^"]*"' frontend/test-report.xml | sort -u | head` —— 出现以 `/`（或 `C:\`）开头即是，应修生成脚本（见 3.3），不要在 `codecov.yml` 里打补丁。
  - 两种前缀来自**两种执行形态**（容器 job `/__w/<repo>/<repo>` vs 裸 runner `/home/runner/work/<repo>/<repo>`），不是两次不同的 bug。
  - 历史数据不会迁移：已上传的绝对路径记录保留到 Codecov 60 天保留期满才自然淘汰。**UI 上还能看到旧记录 ≠ 修复没生效**，要看最新一次 run。
- **覆盖率双份/互相覆盖**：确认 `.github/codecov.yml` 的 `flags` 已区分 `go` / `node`，且 `status` 为 `informational`。

## 7. 注意事项

- GA 与 CNB 都跑一遍测试，各管各的、互不依赖（GA 生成产物并上报 Codecov，CNB 负责门禁）。
- `frontend/package.json` 的 `packageManager` 为 `pnpm@12.14.0`，`frontend/.nvmrc` 为 `25.9.0`——工作流用 `node-version-file` / `go-version-file` 动态读取，不要写死版本号。
- `flags` 命名沿用本项目 CNB 约定：Go 侧 `go`、前端侧 `node`（对应 CNB `testing:coverage` 的 `key: go` / `key: node`）。
- 文档命名注意 `docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线命名（`docs/codecov.md`）。
