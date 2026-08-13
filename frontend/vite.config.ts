import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import wails from '@wailsio/runtime/plugins/vite'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const bindings = resolve(__dirname, 'bindings')

export default defineConfig({
  plugins: [svelte(), tailwindcss(), wails(bindings)],
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
