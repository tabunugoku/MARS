import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const API_TARGET = 'http://localhost:25600'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
      // Fluid's icon-context imports Lucide defaults we never show; see the stub.
      'lucide-react': path.resolve(import.meta.dirname, './src/lib/lucide-stub.ts'),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    // Dev against the Go server: COMINAVI_DEV=1 go run ./cmd/cominavi
    proxy: {
      '/api': {
        target: API_TARGET,
        changeOrigin: true,
        // The Go server refuses cookie writes whose Origin is not its own
        // host. A request from this dev page is same-origin in spirit, so it
        // is presented as coming from the target; any other Origin is passed
        // through unchanged and still refused.
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq, req) => {
            if (req.headers.origin === `http://${req.headers.host}`) proxyReq.setHeader('origin', API_TARGET)
          })
        },
      },
    },
  },
})
