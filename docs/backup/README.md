# docs/backup/ —— 非活跃归档区

> 2026-10-05 归档（原 docs/archive/，同日改为 docs/backup/ 双区结构）。收录**已经完成使命**的过程文档：历史会话记录、一次性评审/审计、
> 已被后续版本取代的设计稿。它们不再维护，但保留了"当时为什么这么做"的决策依据，
> 排查历史问题时还有参考价值。
>
> **当前活文档在 `docs/active/`**（目录结构见 `docs/active/README.md`）；判断项目现状以代码和
> `git log` 为准（本目录内容一律以当时的日期为准）。

| 子目录 | 内容 | 归档原因 |
| --- | --- | --- |
| `superpowers/` | （active 中已无对应目录）外部 AI 工具（superpowers）生成的 plans/specs | 一次性工作产物，2026-08-30 契约符合性审计后未再更新 |
| `project-state/history/` | 2026-08~10 的会话记录、里程碑验证日志（原 `project-state/history/`） | 快照类记录，`project-state/HANDOFF.md` 与 `state.json` 才是活检查点 |
| `frontend-refactor/` | （active 中已无对应目录）web→web2 前端重构系列（00-plan ~ 04-align-pack-root） | 重构已完成并被 web3 取代；`IconRail.tsx` 等处注释仍引用其章节号 |
| `reviews/` | （active 中已无对应目录）数据库/内容提取的交叉评审与盲审报告 | 一次性评审输出，结论已落进代码与迁移注释 |
| `api/` | 2026-08-30 的契约符合性审计与实现审计 | 带日期的审计快照；现行契约以 `docs/api/contract.md` 为准 |
| `design/` | web2 时代的启动稿、分步实现计划、盲审包、旧画布方案 | 已被 `web3-ide-shell-v3.md` 及后续设计取代 |
| `architecture/` | v7 之前的能力草案、预研报告、评审轮次、launcher-core 的计划/规格/技术稿/盲审 | v7 定稿（`backend-architecture-v7.md`）与其中的结论已吸收；launcherCore 已落地，过程稿完成使命 |
