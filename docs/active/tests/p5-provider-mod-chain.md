# P5 Provider/模组主链路人机共读测试矩阵

| 编号 | 场景 | 预期 | 自动化证据 |
|---|---|---|---|
| P5-01 | fixture 搜索 `alpha` | 返回统一 Project DTO 和 total | `provider.TestFixtureAdapterNormalizesSearchAndDownload` |
| P5-02 | fixture download | 返回文件名、SHA-1、大小，不泄漏平台类型 | 同上 |
| P5-03 | Provider 503 fixture | 映射 `provider unavailable` | 同上 |
| P5-03a | HTTP adapter 404/429/5xx/timeout | 映射稳定 provider 错误，不 panic | `provider.TestHTTPAdapterSearchAndErrorMapping`, `TestHTTPAdapterTimeout` |
| P5-03b | HTTP adapter DTO 与元数据 | Modrinth project/version/dependency/file JSON 映射到统一 DTO；CurseForge `{data:...}` envelope、数值 ID、文件 hash 也必须被归一化 | `provider.TestHTTPAdapterMetadataAndDownloadJSON`, `TestHTTPAdapterCurseForgeEnvelopeNormalization` |
| P5-04 | 创建包后添加远端模组 | 同一事务写 pack_mods + jar_index + activity/outbox | `service.TestP5ModChainSearchAddResolveAndHealth` |
| P5-05 | 求解缺失依赖 | 写 required dependency、error conflict 和锁快照 hash | 同上 |
| P5-06 | 解决冲突后健康检查 | pending error 为 0，healthy=true | 同上 |
| P5-07 | 两个包共享同一 SHA-1 | `jar_index` 只有一条、两个 `pack_mods` 各自归属；跨包更新/删除/冲突操作返回 not found | `service.TestP5AcceptanceModLifecycleAndCrossPackIsolation` |
| P5-08 | 模组启停、版本切换、移除 | disabled/installed/version 更新可验证，移除不影响其他包 | 同上 |
| P5-09 | 未配置 Provider 与 404/429/503 | service 返回稳定领域错误；HTTP 返回错误信封、request-id 和预期状态码 | `service.TestP5AcceptanceUnconfiguredAndInvalidProviderResults`, `httpapi.TestP5HTTPModChainAndStableProviderErrors` |
| P5-10 | 锁快照与依赖证据 | 依赖绑定 lock、快照可解析且 hash 存在，重复求解冲突按 fingerprint 幂等，历史快照不变 | `service.TestP5AcceptanceLockSnapshotAndResolutionEvidence` |
| P5-11 | HTTP 主链路 JSON | 搜索、加入、列表、启停、求解、锁、健康、移除的响应字段和状态码符合契约 | `httpapi.TestP5HTTPModChainAndStableProviderErrors` | 通过 |

执行：`go test ./internal/provider ./internal/store ./internal/service ./internal/httpapi`、`go vet ./internal/provider ./internal/store ./internal/service ./internal/httpapi`。

Luna 独立验收提交 `67e5226`：矩阵中的 fixture、HTTP adapter、service/store 和 HTTP 主链路用例均通过。曾发现的 `nextCursor`、resolve activity/outbox、CurseForge envelope/数值 ID/字段映射问题已修复并重验。未覆盖真实 CF/MR 网络、异步 download task、blobstore 原子落盘；对应风险见 `ISSUE-P5-provider-mod-chain.md`。
