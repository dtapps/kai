# Node 单元测试覆盖率实现文案（可复制到其他项目交给 AI 实现）

> 来源：kai 项目实战方案（CNB 云原生构建 + testing:coverage 内置任务 + vitest，前端 Svelte 5 + Vite + TypeScript + pnpm）。

# 任务：为 Node/Svelte 项目实现单元测试覆盖率上报与红线门禁（CNB 云原生构建）

## 背景与目标

- 构建平台是 CNB（cnb.cool），流水线配置在 `.cnb.yml`（或 `.cnb/workflows/*.yml`）。
- 使用 CNB 内置任务 `type: testing:coverage` 上报覆盖率：push 全量红线（lines），PR 增量红线（diffLines，仅 PR 类事件计算）。
- 技术栈：Vite + Svelte 5 + TypeScript + pnpm。测试框架选 **vitest**（不用 jest：vitest 与 Vite 原生集成，`.svelte` 单文件组件经 vite-plugin-svelte 直接处理，零 transform 配置）。

## 实现步骤

### 第 1 步：安装依赖

```bash
pnpm add -D vitest @vitest/coverage-v8
```

### 第 2 步：创建 vitest.config.ts（放在前端目录根）

```ts
import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

// 说明：未直接 mergeConfig(vite.config.ts)，因为 vite.config 注入了 @wailsio/runtime 的 wails 插件，
// 该插件依赖 wails 构建期生成的 bindings 目录（测试环境不存在），会导致 vitest 启动失败。
// 这里仅保留测试必需的 svelte 插件与 @bindings 别名，并排除所有依赖 bindings/runtime 的模块，
// 避免 coverage 的 all:true 在加载这些文件时因缺失 bindings 而报错。
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      "@bindings": resolve(__dirname, "src/bindings"),
    },
  },
  test: {
    environment: "node", // 纯函数/store 测试无需 DOM；组件测试后再切 jsdom
    setupFiles: ["./src/test/setup.ts"],
    coverage: {
      provider: "v8",
      reporter: ["lcov", "text"], // lcov 供 CI 插件解析，text 供本地阅读
      reportsDirectory: "./coverage",
      include: ["src/**/*.{ts,svelte}"],
      exclude: [
        "src/test/**", // 测试自身不计入
        "src/bindings/**", // wails 生成，非源码
        "src/runtime/**", // 依赖 @wailsio/runtime
        "src/i18n/**", // 间接依赖 @bindings
        "src/constants/lang.ts", // 依赖 @bindings（待 bindings 就绪后可纳入）
        "src/stores/theme.ts", // 依赖 @bindings + DOM
        "src/stores/ui.ts", // 依赖 @bindings + runtime
        "src/utils/analytics.ts", // 依赖 runtime
        "src/**/*.d.ts",
      ],
      all: true, // 全量度量：未测试文件按 0% 计入分母，指标不虚高
    },
  },
});
```

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

Object.defineProperty(globalThis, "localStorage", {
  value: localStorageStub,
  configurable: true,
  writable: true,
});

// navigator.language：node 无此属性；i18n 'auto' 模式解析系统语言时需要
Object.defineProperty(globalThis, "navigator", {
  value: { language: "zh-CN" },
  configurable: true,
  writable: true,
});

// 屏蔽 debug 噪音；console.error 保留可见，避免掩盖真实问题
console.debug = () => {};
```

### 第 4 步：编写测试（从纯函数与 store 入手，不要一上来写组件测试）

每个测试文件头部（Svelte store 用 get() 读当前值；localStorage 每个用例前清空）：

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { get } from "svelte/store";

beforeEach(() => {
  localStorage.clear();
});
```

优先覆盖：工具函数（格式化/CSS 变量拼接）、Svelte store（持久化读写/损坏值回退）、常量模块（主题/事件名）、纯逻辑模块。

### 第 5 步：package.json 增加脚本

```json
"scripts": {
  "test": "vitest run",
  "test:coverage": "vitest run --coverage"
}
```

### 第 6 步：.gitignore 增加 `frontend/coverage`（或项目对应路径）

### 第 7 步：流水线配置（push 与 PR 各用独立并行流水线）

CNB 语义：事件键下的值是 Pipeline 数组，同事件多个条目 = 多流水线并行；并入同一 pipeline 的 stages 会串行，不要这样做。

```yaml
"**":
  push:
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile # 复用已预装 Node + pnpm 的构建环境
        volumes:
          - /root/.local/share/pnpm/store:copy-on-write # pnpm 存储缓存
      stages:
        - name: 安装前端依赖
          script: pnpm --dir ./frontend install --frozen-lockfile
        - name: 生成前端测试覆盖率
          script: pnpm --dir ./frontend test:coverage # 产出 frontend/coverage/lcov.info
        - name: 上报覆盖率（全量红线）
          type: testing:coverage
          options:
            lines: 1 # 初始值=当前基线往下留余量（本项目基线 1.43%，视图组件尚未建测试）
            key: node # 多报告数据键：与 Go 侧（key: go）区分，各自写入 meta map 互不覆盖
            name: Node Coverage # 徽章显示名，与 key 一对一对应
            pattern: "frontend/coverage/lcov.info"
            breakIfNoCoverage: true
          exports:
            lines: NODE_COVERAGE_LINES
            diff_pct: NODE_COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "node coverage: lines=$NODE_COVERAGE_LINES diff=$NODE_COVERAGE_DIFF_PCT"'
  pull_request:
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile
        volumes:
          - /root/.local/share/pnpm/store:copy-on-write
      stages:
        - name: 安装前端依赖
          script: pnpm --dir ./frontend install --frozen-lockfile
        - name: 生成前端测试覆盖率
          script: pnpm --dir ./frontend test:coverage
        - name: 上报覆盖率（增量红线）
          type: testing:coverage
          options:
            diffLines: 50 # 本 PR 变更代码覆盖率，低于此值阻断
            lines: 1 # 全量底线与 push 一致
            key: node # 多报告数据键：与 Go 侧（key: go）区分，各自写入 meta map 互不覆盖
            name: Node Coverage # 徽章显示名，与 key 一对一对应
            pattern: "frontend/coverage/lcov.info"
            breakIfNoCoverage: true
          exports:
            lines: NODE_COVERAGE_LINES
            diff_pct: NODE_COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "node coverage: lines=$NODE_COVERAGE_LINES diff=$NODE_COVERAGE_DIFF_PCT"'
```

## 关键决策（实现时遵循）

1. **vitest 而非 jest**：与 Vite 原生集成；本项目**未**直接 mergeConfig(vite.config)——vite.config 注入的 wails 插件依赖构建期生成的 bindings，测试环境不存在会让 vitest 启动失败；改为独立配置，保留 svelte 插件与 `@bindings` 别名。
2. **`all: true` 全量度量**：未测试文件按 0% 计入分母。初始数字会很低（视图组件没测试时本项目仅 1.43%），但诚实；真正驱动质量的是 PR 增量红线。
3. **排除无法在 node 环境加载的模块**：wails 的 `bindings` 是构建期生成、测试环境缺失，所有 import `@bindings` / `@wailsio/runtime` 的模块（`constants/lang.ts`、`i18n/**`、`stores/theme.ts`、`stores/ui.ts`、`utils/analytics.ts`、`runtime/**`、`bindings/**`）必须排除，否则 `all:true` 加载即报错；入口文件（`windows/*/main.ts`，导入期 `app.mount()`）同理。
4. **不设 `lang`**：lang=go 仅 golang 格式报告需要；lcov 由插件自动识别。
5. **红线棘轮**：lines 初始贴基线下沿；diffLines: 50（变更代码过半覆盖）长期不动或逐步上调。
6. **测试从纯函数/store 开始**：组件测试需要 @testing-library/svelte + jsdom/happy-dom，作为第二步。
7. **YAML 细节**：script 含 `": "` 等特殊字符时整体单引号包裹；独立并行流水线，勿并入既有 stages。
8. **多报告数据键 key/name**：同一仓库若同时上报多份覆盖率（如 Go + Node），必须为每个 `testing:coverage` 设置不同 `key`（如 `go` / `node`）与对应 `name`（如 `Go Coverage` / `Node Coverage`）；否则多份报告写入同一 meta map 会互相覆盖，徽章只剩最后一份。

## 验收标准

- `pnpm test` 全绿；`pnpm test:coverage` 产出 `frontend/coverage/lcov.info`，total 与预期一致。
- `svelte-check`（pnpm check）与 `prettier`（pnpm format:check）对新测试文件零报错（测试文件也在类型检查范围内）。
- YAML 解析正确；首次推送后确认徽章生成（与 Go 侧 `Go Coverage` 徽章并存，互不覆盖）、pattern 解析正常、多流水线并行启动。
