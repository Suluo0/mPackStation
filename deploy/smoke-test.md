# 单机部署 Smoke 验收

## 目的

验证一个全新的分发目录可以在独立临时数据目录中启动、返回探针、关闭，并且不会触碰仓库 `data/` 或其他用户数据。该测试只操作自己创建的临时目录和自己启动的进程。

## Windows

在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/package.ps1 -Version smoke
powershell -ExecutionPolicy Bypass -File deploy/windows/smoke-test.ps1
```

脚本会：

1. 使用已有 smoke 分发包；不存在时明确失败，不会隐式重新构建。
2. 在系统临时目录创建唯一数据目录。
3. 检查测试端口并启动本次测试的 `mpackstation.exe`。
4. 轮询 `/api/healthz` 和 `/api/readyz`。
5. 检查响应状态和 JSON 字段。
6. 只结束自己启动的进程，并清理自己创建的临时目录。

端口被占用时测试失败，不会停止或重启占用端口的既有进程。

## 人机共读验收记录

| 编号 | 前置条件 | 操作 | 通过标准 | 证据 |
|---|---|---|---|---|
| DEP-001 | 已生成 smoke 分发包 | 在干净临时数据目录启动 | 进程启动且无 panic | 进程日志、退出码 |
| DEP-002 | 服务已启动 | GET `/api/healthz` | HTTP 200，状态为 `ok` | 请求与响应 |
| DEP-003 | 数据库可用 | GET `/api/readyz` | HTTP 200，状态为 `ready`，`db=true` | 请求与响应 |
| DEP-004 | 服务运行中 | 发送终止信号 | 在超时内退出，不产生第二写实例 | 退出码、日志 |
| DEP-005 | 服务已关闭 | 使用同一数据目录再次启动 | 探针再次就绪，数据库未损坏 | 两次启动日志 |
| DEP-006 | 已有监听进程 | 执行 dev/smoke | 脚本明确报告端口占用，不停止已有进程 | 脚本输出 |
| DEP-007 | 检查分发目录 | 列出包内容 | 不包含 `data/`、数据库、缓存、JAR、密钥和 `.env` | 文件清单 |

任何一个适用项缺少可复核证据，都只能记为“尚未证明合格”。

