# ISSUE-P7-BUILD-PUBLISH

状态：P7 独立验收通过（2026-08-29；Sol 实现 + Luna 独立测试）
负责人：Sol｜P7 构建/产物/发布开发
独立验收：Luna｜P7 测试负责人

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
- P7 独立 HTTP 验收：`go test ./internal/httpapi -run '^TestP7' -count=1 -timeout=180s` 通过。
- 全量门禁：`go test ./... -count=1 -timeout=240s`、`go vet ./...`、`go fmt ./...` 均通过。
- 独立测试矩阵：`docs/tests/p7-build-publish-acceptance.md`，覆盖 P7-HTTP-001/002、BUILD、ARTIFACT、FILE、PUBLISH、TASK 全部编号。

## 已知风险

- 发布 Provider 使用现有标准化 Adapter；真实 CF/MR 网络、上游回调和平台配额仍需后续在线环境验证。
- 当前单进程 worker 已在启动时恢复任务并注册 build/publish handler；多进程协调和跨版本升级恢复仍需部署 smoke 补强。
- Windows 测试环境未授予创建 symlink 的权限，symlink 子用例记录为 N/A；普通路径穿越、根路径和导出 marker 仍已执行。
