import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// 构建产物输出到后端静态目录，保持与现有服务兼容。
export default defineConfig({
  plugins: [react()],
  base: './', // 使用相对路径，方便反向代理访问
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': 'http://localhost:8080',
      '/static': 'http://localhost:8080'
    }
  },
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  build: {
    outDir: resolve(__dirname, '../pkg/webui/frontend'),
    emptyOutDir: true,
    sourcemap: false
  }
});
