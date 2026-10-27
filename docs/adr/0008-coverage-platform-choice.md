# ADR-0008：覆盖率平台取舍（CNB + Codecov + Coveralls，删除 GitLab）

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-10-24

## 背景

仓库曾同时维护四套覆盖率：

- GitLab：自建 CI 镜像 + `go-coverage` / `node-coverage`，上传 GitLab 内置覆盖率；
- CNB：`testing:coverage` 内置任务；
- Codecov：覆盖率 + bundle + 测试分析；
- Coveralls：逐行 HTML。

GitLab 侧成本高、能力弱：一次流水线约 10–16 compute minutes，免费私有项目每月只有 400 分钟；而且 GitLab 是 pull mirror，开发在 GitHub/CNB，MR 小部件与 diff 注释永远触发不了。

## 决策

删除 GitLab 的全部覆盖率 job 与自建 CI 镜像，GitLab 只保留发布同步：

- `.gitlab-ci.yml` 只剩 `release` job；
- 覆盖率由 CNB（门禁预留）+ Codecov（可视化）+ Coveralls（逐行）承担；
- 原方案与踩坑记录收进 `docs/gitlab-ci.md` §6。

## 后果

正面：

- GitLab 额度全部留给发布同步。
- 覆盖率数据仍有三个视角，且都比 GitLab 内置强。
- 少维护一套 CI 镜像与 Cobertura 转换。

负面 / 约束：

- GitHub 的 Codecov 与 Coveralls 会各自重复跑一遍测试（隔离性换来的成本）。
- CNB 也跑一遍覆盖率；三处口径需要人工保持一致（范围、`-vet=off`、flag 名）。
- CNB 的 `lines` / `diffLines` 红线目前是注释状态，实际没有硬门禁；需要时手动取消注释。
- 历史方案只能从文档回溯，不能再从 `.gitlab-ci.yml` 直接恢复。

## 证据

- `.gitlab-ci.yml` 头注释：删除原因、成本、历史方案位置。
- `.cnb/workflows/{push,pull}-coverage.yml`：`testing:coverage`，红线注释状态。
- `.github/workflows/codecov.yml`、`coveralls.yml`：两套上传。
- `docs/gitlab-ci.md` §6：历史实现。
