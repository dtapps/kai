# docs 文档规划与缺口分析

> 分析时间：2026-10-26 · 范围：`docs/`（24 个文件 / 9,634 行）、根目录 `README*.md`、GitHub / CNB / GitLab 三套流水线 + Gitee 发布同步
>
> 目的：回答「docs 还缺哪些文档」，给出优先级、内容大纲与维护机制。
>
> **2026-10-27 更新**：P0（6 篇 + 可选架构）与 P1（9 篇 + 10 条 ADR）已全部落盘，索引见 `docs/README.md`；本文保留原始缺口分析作为记录，P2 仍未创建。

---

## 0. 结论先行

1. **现有 docs 不是一套文档体系，而是两类过程的副产物**：
   - 16 篇 2026-08-08 ~ 09-01 的开发日志（8,113 行，占 85%）；
   - 6 篇 CI / 覆盖率专题（970 行）；
   - 1 篇项目规划 `PLAN.md`（276 行，已按现状更新）。
2. **四类核心读者没有对应文档**（2026-10-26 盘点时；现已补齐，见 `docs/README.md`）：
   - 新贡献者：无开发环境、代码生成、测试指南；
   - 使用者：无安装 / 配置 / 排障手册（只有 README 的产品介绍）；
   - 维护者：无发布流程、CI 总览、平台能力矩阵（架构主干已并入 PLAN.md）；
   - 未来的自己：关键决策只散落在 8,113 行日志里，缺 ADR。
3. **准确性债务已于 2026-10-26 全部处理**：过期专题重写、断链修复、README/中英文/构建元数据统一（详见 §3）。当前主要缺口转为**结构性问题**（无索引 / 无分层 / 无归档）与新读者文档。
4. **P0 6 篇 + P1 9 篇已补齐**（2026-10-27）；`architecture.md` 未单独拆出，主干仍在 `PLAN.md`；P2 7 篇未创建。

---

## 1. 现状盘点

### 1.1 文件清单

| 文件                                       | 行数   | 性质                                         | 状态                                                          |
|--------------------------------------------|-------:|----------------------------------------------|---------------------------------------------------------------|
| `2026-08-08.md` ~ `2026-08-18.md`（12 篇） | 6,000+ | 开发日志（任务/根因/改动/遗留）              | 历史快照，已冻结                                              |
| `2026-08-20.md` ~ `2026-09-01.md`（4 篇）  |   ~600 | 同上                                         | 历史快照，已冻结                                              |
| `PLAN.md`                                  |    276 | 项目规划与架构现状                           | **已更新**：技术栈/目录/里程碑/约束对齐当前实现               |
| `ci-image.md`                              |    160 | CNB / GitHub 两份 CI Dockerfile 差异与排错   | **已更新**：codecov.yml 名称、仅 amd64、diff 规模、表格修复   |
| `codecov.md`                               |    147 | Codecov 三支柱（覆盖率 / bundle / 测试分析） | **已更新**：action v7、容器镜像、Vitest 5、CNB 红线未启用状态 |
| `coveralls.md`                             |    163 | Coveralls 接入                               | **已重写**：官方 Action 单 job 双 flag，不再有 goveralls      |
| `coverage-go.md`                           |    155 | 「可复制到其他项目」的 Go 覆盖率实现模板     | **已更新**：测试分布、路径、红线未启用状态                    |
| `coverage-node.md`                         |    199 | 同上，前端模板                               | **已更新**：vitest 配置、排除范围、红线状态                   |
| `gitlab-ci.md`                             |    146 | GitLab 发布同步                              | **已重写**：原覆盖率方案移入历史章节                          |

### 1.2 结构性问题

- **没有索引**：`docs/` 无 `README.md`，新人和 AI 都无法判断「该读哪篇、哪篇还有效」。
- **没有分层**：日志、规划、CI 运维混在根目录；日志按日期命名，专题按主题命名，无统一前缀/目录。
- **没有归档**：08–09 月的日志与已失效的 GitLab 覆盖率方案仍是「当前文档」。
- **没有图片资源目录**：架构图、界面截图无处安放（docs 下 0 个非 md 文件）。
- **日志断更**：最后一篇是 2026-09-01，但 9 月至 10 月的 120+ 次提交（CI 双镜像、Codecov、Coveralls、GitLab 发布同步、前端测试）只存在于 commit message，没有过程记录。
- **与知识库耦合**：`.cnb/workflows/knowledge-base.yml` 以 `include` 覆盖 `docs/**/*.md` + README / CONTRIBUTING / SECURITY / CHANGELOG 入库，日志与模板会被当作知识检索；`codewiki.yml` 在 tag 时生成 Code Wiki。

---

## 2. 缺口矩阵（按读者 × 场景）

| 读者      | 场景                     | 现有                | 缺口                                            |
|-----------|--------------------------|---------------------|-------------------------------------------------|
| 新贡献者  | 跑起来、改代码、跑测试   | 无                  | 环境准备、代码生成链、测试/覆盖率、常见坑       |
| 新贡献者  | 看懂代码                 | PLAN.md（已更新）   | 事件/IPC 细节、数据流图、ADR                    |
| 使用者    | 安装、授权、配引擎、排障 | README 功能表       | 用户手册、引擎 API Key 申请、OCR 安装、FAQ      |
| 维护者    | 发版、同步 3 个镜像平台  | 无                  | 发布手册、版本规则、产物矩阵、回滚              |
| 维护者    | 改 CI                    | 6 篇分散专题        | CI 总览（workflow 地图、密钥、触发条件）        |
| 维护者    | 升依赖 / 换镜像          | 无                  | 维护手册（Dependabot、镜像版本、文档过期检查）  |
| 安全/合规 | 漏洞披露、隐私、密钥     | 无                  | SECURITY / PRIVACY、凭据加密说明、SBOM/SCA 流程 |
| 所有人    | 「这条为什么这么写」     | 埋在 8,113 行日志里 | ADR 决策记录                                    |

---

## 3. 准确性债务（2026-10-26 已修复）

> 下列问题已全部处理，保留记录供追溯。后续如再次偏离实现，按 §7 的维护机制回归。

| #  | 位置                                     | 原问题                                                                             | 处理结果                                                                                                           |
|----|------------------------------------------|------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| 1  | `PLAN.md`                                | 技术栈写 Vue 3 + Naive UI + Pinia，目录含 `SelectionPopup.vue`，M6 标「待开始」    | 重写为《Kai 项目规划与架构现状》：Svelte 5 + Tailwind 4 + Vite 8、真实目录与包职责、M1–M6 全部完成、约束与命令更新 |
| 2  | `coveralls.md`                           | 整篇按 goveralls CLI + 并行 job + 镜像预装描述                                     | 按当前官方 Action 单 job 双 flag 重写，补充 token、缓存、排错                                                      |
| 3  | `gitlab-ci.md`                           | 描述已删除的五个覆盖率 job                                                         | 重写为「GitLab 发布同步」，原方案与坑点收进 §6 历史章节                                                            |
| 4  | `coverage-go.md`                         | 「全部测试在 configstore / updater」、红线为启用态、`.cnb/Dockerfile` 路径         | 更新测试分布（26 个 `_test.go`）、`.cnb/ci/Dockerfile`、三卷缓存、红线未启用状态与恢复方式                         |
| 5  | `coverage-node.md`                       | vitest 配置/排除范围过时、基线 1.43% 过时、红线为启用态、Dockerfile 路径           | 按当前 `vitest.config.ts` 重写：`@bindings` 指向 `frontend/bindings`、exclude 仅 test/d.ts、红线未启用             |
| 6  | `codecov.md` / `.github/codecov.yml`     | 引用 `coverage.yml`、action v5、`ubuntu-24.04 + apt` 运行环境、Vitest 3.2.7        | 改为 `codecov.yml`、action v7、GHCR 容器镜像、Vitest 5、Go 测试分布；配置头注释同步                                |
| 7  | `ci-image.md`                            | 引用 `coverage.yml`、声称多架构 amd64+arm64、diff 21 行、两处表格被竖线截断        | 改为 `codecov.yml`、仅 amd64 + 「加回 arm64」章节、diff 约 60 行、表格重写                                         |
| 8  | `.gitlab-ci.yml`                         | 引用不存在的 `docs/2026-10-23.md`                                                  | 改为 `docs/gitlab-ci.md §6`                                                                                        |
| 9  | `README.md` / `README_EN.md` / `build/*` | 「划词」与功能表不一致、Nightly 已取消、Wails beta.9、自定义标题栏已知问题已不存在 | 统一为「复制键取词翻译」；改为预发布；beta.34；删除过期已知问题；增加文档章节；重新生成 build 资源                 |
| 10 | 覆盖率门禁表述                           | 多处把 CNB 红线写成已启用                                                          | 统一改为「预留、当前注释未启用」，并说明恢复方式                                                                   |

---

## 4. 待补文档清单（P0/P1 已完成）

> 2026-10-27：P0 的 6 篇与 P1 的 9 篇已落盘，`architecture.md` 未单独拆出；P2 仍未创建。以下保留原始大纲。
>
> 工作量按「熟悉项目的人写 + 对照代码核对」估算。P0 = 不做会持续付出沟通/返工成本；P1 = 团队扩张或多平台维护前必须有；P2 = 锦上添花。

### P0（建议本周完成，6 篇 + 1 篇可选）

#### P0-1 `docs/README.md` — 文档索引与写作规范
- 文档地图（按读者分组）、每篇一行摘要与「最后核对版本/日期」；
- 命名规则（专题用 `kebab-case.md`，平台/CI 放 `docs/ci/`，日志与过期方案放 `archive/`）；
- 写作约定：先写结论、代码路径可点击、命令可复制、不写「未来会…」式承诺；
- 明确「哪些文档随代码同步更新」（配置字段、事件、CI 任务）。

#### P0-2（可选）`docs/architecture.md` — 架构细节（主干已并入 PLAN.md）
- 说明：更新后的 `docs/PLAN.md` 已覆盖主干；若需要图/表更细，再从中拆出本文。
- 运行时拓扑：单进程、四窗口（`translate` / `settings` / `screenshot` / updater）、系统托盘、单实例锁；
- 分层与依赖方向：`main.go` 组装 → `internal/service`（RPC 门面）→ `internal/translate`（编排）/ `internal/settings` / `internal/hotkey` / `internal/execkey` / `internal/selection` → `internal/engine` + 三个 store；
- 21 个 internal 包的职责一句话表；
- 四条主数据流：输入翻译、截图翻译（含改语言重翻）、复制键翻译、更新检查；
- 事件清单（`internal/events/events.go` 的 13 个事件、payload、谁发谁收）；
- 存储：`settings.json`、`config.db`（引擎凭据，设备指纹派生密钥加密）、`history.db`、`httplog.db`；
- 平台差异与构建标签（darwin/windows/linux、CGO、Swift bridge dylib）；
- 关键第三方选型理由（Wails 3 beta、Svelte 5、viper、sqlc、PostHog、updater providers）。

#### P0-3 `docs/development.md` — 开发与代码生成
- 工具链版本：Go 1.27.3、Node/pnpm 12.14、Wails 3 beta.34、sqlc 1.31.1、golangci-lint、govulncheck；
- 首次初始化：`make setup` / `make deps` / `make dev`；
- 代码生成链：`make bindings`（Wails 绑定）、`icons`、`build-assets`、`sqlc`、`i18n`（split → 合并）、`swift-build`；说明各自产物是否入库；
- 日常校验：`make format` / `make lint-go` / `make lint-frontend` / `make test-go` / `make fuzz-go` / `make vuln-go` / `make check` / `make check-cross`；
- 已知坑（每条都写「现象 → 原因 → 解法」）：`frontend/dist` 需占位目录、`go build .` 而非 `./...`、`-vet=off` 的原因、macOS 缺 `libkai_bridge.dylib`、Windows WebView2、macOS 辅助功能/屏幕录制授权、sqlc 生成代码勿手改。

#### P0-4 `docs/configuration.md` — 配置与数据目录
- 数据目录：正式 `~/.kai/`、开发 `~/.kai.dev/`；`data/`（三库）、`logs/`（`kai.log` / `frontend.log` / `kai-bridge.log`）；
- `settings.json` 全字段表：`language`、`theme`、`default_from/to`、`hotkeys.input/screenshot`、`execkeys.copy`（含 `fallback`）、`auto_clipboard` + `copy_key_snapshot`、`tts`、`http_log`、`log`、`proxy`、`dns_configs`、`updater`、`analytics_*`；每项默认值 + 取值范围 + 是否暴露 UI；
- 引擎配置在 `config.db`：字段、启停、多实例、凭据加密方式与「删库即重置」的边界；
- 代理 / DNS / 日志 / 更新源的行为与优先级；
- 构建期注入变量（`VERSION` / `BUILD_TIME` / `GIT_COMMIT` / `DEV` / `GITHUB_TOKEN` / `CNB_TOKEN` / `POSTHOG_*`）与 `.env` 的关系。

#### P0-5 `docs/release.md` — 发布手册
- 版本规则（当前 `v0.2.x`）、tag 与 `VERSION` 的关系；
- GitHub Release 工作流：手动输入 version / commit / prerelease / nsis_version；产物矩阵（darwin/win × amd64/arm64，dmg / nsis / msix）、Windows Authenticode 签名 secret、macOS 公证（如有）；
- 发布后同步链：CNB（`.cnb.yml`）、GitLab（trigger API）、Gitee；各平台 secret 缺失时的降级行为；Gitea / GitCode 不发布 Release；
- 更新源（GitHub / CNB）与界面语言的对应关系、预发布通道判断逻辑；
- 失败排查与回滚（删 tag / 重跑 job / 手动触发同步）；
- 每季度校验一次发布 checklist。

#### P0-6 `docs/user-guide.md` — 用户手册
- 安装（macOS dmg / Windows 安装包）、首次启动引导；
- macOS 授权：辅助功能（复制键）、屏幕录制（截图）；
- 触发方式与快捷键（输入 `⌥A`、截图 `⌥S`、复制键），如何录制/禁用；
- 各翻译来源的申请入口与配置字段（DeepL / Google / OpenAI / Anthropic / Gemini / 百度 / 腾讯 / 有道 / macOS 系统翻译）；
- OCR：macOS 系统识别 vs Windows 安装 tesseract；
- 自动更新、界面语言、主题；
- FAQ（已知问题从 README §6 迁移并扩写）、日志获取方式。

#### P0-7 `docs/ci/README.md` — CI 总览（入口文档）
- GitHub Actions（5 个 workflow）为主，CNB（10 个）做门禁/覆盖率/安全扫描，GitLab 只做发布同步，Gitee 由 GitHub 工作流推送；GitCode workflow 保留但当前不发布，Gitea 仅代码镜像；
- 每个 workflow 的触发条件、关键 job、所需 secret、产物；
- 覆盖率三处口径：CNB 内置 `testing:coverage`、Codecov、Coveralls（各自「看什么、红线在哪」）；
- 指向现有专题：`ci-image.md`、`codecov.md`、`coveralls.md`、`gitlab-ci.md`（修好后）；
- 新增一条工作流时该改哪些文件（避免只改一处）。

### P1（迭代期内补，9 篇）

| 文档                  | 必须覆盖的内容                                                                                                                                        | 依据                                                                                      |
|-----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------|
| `ipc-api.md`          | Service 方法清单与签名、13 个事件的 payload、窗口名常量、前端 `bindings` 生成约定、新增 RPC/事件的步骤                                                | `internal/service/*_wrapper.go`、`internal/events/events.go`、`internal/model/windows.go` |
| `engines.md`          | 翻译引擎能力矩阵（语言、鉴权、超时、限流、错误码）、OCR 提供方矩阵、新增引擎 checklist（注册、默认值、i18n、测试）                                    | `internal/engine/*`、`internal/service/engine_wrapper.go`                                 |
| `i18n.md`             | split 文件结构（`locales/split/<locale>/*.json`）、合并脚本、key 命名、`Locale` 与 `Language` 两套体系不可混用、新增语言步骤、前后端对齐、死 key 清理 | `scripts/merge-*-i18n.*`、`internal/i18n`、`frontend/src/i18n`                            |
| `testing.md`          | 测试分层（Go/前端）、覆盖率命令与当前基线、为何「红线暂未启用」、fuzz 用法、`-vet=off` 的原因、JUnit/Cobertura 产物供 Codecov/Coveralls 消费          | `.cnb/workflows/*-coverage.yml`、`vitest.config.ts`                                       |
| `data-storage.md`     | 三库 schema、sqlc 生成流程（`query.sql` + `sqlc.yaml`）、迁移策略、清理/重置、跨库引用（history.engine_id → config.engines.id 无外键）                | `internal/*store/schema.sql`、`Makefile:64`                                               |
| `platforms.md`        | 平台能力矩阵（系统翻译/系统 OCR/截图触发/自动更新/TTS）、构建标签、CGO 开关、Swift 桥接、窗口标题栏差异                                               | `internal/service/accessibility_*.go`、`README.md §3`                                     |
| `security-privacy.md` | 漏洞披露渠道、secret 管理（5 平台 token）、凭据加密（机器指纹派生）、依赖扫描（govulncheck / CNB SCA / SBOM）、PostHog 采集边界与关闭方式             | `internal/configstore/crypto.go`、`.cnb/security/`、`README.md §2`                        |
| `troubleshooting.md`  | 按现象索引：窗口被遮挡、快捷键不生效、OCR 失败、更新检查失败、代理/DNS、日志位置、如何复现与上报                                                      | README §6、日志包、08-24/09-01 日志                                                       |
| `adr/`                | 决策记录（见 §6 候选清单）                                                                                                                            | 08–10 月日志                                                                              |

### P2（可选，7 篇）

| 文档             | 说明                                                                                     |
|------------------|------------------------------------------------------------------------------------------|
| `logging.md`     | 日志分级、滚动/压缩/清理、HTTP 日志库与脱敏、前端日志转发                                |
| `analytics.md`   | 事件字典（名称/触发/属性/脱敏规则）、新增事件流程、关闭开关的行为                        |
| `style-guide.md` | Go/TS/Svelte 约定、错误包装与 i18n、注释风格、提交信息规范                               |
| `glossary.md`    | 术语：注册键 vs 执行键、复制键、会话（session）、引擎 ID、窗口名、`Locale` vs `Language` |
| `maintenance.md` | Dependabot 合并策略、CI 镜像升级、Wails beta 升级、文档季度审查清单                      |
| `design/`        | 界面截图、交互稿、图标源文件（当前 docs 无任何图片资源）                                 |
| `archive/`       | 过期方案与开发日志归档（PLAN、GitLab 覆盖率、08–09 日志）                                |

---

## 5. 建议目录结构

```text
docs/
├── README.md                  # 索引 + 写作规范
├── architecture.md            # 架构现状（含 IPC / 数据流概览）
├── development.md             # 开发环境与代码生成
├── configuration.md           # 配置项与数据目录
├── ipc-api.md                 # Service / 事件 / bindings
├── engines.md                 # 翻译与 OCR 引擎
├── i18n.md                    # 国际化
├── testing.md                 # 测试与覆盖率
├── data-storage.md            # 数据库与迁移
├── platforms.md               # 平台能力与构建标签
├── release.md                 # 发布与多平台同步
├── user-guide.md              # 用户手册
├── troubleshooting.md         # 排障
├── security-privacy.md        # 安全与隐私
├── logging.md                 # 日志（P2）
├── analytics.md               # 统计事件字典（P2）
├── style-guide.md             # 代码与提交规范（P2）
├── glossary.md                # 术语（P2）
├── maintenance.md             # 维护手册（P2）
├── ci/
│   ├── README.md              # CI 总览（P0-7）
│   ├── ci-image.md            # 现有
│   ├── codecov.md             # 现有
│   ├── coveralls.md           # 现有（需重写）
│   └── gitlab-ci.md           # 现有（需重写）
├── adr/                       # 决策记录
│   ├── 0001-copy-key-instead-of-ax.md
│   ├── 0002-domain-services-split.md
│   ├── 0003-hidden-window-webview2.md
│   ├── 0004-windows-gpu-flags.md
│   ├── 0005-single-instance.md
│   ├── 0006-native-titlebar-event-close.md
│   ├── 0007-coverage-platform-choice.md
│   └── 0008-update-source-routing.md
├── guides/                    # 可复用模板（coverage-go / coverage-node 移入）
├── archive/                   # 过期文档与日志
│   ├── PLAN-2026-08.md
│   ├── gitlab-coverage-2026-10.md
│   └── logs/2026-08/ 2026-09/
└── assets/                    # 截图与图
```

根目录另建议补：`CONTRIBUTING.md`（指向 `docs/development.md`）、`SECURITY.md`（指向 `docs/security-privacy.md`）、`CHANGELOG.md`（Keep a Changelog + 版本小节）。

---

## 6. 候选 ADR（从日志中提炼）

| ADR  | 主题                                                                    | 来源                                       |
|------|-------------------------------------------------------------------------|--------------------------------------------|
| 0001 | 用复制键 + robotgo 模拟 Cmd+C 取代 AX 选区读取                          | `2026-08-08.md`                            |
| 0002 | AppService 大泥球拆分为领域 Service                                     | `2026-08-09.md`                            |
| 0003 | 主题/语言 auto 以系统为唯一来源，移除 naive 兜底                        | `2026-08-10.md`、`2026-08-12.md`           |
| 0004 | 窗口一律 `Hidden: true` 创建，规避 WebView2 controller 竞态（80010108） | `2026-08-24.md`                            |
| 0005 | Windows 全局注入 GPU 兜底参数（`--disable-gpu*`）                       | `2026-08-24.md`                            |
| 0006 | 单实例锁实现方式                                                        | `2026-08-25.md`                            |
| 0007 | 翻译/更新窗口改原生标题栏，关闭清空走事件                               | `2026-08-20.md`、`2026-09-01.md`           |
| 0008 | 覆盖率平台取舍：保 CNB + Codecov + Coveralls，删 GitLab 覆盖率          | `.gitlab-ci.yml` 头注释、`2026-10-24` 提交 |
| 0009 | Markdown 用 markdownfmt 取代 prettier                                   | 提交 `b76ad72`                             |
| 0010 | 更新源双 provider（GitHub/CNB）按界面语言路由                           | `2026-08-15.md`、`README.md §4`            |

---

## 7. 维护机制（比篇数更重要）

1. **索引即契约**：每篇文档头部写 `status`（有效 / 待更新 / 已归档）、`applies_to`（版本或日期）、`last_verified`；`docs/README.md` 汇总。
2. **同 PR 更新**：改动以下内容时，PR 必须同步对应文档（可写进 PR 模板）：
   - `internal/settings`、`configstore` schema → `configuration.md` / `data-storage.md`
   - `internal/events`、`*_wrapper.go` → `ipc-api.md`
   - `internal/engine`、引擎注册 → `engines.md`
   - `.github/workflows`、`.cnb/`、`.gitlab-ci.yml` → `ci/`
   - `build/`、release workflow → `release.md`
3. **CI 校验**：
   - 已支持 `markdownfmt`（`make format-markdown`），保持；
   - 建议新增链接检查（lychee / markdown-link-check），至少覆盖 `docs/**/*.md` 与 README；
   - 建议 CI 检查 `docs/README.md` 索引与文件列表一致（防止「新增文档没人知道」）。
4. **过期策略**：
   - 日志按月归档到 `archive/logs/YYYY-MM/`，不再作为「当前文档」；
   - 规划类文档一旦被实现取代，立即移入 `archive/` 并在原地留一行指针；
   - 每季度做一次「文档对代码」抽查（配置字段、事件、workflow 数量）。
5. **知识库口径**：`.cnb/workflows/knowledge-base.yml` 的 `include` 只收正式文档（`README*.md` + `docs/*.md` + `docs/ci/*.md` + `docs/adr/*.md`），排除 `archive/`、`guides/`、`logs/`。
6. **双语策略**：中文为唯一事实源；`README_EN.md` 与用户手册英文版随发版同步，其余内部文档不强制英文。

---

## 8. 建议执行顺序（已完成 P0/P1）

> 除 P2 外，下表 P0/P1 项已于 2026-10-27 完成。

| 阶段    | 内容                                                                                                                               | 产出                           |
|---------|------------------------------------------------------------------------------------------------------------------------------------|--------------------------------|
| Day 1   | ~~修 §3 的 10 项债务~~（2026-10-26 已完成）                                                                                        | 文档不再误导                   |
| Day 1–2 | `docs/README.md` 索引 + 目录重构（`ci/`、`archive/`、`guides/`）                                                                   | 可导航                         |
| Day 3–5 | `architecture.md` + `development.md`                                                                                               | 新人可独立跑起来并看懂主干     |
| Day 6–7 | `configuration.md` + `user-guide.md`                                                                                               | 配置与使用问题自助解决         |
| Day 8–9 | `release.md` + `ci/README.md`                                                                                                      | 发版与 CI 变更不再依赖口口相传 |
| Week 2  | P1 的 `ipc-api` / `engines` / `i18n` / `testing` / `data-storage` / `platforms` / `security-privacy` / `troubleshooting` + ADR ×10 | 维护成本显著下降               |
| 之后    | P2 按需，季度审查                                                                                                                  | 长期不腐化                     |

> 最小可用集（1 天）：`docs/README.md` + `docs/development.md` + `docs/configuration.md`（架构主干已有 PLAN.md）。
