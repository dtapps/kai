# 发布手册

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

只讲发版。CI 总览见 `ci/README.md`；secret 的权限与轮换见 `security-privacy.md`；更新器匹配规则见 `configuration.md` §4.4。

## 1. 版本规则

- 版本形如 `vMAJOR.MINOR.PATCH`，当前是 `v0.2.x`（最新 tag `v0.2.7`）。
- 版本号目前**手动**在 Release 工作流里输入，没有从 commit 自动推导。
- 输入只填数字部分（例如 `0.2.8`），工作流会拼成 tag `v0.2.8`；不要输入 `v0.2.8`，否则会得到 `vv0.2.8`。
- `buildinfo.Version` 由同一个输入经 `-ldflags` 注入；`v` 只属于 tag，不属于二进制版本串。
- 预发布用同一个工作流勾选 `prerelease`，不再有每日 Nightly。

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

| 输入           | 必填 | 说明                                     |
|----------------|------|------------------------------------------|
| `version`      | 是   | 数字版本，例如 `0.2.8`                   |
| `commit`       | 否   | 要构建的提交；留空用默认分支最新提交     |
| `prerelease`   | 否   | 勾选后发布为预发布（`prerelease: true`） |
| `nsis_version` | 否   | NSIS 版本，留空用 `3.13`                 |

job 顺序：

1. `prepare`：生成统一的 `BUILD_TIME`（`scripts/get-build-time.sh`）与 `GIT_COMMIT`。
2. `build`：矩阵构建 4 个组合。
3. `release`：汇总产物、生成 `SHA256SUMS` 与发布说明、发布 GitHub Release。
4. `sync-cnb` / `sync-gitee` / `sync-gitlab` / `sync-gitcode`：并行触发各平台同步。
5. `notify`：ServerChan3 通知（配置了 `SERVERCHAN_SENDKEY` 才发）。

构建矩阵：

| Runner           | GOOS      | GOARCH  | 打包任务                  |
|------------------|-----------|---------|---------------------------|
| `macos-26`       | `darwin`  | `arm64` | `darwin:package`          |
| `macos-26-intel` | `darwin`  | `amd64` | `darwin:package`          |
| `windows-2025`   | `windows` | `amd64` | `windows:package`（NSIS） |
| `windows-11-arm` | `windows` | `arm64` | `windows:package`（NSIS） |

Linux 构建已注释掉（`release.yml` 矩阵），当前不发布 Linux 产物。

SQLite 驱动：Windows 用 `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`）；macOS 用 `sqlite_mattn` + `CGO_ENABLED=1`（`EXTRA_TAGS=sqlite_mattn`）。

## 4. 产物与签名

### 4.1 产物命名

| 文件                                 | 用途                   | 是否出现在 Release 下载表 |
|--------------------------------------|------------------------|---------------------------|
| `Kai-<v>-darwin-arm64.zip`           | macOS Apple 芯片安装包 | 是                        |
| `Kai-<v>-darwin-amd64.zip`           | macOS Intel 安装包     | 是                        |
| `Kai-<v>-windows-amd64-install.exe`  | Windows x64 安装包     | 是                        |
| `Kai-<v>-windows-arm64-install.exe`  | Windows ARM64 安装包   | 是                        |
| `updater-Kai-<v>-darwin-<arch>.zip`  | macOS 自更新包         | 否（隐藏）                |
| `updater-Kai-<v>-windows-<arch>.zip` | Windows 自更新包       | 否（隐藏）                |
| `SHA256SUMS`                         | 全局校验和             | 资产有，下载表不列        |
| `BUILD_TIME` / `GIT_COMMIT`          | 仅预发布版本附带       | 资产有，下载表不列        |

macOS 产物是 `.app` 的 zip，不是 dmg。自更新包必须是 `.app` 归档，才能保留签名。

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
| GitLab | `sync-gitlab` 调 Trigger API，VERSION / PRERELEASE               | `SYNC_GITLAB_TRIGGER_TOKEN`                  | 跳过     |
| Gitea  | 无 Release 同步，只作代码镜像                                    | —                                            | —        |

- CNB 侧 `.cnb.yml` 的 `api_trigger_release` 会下载 GitHub 产物、删除同名 Release/Tag、重建 Release 并上传附件。
- GitLab 侧 `.gitlab-ci.yml` 的 `release` job 把附件传到 Generic Package Registry，再建 Release 与直链。
- Gitee 侧 `.workflow/release.yml` 由 Webhook 拉起，负责把资产搬到 Gitee Release。
- 所有同步 job 都“尽力而为”：失败只打 `::warning::`，不回滚 GitHub Release。

> **GitCode 当前不发布**：仓库里仍保留 `sync-gitcode` job 与 `.gitcode/workflows/release.yml`，但它们不在当前发布链路中；文档与发布 checklist 均按 GitHub / CNB / GitLab / Gitee 四个平台执行。

## 6. 更新源与产物对应

- 更新器只匹配 `updater-` 前缀的 `.zip` / `.tar.gz`，并按平台/架构字符串过滤（`pkg/wails-updater-providers/matcher.go`）。
- 稳定版：比较 tag。
- 预发布：读取 Release 附带的 `GIT_COMMIT` / `BUILD_TIME` 文件，与本地构建时间/commit 比较；缺文件会报「预发布版本缺少 git commit / build time 附带文件」。
- 下载后校验 `SHA256SUMS`。
- 中文界面默认走 CNB，英文默认走 GitHub；可用 `settings.json` 的 `updater.source` 强制。

## 7. 回滚与补发

1. GitHub：`gh release delete v<version> --yes --cleanup-tag`，或页面上删除 Release 与 tag。
2. 用同一版本号重跑 Release 工作流；工作流会先删除同名旧 Release（`--cleanup-tag`）再发布。
3. 镜像平台：CNB 会删旧 Release/Tag 再重建；GitLab 会删旧 Release 但保留 tag；Gitee 按页面删除后重跑同步。
4. 只想补发某个镜像：单独重跑对应 job 或手动触发其工作流；不要重新构建 GitHub Release，除非产物本身要换。

## 8. 已知缺口（发布前务必核对）

- `prepare` job 只声明了 `build_time` / `git_commit` 两个 output（`release.yml:51`），但「同步版本到构建资源」步骤引用了 `needs.prepare.outputs.version`（`release.yml:372`）。该表达式当前解析为空，可能把 `build/config.yml` 的 `info.version` 写成空值，进而影响 `Info.plist` 的 `CFBundleShortVersionString`。二进制内的 `Version` 由 `BUILD_ARGS` 的 `VERSION` 正确注入，但 App 包元数据需要发布后抽查。
- `build/config.yml` 的初始 `info.version` 是 `0.0.1`；发版前手动改成目标版本，或确认工作流已正确覆盖。
- `scripts/get-build-time.sh` 生成的是 UTC RFC3339；预发布比较依赖这个格式，不要手改成其它格式。
- Release 说明的「更新内容」取 `git log <prev_tag>..HEAD`；如果 tag 顺序不规范，说明会不准。

## 9. 发布后验证清单

- [ ] GitHub Release 有 4 个安装包 + 2/4 个 `updater-` 包 + `SHA256SUMS`（预发布另有 `BUILD_TIME` / `GIT_COMMIT`）。
- [ ] 下载 macOS arm64 包，`unzip` 后 `Kai.app` 能启动；`defaults read .../Info.plist CFBundleShortVersionString` 与版本一致。
- [ ] 下载 Windows 安装包，安装后 `Kai.exe` 能启动，版本号正确。
- [ ] 托盘「检查更新」能识别新版本；预发布版本能被预发布通道识别。
- [ ] CNB / GitLab / Gitee Release 页面能打开，附件直链可下载。
- [ ] `updater-` 包只出现在附件里，不出现在下载表格。
- [ ] 通知（如启用）已发出。
