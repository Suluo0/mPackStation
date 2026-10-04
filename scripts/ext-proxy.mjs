#!/usr/bin/env node
/* 外部访问代理：把组网网卡（默认 EasyTier 虚拟 IP）上的 5271 转发到本机回环的
   vite 前端。用途：人在外面，用手机 / 其他 mesh 设备访问家里 Mac 上的工作台。

   设计约束（2026-10-04）：应用本体仍只监听 127.0.0.1 —— vite.config.ts 头注里
   「无鉴权模式」的安全前提不变。这个转发口只绑 mesh 虚拟 IP，不绑 0.0.0.0，
   所以局域网与公网都够不到它；能访问的只有拿着 EasyTier 网络密钥的设备。
   后端的 Host/Origin 校验对私网同源访问本来就放行（httpapi.validHost/validOrigin），
   因此走这条链路不需要任何应用侧改动。

   可用环境变量：EXT_HOST（默认自动探测 EasyTier 虚拟 IP）、EXT_PORT（默认 5271）、
   EXT_TARGET（默认 127.0.0.1:5271）。 */
import net from 'node:net';
import {execFileSync} from 'node:child_process';

function detectEasyTierIP() {
  try {
    const out = execFileSync('easytier-cli', ['node'], {encoding: 'utf8', timeout: 4000});
    // 输出是表格，本机虚拟 IP 在 Virtual IP 列；取第一个 IPv4 即可（本机行在最前）。
    const m = out.match(/(\d+\.\d+\.\d+\.\d+)\/\d+/);
    return m ? m[1] : null;
  } catch {
    return null; // easytier 不在线或没装
  }
}

const target = process.env.EXT_TARGET ?? '127.0.0.1:5271';
const port = Number(process.env.EXT_PORT ?? 5271);
let host = process.env.EXT_HOST ?? '';
if (!host) {
  host = detectEasyTierIP();
  if (!host) {
    console.error('[ext-proxy] 没探测到 EasyTier 虚拟 IP（easytier 在线吗？）。可用 EXT_HOST=x.x.x.x 指定后重试。');
    process.exit(1);
  }
}
const sep = target.lastIndexOf(':');
const tHost = target.slice(0, sep);
const tPort = Number(target.slice(sep + 1));

const server = net.createServer(client => {
  const upstream = net.connect(tPort, tHost);
  client.pipe(upstream);
  upstream.pipe(client);
  const cleanup = () => { client.destroy(); upstream.destroy(); };
  client.on('error', cleanup);
  upstream.on('error', cleanup);
  client.on('close', cleanup);
  upstream.on('close', cleanup);
});

server.on('error', e => {
  console.error(`[ext-proxy] 监听 ${host}:${port} 失败: ${e.message}`);
  process.exit(1);
});
server.listen(port, host, () => {
  console.log(`[ext-proxy] ${host}:${port} → ${target}`);
  console.log(`[ext-proxy] mesh 内设备打开 http://${host}:${port} 即可访问工作台`);
});
