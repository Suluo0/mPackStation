# API 文档入口

本目录采用“规范、契约、状态、证据”分层。接口开发和联调以契约为准，代码现状不得反向修改契约。

## 阅读顺序

1. [`standards.md`](./standards.md)：全局规则
2. [`contract.md`](./contract.md)：逐接口请求、响应和错误
3. [`dto.md`](./dto.md)：跨接口复用的数据结构
4. [`errors.md`](./errors.md)：错误码与 HTTP 状态映射
5. [`auth.md`](./auth.md)：单机 token 和安全边界
6. [`implementation-status.md`](./implementation-status.md)：当前实现、自动化测试、浏览器验收状态
7. [`integration-matrix.md`](./integration-matrix.md)：前端页面到 API 的接线情况
8. [`audit/`](./audit/)：带日期的审计记录

`inventory.md` 是早期盘点资料，保留用于追溯，不作为当前状态的唯一来源。

## 文档状态词

- **规范**：必须满足的目标
- **已实现**：代码已具备能力
- **自动化通过**：测试已实际执行并通过
- **浏览器验收通过**：真实前端页面已调用并检查结果
- **联调完成**：前后端、错误路径和刷新/重试行为均验证
- **阻塞**：存在明确外部依赖或产品决策
