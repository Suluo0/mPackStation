/// <reference types="vite/client" />

/** 构建期由 vite.config.ts 的 define 注入。为空表示未配置令牌，写请求会 401。 */
declare const __MPACK_WRITE_TOKEN__: string;
