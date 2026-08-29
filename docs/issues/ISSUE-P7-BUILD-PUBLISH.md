# ISSUE-P7-BUILD-PUBLISH

状态：实现完成，待独立验收与 HTTP 接线
负责人：Luna｜P7 构建/产物/发布开发负责人
独立验收：待派 Luna｜P7 测试负责人

## 范围

- delivery checks、pack version 输入快照、可复现本地 zip 和 artifact 登记。
- artifact SHA-256/来源快照、导出目录安全边界和清单校验。
- CurseForge/Modrinth 发布任务、远端状态查询、失败恢复。

## 验收门槛

- 相同锁定快照和输入生成相同 artifact 指纹；临时文件失败不会登记为有效产物。
- 发布任务支持幂等键、状态轮询和可恢复失败；非幂等发布不自动重试。
- 路径、磁盘空间、缺失输入和校验失败返回稳定错误码，不泄漏绝对路径或凭据。
- 构建/导出/发布的成功、失败、取消、重启恢复和重复提交均有测试。
- 干净目录构建、打包和 smoke 证据可复现。

## 交付物

- P7 生产代码、契约和必要递增 migration。
- 独立 Luna 测试矩阵、fixture、失败样本和 issue 结论。
- commit、产物校验值和剩余风险说明。

## 当前实现证据

- `internal/service/p7_build.go`：输入快照规范化、SHA-256 来源指纹、固定时间戳排序 ZIP、导出目录登记、临时文件校验后原子 rename、artifact 幂等登记。
- `internal/service/p7_publish.go`：releases 幂等键、local 发布、Provider 发布状态保存、显式失败重试和远端状态轮询；同一请求不会自动再次触发非幂等 Provider 调用。
- `internal/store/p7_repo.go`：P7 领域表的唯一 repository 边界；不修改既有 migration。
- 当前 `go test ./internal/service ./internal/store` 与 `go vet ./internal/service ./internal/store` 已通过；全量门禁和独立代理复核仍待完成。

## 已知风险

- 本提交未修改 `httpapi.go`，P7 暂未暴露真实 HTTP 端点，也未在进程装配处注册独立发布 worker/task handler；调用方需先使用 service API。
- 发布 Provider 使用现有标准化 Adapter；生产 Provider 的远端幂等语义和真实状态回调仍需后续契约测试验证。
