import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  base: '/console/',
  publicDir: '../assets',
  build: {
    outDir: '../consoleassets/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/workorch-console.js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/workorch-console[extname]',
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return undefined;
          }
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/scheduler/')) {
            return 'vendor-react';
          }
          if (
            id.includes('/antd/') ||
            id.includes('/@ant-design/icons/') ||
            id.includes('/@ant-design/icons-svg/') ||
            id.includes('/@ant-design/cssinjs/') ||
            id.includes('/@ant-design/colors/') ||
            id.includes('/@ant-design/fast-color/') ||
            id.includes('/classnames/') ||
            id.includes('/copy-to-clipboard/') ||
            id.includes('/dayjs/') ||
            id.includes('/rc-') ||
            id.includes('/react-is/') ||
            id.includes('/resize-observer-polyfill/') ||
            id.includes('/scroll-into-view-if-needed/') ||
            id.includes('/throttle-debounce/')
          ) {
            return 'vendor-antd-runtime';
          }
          if (id.includes('/zustand/')) {
            return 'vendor-state';
          }
          return undefined;
        }
      }
    }
  },
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
      '/readyz': 'http://127.0.0.1:8080',
      '/metrics': 'http://127.0.0.1:8080'
    }
  }
});
