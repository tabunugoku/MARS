import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

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
    proxy: { '/api': 'http://localhost:25600' },
  },
})
