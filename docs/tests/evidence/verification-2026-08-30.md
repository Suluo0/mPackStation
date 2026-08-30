# 2026-08-30 当前后端测试复核

基线：HEAD 132eb14becab72170ae8901d22efd859c3d8a941；测试时有前端与 README 未提交变更。本轮没有修改功能或测试代码，也没有创建子代理。

工具：D:/workIn/mPackStation/.tools/go/bin/go.exe，go1.27.0 windows/amd64。
工作目录：D:/workIn/mPackStation/apps/server。

## 实际执行

- go test ./... -count=1 -timeout=180s：退出码 0。
- go test ./... -json -count=1 -timeout=180s：退出码 0。
- go vet ./...：退出码 0。
- gofmt -l cmd internal：列出 8 个文件，格式门禁未通过；仅检查，未修改。

JSON 终态事件见同目录 go-test-terminal-events-2026-08-30.jsonl。该文件保留本轮命令输出的 pass/fail/skip 事件和 N/A 日志，不是全部 stdout，也不是历史执行证据。

## 计数

9 个有测试的包；77 个顶层 Test + 1 个 Example 全部 pass，48 个子测试 pass，0 个 fail。cmd/server、internal/instlock 无测试文件，不计为通过测试的包。符号链接安全子场景因权限不足记录 N/A，未计入 Go skip，不能视为已验证。

P3 前缀测试 6；P4 14（DB 6、runner 6、HTTP 2）；P5 5（另有 provider 包 5 个测试）；P6 8（service 4、HTTP 4）；P7 14（service 3、httpapi 目录 11，但该目录中的多项直接调用 service）。这些数字是顶层测试函数数量，不是接口覆盖率。

## 核对到的覆盖边界

- HTTP 测试通过 ServeHTTP 调用真实路由/中间件，连接临时真实 SQLite；P5/P6 包含成功与错误流程。
- Provider HTTP adapter 使用本地 httptest.NewServer；P7 发布使用计数模拟适配器；未进行线上 CurseForge/Modrinth 发布。
- P7 真正构建 ZIP 并检查字节、元数据、hash、下载；任务恢复用修改 lease 到期时间并调用 Recover 验证，不等同进程 kill/restart 测试。
- P3/P4 部分路由存在测试没有严格断言预期状态，500/501 也可能漏检；P3 空库看板遍历不保证非空包字段被检查。
- 未找到调用导入 Inspect/Confirm 的自动化测试，表存在测试不能代替导入行为测试。
- 本轮没有做浏览器联调、干净部署或真实进程故障恢复测试。

## 格式检查输出

```text
cmd\server\main.go
internal\blobstore\blobstore.go
internal\blobstore\blobstore_test.go
internal\httpapi\httpapi.go
internal\obs\obs.go
internal\obs\obs_test.go
internal\service\import_service.go
internal\store\import_repo.go
VET_EXIT=0
```

结论：当前已编写的自动化测试实际执行并通过；用例覆盖与断言仍有缺口，格式门禁未通过，不能扩大为 P0–P7 全部验收合格。当前工具存在和当前复跑结果不能倒推出历史每次执行；此前“找不到 Go，因此从未执行”的推断不成立。
