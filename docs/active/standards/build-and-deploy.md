# 构建与部署规范

## 统一入口

所有工程构建命令从仓库根目录执行。macOS/Linux 用 bash 脚本，Windows 用 PowerShell/bat，两套入口端口、数据目录、代理口径完全一致（2026-10-05 对齐）：

```bash
./scripts/dev.sh                 # macOS/Linux：后端 18872 + 前端 5271
./scripts/dev-stop.sh
```

```powershell
./scripts/dev.ps1                # Windows：同口径
./scripts/dev.ps1 -DataDir D:\mpack-data
./scripts/build.ps1 -Version 0.1.0-dev
./scripts/build-launcher.ps1     # 编译启动器内核 → .tools/launcher/
./scripts/dev-reset.ps1          # 归档并清空测试数据，回到干净起点
./scripts/test.ps1
./scripts/verify.ps1 -AllowIncomplete
./scripts/package.ps1 -Version 0.1.0-dev
```

> 唯一权威口径见仓库根 `AGENTS.md`「服务与环境铁律」：后端 `127.0.0.1:18872`、前端 `5271`（`apps/web3`）。`5173/5273/5274/5275/5276/18871/18880` 一律作废。`scripts/dev.sh` 曾单方面声明「Windows 版退役」，2026-10-05 已恢复双入口——Windows 脚本此前一直指向已作废的 `apps/web`（web1）与 `18871/5273`，直接运行会起错前端、写错数据目录。

`verify.ps1` 默认是严格模式：除构建和静态检查外，还会检查 v7 预期 HTTP 路由；尚未实现的能力会使验证失败。开发基座尚未完成时，可使用 `-AllowIncomplete`，但输出中的缺口必须记录，不能作为完整验收通过。

## 构建约束

- 前端使用 `npm ci` 和 lockfile；不得使用 `npm install` 作为可重复构建步骤。
- Go 使用仓库内 `.tools/go/bin/go.exe`（存在时）或 PATH 中的 Go。
- 版本只能由 `-Version` 或 `MPACK_VERSION` 提供；默认值 `0.1.0-dev` 只用于本地开发。
- 构建会记录 Git commit 和 UTC 构建时间，但运行时不从工作树推断版本。
- 工程构建输出到 `dist/build`，分发输出到 `dist/package` 和 `dist/mpackstation-<version>.zip`。
- 分发包不包含 `data/`、数据库、缓存、JAR、导出物、API Key、token 或开发环境文件。
- 启动器内核（`launcherCore`，Rust）不在 `build.ps1` 的构建范围内，需单独 `cargo build --release`（或 `scripts/build-launcher.ps1`）。cargo 的 target 目录不要放在网络盘或外置卷上，增量编译会慢一个数量级。

## 开发环境

`dev.sh` / `dev.ps1` 检查 18872 与 5271 是否空闲；任一端口被占用即失败，不会停止既有服务。它会启动本次开发服务并将日志写入 `.tmp/dev/`，不会自动终止进程。开发者必须自行确认 PID 后结束自己启动的进程，或用 `dev-stop.sh` / `dev-stop.ps1` 收尾。

开发数据目录：macOS/Linux 为 `/tmp/mpack-data`，Windows 为 `%TEMP%\mpack-data`（可用 `MPACK_DEV_DATA` 或 `-DataDir` 覆盖）。测试数据全部可再生产——包记录、模组台账、任务日志、导入暂存、项目实例目录都能从整合包重新生成。要回到干净起点用 `dev-reset.ps1`（归档而非直接删），不要手工挑单个文件删，否则容易出现「目录删了、库里还留着包记录」的半清理状态。

`dev.ps1` 会先探测启动器内核并注入 `MPACK_LAUNCHER_BIN`。内核查找顺序：环境变量 → `<数据目录的父目录>/.tools/launcher/mpack-launcher[.exe]` → PATH。内核缺失只影响「安装整合包 / 启动游戏」两个任务（提交时同步返回 503），其余能力正常。

## 部署边界

- 默认只监听 `127.0.0.1`。
- `data/` 必须使用绝对路径；数据和程序文件分离。
- 升级前先备份 SQLite 与配置元数据，再替换程序并等待就绪探针。
- migration、quick check 或 foreign key check 失败时不得报告 ready。
- 卸载程序不删除用户数据和导出物。
- 正式部署验收必须在干净临时目录执行 [单机部署 Smoke 验收](../deploy/smoke-test.md)。

## 当前已知限制

当前服务仍只有健康/就绪探针，尚未具备 v7 的完整业务 API、正式 migration runner、静态资源服务和任务恢复能力。因此本规范提供的是工程入口和验收边界，不宣称这些业务能力已经完成。

