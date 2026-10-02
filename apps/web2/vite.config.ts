import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';

/* 写操作令牌注入(auth.md 决策 D-8)：VITE_MPACK_TOKEN 环境变量优先；否则读后端数据目录的
   runtime-token。禁止任何硬编码兜底 —— 注入为空时写请求会被后端 401，错误如实抛给界面。 */
function resolveWriteToken(): string {
  if (process.env.VITE_MPACK_TOKEN) return process.env.VITE_MPACK_TOKEN;
  try {
    return readFileSync(process.env.MPACK_TOKEN_FILE ?? resolve(__dirname, '../../data/runtime-token'), 'utf8').trim();
  } catch {
    return '';
  }
}

const token = resolveWriteToken();
if (!token) {
  console.warn('[web2] 写令牌为空：读接口可用，写接口会被后端 401。按方案 §6 的方式注入 VITE_MPACK_TOKEN。');
}

export default defineConfig({
  plugins: [react()],
  define: {__MPACK_WRITE_TOKEN__: JSON.stringify(token)},
  server: {
    host: '0.0.0.0',
    // 默认 5274 给隔离验收裸跑用；正式入口由 scripts/dev.sh 传 --port 5273 覆盖。
    port: 5274,
    proxy: {
      // changeOrigin 必须保持 false：后端用透传的 Host 判定同源，改写后局域网 Origin 会被判成跨站 403。
      // 默认打日常后端 18871（scripts/dev.sh 起的那个）；验收/链路测试用 VITE_API_TARGET 显式指到隔离基座。
      '/api': {target: process.env.VITE_API_TARGET || 'http://127.0.0.1:18871', changeOrigin: false},
    },
  },
});
