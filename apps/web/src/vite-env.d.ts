/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** 写操作令牌，必须与服务端 MPACK_TOKEN 一致。未设置时 dev 下回落到 "test"。 */
  readonly VITE_MPACK_TOKEN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
