# Codecov 接入说明（kai）

本仓库的**覆盖率 / 打包体积 / 测试结果**统一上报到 **Codecov**，由 GitHub Actions 工作流驱动（与 CNB 的硬性红线门禁相互独立）。

## 1. 总览：三个支柱 + 职责分离

| 支柱                     | 内容            | 本仓库实现                                                                 | Codecov 里看什么                      |
| ------------------------ | --------------- | -------------------------------------------------------------------------- | ------------------------------------- |
| 覆盖率 Coverage          | 代码行覆盖率    | `codecov/codecov-action` 上传 Go `coverage.out` 与前端 `lcov.info`         | Coverage 面板、PR 评论、patch 状态    |
| 打包体积 Bundle Analysis | 前端 chunk 体积 | `@codecov/vite-plugin`（构建时上传）                                       | Bundle 面板（体积趋势 / 回归）        |
| 测试结果 Test Analytics  | 单测运行结果    | Vitest `json` reporter → 脚本转 JUnit → `codecov/test-results-action` 上传 | Tests 面板（时长 / flaky / 失败分析） |

> **职责分离（重要）**：CNB 流水线（`testing:coverage`）负责**硬性红线门禁**（`lines` / `diffLines`），会阻断低覆盖率合并；GitHub Actions 只负责**把数据上报到 Codecov** 做可视化（PR 评论、徽章、patch 状态）。因此 `.github/codecov.yml` 里的 `status` 全部设为 `informational`，避免与 CNB 双重门禁。
>
> Codecov 的上传步骤**不放在 CNB**（CNB 是 CNB，GitHub 是 GitHub）——覆盖率产物在 GitHub Actions 环境内生成并上传。

## 2. 涉及文件清单

| 文件                                        | 作用                                                                                         |
| ------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `.github/codecov.yml`                       | Codecov 规则：`flags`（go / node）、`status: informational`、忽略生成代码                    |
| `.github/workflows/coverage.yml`            | GitHub Actions 流水线：装依赖 → 跑 Go/前端测试 → 构建前端 → 上传覆盖率 / 测试结果 / 打包体积 |
| `frontend/vite.config.ts`                   | 条件加载 `@codecov/vite-plugin`（仅当 `CODECOV_TOKEN` 存在时实例化）                         |
| `frontend/scripts/vitest-json-to-junit.mjs` | 零依赖脚本：Vitest JSON → JUnit XML                                                          |
| `frontend/.npmrc`                           | pnpm 设置：放行 `@codecov/vite-plugin` 对 Vite 8 的 peer（详见第 5 节）                      |
| `frontend/package.json`                     | `devDependencies`：`@codecov/vite-plugin`                                                    |

## 3. 各功能详解

### 3.1 覆盖率（Coverage）

- Go：复用 CNB 的同款口径
  ```
  go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...
  ```
  - 范围 `./internal/... ./pkg/...`：含全部测试（`internal/configstore` + `pkg/wails-updater-providers`）；不含根 main 包——它 `//go:embed all:frontend/dist`，无前端产物会编译失败。
  - `-vet=off`：`swiftbridge`/`selection`/`engine` 存在 `fmt.Errorf` 非常量格式串，vet 会挂。
  - 本项目无 ent 生成代码，无需排除。
  - 产出 `coverage.out`（flag: `go`）。
- 前端：`pnpm test:coverage`（Vitest v8 覆盖率，产出 `frontend/coverage/lcov.info`，flag: `node`）。
- 上传：`codecov/codecov-action@v5`，分两步分别带 `flags: go` / `flags: node`。

### 3.2 打包体积（Bundle Analysis）

- 插件 `@codecov/vite-plugin@2.0.1` 在 `frontend/vite.config.ts` 中**条件加载**：
  ```ts
  const codecovPlugins = process.env.CODECOV_TOKEN
    ? [
        codecovVitePlugin({
          enableBundleAnalysis: true,
          bundleName: "kai-frontend",
          uploadToken: process.env.CODECOV_TOKEN,
        }),
      ]
    : [];
  export default defineConfig({ plugins: [svelte(), tailwindcss(), wails(bindings), ...codecovPlugins] });
  ```
- 上传发生在 `pnpm build`（= `vite build --mode production`）这一步（coverage.yml 的「构建前端」步骤）。
- ⚠️ **兼容性注意**：该插件 peer 仅声明支持 Vite `4/5/6`，而本仓库为 **Vite 8.3.4**。因此：
  1. `frontend/.npmrc` 用 `peer-dependency-rules.allowed-versions["@codecov/vite-plugin[vite]"]=8` + `strict-peer-dependencies=false` 放行安装（pnpm 12 已不在 `package.json` 读 `pnpm` 字段）；
  2. coverage.yml 的「构建前端」步骤设了 `continue-on-error: true`：万一插件与 Vite 8 不兼容导致 `vite build` 崩溃，也**不会拖垮**上方的覆盖率 / 测试结果上传。
- 本地 / CNB 构建都没有 `CODECOV_TOKEN`，插件根本不会实例化，绝不影响主构建。

### 3.3 测试结果（Test Analytics）

- Vitest `3.x` **没有内置 junit reporter**（社区 `@vitest/junit` 也不可用），改用零依赖方案：
  1. 测试步骤额外挂内置 `json` reporter 产出 `test-results.json`：
     ```
     pnpm test:coverage --reporter=default --reporter=json --outputFile=test-results.json
     ```
     （`coverage/lcov.info` 仍正常产出，不受影响。）
  2. 转换步骤用 `node scripts/vitest-json-to-junit.mjs` 把 JSON 转成 `test-report.xml`（JUnit 格式）。
  3. 上传步骤用 `codecov/test-results-action@v1` 上传 `frontend/test-report.xml`。
- **当前仅接入前端（Svelte）测试结果**。Go 侧未接入：一方面 `go test` 也不原生产出 JUnit（需 `gotestsum`），另一方面为避免改动已调好的 Go 覆盖率步骤、引入额外工具风险。如后续需要 Go 测试分析，可在 GA 用 `gotestsum --junitfile` 生成 XML 后一并上传。

## 4. 必需的密钥

- `CODECOV_TOKEN`：在 GitHub 仓库 **Settings → Secrets → Actions** 中配置（Codecov 项目页获取）。覆盖率、Bundle、测试结果三个动作共用此 token。

## 5. 运行环境

- `runs-on: ubuntu-24.04`（**必须 24.04**，因为只有它有 `libwebkitgtk-6.0-dev`）。
- 工作流开头 `sudo apt-get install pkg-config libgtk-4-dev libwebkitgtk-6.0-dev`：本项目用 Wails，Go 测试会 CGO 链接 GTK4/WebKit 系统库，标准 runner 缺这些库会在 `ld` 阶段失败。这与 `.cnb/Dockerfile` 里装的库一致。

## 6. 验证与排错

在 Codecov 项目页应能看到三个面板：**Coverage / Bundle / Tests**。

- **Go 测试 `ld` 报错 `cannot find -lwebkitgtk-6.0` 等**：系统库未装，确认 `apt-get install` 步骤在跑且 runner 为 `ubuntu-24.04`。
- **Bundle 面板空白**：检查「构建前端」步骤是否真的上传成功；插件与 Vite 8 不兼容时该步骤会 `continue-on-error` 失败但不阻断（属预期，可忽略或等插件支持 Vite 8）。
- **Tests 面板空白**：检查 `test-results.json` 是否生成、`vitest-json-to-junit.mjs` 是否产出 `test-report.xml`、`codecov/test-results-action` 是否拿到 token。
- **覆盖率双份/互相覆盖**：确认 `.github/codecov.yml` 的 `flags` 已区分 `go` / `node`，且 `status` 为 `informational`。

## 7. 注意事项

- GA 与 CNB 都跑一遍测试，各管各的、互不依赖（GA 生成产物并上报 Codecov，CNB 负责门禁）。
- 工作流用 `node-version-file`（`frontend/.nvmrc`）/ `go-version-file`（`go.mod`）动态读取版本，不要写死版本号。
- `flags` 命名沿用本项目 CNB 约定：Go 侧 `go`、前端侧 `node`（对应 CNB `testing:coverage` 的 `key: go` / `key: node`）。
- 文档命名注意 `docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线命名（`docs/codecov.md`）。
