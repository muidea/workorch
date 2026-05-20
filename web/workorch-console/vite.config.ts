import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  base: '/console/',
  build: {
    outDir: '../../internal/modules/application/workorchd/service/server/consoleassets/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/workorch-console.js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/workorch-console[extname]',
        manualChunks: {
          'vendor-react': ['react', 'react-dom'],
          'vendor-antd': ['antd', '@ant-design/pro-components', '@ant-design/icons'],
          'vendor-state': ['zustand']
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
