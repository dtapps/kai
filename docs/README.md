# Kai 文档索引与写作规范

> status: 有效 · last_verified: 2026-10-27 · 基线 `de28c9e`（master，`go.mod` 依赖 Wails v3.0.0）

这是 `docs/` 的入口。先按读者找到对应文档；每篇文档头部都标注 `status` 与 `last_verified`，没有标注的旧文按「历史快照」看待。

## 1. 文档地图

### 1.1 使用者

| 文档                                       | 内容                                                 | 最后核对   |
|--------------------------------------------|------------------------------------------------------|------------|
| [README](../README.md)                     | 产品功能、平台边界、匿名统计、发布渠道（不在此重复） | 2026-10-27 |
| [README_EN](../README_EN.md)               | 英文版产品介绍                                       | 2026-10-27 |
| [user-guide.md](user-guide.md)             | 安装、授权、快捷键、引擎配置、OCR、更新、数据重置    | 2026-10-27 |
| [troubleshooting.md](troubleshooting.md)   | 按现象排障、日志位置、复现与上报                     | 2026-10-27 |
| [security-privacy.md](security-privacy.md) | 凭据加密、匿名统计边界、漏洞披露、CI secret 与签名   | 2026-10-27 |

### 1.2 新贡献者

| 文档                               | 内容                                                                  | 最后核对   |
|------------------------------------|-----------------------------------------------------------------------|------------|
| [development.md](development.md)   | 工具链、初始化、代码生成链、日常校验、已知坑                          | 2026-10-27 |
| [testing.md](testing.md)           | Go / 前端测试分层、覆盖率命令与基线、`-vet=off`、fuzz、CI 产物        | 2026-10-27 |
| [ipc-api.md](ipc-api.md)           | Service RPC、13 个应用事件、窗口常量、bindings 生成约定、扩展步骤     | 2026-10-27 |
| [i18n.md](i18n.md)                 | 后端 split JSON、前端 TS 字典、`Locale`/`Language` 两套体系、新增语言 | 2026-10-27 |
| [engines.md](engines.md)           | 翻译 / OCR 引擎能力矩阵、鉴权字段、新增引擎 checklist                 | 2026-10-27 |
| [platforms.md](platforms.md)       | 平台能力矩阵、build tags、CGO、Swift 桥、WebView2                     | 2026-10-27 |
| [data-storage.md](data-storage.md) | 三个 SQLite schema、sqlc 生成、迁移、跨库引用、重置与备份             | 2026-10-27 |

### 1.3 维护者

| 文档                                 | 内容                                                                   | 最后核对   |
|--------------------------------------|------------------------------------------------------------------------|------------|
| [PLAN.md](PLAN.md)                   | 项目规划与架构主干（技术栈、目录、模块、里程碑、约束）                 | 2026-10-27 |
| [configuration.md](configuration.md) | `settings.json` 全字段、`config.db` 引擎配置、代理/DNS/日志/更新优先级 | 2026-10-27 |
| [release.md](release.md)             | 版本规则、Release 工作流、产物矩阵、签名、CNB/GitLab/Gitee 同步、回滚  | 2026-10-27 |
| [ci/README.md](ci/README.md)         | GitHub / CNB / GitLab / Gitee 流水线总览与 secret 清单（GitCode 保留） | 2026-10-27 |
| [ci-image.md](ci-image.md)           | CNB 与 GitHub 两份 CI Dockerfile 的差异与排错                          | 2026-10-27 |
| [codecov.md](codecov.md)             | Codecov 覆盖率 / bundle / 测试分析                                     | 2026-10-26 |
| [coveralls.md](coveralls.md)         | Coveralls 单 job 双 flag                                               | 2026-10-26 |
| [gitlab-ci.md](gitlab-ci.md)         | GitLab 发布同步（覆盖率方案已下线）                                    | 2026-10-26 |
| [coverage-go.md](coverage-go.md)     | Go 覆盖率实现模板（可复用到其他项目）                                  | 2026-10-26 |
| [coverage-node.md](coverage-node.md) | 前端覆盖率实现模板                                                     | 2026-10-26 |
| [adr/](adr/)                         | 10 条关键决策记录（复制键、Service 拆分、窗口隐藏、更新源等）          | 2026-10-27 |

### 1.4 历史与规划

| 文档                         | 定性                                | 最后核对   |
|------------------------------|-------------------------------------|------------|
| `2026-08-08.md` ~ `09-01.md` | 开发日志，已冻结，只作追溯          | 2026-09-01 |
| [docs-plan.md](docs-plan.md) | 历史缺口分析；P0/P1 已补齐，P2 待定 | 2026-10-27 |

### 1.5 仓库根

| 文件                                  | 内容                                | 最后核对   |
|---------------------------------------|-------------------------------------|------------|
| [CONTRIBUTING.md](../CONTRIBUTING.md) | 贡献入口：本地跑起来、检查、PR 约定 | 2026-10-27 |
| [SECURITY.md](../SECURITY.md)         | 漏洞报告渠道、适用范围、已知边界    | 2026-10-27 |
| [CHANGELOG.md](../CHANGELOG.md)       | 版本变更记录（从 v0.2.0 起）        | 2026-10-27 |

## 2. 命名与目录规则

- 专题文档用 `kebab-case.md`，放在 `docs/` 根目录；CI 专题放 `docs/ci/`；决策记录放 `docs/adr/`。
- ADR 文件名 `NNNN-短标题.md`，编号只增不删；被取代时改 `status` 为「已被 ADR-NNNN 取代」，不删文件。
- 开发日志按 `YYYY-MM-DD.md` 放在 `docs/` 根目录，冻结后不再修改；新日志不要覆盖旧日志。
- 图片、图表等非 Markdown 资源目前没有目录约定；新增前先与维护者确认，不要散落在根目录。

## 3. 写作规范

- 开篇第一行写状态：`> status: 有效 · last_verified: YYYY-MM-DD · 基线 <commit>`。状态取值：`有效` / `待更新` / `已归档` / `草案`；ADR 另用 `已采纳` / `已被 ADR-NNNN 取代` / `已废弃`。
- 结论先行。每节第一段回答「是什么 / 为什么」，再给细节。
- 命令必须可复制：写完整命令，不写 `<你的路径>` 之类占位；路径用仓库内真实路径。
- 引用代码位置用反引号包路径，例如 `internal/events/events.go`；关键行为尽量带函数名或行号。
- 不写「未来会支持」「敬请期待」式承诺。未实现的能力标「未实现」并给出代码依据。
- 不重复 `README.md`、`PLAN.md` 和现有专题文档的内容；需要时只链接并补一句上下文。
- 中文为唯一事实源。内部文档不强制英文；`README_EN.md` 随发版同步。
- 所有 Markdown 用仓库统一的 `markdownfmt` 格式化（`make format-markdown`），不要用 Prettier。

## 4. 同 PR 更新契约

改动以下内容时，同一个 PR 必须同步对应文档，否则视为文档债务：

| 代码 / 配置                                                                | 必须更新                                                     |
|----------------------------------------------------------------------------|--------------------------------------------------------------|
| `internal/settings/`、`settings.json` 字段                                 | `configuration.md`；字段增减同时改本文 §1 摘要               |
| `internal/configstore/` schema、加密逻辑                                   | `configuration.md`、`data-storage.md`、`security-privacy.md` |
| `internal/events/`、`*_wrapper.go` 方法                                    | `ipc-api.md`                                                 |
| `internal/engine/`、引擎注册与 schema                                      | `engines.md`、`configuration.md`                             |
| `internal/i18n/`、`scripts/merge-*-i18n.*`                                 | `i18n.md`                                                    |
| `.github/workflows/`、`.cnb/`、`.gitlab-ci.yml`、`.workflow/`、`.gitcode/` | `ci/README.md` + 对应专题                                    |
| `Makefile` 目标、`build/`、版本注入                                        | `development.md`、`release.md`                               |
| 平台能力（darwin/windows/linux）                                           | `platforms.md`、`user-guide.md`                              |

## 5. 维护节奏

- 每次发版前核对 `release.md` 的 checklist，并更新受影响文档的 `last_verified`。
- 每季度做一次「文档对代码」抽查：`settings` 字段、事件数量、workflow 数量、引擎清单、Makefile 目标。
- 新增文档后立刻登记到本文 §1；知识库（`.cnb/workflows/knowledge-base.yml`）的 `include` 已覆盖 `docs/**/*.md`、`README.md`、`README_EN.md`、`CONTRIBUTING.md`、`SECURITY.md`、`CHANGELOG.md`，新增 `docs/` 文档会自动收录。
- 过期文档不删，改 `status: 已归档` 并在文首留一行指针；需要时再移入归档目录。
