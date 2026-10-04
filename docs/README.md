# docs/ —— 双区结构：active / backup

- **`active/`** —— 唯一可以编辑的文档区。仍在生效的契约、架构、规范、规格、问题台账、验收证据、活检查点都在这里。
- **`backup/`** —— 非活跃归档区。与 active **同构镜像**（backup/api ↔ active/api……）；完成使命的文档移到这里，只读参考，不再维护。

## 文档维护规则（2026-10-05 用户定稿，AGENTS.md 同步收录）

1. **文档只允许维护到 `active/` 目录**：修改、新增一律发生在 active；`backup/` 只读，不允许直接编辑。
2. **任务完成提交时，应当对活跃的文档归档为非活跃状态**：把本次任务中已被取代或执行完毕的文档，从 `active/` 移到 `backup/` 的同构路径下（例如 `active/api/x.md` → `backup/api/x.md`），随本次提交一起入库。

目录明细见 `active/README.md`；归档分组的来龙去脉见 `backup/README.md`。
