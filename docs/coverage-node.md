# Node 单元测试覆盖率实现文案（可复制到其他项目交给 AI 实现）

> 来源：kai 项目实战方案（CNB 云原生构建 + `testing:coverage` 内置任务 + vitest，前端 Svelte 5 + Vite 8 + TypeScript 7 + pnpm 12）。

# 任务：为 Node/Svelte 项目实现单元测试覆盖率上报与红线门禁（CNB 云原生构建）

## 背景与目标

- 构建平台是 CNB（cnb.cool），流水线配置按职责拆在 `.cnb/workflows/*.yml`，由仓库根 `.cnb.yml` 的 `include:` 引入。
- 使用 CNB 内置任务 `type: testing:coverage` 上报覆盖率：push 全量覆盖率，PR 增量覆盖率（`diffLines` 仅 PR 类事件计算）。红线是否生效由 `lines` / `diffLines` 是否取消注释决定。
- 技术栈：Vite + Svelte 5 + TypeScript + pnpm。测试框架选 **vitest**（不用 jest：vitest 与 Vite 原生集成，`.svelte` 单文件组件经 `vite-plugin-svelte` 直接处理，零 transform 配置）。

> **kai 当前状态**：`.cnb/workflows/push-coverage.yml` / `pull-coverage.yml` 中 `lines: 90`、`diffLines: 80` 均为注释状态，**红线未启用**；`breakIfNoCoverage: true` 保持开启（报告缺失即失败）。下文 YAML 里的数值是「启用时怎么填」的示例。

## 实现步骤

### 第 1 步：安装依赖

```bash
pnpm add -D vitest @vitest/coverage-v8
```

当前仓库实际版本：`vitest@^5.0.5`、`@vitest/coverage-v8@^5.0.5`。

### 第 2 步：创建 vitest.config.ts（放在前端目录根）

```ts
import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// 说明：未直接 mergeConfig(vite.config.ts)，因为 vite.config 注入了 @wailsio/runtime 的 wails 插件，
// 该插件依赖 wails 构建期生成的 bindings 目录（测试环境不存在），会导致 vitest 启动失败。
// 这里仅保留测试必需的 svelte 插件与 @bindings 别名。
// @bindings 指向仓库内真实生成的 bindings 目录（../bindings，随 git 提交），
// 因此 i18n / stores / runtime 等依赖 bindings 的模块都能被直接引入并测试；
// 其中会真正发起 RPC 的函数在用例内用 vi.mock 替换。
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      '@bindings': resolve(__dirname, 'bindings'),
    },
  },
  test: {
    environment: 'node', // 纯函数/store 测试无需 DOM；需要 DOM 的模块在用例内自行打桩
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['lcov', 'text'], // lcov 供 CNB 插件解析，text 供本地阅读
      reportsDirectory: './coverage',
      include: ['src/**/*.{ts,svelte}'],
      exclude: ['src/test/**', 'src/**/*.d.ts'],
      all: true, // 全量度量：未测试文件按 0% 计入分母，指标不虚高
    },
  },
})
```

要点：

- **不要直接 `mergeConfig(vite.config.ts)`**：`vite.config.ts` 里 `wails(bindings)` 插件依赖 Wails 生成物，测试环境没有，vitest 启动会失败。
- **`@bindings` 指向 `frontend/bindings`**（不是 `src/bindings`）。该目录随 git 提交，因此 i18n / stores / runtime 等模块可直接引入并测试；真正发 RPC 的函数在用例里 `vi.mock`。
- 当前 `exclude` 只有 `src/test/**` 与 `src/**/*.d.ts`。早期版本曾排除 `i18n/**`、`stores/theme.ts`、`stores/ui.ts`、`utils/analytics.ts` 等，后来 bindings 随仓库提交 + 用例内打桩，**这些排除已全部撤销**，不要再照抄旧版。

### 第 3 步：创建测试环境桩 src/test/setup.ts

node 环境没有 localStorage/navigator，而 store 常在导入期读取它们，不补桩会加载即崩：

```ts
const memory = new Map<string, string>();

const localStorageStub: Storage = {
  get length() {
    return memory.size;
  },
  clear() {
    memory.clear();
  },
  getItem(key: string) {
    return memory.get(key) ?? null;
  },
  key(index: number) {
    return Array.from(memory.keys())[index] ?? null;
  },
  removeItem(key: string) {
    memory.delete(key);
  },
  setItem(key: string, value: string) {
    memory.set(key, value);
  },
};

Object.defineProperty(globalThis, 'localStorage', {
  value: localStorageStub,
  configurable: true,
  writable: true,
});

// navigator.language：node 无此属性；i18n 'auto' 模式解析系统语言时需要
Object.defineProperty(globalThis, 'navigator', {
  value: { language: 'zh-CN' },
  configurable: true,
  writable: true,
});

// 屏蔽 debug 噪音；console.error 保留可见，避免掩盖真实问题
console.debug = () => {};
```

### 第 4 步：编写测试（从纯函数与 store 入手，不要一上来写组件测试）

每个测试文件头部（Svelte store 用 `get()` 读当前值；localStorage 每个用例前清空）：

```ts
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';

beforeEach(() => {
  localStorage.clear();
});
```

优先覆盖：工具函数（格式化/CSS 变量拼接）、Svelte store（持久化读写/损坏值回退）、常量模块（主题/事件名）、纯逻辑模块。当前仓库有 11 个测试文件（`constants` / `runtime` / `stores` / `utils` / `i18n`）。

### 第 5 步：package.json 增加脚本

```json
"scripts": {
  "test": "vitest run",
  "test:coverage": "vitest run --coverage"
}
```

### 第 6 步：.gitignore 增加 `frontend/coverage`（或项目对应路径）

### 第 7 步：流水线配置（push 与 PR 各用独立并行流水线）

CNB 语义：事件键下的值是 **Pipeline 数组**，同事件多个条目 = 多流水线并行；并入同一 pipeline 的 stages 会串行，不要这样做。

`.cnb/workflows/push-coverage.yml`（节选，实际文件含注释与 exports）：

```yaml
"**":
  push:
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/ci/Dockerfile # 复用已预装 Node + pnpm 的构建环境
        volumes:
          - /root/.cache/go-build:copy-on-write
          - /root/.local/share/pnpm/store:copy-on-write # pnpm 存储缓存
          - /go/pkg/mod:copy-on-write
      stages:
        - name: 安装所有依赖
          script: make deps
        - name: 生成前端测试覆盖率
          script: pnpm --dir ./frontend test:coverage # 产出 frontend/coverage/lcov.info
        - name: 上报覆盖率（全量）
          type: testing:coverage
          options:
            key: node # 多报告数据键：与 Go 侧（key: go）区分，各自写入 meta map 互不覆盖
            name: Node Coverage # 徽章显示名，与 key 一对一对应
            # lines: 90 # 全量红线(%)：取消注释才生效；kai 当前未启用
            pattern: 'frontend/coverage/lcov.info'
            breakIfNoCoverage: true # 找不到覆盖率报告则报错终止，避免静默通过
          exports:
            lines: NODE_COVERAGE_LINES
            diff_pct: NODE_COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "node coverage: lines=$NODE_COVERAGE_LINES diff=$NODE_COVERAGE_DIFF_PCT"'
```

`.cnb/workflows/pull-coverage.yml` 与 push 基本相同，只是把 `type: testing:coverage` 的注释换成增量红线：

```yaml
            # diffLines: 80 # 增量红线(%)：取消注释才生效
            # lines: 90 # 全量底线与 push 一致，防总量被单 PR 拉低
```

## 关键决策（实现时遵循）

1. **vitest 而非 jest**：与 Vite 原生集成；本项目**未**直接 `mergeConfig(vite.config)`——vite 配置注入的 wails 插件依赖构建期生成的 bindings，测试环境不存在会让 vitest 启动失败；改为独立配置，保留 svelte 插件与 `@bindings` 别名。
2. **`all: true` 全量度量**：未测试文件按 0% 计入分母，指标不虚高。基线以 `pnpm --dir frontend test:coverage` 的实际输出为准，不要照抄历史数字（早期 1.43%、14.38% 都是旧快照）。
3. **排除范围尽量小**：当前只排除 `src/test/**` 与 `*.d.ts`。不要因为某模块 import `@bindings` 就整体排除——`frontend/bindings` 随仓库提交，用例内 `vi.mock` 真实 RPC 即可。只有确实无法在 node 环境加载的模块才加 `exclude`，并在本文件记录原因。
4. **不设 `lang`**：`lang=go` 仅 golang 格式报告需要；lcov 由插件自动识别。
5. **红线棘轮**：启用时 `lines` 贴当前基线往下留余量，`diffLines: 80`（变更代码高覆盖）长期不动或逐步上调；kai 当前两者均未启用。
6. **测试从纯函数/store 开始**：组件测试需要 `@testing-library/svelte` + jsdom/happy-dom，作为第二步。
7. **YAML 细节**：script 含 `": "` 等特殊字符时整体单引号包裹；独立并行流水线，勿并入既有 stages。
8. **多报告数据键 key/name**：同一仓库若同时上报多份覆盖率（如 Go + Node），必须为每个 `testing:coverage` 设置不同 `key`（如 `go` / `node`）与对应 `name`（如 `Go Coverage` / `Node Coverage`）；否则多份报告写入同一 meta map 会互相覆盖，徽章只剩最后一份。

## 验收标准

- `pnpm test` 全绿；`pnpm test:coverage` 产出 `frontend/coverage/lcov.info`，total 与 CI 输出一致。
- `pnpm check`（svelte-check）与 `pnpm format:check`（prettier）对新测试文件零报错（测试文件也在类型检查范围内）。
- YAML 解析正确；首次推送后确认徽章生成（与 Go 侧 `Go Coverage` 徽章并存，互不覆盖）、pattern 解析正常、多流水线并行启动。
