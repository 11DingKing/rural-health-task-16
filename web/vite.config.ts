import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// https://vitejs.dev/config/
// During development the Vite dev server proxies API calls and health
// checks to the Go backend listening on port 52661. In production the
// Go server serves the built assets from web/dist at the same origin,
// so the proxy is only needed for `npm run dev`.
const BACKEND = 'http://localhost:52661';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: BACKEND, changeOrigin: true },
      '/healthz': { target: BACKEND, changeOrigin: true },
      '/readyz': { target: BACKEND, changeOrigin: true },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    emptyOutDir: true,
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom'],
          'antd-vendor': ['antd'],
          router: ['react-router-dom'],
        },
      },
    },
  },
});
