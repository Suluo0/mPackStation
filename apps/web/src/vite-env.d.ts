/// <reference types="vite/client" />

/** 构建期由 vite.config.ts 注入的写操作令牌(来源:VITE_MPACK_TOKEN 或 data/runtime-token)。 */
declare const __MPACK_WRITE_TOKEN__: string;

interface ImportMetaEnv {
  /** 写操作令牌,优先于 data/runtime-token。不存在任何硬编码兜底。 */
  readonly VITE_MPACK_TOKEN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
