import { createHash } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

const API_TARGET = 'http://localhost:25600'

/** Stamps dist/sw.js with this build: a version derived from index.html (so
 *  every deploy changes the worker's bytes, the browser installs it and old
 *  caches are pruned) and the entry files index.html loads (precached with
 *  the shell, so the offline fallback never points at files it lacks). */
function stampServiceWorker(): Plugin {
  let outDir = 'dist'
  return {
    name: 'mars-sw-stamp',
    apply: 'build',
    configResolved(c) {
      outDir = path.resolve(c.root, c.build.outDir)
    },
    closeBundle() {
      const sw = path.join(outDir, 'sw.js')
      const html = fs.readFileSync(path.join(outDir, 'index.html'), 'utf8')
      const version = createHash('sha256').update(html).digest('hex').slice(0, 12)
      const entry = [...new Set([...html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)].map((m) => m[1]))]
      const src = fs.readFileSync(sw, 'utf8')
      const out = src
        .replace('const VERSION = "dev";', `const VERSION = ${JSON.stringify(version)};`)
        .replace('const ENTRY = [];', `const ENTRY = ${JSON.stringify(entry)};`)
      if (out === src || !out.includes(version)) throw new Error('sw.js: VERSION/ENTRY placeholders not found')
      fs.writeFileSync(sw, out)
    },
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss(), stampServiceWorker()],
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
    // Dev against the Go server: MARS_DEV=1 go run ./cmd/mars
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
      // The OPDS catalog and the Komga-compatible API (read-only apps; no
      // cookie writes from this page), so their addresses work in dev too.
      '/opds': { target: API_TARGET, changeOrigin: true },
      '/komga': { target: API_TARGET, changeOrigin: true },
    },
  },
})
