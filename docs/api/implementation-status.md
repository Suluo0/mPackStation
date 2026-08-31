# 实现与验收状态

本表只记录当前工作树的实然状态;目标规范见 `standards.md` / `contract.md`。
2026-08-30 契约追平轮次后更新(见 `audit/2026-08-30-contract-conformance.md`)。

| 领域 | 后端能力 | 前端真实 API | 自动化测试 | 浏览器验收 | 结论 |
|---|---|---|---|---|---|
| 看板/系统健康 | 已实现 | 已接 | 已执行通过 | 部分 | 主流程可用;需重启服务验证健康三态 |
| 整合包 | 已实现 | 已接列表、工作台 | 已执行通过 | 部分 | 重名 422 pack_name_duplicate、MC 版本候选 422 已落地并经 curl 验证 |
| 模组/依赖 | 已实现 | 已接主要列表和操作 | 已执行通过 | 部分 | mod-search 参数 q 对齐契约;mod_incompatible_version 校验仍未实现(契约已定义,实现缺口) |
| 导入 | 已实现两阶段流程 | 已接 | 已执行通过 | 未完整验收 | 契约追平完成:400/410/409/422 拆分、幂等重放返原任务、响应不再内嵌 task,全部经 curl 实测 |
| 内容/任务书 | 已实现 | 已接基础读取/保存 | 已执行通过 | 未完整验收 | 412 revision_conflict、422 细分(content_invalid/quest_*)、apply 出参带 revision、前端 revision_conflict 专门提示已落地 |
| 构建/产物 | 已实现 | 部分接线 | P7 用例已执行 | 未完整验收 | build_blocked(422)/artifact_expired(410)/pack_version_not_found 等错误码已追平;下载和页面闭环未完成 |
| 发布 | 已实现 provider/异步任务 | API 函数已有,页面未完全闭合 | P7 用例已执行 | 未完整验收 | 异步入队出参对齐契约 {taskId, reused};release_not_retryable(409)/release_artifact_not_ready(422) 已落地;真实 provider 凭据场景待验证 |
| 任务 | 已实现列表/控制/详情/日志 | 列表和控制已接 | 已执行通过 | 部分 | Task DTO 已全局统一(dto.md 结构),tasks/activities 均为 ListEnvelope,progress 归一 0-100 int |
| 设置 | 部分能力 | 已接 system/status | 无完整测试 | 未验收 | 硬编码区块(清理缓存/默认包配置/恢复默认)已按 D-11 移除;/api/settings 与 cache purge 需要时再立项 |
| 鉴权 | 已实现 | 构建期注入 | 已执行通过 | 部分 | token 启动生成并持久化 data/runtime-token(0600),test 兜底已删;前端 vite define 注入已验证 |

最近一次可复现验证(2026-08-31):后端 `go build/vet/fmt/test ./...` 全绿;前端 `tsc --noEmit` + `npm run build` 通过;curl 契约矩阵 29 项 + 前端 API 层 E2E 22 项(直连与 vite 代理各一轮)全部 PASS(证据见 audit 文件)。两者不等同于全量浏览器联调完成。
