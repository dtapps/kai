# CI 总览

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master）

先看这张地图，再进专题。镜像细节见 `ci-image.md`，Codecov / Coveralls / GitLab 见各自专题；本文只做总览、触发条件、secret 清单与「新增流水线要改哪里」。

## 1. 平台分工

| 平台    | 角色                                                    | 入口                             |
|---------|---------------------------------------------------------|----------------------------------|
| GitHub  | 主开发 CI（push/PR 检查、覆盖率上报、发布）             | `.github/workflows/`             |
| CNB     | 硬门禁与安全扫描（格式/检查/覆盖率/SCA/AI 评审/知识库） | `.cnb.yml` + `.cnb/workflows/`   |
| GitLab  | 只做发布同步，不跑测试                                  | `.gitlab-ci.yml`                 |
| Gitee   | 只做发布同步（Gitee Go）                                | `.workflow/release.yml`          |
| GitCode | 工作流保留，当前不发布                                  | `.gitcode/workflows/release.yml` |
| Gitea   | 仅代码镜像，无 CI/Release                               | —                                |

## 2. GitHub Actions（5 个）

| Workflow       | 文件            | 触发                                     | 关键内容                                                                                        |
|----------------|-----------------|------------------------------------------|-------------------------------------------------------------------------------------------------|
| PushRequest    | `push.yml`      | push / PR 所有分支                       | GHCR 镜像内：`make deps` → bindings/icons/build-assets/sqlc/i18n → `make format` → `make check` |
| Codecov        | `codecov.yml`   | push / PR                                | Go + 前端覆盖率、前端 bundle、JUnit 测试结果 → Codecov                                          |
| Coveralls      | `coveralls.yml` | push / PR                                | 单 job 跑 Go + 前端覆盖率，两个 flag 上传 Coveralls                                             |
| Release        | `release.yml`   | 仅 `workflow_dispatch`                   | 4 个构建组合 → GitHub Release → CNB/Gitee/GitLab 同步（GitCode 保留未启用）→ 通知               |
| Build CI Image | `ci-image.yml`  | `Dockerfile` 变更、每周一 UTC 0 点、手动 | 构建并推送 `ghcr.io/<repo>/ci:latest` + sha 标签，保留最近 5 个版本                             |

共同点：

- 所有消费镜像的 job 都用 `container: ghcr.io/${{ github.repository }}/ci:latest`；首次使用前必须先跑一次 `Build CI Image`。
- Go module、Go build cache、pnpm store 都做了缓存；key 分别按 `go.sum` / `pnpm-lock.yaml` 命中。
- push / PR 工作流都开了 concurrency，同分支新推送会取消旧运行。

## 3. CNB（10 个 workflow）

`.cnb.yml` 用 `include:` 引入 `.cnb/workflows/*.yml`。

| Workflow          | 文件                    | 触发                              | 作用                                                                        |
|-------------------|-------------------------|-----------------------------------|-----------------------------------------------------------------------------|
| push              | `push.yml`              | push                              | 生成链 → `make format` → `make check`（与 GitHub 重复但独立）               |
| push-coverage     | `push-coverage.yml`     | push                              | Go / 前端全量覆盖率，`testing:coverage` 上报                                |
| pull-coverage     | `pull-coverage.yml`     | PR                                | Go / 前端增量覆盖率（`diffLines` 仅 PR 事件计算）                           |
| cnb-sechook       | `cnb-sechook.yml`       | PR                                | 敏感信息 / 代码扫描摘要                                                     |
| code-review       | `code-review.yml`       | PR                                | AI 代码评审（`deepseek-v4.1-flash`，可评论到 PR）                           |
| sca-vulnerability | `sca-vulnerability.yml` | PR                                | 开源组件漏洞扫描，`failOnSeverity: high`                                    |
| sca-license       | `sca-license.yml`       | PR                                | License 扫描，`failOnSeverity: high`                                        |
| sca-sbom          | `sca-sbom.yml`          | PR                                | 生成 SBOM Delta 到 `.cnb-sca/sbom-delta.json`                               |
| codewiki          | `codewiki.yml`          | `tag_push`                        | 生成 Code Wiki 到 `codewiki/`                                               |
| knowledge-base    | `knowledge-base.yml`    | push / issue.open / issue.comment | 把 `docs/**/*.md` + README / CONTRIBUTING / SECURITY / CHANGELOG 入库知识库 |

CNB 的检查与覆盖率都现场 `docker: build: .cnb/ci/Dockerfile`，并用 copy-on-write 卷缓存 `/root/.cache/go-build`、pnpm store、`/go/pkg/mod`。

## 4. GitLab / Gitee（GitCode 保留）

| 平台    | 触发                                                                                  | 产物                                                                                       |
|---------|---------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| GitLab  | GitHub `sync-gitlab` 调 `POST /projects/:id/trigger/pipeline`；或网页手动填 `VERSION` | GitLab Release，附件在 Generic Package Registry，直链 `/-/releases/<tag>/downloads/<file>` |
| Gitee   | GitHub `sync-gitee` 调 Gitee Go Webhook（eventType `kai-release`）                    | Gitee Release，正文渲染为 HTML                                                             |
| GitCode | 工作流保留（`sync-gitcode` / `.gitcode/workflows/release.yml`），**当前不发布**       | —                                                                                          |

共同约束：

- 都从 GitHub Release 下载产物，不重新编译。
- 都会把正文里的 GitHub 链接替换成自己的域名；GitLab 侧如果仍有残留 GitHub 链接会直接失败（避免发出 404 页面）。
- token 未配置时对应 job 跳过，不影响 GitHub Release。

## 5. 覆盖率三处口径

| 平台      | 入口                                                            | 看什么                                 | 门禁状态                                 |
|-----------|-----------------------------------------------------------------|----------------------------------------|------------------------------------------|
| CNB       | `push-coverage.yml` / `pull-coverage.yml` 的 `testing:coverage` | `lines` / `diffLines` 徽章与红线       | `lines` / `diffLines` 均注释，**未启用** |
| Codecov   | `codecov.yml` + `.github/codecov.yml`                           | 文件树、patch、bundle、tests、6 个组件 | 全部 `informational`，不阻塞             |
| Coveralls | `coveralls.yml`                                                 | 逐行 HTML，`go` / `node` 两个 flag     | 由 Coveralls 后台配置                    |

命令与产物：

```bash
# Go（与 CI 一致的范围）
go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...

# 前端
cd frontend && pnpm test:coverage
```

- Go 覆盖率 → `coverage.out`；前端 → `frontend/coverage/lcov.info`。
- Codecov 还消费 `go-test-report.xml`（gotestsum）与 `frontend/test-report.xml`（Vitest JSON 经脚本转 JUnit）。
- 当前基线见 `testing.md` §5。

## 6. Secret 清单

### 6.1 GitHub 仓库 secrets

| Secret                                        | 用途                                         | 未配置时             |
|-----------------------------------------------|----------------------------------------------|----------------------|
| `GITHUB_TOKEN`                                | Actions 内置权限；发布 Release、触发其它平台 | 由 GitHub 自动提供   |
| `UPDATER_TOKEN`                               | 编进客户端的 GitHub 只读 token（更新器用）   | 更新器 GitHub 源受限 |
| `CNB_TOKEN`                                   | 编进客户端的 CNB token（更新器用）           | CNB 源可能 401       |
| `POSTHOG_TOKEN` / `POSTHOG_PROJECT_ID`        | 匿名统计构建期注入                           | 统计自动 no-op       |
| `WINDOWS_SIGN_*`（4 个）                      | Windows Authenticode                         | 跳过签名             |
| `APPLE_*`（3 个）                             | macOS 签名与公证                             | 跳过签名/公证        |
| `SYNC_CNB_TOKEN`                              | 触发 CNB 同步构建（仅 trigger 权限）         | 跳过 CNB 同步        |
| `SYNC_GITEE_WEBHOOK_URL` / `SYNC_GITEE_TOKEN` | 触发 Gitee Go 流水线                         | 跳过 Gitee 同步      |
| `SYNC_GITLAB_TRIGGER_TOKEN`                   | 触发 GitLab 流水线                           | 跳过 GitLab 同步     |
| `SYNC_GITCODE_TOKEN`                          | 保留，当前不参与发布                         | 忽略                 |
| `SERVERCHAN_SENDKEY`                          | 发布完成通知                                 | 跳过通知             |
| `CODECOV_TOKEN`                               | 上传 Codecov                                 | 上传失败/跳过        |
| `COVERALLS_REPO_TOKEN`                        | 上传 Coveralls                               | 上传跳过             |

### 6.2 其它平台变量

| 平台    | 变量                                                                                              | 说明                                           |
|---------|---------------------------------------------------------------------------------------------------|------------------------------------------------|
| CNB     | `KAI_GITHUB_TOKEN`（来自 `cnb.cool/dtapp/envs` 的 `github.yml`）、`CNB_TOKEN`、`CNB_API_ENDPOINT` | 前两个用于绕开 GitHub API 限流/调 CNB Open API |
| GitLab  | `KAI_GITHUB_TOKEN`、`GL_RELEASE_TOKEN`（可选，PAT）、`CI_JOB_TOKEN`                               | 无 `GL_RELEASE_TOKEN` 时用 job token           |
| Gitee   | `VERSION`、`PRERELEASE`、`KAI_GITHUB_TOKEN`                                                       | 由 webhook `params` 传入或全局参数配置         |
| GitCode | `GITCODE_TOKEN`、`KAI_GITHUB_TOKEN`                                                               | 保留，当前不发布                               |

详细权限与轮换见 `security-privacy.md`。

## 7. 新增 / 修改流水线时要改哪些文件

| 改动                 | 必须同步                                                                                                                               |
|----------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| 新增 CNB workflow    | `.cnb/workflows/<name>.yml` + `.cnb.yml` 的 `include`                                                                                  |
| 新增 GitHub workflow | `.github/workflows/<name>.yml`                                                                                                         |
| 新增 CI 工具         | `.cnb/ci/Dockerfile` + `.github/ci/Dockerfile`（两份都改）                                                                             |
| 新增覆盖率任务       | `.cnb/workflows/{push,pull}-coverage.yml`、`.github/codecov.yml`、`codecov.yml` / `coveralls.yml`、`docs/ci/README.md`                 |
| 发布相关             | `.github/workflows/release.yml`、`.cnb.yml`、`.gitlab-ci.yml`、`.workflow/release.yml`、`.gitcode/workflows/release.yml`、`release.md` |
| 文档目录变化         | `.cnb/workflows/knowledge-base.yml` 的 `include`（`docs/**/*.md` 递归覆盖子目录）                                                      |
| 新增必过检查         | 分支保护 / 合并规则里把新 job 加为 required status checks                                                                              |

## 8. 专题文档

| 文档                                    | 内容                                                |
|-----------------------------------------|-----------------------------------------------------|
| [ci-image.md](../ci-image.md)           | CNB / GitHub 两份 Dockerfile 的差异、镜像分发、排错 |
| [codecov.md](../codecov.md)             | Codecov 三支柱、flag、组件、上传与排错              |
| [coveralls.md](../coveralls.md)         | Coveralls 单 job 双 flag、token、排错               |
| [gitlab-ci.md](../gitlab-ci.md)         | GitLab 发布同步、历史覆盖率方案                     |
| [coverage-go.md](../coverage-go.md)     | Go 覆盖率模板（可复制到其他项目）                   |
| [coverage-node.md](../coverage-node.md) | 前端覆盖率模板                                      |
| [testing.md](../testing.md)             | 测试分层与本地命令                                  |
