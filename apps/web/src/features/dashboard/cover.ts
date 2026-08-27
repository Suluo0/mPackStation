/* 封面占位：mock 期没有真实封面图，用 CSS 渐变生成 MC 风景感色块。
   每个包按 id 哈希稳定取色，纯展示逻辑，不进数据契约。 */

const palettes: Array<[string, string]> = [
  ['#7ec8f2', '#3d7a4f'], // 晴天草原
  ['#f2a65e', '#7a4b9e'], // 黄昏山地
  ['#5ed3b3', '#1f6f8b'], // 海洋群岛
  ['#e26d5c', '#5b2a4a'], // 下界荒原
  ['#a3d977', '#3f7d3a'], // 针叶林
  ['#8b8bd8', '#33335c'], // 末地夜空
];

export function packCoverGradient(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  const [from, to] = palettes[hash % palettes.length];
  return `linear-gradient(160deg, ${from}, ${to})`;
}
