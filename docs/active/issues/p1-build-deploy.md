# P1 构建与部署工程化任务

状态：进行中（脚本基座已提交，完整 P1 验收等待后端基座能力补齐）

## 范围

- 建立 Windows PowerShell 的 dev/build/test/verify/package 统一入口。
- 生成不携带用户数据和密钥的单机分发目录与 zip。
- 提供干净临时目录启动、探活、关闭、再次启动的 smoke 验收路径。
- 明确端口占用和已有服务保护行为。

## 完成条件

- `scripts/build.ps1` 可重现构建前端与 Go 服务，并记录版本元数据。
- `scripts/verify.ps1` 对构建检查和未完成 v7 路由进行明确 PASS/FAIL/WARNING 报告。
- `scripts/package.ps1` 不打包 `data/`、缓存、JAR、导出物或秘密。
- `deploy/windows/smoke-test.ps1` 只清理自己创建的临时目录和自己启动的进程。
- 对应人机共读用例见 `deploy/smoke-test.md`。

## 当前阻塞/后续工作

- 服务入口还未加载 v7 全量配置和正式 migration runner。
- 业务路由、任务恢复、文件巡检和静态资源服务尚未完成。
- 当前 smoke 仅证明现有健康/就绪探针和进程生命周期，不能代替 P2-P7 业务验收。

