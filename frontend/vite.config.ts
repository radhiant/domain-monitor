import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'

// One agent serves every domain, so unlike the server dashboard there is a
// single proxy target here rather than one entry per monitored node.
const AGENT = process.env.AGENT_URL || 'http://localhost:9292'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  build: {
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        manualChunks: {
          // ECharts is only needed on the detail view; splitting it keeps the
          // wall display, which is what actually runs 24/7, small to load.
          'echarts': ['echarts'],
          'vue-vendor': ['vue', 'pinia', 'lucide-vue-next']
        }
      }
    }
  },
  server: {
    port: 3001,
    host: true,
    proxy: {
      '/api': {
        target: AGENT,
        changeOrigin: true
      },
      '/ws': {
        target: AGENT.replace(/^http/, 'ws'),
        ws: true
      }
    }
  }
})
