import {defineConfig} from 'vite';
import react from '@vitejs/plugin-react';

/* 无鉴权模式（用户 2026-10-03 定调）：本工具是本机单人 IDE，后端只监听
   127.0.0.1，物理上局域网够不到，所以不需要写令牌，也不需要「前端怎么拿到
   令牌」这条链路 —— 那条链路本身正是 401 故障的来源（vite 曾读到另一个实例
   遗留的过期 runtime-token，导致读接口正常、写接口全挂）。

   配套约束：server.host 必须是 127.0.0.1。一旦改回 0.0.0.0，等于把写库
   后门开给整个局域网，那时必须把鉴权加回来。 */
export default defineConfig({
  plugins: [react()],
  server: {
    // 只监听回环，不对局域网开放。见文件头说明。
    host: '127.0.0.1',
    // 唯一标准端口（AGENTS.md 定稿）：前端 5271，代理回唯一后端 18872。
    port: 5271,
    proxy: {
      '/api': {target: process.env.VITE_API_TARGET || 'http://127.0.0.1:18872'},
    },
  },
});
