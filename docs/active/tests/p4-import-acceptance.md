# P4 导入验收用例

| 编号 | 场景 | 期望 |
|---|---|---|
| P4-IMPORT-001 | `local_zip` inspect 合法压缩包 | 返回 preview id、一次性 token、输入 SHA-256、过期时间和条目数，不创建业务包 |
| P4-IMPORT-002 | 压缩包路径穿越、绝对路径、冒号路径 | 返回稳定 `unsafe_archive` 错误，不落库、不提交任务 |
| P4-IMPORT-003 | 单条目超过 512 MiB 或总展开量超过 2 GiB | 返回 `unsafe_archive`，临时文件清理 |
| P4-IMPORT-004 | CurseForge/Modrinth URL | 仅接受 HTTPS 且 host 属于对应官方域名；其他 host 拒绝 |
| P4-IMPORT-005 | confirm 缺 token/hash/idempotency key | 返回 `invalid_argument` |
| P4-IMPORT-006 | preview 过期、重复确认或 token/hash 不匹配 | 返回 `preview_expired`，不得生成第二个任务 |
| P4-IMPORT-007 | 相同 Idempotency-Key 重复 confirm | 返回同一任务并标记 `reused=true`；不同 payload 返回冲突 |
| P4-IMPORT-008 | 后台 worker 执行 Import task | 任务可恢复、可取消；成功后创建包，失败留下明确任务错误，不出现静默半成品 |

执行入口：`POST /api/packs/import/inspect` 预览，`POST /api/packs/import` 确认。单机版以 JSON `content`（base64）承载本地 ZIP；URL 只保存受限来源，后续 handler 负责下载与校验。
