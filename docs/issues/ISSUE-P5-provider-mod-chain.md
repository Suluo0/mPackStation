# ISSUE-P5-001 Provider 与模组主链路

状态：已实现 P5 单机 fixture 闭环（本提交）；真实平台 HTTP、blobstore 和异步下载任务列入后续增强。

## 范围

- `internal/provider` 提供统一领域 DTO、Provider 接口、注册表、标准库 HTTP CurseForge/Modrinth adapter 和离线 fixture adapter。
- `internal/service` 编排包作用域搜索、模组加入/移除/启停/版本选择、SHA-1/JAR 索引、依赖求解、不可变锁快照、冲突与健康摘要。
- `internal/store` 是唯一 SQL 边界；写入模组、JAR 索引、锁、依赖、冲突同时产生 outbox/activity。
- `internal/httpapi` 暴露 `/api/packs/{packId}/mods`、`mod-search`、`resolve`、`locks`、`conflicts`、`health` 端点。

## 安全与边界

Provider 平台形状不泄漏到 service/httpapi；fixture 下载只登记 `jar://<sha1>` 对象句柄，不接受客户端服务器路径。当前不引入 tenant/workspace、账号管理或多实例协作。

## 后续风险

真实运行时需把 download 接入持久化 task 和 blobstore 的 temp→校验→事务登记→rename 流程，并补统一限流/熔断、离线缓存与远端发布任务；HTTP adapter 已具备 endpoint、超时和 404/401/429/5xx 映射，默认不启用任何真实端点。
