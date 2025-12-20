import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/auth/ws': {
        target: 'http://localhost:52847',
        changeOrigin: true,
        ws: true,
      },
      '/auth/configure': {
        target: 'http://localhost:52847',
        changeOrigin: true,
      },
      '/auth/context': {
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
    outDir: '../internal/ui/dist',
    emptyOutDir: true,
    sourcemap: false,
  },
})
