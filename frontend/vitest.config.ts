import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// 扫描 / 排除清单由 Makefile「测试覆盖率」节通过环境变量注入（唯一来源），
// 新增目录只改 Makefile；不经过 make 直接跑 vitest 时回退到默认值，行为不变。
// 多个 glob 用空格分隔（不要用逗号，也不要用 {a,b} 花括号，会被拆分导致匹配不到文件）。
const list = (value: string | undefined, fallback: string[]) =>
  value ? value.split(/\s+/).filter(Boolean) : fallback

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
    include: list(process.env.VITEST_TEST_INCLUDE, ['src/**/*.test.ts']),
    coverage: {
      provider: 'v8',
      reporter: ['lcov', 'text'], // lcov 供 CNB 插件解析，text 供本地阅读
      reportsDirectory: './coverage',
      include: list(process.env.VITEST_COVER_INCLUDE, ['src/**/*.ts', 'src/**/*.svelte']),
      exclude: list(process.env.VITEST_COVER_EXCLUDE, ['src/test/**', 'src/**/*.d.ts']),
      // 全量度量：include 显式列出源码目录，未被测试 import 的文件也按 0% 计入分母
      //（Vitest 5 已移除 coverage.all，全量由 coverage.include 实现）
    },
  },
})
