# 构建与部署规范

## 统一入口

所有工程构建命令从仓库根目录执行。PowerShell 是 Windows 的受支持入口：

```powershell
./scripts/dev.ps1
./scripts/build.ps1 -Version 0.1.0-dev
./scripts/test.ps1
./scripts/verify.ps1 -AllowIncomplete
./scripts/package.ps1 -Version 0.1.0-dev
```

`verify.ps1` 默认是严格模式：除构建和静态检查外，还会检查 v7 预期 HTTP 路由；尚未实现的能力会使验证失败。开发基座尚未完成时，可使用 `-AllowIncomplete`，但输出中的缺口必须记录，不能作为完整验收通过。

## 构建约束

- 前端使用 `npm ci` 和 lockfile；不得使用 `npm install` 作为可重复构建步骤。
- Go 使用仓库内 `.tools/go/bin/go.exe`（存在时）或 PATH 中的 Go。
- 版本只能由 `-Version` 或 `MPACK_VERSION` 提供；默认值 `0.1.0-dev` 只用于本地开发。
- 构建会记录 Git commit 和 UTC 构建时间，但运行时不从工作树推断版本。
- 工程构建输出到 `dist/build`，分发输出到 `dist/package` 和 `dist/mpackstation-<version>.zip`。
- 分发包不包含 `data/`、数据库、缓存、JAR、导出物、API Key、token 或开发环境文件。

## 开发环境

`dev.ps1` 检查 5273 和 18871 是否空闲；任一端口被占用即失败，不会停止既有服务。它会启动本次开发服务并将日志写入 `.tmp/dev/`，不会自动终止进程。开发者必须自行确认 PID 后结束自己启动的进程。

## 部署边界

- 默认只监听 `127.0.0.1`。
- `data/` 必须使用绝对路径；数据和程序文件分离。
- 升级前先备份 SQLite 与配置元数据，再替换程序并等待就绪探针。
- migration、quick check 或 foreign key check 失败时不得报告 ready。
- 卸载程序不删除用户数据和导出物。
- 正式部署验收必须在干净临时目录执行 [单机部署 Smoke 验收](../deploy/smoke-test.md)。

## 当前已知限制

当前服务仍只有健康/就绪探针，尚未具备 v7 的完整业务 API、正式 migration runner、静态资源服务和任务恢复能力。因此本规范提供的是工程入口和验收边界，不宣称这些业务能力已经完成。

