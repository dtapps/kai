# Coveralls 接入说明（kai）

本文只讲一件事：**Coveralls 怎么接、怎么跑、怎么排错**。

- 上报入口：`.github/workflows/coveralls.yml`，独立工作流，不依赖 Codecov 流程。
- 上报内容：Go 与前端两条覆盖率，分别打 `go` / `node` 两个 flag，落在同一个 build 里。
- 仓库内**没有** `.coveralls.yml`：阈值、PR 注释、徽章都在 Coveralls 后台仓库设置里配。
- 上传用 **`coverallsapp/github-action@v2`**，镜像里**没有** Coveralls CLI；`gotestsum` 已预装在 CI 镜像中。

## 1. 涉及文件

| 文件                              | 作用                                                                                                          |
|-----------------------------------|---------------------------------------------------------------------------------------------------------------|
| `.github/workflows/coveralls.yml` | 唯一入口：装依赖 → 跑 Go/前端测试 → 分别上传 `flag=go` / `flag=node`                                          |
| `.github/ci/Dockerfile`           | 提供 `ghcr.io/<owner>/<repo>/ci:latest` 镜像（Go/Node/pnpm/GTK4/WebKit6，含 `gotestsum v1.13.0`）             |
| `Makefile`（`deps` 目标）         | `go mod download` + `pnpm --dir ./frontend install`                                                           |
| `coverage.out`                    | Go 原始覆盖率（`-coverprofile` 产物），**保持模块前缀原样**交给官方 Action（`format: golang` 自己会还原路径） |
| `frontend/coverage/lcov.info`     | 前端覆盖率产物（`pnpm test:coverage` 生成）                                                                   |

> 历史提醒：本仓库曾用 `goveralls` CLI 上报 Go 覆盖率，2026-10-24 前后已改为官方 Action，镜像里的 `GOVERALLS_VERSION` 也一并移除。**不要再加回来**——多一个不受工作流版本控制的二进制，只会让「版本只在 Dockerfile 的 ARG 里」这条唯一事实来源失效。

## 2. 工作流结构

### 2.1 触发与并发

```yaml
on:
  push:
  pull_request:

permissions:
  contents: read
  packages: read # 拉取 GHCR 上的 CI 镜像

concurrency:
  group: coveralls-${{ github.ref }}
  cancel-in-progress: true
```

同一分支连续 push 会取消上一次运行，避免同一 commit 重复上传。

### 2.2 为什么是单个 job

Coveralls 以 **build** 为单位聚合报告。`go` / `node` 两份数据必须落进**同一个 build**，才能靠 flag 合并成一份总覆盖率。

- **单 job**：两次上传天然同 build、同 commit、同 job 环境，flag 直接合并，确定性最高。
- **拆两个 job**：需要完整的 `parallel: true` + `parallel-finished: true` 协议，两套上传器混用容易「build 一直不 finalize」或「两份报告各算各的」，故不采用。

### 2.3 为什么与 Codecov 分开

- **故障隔离**：Coveralls 侧故障 / token 失效不会拖垮 Codecov 上传与测试 job。
- **凭证隔离**：Coveralls 用 `COVERALLS_REPO_TOKEN`，不扩散到 Codecov 流程。
- **代价**：测试会再跑一遍（与 `.github/workflows/codecov.yml` 重复）。这是「独立」换来的隔离性。

## 3. 运行环境

| 项          | 值                                                                       |
|-------------|--------------------------------------------------------------------------|
| Runner      | `ubuntu-latest`                                                          |
| 容器        | `ghcr.io/${{ github.repository }}/ci:latest`（预构建，见 `ci-image.md`） |
| Go          | 镜像内 Go 工具链（`GOPATH=/go`、`/go/bin` 已在 PATH）                    |
| Node / pnpm | 镜像内 Node + pnpm；pnpm store 目录动态读取后缓存                        |
| 缓存        | Go module、Go build、pnpm store 三份                                     |
| 超时        | 默认（未单独设置）                                                       |

`make deps` 负责下载 Go 模块与前端依赖；镜像已装好 GTK4/WebKit6，无需再 `apt-get install`，也无需 `setup-go` / `setup-node`。

## 4. 上报细节

### 4.1 Go 覆盖率

```bash
gotestsum --junitfile go-test-report.xml -- -vet=off \
  -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) \
  -coverprofile=coverage.out \
  ./internal/... ./pkg/...
```

- `gotestsum` 只是 `go test` 的包裹，参数原样透传；`coverage.out` 照常产出，`go-test-report.xml` 本工作流不上传（测试结果只进 Codecov）。
- 范围 `./internal/... ./pkg/...`：不含根 `main` 包——它 `//go:embed all:frontend/dist`，覆盖率流水线不构建前端，含它会编译失败。
- `-vet=off`：`pkg/swiftbridge`、`internal/selection`、`internal/engine` 存在 `fmt.Errorf` 非常量格式串，vet 会挂。

上传：

```yaml
- uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    file: coverage.out
    format: golang
    flag-name: go
```

**为什么不用「把 `coverage.out` 直接传给默认格式」**：`coverage.out` 是 Go `-coverprofile` 格式，不是 lcov；必须显式 `format: golang`，Action 才会按 Go 规则还原路径。

### 4.2 前端覆盖率

```bash
cd frontend && pnpm test:coverage   # vitest v8 → frontend/coverage/lcov.info
```

上传：

```yaml
- uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    path-to-lcov: ./frontend/coverage/lcov.info
    flag-name: node
```

前端产物是标准 lcov，用 `path-to-lcov`，不要带 `format`。

### 4.3 两个 flag 的意义

| flag   | 来源                          | 统计范围                                          |
|--------|-------------------------------|---------------------------------------------------|
| `go`   | `coverage.out`                | `internal/**`、`pkg/**`                           |
| `node` | `frontend/coverage/lcov.info` | `frontend/src/**`（已排除 bindings / 测试自身等） |

Coveralls 后台会把两个 flag 合并成同一次 build 的总覆盖率；想看单侧趋势时按 flag 筛选。

## 5. 密钥

| Secret                 | 必填 | 用途                                                           |
|------------------------|------|----------------------------------------------------------------|
| `COVERALLS_REPO_TOKEN` | 是   | Coveralls 项目 token；未配置时两个上传步骤因 `if` 条件直接跳过 |
| `GITHUB_TOKEN`         | 否   | 由 Actions 自动注入，Action 用来补 commit / branch / PR 元数据 |

注意：

- 上传步骤写了 `if: env.COVERALLS_REPO_TOKEN != ''`，未配置 token 时**不会失败**，但 Coveralls 上不会有数据。
- 上传步骤同时写了 `continue-on-error: true`：Coveralls 侧故障不阻断测试与其它步骤。

## 6. 验证与排错

本地/CI 自检顺序：

```bash
# 1. 确认 Go 覆盖率产物
go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) \
  -coverprofile=/tmp/coverage.out ./internal/... ./pkg/...
go tool cover -func=/tmp/coverage.out | tail -1

# 2. 确认前端 lcov
cd frontend && pnpm test:coverage && ls -lh coverage/lcov.info
```

| 现象                                             | 原因 / 处理                                                                                        |
|--------------------------------------------------|----------------------------------------------------------------------------------------------------|
| Coveralls 上只有 `go` 或只有 `node`              | 对应上传步骤被 `if` 跳过（token 为空）或该步骤失败；看 Actions 日志里两次 `coverallsapp` 步骤      |
| `flag-name` 不生效、两份报告互相覆盖             | 没设 `flag-name`，或两次上传不在同一个 build（确认没有拆 job / 改过 `github-token` 或 build 标识） |
| Go 覆盖率路径显示异常（带模块前缀 / 找不到文件） | 用了 `format: lcov` 或漏了 `format: golang`；Go 的 `coverage.out` 必须走 `format: golang`          |
| 前端覆盖率 0% / 文件对不上                       | `lcov.info` 路径错（应为 `./frontend/coverage/lcov.info`），或 vitest 覆盖率 include/exclude 改了  |
| `gotestsum: command not found`                   | 用了未预装 `gotestsum` 的镜像（如 CNB 那份），或 `/go/bin` 不在 PATH                               |
| 上传返回 422 / 401                               | `COVERALLS_REPO_TOKEN` 失效、仓库未在 Coveralls 开通，或 token 与仓库不匹配                        |

## 7. 注意事项

- **Coveralls 与 Codecov 各跑一遍测试**：两边独立，任何一边挂都不影响另一边；不要为了省时间把两者合并成一个 job（故障会互相传导）。
- **不要在工作流里 `go install`**：`gotestsum` 等工具由 CI 镜像提供，工作流里装 `@latest` 会覆盖钉死版本。
- **Go 与前端 flag 名固定为 `go` / `node`**：与 CNB `testing:coverage` 的 `key`、Codecov 的 `flags` 保持一致，便于跨平台对照。
- **文档命名**：`docs/*-*-*.md`（双横线）会被 `.gitignore` 忽略，本文档用单横线命名（`docs/coveralls.md`）。
