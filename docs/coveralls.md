# Coveralls 接入说明（kai）

本文只讲一件事：**Coveralls 怎么接、怎么跑、怎么排错**。

- 上报入口：`.github/workflows/coveralls.yml`，独立工作流，不依赖其他流程。
- 上报内容：Go 与前端两条覆盖率，分别打 `go` / `node` 两个 flag，合并为一个 build。
- 仓库内**没有** `.coveralls.yml`：阈值、PR 注释、徽章都在 Coveralls 后台仓库设置里配。
- **goveralls / gotestsum 不需要在工作流里安装**：CI 镜像已预装（`GOVERALLS_VERSION=v0.0.12`、`GOTESTSUM_VERSION=v1.13.0`）。

## 1. 涉及文件

| 文件                              | 作用                                                                                    |
|-----------------------------------|-----------------------------------------------------------------------------------------|
| `.github/workflows/coveralls.yml` | 唯一入口：装依赖 → 跑 Go/前端测试 → 分别上报 `flag=go` / `flag=node`                    |
| `.github/ci/Dockerfile`           | 提供 `ghcr.io/<repo>/ci:latest` 镜像（Go/Node/pnpm/GTK4/WebKit6，含 goveralls v0.0.12） |
| `Makefile`（`deps` 目标）         | `go mod download` + `pnpm --dir ./frontend install`                                     |
| `coverage.out`                    | Go 原始覆盖率，**保持模块前缀原样**直接交给 goveralls（它自己会还原路径）               |
| `frontend/coverage/lcov.info`     | 前端覆盖率产物（`pnpm test:coverage` 生成）                                             |

## 2. 工作流结构

### 2.1 触发与并发

```yaml
on:
  push:
  pull_request:

concurrency:
  group: coveralls-${{ github.ref }}
  cancel-in-progress: true
```

同一分支连续 push 会取消上一次运行，避免同一 commit 重复上传。

### 2.2 为什么是单个 job

Coveralls 以 **build** 为单位聚合报告。go / node 两份数据必须落进**同一个 build**，才能靠 flag 合并成一份总覆盖率。

- **单 job**：两次上传天然同 build、同 commit、同 job id，flag 直接合并，确定性最高。
- **多 job + parallel**：需要 `parallel: true` + 最后一个 `parallel-finished: true` 收尾，goveralls 侧还要 `-parallel -jobid`。两套工具混用 parallel 协议，容易出现「build 一直不 finalize」或「两份报告各算各的」。

当前用单 job 顺序执行，用一点时间换确定性。将来真要拆（比如 Go 测试太慢），写法是：

```yaml
jobs:
  coverage-go: # goveralls -parallel -flagname=go -jobid=${{ github.job }}
  coverage-node: # action: parallel: true, flag-name: node
  finish: # needs: [coverage-go, coverage-node] → parallel-finished: true
```

拆之前先确认 goveralls 版本的 `-parallel` / `-jobid` 行为，别照抄。

### 2.3 运行环境

复用预构建镜像：

```yaml
container:
  image: ghcr.io/${{ github.repository }}/ci:latest
```

镜像基于 `golang:1.27.3-trixie`，内置 Node `26.12.1`、pnpm `12.15.0`、GTK4 / WebKit6，以及钉好版本的 Go 工具链：

| 工具      | 版本                        | 来源                     |
|-----------|-----------------------------|--------------------------|
| gotestsum | `GOTESTSUM_VERSION=v1.13.0` | `go install ...@版本`    |
| gojq      | `GOJQ_VERSION=v0.12.19`     | 同上                     |
| goveralls | `GOVERALLS_VERSION=v0.0.12` | 同上，装到 `$GOPATH/bin` |

`ENV PATH="$GOPATH/bin:$PATH"` 已经包含 `/go/bin`，所以工作流里可以直接调 `goveralls`， **不需要** `setup-go`、`setup-node`、`apt-get install`——Wails 的 CGO 链接依赖（GTK4/WebKit6）也齐了。 镜像由 `ci-image.yml` 推送，**首次运行前必须已存在**，否则容器起不来。

### 2.4 缓存

| 缓存           | 路径                        | key                                    |
|----------------|-----------------------------|----------------------------------------|
| Go module      | `/go/pkg/mod`               | `hashFiles('**/go.sum')`               |
| Go build cache | `$HOME/.cache/go-build`     | `hashFiles('**/go.sum')`               |
| pnpm store     | `pnpm config get store-dir` | `hashFiles('frontend/pnpm-lock.yaml')` |

## 3. Go 覆盖率：goveralls

### 3.1 为什么不用 action

`coverallsapp/github-action@v2` 的 `action.yml` 只有 `path-to-lcov` 一个覆盖率输入，**没有** `file`， 也**没有** `format`。所以不存在 `format: golang`——Go 的 `coverage.out` 不能直接喂给它。

### 3.2 步骤（两步，没有安装步骤）

```yaml
- name: 生成 Go 测试覆盖率
  run: |
    gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...

- name: 上传 Go 覆盖率到 Coveralls
  if: env.COVERALLS_REPO_TOKEN != ''
  continue-on-error: true
  run: |
    goveralls \
      -coverprofile=coverage.out \
      -service=github \
      -repotoken="$COVERALLS_REPO_TOKEN" \
      -flagname=go
```

**goveralls 来自镜像，工作流里不要 `go install`。** 镜像用 `GOVERALLS_VERSION=v0.0.12` 预装， v0.0.12 实测支持本次用到的全部参数：`-coverprofile` / `-service` / `-repotoken` / `-flagname` / `-parallel` / `-jobid` / `-shallow` / `-endpoint`。

在工作流里 `go install ...@latest` 是负优化：每次跑都要下载编译、拖慢 CI，还会把镜像钉死的版本 覆盖成某个不确定的 latest，破坏可重现性。升级 goveralls 改的是 Dockerfile 的 ARG，重推镜像。

### 3.3 三个必须知道的细节

- **路径前缀不要动**。`go test -coverprofile` 写出的路径是模块限定的：

  ```
  cnb.cool/dtapp/kai/internal/useragent/useragent.go:18.2,21.1 3 0
  ```

  goveralls v0.0.12 自己就能还原：`gocover.go` 的 `findRootPackage()` 读 `go.mod` 拿到模块路径， `findFile()` 用 `filepath.Rel(rootPackage, file)` 剥成 `internal/useragent/useragent.go`， 再 `Join(rootDir, relPath)` 落到工作区真实文件；上传前 `getCoverallsSourceFileName()` 再按 `.git` 根裁成仓库相对路径。所以 `coverage.out` **原样**喂进去就是对的。

  反过来，先用 `sed` 剥掉前缀会**直接失败**（真实 CI 报错）：

  ```
  cannot find file "internal/analytics/analytics.go":
    can't find "analytics.go": package internal/analytics is not in std (/usr/local/go/src/internal/analytics)
  ```

  原因：剥完后 `filepath.Rel` 得到 `../..` 开头的路径，`findFile` 落到 `build.Import("internal/analytics", ".", build.FindOnly)`——模块模式下，相对模块根的裸路径 不是合法 import path（`internal/analytics` 又不是 std），于是整份报告解析失败。 `sed "s|^$(go list -m)/||"` 是 GOPATH 时代的老配方，模块模式下必须去掉。

- **`-service=github`**：让 goveralls 从 `GITHUB_*` 环境变量取分支 / commit / PR 元数据。

- **`-flagname=go`**：给这份报告打标签，与前端那份合并成同一 build 的两个 flag。

### 3.4 统计口径

统计 `./internal/... ./pkg/...`：含全部有测试的包（含 `pkg/wails-updater-providers`、`pkg/swiftbridge`）， **不含根 main 包**——它的 `//go:embed all:frontend/dist` 在无前端产物时会编译失败。 本项目未引入 ent，无需排除生成代码。`-vet=off` 因为 `swiftbridge`/`selection`/`engine` 存在 `fmt.Errorf` 非常量格式串，vet 会挂。`gotestsum` 产出的 `go-test-report.xml` 只用于命令对齐与本地排查， Coveralls **不接收** JUnit。

## 4. 前端覆盖率：coveralls action

```yaml
- name: 生成前端测试覆盖率
  working-directory: frontend
  run: pnpm test:coverage

- name: 上传前端覆盖率到 Coveralls
  if: env.COVERALLS_REPO_TOKEN != ''
  continue-on-error: true
  uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    path-to-lcov: ./frontend/coverage/lcov.info
    flag-name: node
```

- 输入名是 **`path-to-lcov`**，不是 `file`。
- `flag-name: node` 与 Go 的 `-flagname=go` 配对。
- `github-token` 用内置 `GITHUB_TOKEN`；job 级已注入 `COVERALLS_REPO_TOKEN` 环境变量，私有仓库用它做回退认证。

## 5. 密钥

- `COVERALLS_REPO_TOKEN`：GitHub 仓库 **Settings → Secrets → Actions** 添加。 取值路径：Coveralls → 选择仓库 → **Settings / Tokens**。

行为约定：

- **fork PR**：GitHub 不向 fork 的 PR 传递 secrets，token 为空 → 上传步骤被 `if: env.COVERALLS_REPO_TOKEN != ''` 跳过，不会因为缺 token 而红。
- **上传失败不阻断 CI**：两个上传步骤都有 `continue-on-error: true`。Coveralls 是观测工具， 不该成为合并的阻塞项。

## 6. 验证与排错

跑完后到 Coveralls 仓库页确认：**最新 build 下同时有 `go` 与 `node` 两个 flag，且合并出一个总覆盖率**。

| 现象                                                                   | 原因 / 处理                                                                                         |
|------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| 上传步骤显示 skipped                                                   | `COVERALLS_REPO_TOKEN` 为空（fork PR 正常；主仓 PR 则是 secret 没配）                               |
| `goveralls: command not found`                                         | 镜像里 `GOVERALLS_VERSION` 那步没装成功，或 `/go/bin` 不在 PATH；**改镜像，别用 `go install` 兜底** |
| `cannot find file "internal/x.go"`、`package internal/x is not in std` | profile 被剥过模块前缀。删掉 `sed`，`coverage.out` 原样喂给 goveralls（见 3.3）                     |
| 文件路径对不上 / 覆盖率 0%                                             | `coverage.out` 首行须是 `mode:`，后续行**带**模块前缀（`cnb.cool/dtapp/kai/internal/...`），别手改  |
| 只看到 `go` 或只看到 `node`                                            | 某一步上传失败被 `continue-on-error` 吞了，展开该 step 日志看 API 返回                              |
| 两个 flag 各算各的、没有合并                                           | 两次上传不在同一 build；确认仍在同一个 job 内（拆分 job 就必须上 parallel 协议）                    |
| 私有仓库 401 / 未授权                                                  | Coveralls 后台未授权该仓库，或 token 类型不对；确认 `COVERALLS_REPO_TOKEN` 属于该仓库               |

本地预演（不真的上传）：

```bash
go test -vet=off \
  -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) \
  -coverprofile=coverage.out ./internal/... ./pkg/...
head -3 coverage.out   # 首行 mode:，后续为 cnb.cool/dtapp/kai/internal/xxx.go:...（前缀要留着）

# 干跑：用假 endpoint，验证文件解析能否通过（不发真实请求）
go run github.com/mattn/goveralls@v0.0.12 -coverprofile=coverage.out \
  -service=github -repotoken=dummy -endpoint=http://127.0.0.1:1
# 只报 dial tcp ... connection refused = 解析通过
# 报 cannot find file ... = profile 前缀被剥过，见 3.3
```

## 7. 注意事项

- `flag` 名一旦定了别乱改，改了等于新建一条趋势线，历史会断档。
- 文档命名：`docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线（`docs/coveralls.md`）。
- goveralls 版本由 `.github/ci/Dockerfile` 的 `GOVERALLS_VERSION`（当前 `v0.0.12`）钉死； 升级改 ARG 后重推镜像，**不要**在工作流里 `go install`。
- Coveralls 后台需先授权该仓库，否则上传返回 401。
- 本工作流与 Codecov 的 `coverage.yml` 各自独立：Coveralls 故障不影响 Codecov，反之亦然。
