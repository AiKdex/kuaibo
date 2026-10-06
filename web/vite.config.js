import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import { themeViewScope } from './vite-theme-scope.js'

// AiKmap 前端构建配置
// 开发期代理到本地/服务器后端；生产构建产物由 Go 单二进制内嵌或独立静态托管
export default defineConfig({
  plugins: [vue(), themeViewScope()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 5173,
    proxy: {
      // 开发代理：后端默认 :8080（可用 AIKMAP_DEV_PROXY 环境变量覆盖）
      '/api': {
        target: process.env.AIKMAP_DEV_PROXY || 'http://127.0.0.1:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 900
  }
})
