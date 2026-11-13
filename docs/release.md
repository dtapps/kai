# 发布手册

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

只讲发版。CI 总览见 `ci/README.md`；secret 的权限与轮换见 `security-privacy.md`；更新器匹配规则见 `configuration.md` §4.4。

## 1. 版本规则

- 版本形如 `vMAJOR.MINOR.PATCH`，当前是 `v0.2.x`（最新 tag `v0.2.7`）。
- 版本号在 Release 工作流里输入：手动触发（`workflow_dispatch`）人工填写；定时触发（每天 UTC 00:00）自动生成 nightly 版本（见下）。
- 输入 `0.2.8` 与 `v0.2.8` 等价，统一归一成 tag `v0.2.8`（归一化与格式校验见 `scripts/ci/resolve-prerelease.sh`）。
- 只允许字母、数字和 `. _ + -`；引号、分号、`@`、`/` 会被拒绝，避免拼出非法 tag 或破坏 Release 配置。
- `buildinfo.Version` 由同一版本号经 `-ldflags` 注入，**不带** `v`；`v` 只属于 tag，不属于二进制版本串。
- 预发布**不接受勾选**，只看版本号后缀，四个渠道：`-rc.N` / `-beta.N` / `-alpha.N` / `-nightly.YYYYMMDD`（如 `v0.2.8-nightly.20260101`）；其余是正式版。
- **每日 Nightly（按日期版本化的预发布）**：schedule 触发等价于手动填 `nightly`；`prepare` 先把 `nightly` 推导成版本号再走普通发布流程：`<最近正式版 + 1 个补丁>-nightly.<UTC 构建日期>`（最近正式版 `v0.2.7` → tag `v0.2.8-nightly.20261114`）。每天一个 tag，**只保留最近 3 个 nightly Release**（更早的连 tag 一起删，见 release job 的清理步骤）。
  - 版本基准 = **最近一个正式版 tag + 1 个补丁**（`0.2.7` → `0.2.8`），三个字段共用它：`VERSION_BASE`=`0.2.8`（NSIS/plist）、`VERSION_PKG`=`0.2.8~nightly.20261114`（deb/rpm/arch）、`buildinfo.Version`=`0.2.8-nightly.20261114`。不能沿用正式版号（`0.2.7-nightly.*` 在 semver 里低于 `0.2.7`，客户端会反复提示降级），也不能用裸 `nightly`（非法 semver，脚本会直接拒绝）。客户端仍按 SHA256SUMS 里的 `# build_time=` / `# git_commit=` 注释行判断 nightly 是否真的更新。
  - 同一天重跑/补发会先 `gh release delete v<版本> --cleanup-tag` 删同名 Release 再重建（夜间构建 tag 允许覆盖）：避免旧附件堆积，并刷新 `published_at`（客户端 mirror_updater.go 按「预发布 Release 中 published_at 最新」挑选）。
  - 更新内容：与上一个 nightly tag 对比（`release-changelog.sh` 的 nightly 渠道），首次发该渠道时回退到上一个正式版。
  - 仓库 60 天无活动时 GitHub 会自动禁用 scheduled workflow，需去 Actions 页手动恢复。

### 1.1 打包版本派生

不同打包工具对版本串的格式要求不一样，派生只有一份实现（`scripts/ci/resolve-prerelease.sh` 的 `derive_packaging_versions`），工作流 `prepare` job 统一输出：

| 用途                                                              | 值           | 约束                                                                                                               |
|-------------------------------------------------------------------|--------------|--------------------------------------------------------------------------------------------------------------------|
| tag / Release 标题 / 二进制 `buildinfo.Version`                   | `0.2.8-rc.1` | 原样，用户看到的版本                                                                                               |
| Windows NSIS、macOS plist（`build/config.yml` 的 `info.version`） | `0.2.8`      | 必须 `X.Y.Z`。NSIS 模板自己补 `.0` 成 4 段，传 `0.2.8-rc.1` 会报 `invalid VIFileVersion format, should be X.X.X.X` |
| deb / rpm / arch（nfpm）                                          | `0.2.8~rc.1` | 不允许 `-`：rpm 的 `Version` 字段用 `-` 分隔 Version/Release；换成 `~` 后 deb/rpm 都表示「低于正式版」             |

四个渠道统一处理：`0.2.8-nightly.20260101` 同样派生为 `0.2.8`（NSIS / plist）与 `0.2.8~nightly.20260101`（deb/rpm/arch）。 预发布版本在 Windows 文件属性、macOS App 元数据里显示的是去掉后缀的 `0.2.8`，这是 `X.Y.Z` 限制导致的取舍；预发布标识保留在 tag、Release 标题与 App 内版本串里。

改了映射先跑 `bash scripts/ci/resolve-prerelease.sh --self-test`。

## 2. 发布前检查

1. master 的 PushRequest / Codecov / Coveralls / CNB 流水线全绿。
2. `go.mod`、`frontend/package.json`、两份 CI Dockerfile 的 Wails 版本一致或已确认差异。
3. `build/config.yml` 的 `info.version` 与本次版本一致。
4. 需要签名时，确认对应 secret 已配置（见 §4）。
5. 需要同步镜像时，确认 `SYNC_*` secret 已配置；未配置的同步 job 会跳过，不影响 GitHub Release。
6. 本地至少跑过：

```bash
make check
make check-cross
```

## 3. 工作流：`.github/workflows/release.yml`

手动触发：

```text
Actions → Release → Run workflow
```

输入：

| 输入           | 必填 | 说明                                                                       |
|----------------|------|----------------------------------------------------------------------------|
| `version`      | 是   | 版本号；预发布看后缀，例如 `0.2.8`、`0.2.8-rc.1`、`0.2.8-nightly.20260101` |
| `commit`       | 否   | 要构建的提交；留空用默认分支最新提交                                       |
| `nsis_version` | 否   | NSIS 版本，留空用 `3.13`                                                   |

`prerelease` 输入已移除：是否预发布只由版本号后缀决定。布尔输入会和版本号打架 （曾出现 `v0.2.8-rc.1` 没勾 prerelease、正文按正式版渲染的情况）。

job 顺序：

1. `prepare`：解析版本号（归一化 / 校验 / 预发布判定），生成统一的 `BUILD_TIME`（`scripts/get-build-time.sh`）与 `GIT_COMMIT`，输出 `tag` / `prerelease` / `release_title` / `version`。
2. `build`：矩阵构建 4 个组合。
3. `release`：汇总产物 → 校验版本号与 tag（tag 不存在、tag 数非零、正式版不比已有正式版旧）→ 生成 `SHA256SUMS` → 校验可发布产物数非零 → 生成发布说明 → 发布 GitHub Release。
4. `sync-cnb` / `sync-gitee` / `sync-gitlab` / `sync-gitcode`：并行触发各平台同步。
5. `notify`：ServerChan3 通知（配置了 `SERVERCHAN_SENDKEY` 才发）。

发布说明由 `scripts/ci/render-release-body.sh` 生成，结构固定为 「预发布警示（仅预发布）→ 发布信息 → 下载 → 校验 → 更新内容」； 「更新内容」由 `scripts/ci/release-changelog.sh` 计算：rc 对 rc、beta 对 beta、alpha 对 alpha、nightly 对 nightly、正式版对正式版， 首次发某渠道时回退到上一个正式版，挑不出基准会直接让 job 失败而不是谎报「首次发布」。

CNB / Gitee / GitLab / GitCode 全部镜像 GitHub 的正文，所以正文格式只改这两个脚本。 改了先本地自检：

```bash
bash scripts/ci/resolve-prerelease.sh --self-test
sh scripts/ci/render-release-body.sh --self-test
```

构建矩阵：

| Runner             | GOOS      | GOARCH  | 打包任务                                 |
|--------------------|-----------|---------|------------------------------------------|
| `macos-26`         | `darwin`  | `arm64` | `darwin:package`                         |
| `macos-26-intel`   | `darwin`  | `amd64` | `darwin:package`                         |
| `windows-2025`     | `windows` | `amd64` | `windows:package`（NSIS）                |
| `windows-11-arm`   | `windows` | `arm64` | `windows:package`（NSIS）                |
| `ubuntu-26.04`     | `linux`   | `amd64` | `linux:package`（AppImage/deb/rpm/arch） |
| `ubuntu-26.04-arm` | `linux`   | `arm64` | `linux:package`（AppImage/deb/rpm/arch） |

Linux 单独成 `build-linux` job，跑在组织镜像 `ghcr.io/dtapps/golang/wails3:latest` 里（amd64 / arm64 各一份原生镜像）： 镜像已带 Go / Node / pnpm / wails3 CLI / GTK4 / WebKitGTK 6.0 / gcc，省掉原生 runner 上 apt 装依赖、setup-go、`go install wails3` 这几步的分钟级开销。macOS / Windows 用不了容器，仍在 `build` job。

容器内没有 `/dev/fuse`，靠 `APPIMAGE_EXTRACT_AND_RUN=1` 让 appimagetool 解包运行； 容器内进程是 root、工作区属主是 runner 用户，所以要先 `git config --add safe.directory`，否则 `go build` 的 VCS 盖章会失败。

SQLite 驱动：Windows 用 `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`）；macOS / Linux 用 `sqlite_mattn` + `CGO_ENABLED=1`（`EXTRA_TAGS=sqlite_mattn`）。

## 4. 产物与签名

### 4.1 产物命名

| 文件                                  | 用途                   | 是否出现在 Release 下载表 |
|---------------------------------------|------------------------|---------------------------|
| `Kai-<v>-darwin-arm64.zip`            | macOS Apple 芯片安装包 | 是                        |
| `Kai-<v>-darwin-amd64.zip`            | macOS Intel 安装包     | 是                        |
| `Kai-<v>-windows-amd64-install.exe`   | Windows x64 安装包     | 是                        |
| `Kai-<v>-windows-arm64-install.exe`   | Windows ARM64 安装包   | 是                        |
| `Kai-<v>-linux-<arch>.AppImage`       | Linux 便携版           | 是                        |
| `Kai-<v>-linux-<arch>.deb`            | Debian/Ubuntu 安装包   | 是                        |
| `Kai-<v>-linux-<arch>.rpm`            | Fedora/RHEL 安装包     | 是                        |
| `Kai-<v>-linux-<arch>.pkg.tar.zst`    | Arch Linux 安装包      | 是                        |
| `updater-Kai-<v>-darwin-<arch>.zip`   | macOS 自更新包         | 否（隐藏）                |
| `updater-Kai-<v>-windows-<arch>.zip`  | Windows 自更新包       | 否（隐藏）                |
| `updater-Kai-<v>-linux-<arch>.tar.gz` | Linux 自更新包         | 否（隐藏）                |
| `SHA256SUMS`                          | 全局校验和             | 资产有，下载表不列        |
| `BUILD_TIME` / `GIT_COMMIT`           | 仅预发布版本附带       | 资产有，下载表不列        |

macOS 产物是 `.app` 的 zip，不是 dmg。自更新包必须是 `.app` 归档，才能保留签名。

产物名里的版本段不参与平台解析：`scripts/ci/render-release-body.sh` 按「最后一个 `-darwin-` / `-windows-` / `-linux-` 段」定位平台和架构，所以版本号怎么变都不会把 `Kai-0.2.8-darwin-arm64.zip` 解析错。新增产物格式时同时更新那里的 `parse_artifact_name` / `describe_artifact`，否则会落进「附件（…），解压即用」的兜底行。

### 4.2 Windows Authenticode

配置以下 secret 后，工作流会先签安装包、再签独立 `Kai.exe`（`windows:sign:installer`、`windows:sign`）：

- `WINDOWS_SIGN_CERTIFICATE`（.pfx 的 base64，或 runner 上的证书路径）
- `WINDOWS_SIGN_CERTIFICATE_PASSWORD`
- `WINDOWS_SIGN_THUMBPRINT`
- `WINDOWS_TIMESTAMP_SERVER`

未配置时跳过签名，用户下载会看到 SmartScreen 提示。

### 4.3 macOS 签名与公证

配置以下 secret 后执行 `darwin:sign:notarize`：

- `APPLE_DEVELOPER_ID_CERTIFICATE`
- `APPLE_DEVELOPER_ID_CERTIFICATE_PASSWORD`
- `APPLE_NOTARIZER_KEYCHAIN_PROFILE`

未配置时产物未签名/未公证，首次打开需要用户在「隐私与安全性」里放行。

## 5. 发布后同步

GitHub Release 发布成功后，`release.yml` 的同步 job 会并行触发：

| 平台   | job / 触发方式                                                   | 需要的 secret                                | 未配置时 |
|--------|------------------------------------------------------------------|----------------------------------------------|----------|
| CNB    | `sync-cnb` 调 `POST /-/build/start`，event `api_trigger_release` | `SYNC_CNB_TOKEN`、可选 `CNB_API_ENDPOINT`    | 跳过     |
| Gitee  | `sync-gitee` 调 Gitee Go Webhook（eventType `kai-release`）      | `SYNC_GITEE_WEBHOOK_URL`、`SYNC_GITEE_TOKEN` | 跳过     |
| GitLab | `sync-gitlab` 调 Trigger API，只传 VERSION                       | `SYNC_GITLAB_TRIGGER_TOKEN`                  | 跳过     |
| Gitea  | 无 Release 同步，只作代码镜像                                    | —                                            | —        |

- CNB 侧 `.cnb.yml` 的 `api_trigger_release` 会下载 GitHub 产物、删除同名 Release/Tag、重建 Release 并上传附件；是否预发布同样由版本号后缀推导（`pre_release` 传了也不采信）。
- GitLab 侧 `.gitlab-ci.yml` 的 `release` job 把附件传到 Generic Package Registry，再建 Release 与直链。
- Gitee 侧 `.workflow/release.yml` 由 Webhook 拉起，跑 `scripts/ci/` 下的两个脚本： `gitee-release-download.js`（按跳过规则下载并写清单 `gitee-release.json`）+ `gitee-release-publish.js`（调 Gitee OpenAPI 建 Release、逐个上传附件）。 需要在 Gitee 流水线加全局参数 **`KAI_GITEE_TOKEN`**（个人令牌，repo 读写）——名字不能以 `GITEE_` 开头，那是平台保留前缀。
- 所有同步 job 都“尽力而为”：失败只打 `::warning::`，不回滚 GitHub Release。

> **GitCode 当前不发布**：仓库里仍保留 `sync-gitcode` job 与 `.gitcode/workflows/release.yml`，但它们不在当前发布链路中；文档与发布 checklist 均按 GitHub / CNB / GitLab / Gitee 四个平台执行。

四端一致性约定（改任何一端都要对照这张表）：

- **预发布**：只看版本号后缀，不接受人工勾选；各端都不再接收 `PRERELEASE` / `pre_release` 布尔值。
- **标题**：统一 `Kai <tag>`，预发布追加 `(预发布)`（GitLab Release 没有 prerelease 字段，只能靠标题体现）。
- **附件名**：与 GitHub 逐字一致（`Kai-0.2.8-darwin-arm64.zip`，版本段不带 `v`）。
- **正文**：全部镜像 GitHub 正文，只替换各自平台的链接；Gitee 侧正文里的链接会退化成文字，「更新内容」保留标题、明细换成一句「请前往 GitHub Release 查看」（完整清单在 GitHub 上更有价值）。
- **附件清单**：CNB / GitLab / GitCode 全量搬运；Gitee 的下载脚本只有一条过滤规则——**单文件 >100MB 就跳过**（`AppImage` 112~117MB 因此不在 Gitee），其余（`updater-*`、`BUILD_TIME`/`GIT_COMMIT`、`SHA256SUMS`）都照常上传。
- **Gitee 容量**：单附件 100MB（GVP 项目 200MB），**仓库附件总量 1GB**（含仓库附件与发行版附件）。 脚本另有 `MAX_TOTAL_BYTES=800MB` 的单次安全阈值；发布侧遇到「附件配额不足」会跳过后续更大的文件、只打警告不判失败，并提示去 GitHub 下载。 想让 Gitee 长期保留多个版本，需要定期删旧 Release（删 Release 会连附件一起释放）——可以在 `gitee-release-publish.js` 里加一步按 `DELETE /api/v5/repos/{owner}/{repo}/releases/{id}` 清理，`KAI_GITEE_TOKEN` 已具备该权限。

## 6. 更新源与产物对应

- 更新器只匹配 `updater-` 前缀的 `.zip` / `.tar.gz`，并按平台/架构字符串过滤（`pkg/wails-updater-providers/matcher.go`）。
- 稳定版：比较 tag。
- 预发布：读取 Release 附带的 `GIT_COMMIT` / `BUILD_TIME` 文件，与本地构建时间/commit 比较；缺文件会报「预发布版本缺少 git commit / build time 附带文件」。
- 下载后校验 `SHA256SUMS`。
- 中文界面默认走 CNB，英文默认走 GitHub；可用 `settings.json` 的 `updater.source` 强制。

## 7. 回滚与补发

1. GitHub：`gh release delete v<version> --yes --cleanup-tag`，或页面上删除 Release 与 tag。
2. 用同一版本号重跑 Release 工作流。**工作流不会自动删旧 tag**：`校验版本号与 tag` 步骤发现 tag 已存在就失败， 避免把 tag 挪到新提交上、让历史 Release 的对比基准漂移。所以补发前必须先手工执行第 1 步。夜间构建（`*-nightly.*`）例外：同名 tag 允许覆盖，工作流会自己删掉同名 Release + tag 再重建。
3. 镜像平台：CNB 幂等重建（删旧 Release/Tag 再上传，tag 唯一性由 GitHub 侧保证）；GitLab 会删旧 Release 但保留 tag；Gitee 按页面删除后重跑同步。
4. 只想补发某个镜像：单独重跑对应 job 或手动触发其工作流；不要重新构建 GitHub Release，除非产物本身要换。

## 8. 已知缺口（发布前务必核对）

- `build/config.yml` 的初始 `info.version` 是 `0.0.1`；工作流会用 `prepare` 输出的 `version` 覆盖它，但 App 包内的 `Info.plist` 仍建议发布后抽查。
- `scripts/get-build-time.sh` 生成的是 UTC RFC3339；预发布比较依赖这个格式，不要手改成其它格式。
- 「更新内容」只认 `vX.Y.Z` / `vX.Y.Z-<rc|beta|alpha|nightly>.N` 形式的 tag（nightly 的 N 是日期，如 `.20260101`）；命名不合规的 tag 不会进入对比基准。同渠道对比：nightly 只和上一个 nightly 比，首次发该渠道回退到上一个正式版。
- tag 必须拉全（`release` job 的 `fetch-depth: 0`）：一个 tag 都没有时工作流直接失败，而不是发出一份「首次发布」的错误正文。
- 版本号比已有正式版旧、或 tag 已存在时，`release` job 会在发布前失败，需要先手工清理旧 tag 再重跑。
- Linux 包元数据版本由 `build/linux/Taskfile.yml` 的 `generate:*` 任务注入到 `nfpm.yaml` 的 `${VERSION}`：优先用 `VERSION_PKG`（`~` 代替 `-`），退回 `VERSION`，都没有时是 `0.0.0`（见 §1.1）。
- Linux 打包在 `ghcr.io/dtapps/golang/wails3:latest` 容器里跑，镜像自带 GTK4 + WebKitGTK 6.0 + gcc；appimagetool 靠 `APPIMAGE_EXTRACT_AND_RUN=1` 免 FUSE 运行。镜像若未包含 aarch64 版 appimagetool，`build-linux` 的 arm64 腿会在打包阶段失败。

## 9. 发布后验证清单

- [ ] GitHub Release 有 12 个安装包（macOS 2 / Windows 2 / Linux 8）+ 6 个 `updater-` 包 + `SHA256SUMS`（预发布另有 `BUILD_TIME` / `GIT_COMMIT`）。
- [ ] 正文含「发布信息」表（版本号 / 发布类型 / 提交 / 构建时间）与「校验」节，且 `updater-` 包、`BUILD_TIME`、`GIT_COMMIT` 没有出现在下载表格里。
- [ ] 预发布版本的 Release 标题带「(预发布)」且勾选了 Pre-release；正式版两者都不带。
- [ ] 下载 macOS arm64 包，`unzip` 后 `Kai.app` 能启动；`defaults read .../Info.plist CFBundleShortVersionString` 与版本一致。
- [ ] 下载 Windows 安装包，安装后 `Kai.exe` 能启动，版本号正确。
- [ ] 托盘「检查更新」能识别新版本；预发布版本能被预发布通道识别。
- [ ] CNB / GitLab / Gitee Release 页面能打开，附件直链可下载。
- [ ] `updater-` 包只出现在附件里，不出现在下载表格。
- [ ] 通知（如启用）已发出。
