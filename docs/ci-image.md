# CI 镜像（CNB / GitHub 两份 Dockerfile）

两份 Dockerfile 装的是**同一套环境**（Go 1.27.3 + Node/pnpm + GTK4/WebKit6 + 锁版本的 Go 工具链），
但**构建与分发方式完全不同**：

- **CNB**：`.cnb/ci/Dockerfile`，流水线每次现场 `docker build`，层缓存靠 copy-on-write 卷。
- **GitHub**：`.github/ci/Dockerfile`，由 `ci-image.yml` 预构建推到 GHCR，workflow 用 `container:` 拉取。

两份文件的 `diff` 只有 **21 行**，差异全部集中在「用什么镜像源」和「装不装覆盖率 CLI」。

## 1. 涉及文件

| 文件                               | 作用                                                           |
| ---------------------------------- | -------------------------------------------------------------- |
| `.cnb/ci/Dockerfile`               | CNB 流水线镜像定义，被 5 个 job 以 `docker: build` 引用        |
| `.github/ci/Dockerfile`            | GitHub Actions 镜像定义，比 CNB 多 `gotestsum` + `goveralls`   |
| `.github/workflows/ci-image.yml`   | GitHub 侧唯一构建入口，推送到 `ghcr.io/<owner>/<repo>/ci`      |
| `.github/workflows/push.yml`       | 消费镜像：`container: ghcr.io/<repo>/ci:latest`                |
| `.github/workflows/coverage.yml`   | 同上（Codecov 链路）                                           |
| `.github/workflows/coveralls.yml`  | 同上（Coveralls 链路，依赖镜像里的 `goveralls` / `gotestsum`） |
| `.cnb/workflows/push.yml`          | 消费镜像：`docker: build: .cnb/ci/Dockerfile`（1 处）          |
| `.cnb/workflows/pull-coverage.yml` | 同上（2 处 job）                                               |
| `.cnb/workflows/push-coverage.yml` | 同上（2 处 job）                                               |
| `.cnb.yml`                         | `include` 上述 CNB 工作流                                      |

## 2. 两份 Dockerfile 的差别

### 2.1 完全相同的部分

- **基础镜像** `golang:1.27.3-trixie`。
- **apt 包**：`wget` `xz-utils` `ca-certificates` `curl` `gnupg` `zip` `unzip` `pkg-config` `git` `make`
  `libgtk-4-dev` `libwebkitgtk-6.0-dev` `jq`。
  其中 GTK4 / WebKit6 开发库是硬需求：既供 `go install wails3` 从源码编译，也供后续
  `go build` / `go test` 编译 webview 时 `pkg-config` 找到 `gtk4.pc` / `webkitgtk-6.0.pc`。
- **Node** 26.12.1：按 `uname -m` 选 `x64` / `arm64` 的官方 `.tar.xz`，解到 `/usr/local`（其他架构直接 `exit 1`）。
- **pnpm** 12.15.0：`npm install -g pnpm@${PNPM_VERSION}`。
- **Go 工具链**（全部 `go install` 到 `/go/bin`，版本由 ARG 钉死）：

  | 工具            | 版本             |
  | --------------- | ---------------- |
  | `wails3`        | `v3.0.0-beta.34` |
  | `sqlc`          | `v1.31.1`        |
  | `golangci-lint` | `v2.15.0`        |
  | `govulncheck`   | `v1.9.0`         |
  | `shfmt`         | `v3.14.1`        |
  | `dockerfmt`     | `v0.5.4`         |
  | `yamlfmt`       | `v0.21.0`        |
  | `gojq`          | `v0.12.19`       |

- **收尾**：`apt-get purge -y wget xz-utils` + `autoremove` + `clean`，删
  `/var/lib/apt/lists/*` `/tmp/*` `/root/.npm`。
- `GOPATH=/go`、`PATH=$GOPATH/bin:$PATH`、`ENV GOINSECURE=go.dtapp.net`、`CMD ["bash"]`。

### 2.2 只有 `.cnb/ci/Dockerfile` 有

| 内容                                                                         | 目的                                    |
| ---------------------------------------------------------------------------- | --------------------------------------- |
| `sed -i s\|deb.debian.org\|mirrors.cloud.tencent.com\|g`                     | apt 走腾讯云（境内构建）                |
| `ENV GO111MODULE=on`、`GOPROXY=https://mirrors.cloud.tencent.com/go/,direct` | Go 模块加速                             |
| `npm config set registry` / `pnpm config set registry` 腾讯云                | 前端依赖加速                            |
| `pnpm config set ignore-scripts false`                                       | 允许依赖执行 install 脚本（部分包需要） |
| `ARG GITHUB_MIRROR="https://files.m.daocloud.io"`                            | **声明了但全文未使用**（死参数，见 §7） |

### 2.3 只有 `.github/ci/Dockerfile` 有

| 内容                        | 用途                                   |
| --------------------------- | -------------------------------------- |
| `GOTESTSUM_VERSION=v1.13.0` | `coveralls.yml` 跑测试并产出 JUnit XML |
| `GOVERALLS_VERSION=v0.0.12` | `coveralls.yml` 上报 Go 覆盖率         |

GitHub 侧**不换任何镜像源**：runner 在境外，直连 `nodejs.org` / `proxy.golang.org` / `registry.npmjs.org`。

**为什么 CNB 不需要 `gotestsum` / `goveralls`**：CNB 的覆盖率走内置 `testing:coverage` 任务
（`push-coverage.yml` 注释里写明参考官方 `examples/ecosystem/testing-go-demo`），不经过这两个外部 CLI。

## 3. GitHub 侧：预构建 → GHCR → `container:`

### 3.1 构建（`.github/workflows/ci-image.yml`）

- **触发**：`push` / `pull_request` 改到 `.github/ci/Dockerfile`；`schedule: 0 0 * * 1`（每周一 UTC 0 点，
  拉基础镜像的 apt 安全更新）；`workflow_dispatch`。
- **权限**：`contents: read` + `packages: write`；`concurrency` 同 `github.ref` 取消旧运行。
- **步骤**：checkout → setup-qemu → setup-buildx → login ghcr（`GITHUB_TOKEN`）→ metadata → build-push → 清理旧版本。
- **标签**：`latest`（仅默认分支）+ `sha-<short>`（每次构建，便于回滚）。
- **架构**：`linux/amd64,linux/arm64`，多架构 manifest；`container` 起 job 时按 runner 架构自动取对应层。
- **PR 不推送**：`push: ${{ github.event_name != 'pull_request' }}`，fork PR 无 `packages: write`。
- **缓存**：`cache-from/to: type=gha,mode=max`。
- **保留策略**：`delete-package-versions@v5`，`package-name: ${{ github.event.repository.name }}/ci`
  （即 `kai/ci`），`min-versions-to-keep: 5`。
  注意包名边界：镜像地址是 `ghcr.io/<owner>/<repo>/ci`，但 GHCR 的**包名**是 `<repo>/ci`——
  `owner` 不属于包名，只写 `ci` 会找不到包，进而清不掉历史版本、撑爆配额。

### 3.2 消费

```yaml
container:
  image: ghcr.io/${{ github.repository }}/ci:latest
```

`push.yml` / `coverage.yml` / `coveralls.yml` 三处相同。

> **首次使用前必须先跑一次 `ci-image.yml`**，否则 `container` 拉不到镜像，job 直接起不来。
> 同理：改了 Dockerfile 但还没等重建完成就改 workflow，跑的还是旧镜像（`latest` 滞后）。

## 4. CNB 侧：每次现场 build

```yaml
- runner:
    tags: cnb:arch:amd64
  services:
    - docker # docker build/push 需要 Docker 服务
  docker:
    build: .cnb/ci/Dockerfile
    volumes:
      - /root/.cache/go-build:copy-on-write # Go 构建缓存
      - /root/.local/share/pnpm/store:copy-on-write # pnpm 全局 store
      - /go/pkg/mod:copy-on-write # 只挂模块，避免覆盖镜像内预装的 /go/bin 工具
```

- 引用点共 **5 处**：`push.yml` 1、`pull-coverage.yml` 2、`push-coverage.yml` 2。
- **不落 registry**：没有推送、没有拉取，改 Dockerfile 下一次运行立即生效。
- **只有 amd64**：`cnb:arch:amd64` 钉死 runner 标签，无多架构 manifest。
- **第三个卷是坑位提示**：只挂 `/go/pkg/mod`。若误挂 `/go/bin`，`copy-on-write` 会用宿主机上的旧目录
  把镜像里预装的 `wails3` / `golangci-lint` 等覆盖掉。

代价与收益：冷缓存时首次 build 比 pull 镜像慢；换来的是无需等镜像发布、无 registry 配额问题。

## 5. 版本升级

两边**没有共享机制**，ARG 各写一份。改任何工具版本 = 改两个文件。

| 动作     | CNB                            | GitHub                                     |
| -------- | ------------------------------ | ------------------------------------------ |
| 改版本   | 改 `.cnb/ci/Dockerfile` 的 ARG | 改 `.github/ci/Dockerfile` 的 ARG          |
| 生效时机 | 下次流水线自动 build           | 等 `ci-image.yml` 重建完（push 即触发）    |
| 回滚     | revert 文件                    | 把 `container.image` 换成 `ci:sha-xxxxxxx` |

漂移自检（差异应**只有** `GITHUB_MIRROR`、`GOTESTSUM_VERSION`、`GOVERALLS_VERSION` 三项）：

```bash
diff <(grep '^ARG ' .cnb/ci/Dockerfile | sort) \
     <(grep '^ARG ' .github/ci/Dockerfile | sort)
```

## 6. 排错

| 现象                                                   | 原因 / 处理                                                                                 |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| container job 起不来，`manifest unknown` / 拉取失败    | GHCR 里还没有镜像，或包是私有的不允许匿名拉；先跑 `ci-image.yml`，并检查包可见性            |
| `Package gtk4 was not found` / `webkitgtk-6.0.pc` 缺失 | 镜像缺 `libgtk-4-dev` / `libwebkitgtk-6.0-dev`，wails3 与 webview CGO 都依赖它              |
| `goveralls: command not found`                         | 用了 CNB 那份 Dockerfile（它不装 goveralls），或 `/go/bin` 不在 PATH                        |
| `gotestsum: command not found`                         | 同上                                                                                        |
| CNB 构建卡在 `apt-get update`                          | 腾讯云源不可达；换源那条 `sed` 已用 `\|\|` 兜底 `sources.list` 与 `debian.sources` 两种结构 |
| 运行期 `wget: not found`                               | 收尾阶段 purge 了 `wget` / `xz-utils`；运行期要下载请用 `curl`                              |
| GHCR 存储配额告警                                      | 清理任务没跑成功；核对 `package-name` 是否为 `<repo>/ci`                                    |
| 工具版本在工作流里「莫名其妙」不是 ARG 里写的那个      | 有人在 workflow 里 `go install ...@latest` 覆盖了镜像版本（禁止，见 §7）                    |

## 7. 注意事项

- **两份是复制关系，不是继承关系**：没有共享 `FROM`，改一边不会影响另一边。往任何一边加工具前先问：
  另一边的流水线用不用得上？用不上就只加一边，但要在本文件的 §2.2 / §2.3 记一笔。
- **禁止在 workflow 里 `go install` 兜底**：会把钉死的版本覆盖成不确定的 `latest`，破坏可重现性，
  也失去「版本只在 Dockerfile 的 ARG 里」这一个唯一事实来源。
- **本文出现的版本号是快照**，以两份 Dockerfile 的 ARG 为准；升级后顺手更新 §2.1 的表格。
- **镜像体积**：含 GTK4 / WebKit6 开发库，单架构就有数 GB。这是 GitHub 侧必须限制保留版本数的原因，
  也是 CNB 侧为什么要挂三层 copy-on-write 卷。
- **`.cnb/ci/Dockerfile` 的 `ARG GITHUB_MIRROR` 是死参数**：声明后全文无引用（历史上用于 GitHub
  下载加速）。要么接上 `NODE_URL` / `go install` 的下载路径，要么删掉——留着会让人以为它在生效。
