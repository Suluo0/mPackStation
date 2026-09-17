# ISSUE-FE-004：底部状态栏「索引进度」无真实数据源

- 类型：前端数据缺口（依赖后端索引流水线）
- 状态：Open

## 现象

底部状态栏（`app-statusbar`）的「索引进度」区块一直显示「待开始」，无法反映真实索引进度。

## 根因

数据管道存在，但「模组进包 → 自动创建索引任务」流程尚未实现：

- 后端已定义 `task.KindIndex = "index"`（`apps/server/internal/task/task.go`），任务 DTO 映射为 `type=index-mod`；
- `jar_index` 表（按 sha1 存 sha256、size_bytes、fingerprint_cf）已规划，见 `docs/architecture/backend-architecture-v7.md`；
- 但当前加模组/导入流程不会创建 `index` 任务，`/api/tasks` 中从不出现 `index-mod` 任务，前端取不到进度。

## 设计依据

- `docs/specs/pack-workbench.md`：底部状态条 = 「已选模组、已安装模组、待解决冲突、后台索引进度」。
- `docs/design/dashboard-page-prompt.md` 设计原则 5：后台在索引时必须显示进度，静默后台 = 用户以为卡死。
- `docs/architecture/backend-architecture-v7.md`：`index-mod` = 模组进入包后的 JAR/元数据索引。

## 完成条件

1. 后端在模组进包 / 导入完成后创建 `index` 任务，写入 `jar_index`，进度随 phase 更新。
2. 前端底部状态栏轮询 `/api/tasks` 中 `type=index-mod` 的任务，展示进度条与百分比；无任务时保持「待开始」。

## 验收证据

- 添加/导入模组后，「索引进度」出现真实进度并能推进到完成。
- 无索引任务时仍显示「待开始」，不报错。
