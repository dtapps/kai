# GitLab 发布同步（原覆盖率方案已下线）

本仓库在多个平台有代码镜像（GitHub / CNB / GitLab / Gitee / Gitea / GitCode），但**发布只覆盖 GitHub、CNB、GitLab、Gitee 四个平台**；Gitea / GitCode 不发布 Release。**GitLab 侧现在只做一件事：把 GitHub Release 同步成 GitLab Release。** 覆盖率采集已于 2026-10-24 整体删除。

- 入口：`.gitlab-ci.yml`（只有 `release` 一个 job）
- 触发：GitHub Release 工作流的 `sync-gitlab` job 自动触发，或在 GitLab 手动运行流水线
- 产物：GitLab Release + 上传到 Generic Package Registry 的附件
- 历史覆盖率方案（五个 job / 自建 CI 镜像 / Cobertura）见本文 §6，仅作回溯

## 1. 为什么删掉覆盖率

删除前 `.gitlab-ci.yml` 有五条覆盖率相关 job：`check-image` / `build-image` / `merge-image`（自建 CI 镜像）+ `go-coverage` / `node-coverage`。删除原因（原文写在 `.gitlab-ci.yml` 头注释）：

1. **GitLab 自带覆盖率能力天花板低**：只有一个百分比（多 job 取算术平均）、MR diff 行级注释、默认分支一条折线；没有可下钻的 HTML 报告，不能按文件/目录看，也没有 flags 分组。
2. **同样的数据另外三处都有且更强**：CNB（`lines` / `diffLines` 硬红线）、Codecov（文件树 + patch + bundle + tests）、Coveralls（逐行 HTML）。
3. **最有价值的 MR 小部件和 diff 注释触发不了**：本仓库 GitLab 是 pull mirror，开发在 GitHub / CNB 一侧，不会在 GitLab 开 MR。
4. **成本高**：一次流水线约 10–16 compute minutes（含自建镜像固定开销），GitLab 免费私有项目只有 400 min/月，会挤掉发布同步额度——而发布同步是 GitLab 侧唯一不可替代的能力。

## 2. 触发方式

### 2.1 自动（主路径）

GitHub Release 发布完成后，`.github/workflows/release.yml` 的 `sync-gitlab` job 调用 GitLab Trigger API 创建流水线：

```text
POST /api/v4/projects/:id/trigger/pipeline
  variables[VERSION]=v1.2.3
  variables[PRERELEASE]=false
```

此时 `CI_PIPELINE_SOURCE == "trigger"`，`release` job 自动执行。

### 2.2 手动（补发 / 重发）

- **网页**：GitLab 项目 → CI/CD → Pipelines → Run pipeline，在变量区填 `VERSION`（可加 `PRERELEASE`）。
- **API**：同上 Trigger API，手动 POST。

`rules` 要求 `$VERSION` 非空，没填时**不会创建 job**，避免空跑一次必然失败的发布：

```yaml
rules:
  - if: $CI_PIPELINE_SOURCE == "trigger"
  - if: $CI_PIPELINE_SOURCE == "web" && $VERSION
```

支持的变量：

| 变量         | 必填 | 说明                                                   |
|--------------|------|--------------------------------------------------------|
| `VERSION`    | 是   | 版本号，可写 `1.2.3` 或 `v1.2.3`（脚本自动补 `v`）     |
| `PRERELEASE` | 否   | `true` / `false`，默认 `false`；非这两个值回落 `false` |

## 3. 运行环境

| 项   | 值                                                                |
|------|-------------------------------------------------------------------|
| 镜像 | `debian:trixie-slim`（**必须自带 bash**）                         |
| 预装 | `before_script` 现装 `curl` `jq` `ca-certificates`                |
| 检出 | `GIT_STRATEGY: none`，不检出代码，全部在 `mktemp -d` 临时目录完成 |
| 超时 | 30 分钟                                                           |
| 并发 | `interruptible: true`                                             |

**为什么不用 alpine + `apk add bash`**：Runner 在容器启动后、`before_script` 之前就探测并锁定 shell。在 `before_script` 里装 bash 改变不了已选定的 shell，整个 `script` 仍由 ash 执行；而脚本用了 bash 数组 `AUTH=(-H ...)`，ash 直接 syntax error。所以 bash 必须预装在镜像里。

## 4. 发布流程（job 内部）

1. **规范化版本号**：`1.2.3` → `v1.2.3`；读取 `PRERELEASE`。
2. **认证**：优先 `GL_RELEASE_TOKEN`（PAT，`api` scope）→ 否则 `CI_JOB_TOKEN`。
3. **读取 GitHub Release**：`GET https://api.github.com/repos/dtapps/kai/releases/tags/<tag>`；附件数为 0 直接失败。若配置了 `KAI_GITHUB_TOKEN` 则带 token（绕开匿名 60 次/小时限额），否则匿名。
4. **下载全部附件** 到临时目录 `dist/`。
5. **重写 Release 正文链接**：把 GitHub 的 `/releases/download/<tag>/<file>`、`/releases/tag/<tag>`、`/releases` 等替换为 GitLab 对应路径：
   - GitHub：`/releases/download/<tag>/<file>`
   - GitLab：`/-/releases/<tag>/downloads/<file>`
   - **顺序不能颠倒**（GitLab 多一个 `/-`，`download` 变 `downloads`）；只换域名会 404。
   - 兜底：正文里只要还残留 `github.com/dtapps/kai`，job 直接失败，避免发出点进去 404 的 release 页面。
6. **上传附件到 Generic Package Registry**（GitLab 没有 Release 二进制附件 API）：
   - `PUT /projects/:id/packages/generic/kai/<tag>/<url-encoded-file>`
   - 文件名可能含空格 / 中文（如 `安装包.dmg`），必须 `jq -sRr @uri` 编码，否则 curl 报 URL malformed。
   - `200/201` 成功；`403/409` 视为同版本重发、复用已有文件；其它状态码失败。
7. **幂等创建 Release**：
   - 先 `DELETE /projects/:id/releases/<tag>`（`200/204` 删除；`404` 无旧 release；其它仅告警）。
   - 再 `POST /projects/:id/releases`，`assets.links` 指向 Package Registry，并带 `direct_asset_path` 生成直链 `/-/releases/<tag>/downloads/<file>`。
   - tag 不存在时由 `ref: $CI_DEFAULT_BRANCH` 自动创建，指向默认分支最新提交。
8. **输出**：`${CI_PROJECT_URL}/-/releases/<tag>`。

## 5. 密钥与权限

| 变量 / Secret               | 位置          | 必填 | 用途                                                                              |
|-----------------------------|---------------|------|-----------------------------------------------------------------------------------|
| `CI_JOB_TOKEN`              | GitLab 自动   | 自动 | 默认认证；Releases API 与 Package Registry 均支持                                 |
| `GL_RELEASE_TOKEN`          | GitLab 变量   | 否   | 项目禁用 job token 权限时使用（PAT，`api` scope）；配置后自动切换                 |
| `KAI_GITHUB_TOKEN`          | GitLab 变量   | 否   | 只读 GitHub token，仅用于绕开 `api.github.com` 匿名限额；未配置时匿名回落，不报错 |
| `SYNC_GITLAB_TRIGGER_TOKEN` | GitHub Secret | 是   | GitHub Release 工作流用它 POST trigger API；未配置时同步 job 直接跳过             |

`KAI_GITHUB_TOKEN` 配置位置：GitLab → Settings → CI/CD → Variables；**Masked，但不要勾 Protected**（trigger 流水线的 ref 不是保护分支时取不到值）。名字与 `.cnb.yml` / Gitee 两处保持一致，源头是 `cnb.cool/dtapp/envs` 的 `github.yml`，改名要同步改那份（GitCode 当前不发布）。

`SYNC_GITLAB_TRIGGER_TOKEN` 配置位置：GitHub → Settings → Secrets → Actions，值为 GitLab 项目的 Pipeline trigger token（`glptt-...`）。未配置时 `.github/workflows/release.yml` 的 `sync-gitlab` job 直接跳过，不影响 GitHub Release 本身。

## 6. 历史覆盖率方案（已删除，仅回溯）

> 2026-10-24 删除。保留本节是为了以后需要恢复时知道原方案长什么样、踩过哪些坑。

原 `.gitlab-ci.yml` 的五个 job：

| Job             | 作用                                                                         |
|-----------------|------------------------------------------------------------------------------|
| `check-image`   | 算 `.gitlab/ci/Dockerfile` 内容哈希，查 registry 里 `ci:sha-<hash>` 是否存在 |
| `build-image`   | 缺镜像时在 amd64 runner 上原生 `buildctl` 构建                               |
| `merge-image`   | 用 `docker buildx imagetools create` 把 `latest` 指到新 sha                  |
| `go-coverage`   | `go test -coverprofile` → `gocover-cobertura` → Cobertura 报告               |
| `node-coverage` | `pnpm test:coverage` + `cobertura` reporter → Cobertura 报告                 |

当时的坑（恢复前必读）：

- **BuildKit rootless**：GitLab SaaS runner 没有 root daemon，必须用 rootless buildkit，并处理 `--oci-worker-no-process-sandbox`。
- **镜像判重不能用「Dockerfile 改没改」**：用内容哈希 `sha256sum .gitlab/ci/Dockerfile | cut -c1-12` 得到 `ci:sha-<hash>`，查 registry 是否已存在。
- **`gocover-cobertura` 要装进镜像**：Go 的 coverprofile 不是 Cobertura，GitLab `coverage_report` 只认 Cobertura / JaCoCo。
- **前端要额外的 `cobertura` reporter**：lcov 不被 `coverage_report` 识别。
- **`coverage:` 与 `artifacts:reports:coverage_report` 是两套机制**：前者只出百分比，后者只出行级注释，互不联动。
- **`--output` 只能推一个 tag**：`name=repo:a,repo:b` 会失败；多 tag 用 `merge-image` 的 `imagetools create -t` 补。
- **必须用带 bash 的镜像**：原因见 §3。

恢复时按上表重建 job，并确认 GitLab 项目仍是 pull mirror（否则 MR 相关能力依然用不上）。

## 7. 验证与排错

### 7.1 验证一次发布

1. GitHub Release 已存在且附件不为空。
2. GitLab → CI/CD → Pipelines，能看到 `trigger` 来源的流水线；`release` job 绿色。
3. 打开 `${CI_PROJECT_URL}/-/releases/<tag>`：
   - 正文链接全部指向 GitLab；
   - 每个附件都能通过 `/-/releases/<tag>/downloads/<file>` 下载。
4. 重复跑同一 `VERSION`：应删除旧 release 后重建，附件复用（403/409 分支）。

### 7.2 常见问题

| 现象                                     | 原因 / 处理                                                                                      |
|------------------------------------------|--------------------------------------------------------------------------------------------------|
| 手动运行流水线没有 `release` job         | 没填 `VERSION`；`rules` 要求 `$CI_PIPELINE_SOURCE == "web" && $VERSION`                          |
| `VERSION` 为空报错                       | trigger 调用未传 `variables[VERSION]`，或网页运行没填变量                                        |
| `401` / `403`                            | `CI_JOB_TOKEN` 权限被禁用且未配 `GL_RELEASE_TOKEN`；或 token scope 不是 `api`                    |
| 读取 GitHub Release `404`                | tag 不存在或版本号拼错；确认 GitHub 上已有该 tag 的 Release                                      |
| 下载附件失败 / `API rate limit exceeded` | 未配 `KAI_GITHUB_TOKEN`，共享 runner IP 撞上匿名限额；配置 token 后重试                          |
| 上传附件 `URL malformed`                 | 文件名未 URL 编码；脚本已用 `jq -sRr @uri`，若改过脚本注意保留                                   |
| 正文链接检查失败（job 主动 abort）       | 发布正文里还有 GitHub 链接；补一条 `sed` 规则或手动改正文后重发                                  |
| `syntax error` / `AUTH: not found`       | 镜像不是 bash（如误改回 alpine）；`debian:trixie-slim` 不要动                                    |
| 附件下载 404                             | Release 链接用了 `/releases/download/...`（GitHub 风格）；应为 `/-/releases/<tag>/downloads/...` |
| 同版本重发后附件仍旧                     | Package Registry 对同版本同文件名返回 403/409，脚本复用旧文件；要换文件需删 Package 或升版本     |
