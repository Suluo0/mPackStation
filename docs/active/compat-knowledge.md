# 兼容知识库格式(维护者整理指南)

文件:`apps/server/internal/service/compat_knowledge_baseline.json`(go:embed 进二进制, 只读, 随分发)。

## 入库纪律(硬性)

- **只放人工核实过的条目**;每条 `knownIssues` 必须填 `source`(实测记录或官方 issue/文档链接)。
- 模组一律用双平台身份标识(`mr` = Modrinth 项目 ID, `cf` = CurseForge 项目 ID),填入前必须到两个平台实测核实 ID 属实;只有单平台身份的模组可以只填一个。
- 宁缺毋滥:不确定的条目录测试环境验证后再进基线。

## knownIssues(已知冲突/问题)

```json
{
  "a": {"mr": "…", "cf": "…"},
  "b": {"mr": "…", "cf": "…"},
  "mcVersions": ["1.20.1"],
  "loaders": ["forge"],
  "severity": "fatal",
  "summary": "A 与 B 同时安装会在启动时崩溃(简述现象)",
  "fix": {"type": "install_mod", "mod": {"mr": "…", "cf": "…"}, "note": "加装 X 即可解决"},
  "source": "https://github.com/…/issues/… 或 实测: 1.20.1 forge 47.x 双装即崩"
}
```

- `mcVersions` / `loaders`:空数组 = 普遍适用;填了则只在匹配的包上生效。
- `severity`:`fatal`(启动不了/崩)在冲突列表显示 error;`warning` 显示 warning。
- `fix`:可省略。`type: "install_mod"` 时,用户添加触发模组后服务端**自动加装**修复模组(挑最新兼容当前包的版本;列表标"兼容补丁";可移除)。修复模组已在包内(任一平台身份)则不重复装。
- 无 `fix` 或修复装不上:下次"重新解析依赖"时进冲突列表,`kind = "known_issue"`,summary 附带解法文字。

## recommendations(常见兼容性模组推荐)

```json
{"mr": "…", "cf": "…", "name": "Polymorph", "reason": "一句话说明用途", "mcVersions": [], "loaders": []}
```

展示在模组页"推荐兼容模组"卡片,按包的 MC 版本/loader 过滤,已在包内的自动隐藏,一键添加走正常添加链路(自动钉镜像版本)。

## 匹配机制

包内模组用主源 ID + 镜像 ID 双侧匹配条目的 `mr`/`cf`,所以无论用户从哪个平台添加都能命中;跨平台配对由 `mod_identity_baseline.json` + 本机身份表提供。
