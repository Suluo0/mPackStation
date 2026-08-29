# P5 Provider/模组主链路人机共读测试矩阵

| 编号 | 场景 | 预期 | 自动化证据 |
|---|---|---|---|
| P5-01 | fixture 搜索 `alpha` | 返回统一 Project DTO 和 total | `provider.TestFixtureAdapterNormalizesSearchAndDownload` |
| P5-02 | fixture download | 返回文件名、SHA-1、大小，不泄漏平台类型 | 同上 |
| P5-03 | Provider 503 fixture | 映射 `provider unavailable` | 同上 |
| P5-03a | HTTP adapter 404/429/5xx/timeout | 映射稳定 provider 错误，不 panic | `provider.TestHTTPAdapterSearchAndErrorMapping`, `TestHTTPAdapterTimeout` |
| P5-04 | 创建包后添加远端模组 | 同一事务写 pack_mods + jar_index + activity/outbox | `service.TestP5ModChainSearchAddResolveAndHealth` |
| P5-05 | 求解缺失依赖 | 写 required dependency、error conflict 和锁快照 hash | 同上 |
| P5-06 | 解决冲突后健康检查 | pending error 为 0，healthy=true | 同上 |

执行：`go test ./internal/provider ./internal/store ./internal/service ./internal/httpapi`、`go vet ./internal/provider ./internal/store ./internal/service ./internal/httpapi`。

未覆盖项：真实 CF/MR 网络、异步 download task、blobstore 原子落盘；对应风险见 `ISSUE-P5-provider-mod-chain.md`。
