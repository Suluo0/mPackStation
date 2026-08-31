# 契约验收 (scripts/verify-contract.sh)

重构"行为不变"的可回放证明。四层全绿才算过:

| 层 | 内容 | 计数 |
|----|------|------|
| 1 | go vet / gofmt / go test / 前端 tsc | 全绿 |
| 2 | curl 契约矩阵(`contract/curl-matrix.sh`) | 41 项 |
| 3 | E2E 直连:真实前端 API 层(`apps/web/src/api/*`,同一份 zod)打包后打活后端 | 22 项 |
| 4 | E2E 经 vite 代理(同 harness,base 换 vite dev 端口) | 22 项 |

## 运行

```bash
bash scripts/verify-contract.sh            # 全部四层
bash scripts/verify-contract.sh --skip-proxy  # 跳过层4(不起 vite)
```

Windows 双击: `scripts/verify-contract.bat`(需 git-bash 在 PATH)。

## 行为

- 测试实例: `127.0.0.1:18899`,vite 代理 `5274`;**不动**日常 dev 实例(18871)。
- 数据目录: `scripts/contract/.tmp/`(已 gitignore,每次运行清空重建)。
- 日志: `.tmp/server.log` / `.tmp/vite.log`。
- 退出码 0 = 全绿;非 0 时末尾列出失败层。
- token 来自测试实例自动生成的 `.tmp/data/runtime-token`,无需手配。

## 单独跑某一层

```bash
# curl 矩阵(需要一个已在运行的实例)
BASE=http://127.0.0.1:18899 TOKEN=$(cat scripts/contract/.tmp/data/runtime-token) \
  TMPDIR=scripts/contract/.tmp bash scripts/contract/curl-matrix.sh
```
