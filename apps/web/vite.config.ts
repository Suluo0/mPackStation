import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';

/* 写操作令牌注入(auth.md):VITE_MPACK_TOKEN 环境变量优先;否则 dev 下读取
   后端数据目录的 runtime-token(服务首次启动时生成,0600)。禁止任何硬编码兜底。 */
function resolveWriteToken(): string {
  if (process.env.VITE_MPACK_TOKEN) return process.env.VITE_MPACK_TOKEN;
  try {
    return readFileSync(resolve(__dirname, '../../data/runtime-token'), 'utf8').trim();
  } catch {
    return '';
  }
}

export default defineConfig({
  plugins: [react()],
  define: {
    __MPACK_WRITE_TOKEN__: JSON.stringify(resolveWriteToken()),
  },
  server: {
    port: 5273,
    proxy: {
      // VITE_API_TARGET 可覆盖代理目标(隔离测试用),默认指向本机 dev 后端
      '/api': {target: process.env.VITE_API_TARGET || 'http://127.0.0.1:18871', changeOrigin: false},
    },
  },
});
