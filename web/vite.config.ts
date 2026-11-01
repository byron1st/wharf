import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The dev server proxies /api to the Wharf server from `make run`. The server
// only answers requests whose Host is loopback and whose Origin, if any, is
// that same host. The proxy rewrites Host, and rewrites Origin only for the
// dev server's own pages; any other Origin passes through unchanged so the
// server still rejects cross-site requests with 403.
const apiTarget = new URL(process.env.WHARF_API ?? 'http://127.0.0.1:8080')

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  server: {
    proxy: {
      '/api': {
        target: apiTarget.origin,
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq, req) => {
            if (req.headers.origin === `http://${req.headers.host}`) proxyReq.setHeader('origin', apiTarget.origin)
          })
        },
      },
    },
  },
  build: {
    // Keep the committed dist/.gitkeep that lets go:embed build without a web build.
    emptyOutDir: false,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
})
