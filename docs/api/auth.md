# 单机鉴权与安全边界

当前版本不引入账号体系和多租户隔离。鉴权只用于保护本机写操作，并为未来 GitHub 集成保留入口。

## Token 生命周期

1. 服务首次启动生成随机高熵 token；
2. token 持久化到 `data/runtime-token`（权限仅当前用户可读）；
3. 后续启动复用该 token；
4. 前端通过运行时注入获得 token，禁止源码硬编码和 `test` 兜底；
5. 非 GET/HEAD/OPTIONS 请求必须发送 `X-MPack-Token`。

缺失或错误 token 返回 401；服务尚未生成/读取 token 返回 503 `auth_not_configured`。GET 是否需要 token 不得在各接口自行发挥，统一按本规则执行。

## 其他边界

- Host 仅允许本机地址；Origin 仅允许配置的前端来源；
- 请求体上限 8 MB；
- 导出目录必须预先登记并限制在允许范围；
- provider 凭据只存后端数据目录，不进入前端 bundle 或日志。
