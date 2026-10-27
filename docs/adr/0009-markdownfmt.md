# ADR-0009：Markdown 统一用 markdownfmt

> status: 已采纳（生效中） · last_verified: 2026-10-27 · 决策日期：2026-10-23

## 背景

仓库早期用 Prettier 格式化 Markdown。问题：

- Prettier 对表格、链接引用、中文换行的处理与 Go 生态文档不一致；
- 前端已经用 Prettier，但文档也跟着前端工具链走，CI 镜像需要额外装 Node 侧格式化器；
- 不同编辑器保存后 diff 很大，评审噪音高。

## 决策

Markdown 改用 `markdownfmt`（`github.com/Kunde21/markdownfmt/v3`）：

```bash
make format-markdown
# 内部：
markdownfmt -w <每个 .md>
```

- 版本固定在 CI Dockerfile 的 `ARG MARKDOWNFMT_VERSION=v3.1.0`。
- `make format` 包含 `format-markdown`。
- 前端代码仍用 Prettier；两者分工明确：`.md` 归 markdownfmt，`src/**/*.{ts,svelte,...}` 归 Prettier。

## 后果

正面：

- 文档格式与 Go 工具链一致，CI 镜像少一层 Node 依赖。
- 表格、列表、代码块的输出稳定。

负面 / 约束：

- 本地没装 `markdownfmt` 时，`make format-markdown` 只打印提示并**成功退出**，容易误以为已格式化；需要 `make tool-deps` 或 `go install github.com/Kunde21/markdownfmt/v3/cmd/markdownfmt@latest`。
- `format-markdown` 会扫描仓库里所有 `.md`（排除 node_modules/.git 等），包括历史日志；改历史日志会产生无意义 diff，提交时注意只提交自己改的文件。
- 不要再用 Prettier 格式化 `.md`，否则会和 markdownfmt 互相覆盖。

## 证据

- `Makefile:format-markdown`：安装提示与 `markdownfmt -w` 循环。
- `.github/ci/Dockerfile`、`.cnb/ci/Dockerfile`：`ARG MARKDOWNFMT_VERSION=v3.1.0`。
- 提交 `b76ad72`：用 markdownfmt 替换 Prettier 格式化 Markdown，并移除 goveralls 与 arm64 构建。
