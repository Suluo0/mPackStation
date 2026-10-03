import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';

/* 写操作令牌注入：机制与 web2 一致（VITE_MPACK_TOKEN 环境变量优先，
   否则读后端数据目录的 runtime-token）。禁硬编码兜底 —— 注入为空时写请求 401，错误如实抛给界面。 */
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
  console.warn('[web3] 写令牌为空：读接口可用，写接口会被后端 401。注入 VITE_MPACK_TOKEN 后再启动。');
}

export default defineConfig({
  plugins: [react()],
  define: {__MPACK_WRITE_TOKEN__: JSON.stringify(token)},
  server: {
    host: '0.0.0.0',
    // 唯一标准端口（AGENTS.md 定稿）：前端 5271，代理回唯一后端 18872。
    port: 5271,
    proxy: {
      // changeOrigin 必须为 false：后端用透传的 Host 判定同源（web2 同款约束）。
      '/api': {target: process.env.VITE_API_TARGET || 'http://127.0.0.1:18872', changeOrigin: false},
    },
  },
});
