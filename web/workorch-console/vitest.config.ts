import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    include: ['tests/components/**/*.test.tsx'],
    setupFiles: ['tests/components/setup.ts'],
    // 真实 Ant Design/jsdom 挂载串行执行，避免 CSS 计算争用；保留原单测期限。
    fileParallelism: false,
  },
});
