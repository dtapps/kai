import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// 说明：未直接 mergeConfig(vite.config.ts)，因为 vite.config 注入了 @wailsio/runtime 的 wails 插件，
// 该插件依赖 wails 构建期生成的 bindings 目录（测试环境不存在），会导致 vitest 启动失败。
// 这里仅保留测试必需的 svelte 插件与 @bindings 别名，并排除所有依赖 bindings/runtime 的模块，
// 避免 coverage 的 all:true 在加载这些文件时因缺失 bindings 而报错。
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      '@bindings': resolve(__dirname, 'src/bindings'),
    },
  },
  test: {
    environment: 'node', // 纯函数/store 测试无需 DOM；组件测试后再切 jsdom
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['lcov', 'text'], // lcov 供 CNB 插件解析，text 供本地阅读
      reportsDirectory: './coverage',
      include: ['src/**/*.{ts,svelte}'],
      exclude: [
        'src/test/**', // 测试自身不计入
        'src/bindings/**', // wails 生成，非源码
        'src/runtime/**', // 依赖 @wailsio/runtime
        'src/i18n/**', // 间接依赖 @bindings
        'src/constants/lang.ts', // 依赖 @bindings（待 bindings 就绪后可纳入）
        'src/stores/theme.ts', // 依赖 @bindings + DOM
        'src/stores/ui.ts', // 依赖 @bindings + runtime
        'src/utils/analytics.ts', // 依赖 runtime
        'src/**/*.d.ts',
      ],
      all: true, // 全量度量：未测试文件按 0% 计入分母，指标不虚高
    },
  },
})
