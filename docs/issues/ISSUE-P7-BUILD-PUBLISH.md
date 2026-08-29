# ISSUE-P7-BUILD-PUBLISH

状态：未开始（P6 完成后进入）  
负责人：Sol｜P7 构建发布开发  
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
