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
