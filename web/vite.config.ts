import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      // 認証関連エンドポイントをGoサーバーへプロキシ
      '/auth/ws': {
        target: 'http://localhost:52847',
        changeOrigin: true,
        ws: true, // WebSocket対応
      },
      '/auth/config': {
        target: 'http://localhost:52847',
        changeOrigin: true,
      },
      '/auth/configure': {
        target: 'http://localhost:52847',
        changeOrigin: true,
      },
      '/auth/popup': {
        target: 'http://localhost:52847',
        changeOrigin: true,
      },
      '/callback': {
        target: 'http://localhost:52847',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
  },
})
