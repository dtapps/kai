import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import wails from '@wailsio/runtime/plugins/vite'
import codecovVitePlugin from '@codecov/vite-plugin'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const bindings = resolve(__dirname, 'bindings')

// Bundle Analysis：仅当 CI 注入 CODECOV_TOKEN 时启用 @codecov/vite-plugin。
// 本地 / CNB 构建无该环境变量，插件不实例化，绝不影响主构建（与 coverage.yml 一致）。
const codecovPlugins = process.env.CODECOV_TOKEN
  ? [codecovVitePlugin({ enableBundleAnalysis: true, bundleName: 'kai-frontend', uploadToken: process.env.CODECOV_TOKEN })]
  : []

export default defineConfig({
  plugins: [svelte(), tailwindcss(), wails(bindings), ...codecovPlugins],
  resolve: {
    alias: {
      '@bindings': bindings,
    },
  },
  build: {
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        translate: resolve(__dirname, 'translate.html'),
        settings: resolve(__dirname, 'settings.html'),
        screenshot: resolve(__dirname, 'screenshot.html'),
      },
    },
  },
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
})
