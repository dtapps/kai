# Go 单元测试覆盖率实现文案（可复制到其他项目交给 AI 实现）

> 来源：kai 项目实战方案（CNB 云原生构建 + testing:coverage 内置任务）。

# 任务：为 Go 项目实现单元测试覆盖率上报与红线门禁（CNB 云原生构建）

## 背景与目标

- 构建平台是 CNB（cnb.cool），流水线配置在仓库根的 `.cnb.yml`（或 `.cnb/workflows/*.yml`，由 .cnb.yml include）。
- 使用 CNB 内置任务 `type: testing:coverage` 上报覆盖率：push 事件上报全量覆盖率，pull_request 事件额外计算增量覆盖率（diffLines 仅 PR 类事件生效）。红线是否启用由 `lines` / `diffLines` 是否取消注释决定。
- 红线采用「棘轮策略」：初始值贴着当前基线往下留余量，防止误伤，后续随覆盖率提升逐步上调。

> **kai 当前状态**：`.cnb/workflows/push-coverage.yml` 与 `pull-coverage.yml` 中 `lines` / `diffLines` 均为注释状态，**红线未启用**；下文 YAML 里的数值是「启用时怎么填」的示例，不是当前生效配置。实际文件按职责拆在 `.cnb/workflows/`，由 `.cnb.yml` include。
- 覆盖率生成命令**直接内联到流水线 stage，不增加 Makefile 目标**（详见第 2 步）。

## 实现步骤

### 第 1 步：先测量当前覆盖率基线（定红线前必须做）

在本地运行并记录 total 覆盖率（按本项目实际范围，勿照搬 `./...`）：

```bash
go test -coverprofile=/tmp/cov.out ./internal/... ./pkg/... && go tool cover -func=/tmp/cov.out | tail -1
```

### 第 2 步：覆盖率命令直接写进 CI（不增加 Makefile 目标）

**理由**：coverage 命令只有 CI 一个使用者。放进 Makefile 只会多一层间接，还要处理 `$$` 转义；直接内联到流水线 stage 更直白、流水线自包含。

核心命令（按本项目实际编写；复制到 CI stage 的 script 列表，注意 shell 里是 `$(...)`，Makefile 里才需要写成 `$$(...)`）：

```bash
go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -) -coverprofile=coverage.out ./internal/... ./pkg/...
go tool cover -func=coverage.out
```

要点说明（实现时保留为注释）：

- **范围 `./internal/... ./pkg/...`，不含根 main 包**：本项目现有 26 个 `_test.go`，分布在 `internal/engine`、`internal/translate`、`internal/settings`、`internal/i18n`、`internal/configstore`、`internal/historystore`、`internal/httplogstore`、`internal/network`、`internal/useragent`、`internal/logutil`、`internal/buildinfo`、`pkg/wails-updater-providers` 等；根 main 包 `//go:embed all:frontend/dist` 依赖前端构建产物，覆盖率流水线不构建前端，含它直接编译失败。
- **`-vet=off` 按实际原因保留**：本项目 `pkg/swiftbridge`、`internal/selection`、`internal/engine` 存在 `fmt.Errorf` 非常量格式串，不关 vet 测试直接挂；换项目前先本地 `go vet ./...` 验证是否真需要。
- **本项目无 ent 等生成代码目录**：不要照搬其他项目的 `grep -v '/internal/ent'`，那是无效排除；有生成代码的项目才需要动态排除。
- `go list ... | paste -sd, -`：把包清单拼成逗号分隔喂给 `-coverpkg`，避免硬编码。

### 第 3 步：.gitignore 增加

```
coverage.out
coverage.html
```

### 第 4 步：流水线配置（push 与 pull_request 各用独立并行流水线，不并入既有 CI）

CNB 语义：事件键（如 `'**'`）下的值是 **Pipeline 数组，同事件多个数组条目 = 多条流水线并行执行**（并入同一个 pipeline 的 stages 会变串行，不要这样做）。

`.cnb.yml`（或 workflows 文件）中新增：

```yaml
"**":
  push:
    # ...既有流水线保持不动...
    # 新增：测试覆盖率流水线（与既有流水线并行）
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/ci/Dockerfile # 复用项目构建环境
        volumes:
          - /root/.cache/go-build:copy-on-write # Go 构建缓存
          - /root/.local/share/pnpm/store:copy-on-write # pnpm 全局存储（make deps 可能用到）
          - /go/pkg/mod:copy-on-write # Go 模块缓存
      stages:
        - name: 安装所有依赖
          script: make deps
        - name: 生成测试覆盖率
          script:
            # 命令按本项目实际编写（勿照搬其他项目）：
            # 1. 范围 ./internal/... ./pkg/...：含全部后端测试（26 个 _test.go，覆盖 engine/translate/
            #    settings/i18n/store 等）；不含根 main 包——它 //go:embed all:frontend/dist，
            #    本流水线无前端产物会编译失败。
            # 2. -vet=off：swiftbridge/selection/engine 存在 fmt.Errorf 非常量格式串，vet 会挂。
            # 3. 本项目无 ent 生成代码，无需排除。
            - go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -)
              -coverprofile=coverage.out ./internal/... ./pkg/...
            - go tool cover -func=coverage.out
        - name: 上报覆盖率（全量）
          type: testing:coverage
          options:
            # lines: 10 # 全量红线(%)：取消注释才生效。kai 当前未启用；启用前先按第 1 步测基线，再按「基线-10」取值
            lang: go # golang 格式报告必须指定，否则计算有误差
            key: go # 多报告数据键：与 Node 侧（key: node）区分，各自写入 meta map 互不覆盖
            name: Go Coverage # 徽章显示名，与 key 一对一对应
            pattern: "coverage.out"
            breakIfNoCoverage: false # kai 当前为 false：报告缺失只标记缺数据，不阻断；恢复红线时建议改 true
          exports:
            lines: COVERAGE_LINES
            diff_pct: COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "coverage: lines=$COVERAGE_LINES diff=$COVERAGE_DIFF_PCT"'
  pull_request:
    # ...既有 PR 流水线保持不动...
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/ci/Dockerfile
        volumes:
          - /root/.cache/go-build:copy-on-write
          - /root/.local/share/pnpm/store:copy-on-write
          - /go/pkg/mod:copy-on-write
      stages:
        - name: 安装所有依赖
          script: make deps
        - name: 生成测试覆盖率
          script:
            # 命令按本项目实际编写（勿照搬其他项目）：
            # 1. 范围 ./internal/... ./pkg/...：含全部后端测试（26 个 _test.go，覆盖 engine/translate/
            #    settings/i18n/store 等）；不含根 main 包——它 //go:embed all:frontend/dist，
            #    本流水线无前端产物会编译失败。
            # 2. -vet=off：swiftbridge/selection/engine 存在 fmt.Errorf 非常量格式串，vet 会挂。
            # 3. 本项目无 ent 生成代码，无需排除。
            - go test -vet=off -coverpkg=$(go list ./internal/... ./pkg/... | paste -sd, -)
              -coverprofile=coverage.out ./internal/... ./pkg/...
            - go tool cover -func=coverage.out
        - name: 上报覆盖率（增量）
          type: testing:coverage
          options:
            # diffLines: 50 # 增量红线(%)：取消注释才生效
            # lines: 10 # 全量底线与 push 一致，防总量被单 PR 拉低
            lang: go
            key: go # 多报告数据键：与 Node 侧（key: node）区分，各自写入 meta map 互不覆盖
            name: Go Coverage # 徽章显示名，与 key 一对一对应
            pattern: "coverage.out"
            breakIfNoCoverage: false
          exports:
            lines: COVERAGE_LINES
            diff_pct: COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "coverage: lines=$COVERAGE_LINES diff=$COVERAGE_DIFF_PCT"'
```

## 关键决策（实现时遵循，有冲突按项目实际调整并说明）

1. **范围按项目实际定，勿照搬**：本项目无 ent/mock 等生成代码，真正要排除的是**根 main 包**（`//go:embed all:frontend/dist` 依赖前端构建产物，覆盖率流水线不构建前端会编译失败），故范围为 `./internal/... ./pkg/...`；照搬其他项目的 `grep -v '/internal/ent'` 属无效排除，且 `./internal/...` 旧范围会漏掉 `pkg/wails-updater-providers` 的全部测试。
2. **`-vet=off` 按实际原因保留**：swiftbridge/selection/engine 有 `fmt.Errorf` 非常量格式串，不关 vet 测试直接挂；换项目先本地 `go vet ./...` 验证。
3. **红线初始值 = 当前实测基线往下留余量**（先跑第 1 步拿 total；kai 当前未启用红线），防止新增少量未覆盖代码就误触红线；随覆盖率提升每季度上调一次（棘轮）。
4. **PR 用 diffLines 管增量**：新代码必须过半覆盖，这是长期提升覆盖率的主要驱动力。
5. **breakIfNoCoverage 按需**：kai 当前为 `false`（缺失不阻断）；启用红线时建议改 `true`，防止测试编译失败时覆盖率阶段静默通过造成假绿。
6. **独立并行流水线**：与既有 CI 并行，互不阻塞；不要合并进既有 pipeline 的 stages（会串行）。
7. **覆盖率命令直接内联 CI，不进 Makefile**：coverage 只有 CI 一个使用者；内联可避免 Makefile 的 `$` 转义（`$$(...)` → `$(...)`）与额外跳转，流水线自包含、可读性更好。
8. **YAML 细节**：script 值含 `": "` 等特殊字符时整体用单引号包裹；`make deps` 已包含 `go mod download` 与前端依赖，放在测试前覆盖冷缓存场景。
9. **多报告数据键 key/name**：同一仓库若同时上报多份覆盖率（如 Go + Node），必须为每个 `testing:coverage` 设置不同 `key`（如 `go` / `node`）与对应 `name`（如 `Go Coverage` / `Node Coverage`）；否则多份报告写入同一 meta map 会互相覆盖，徽章只剩最后一份。

## 验收标准

- 本地直接运行第 2 步的内联命令跑通，产出 coverage.out，`go tool cover -func` 有 total 且与第 1 步基线一致。
- YAML 解析正确；push/pull_request 各新增一条独立流水线。
- 首次推送后确认：仓库「洞察 → 单元测试覆盖率」徽章生成，红线数值符合预期。
