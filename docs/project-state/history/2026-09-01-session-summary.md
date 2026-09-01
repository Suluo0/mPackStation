# 2026-09-01 会话详录

分支 `DEV_2609-VK1`,HEAD `3df1ce4`(12 个本地提交未推送),工作区干净。

## 本会话交付(全部四层验收绿后逐项提交)

### 0. 验收基础设施(commit 0bdc621)
`scripts/verify-contract.{sh,bat}` 四层入口 + `scripts/contract/`(curl 矩阵/e2e-harness/README/gofmt 基线)。Windows 坑三连修:gofmt 历史 CRLF 老文件改基线豁免制;`curl -o /dev/null` 恒 exit 23 改 shell 重定向;MSYS 路径传原生工具一律 cygpath 转 Windows 路径。矩阵两处历史假断言修正(crafting payload 撞 fields 白名单;不存在的 imports/apply 路由删除)。

### 1. B 系后端架构改造(行为保持,顺序 B1→B3→B2→B6→B7→B4)
- B1 `9b55619`:消灭 `source any`/`FromSource` 装配,单一 `New(db)` 路径,grep 归零
- B3 `b2d6fda`:四个错误翻译官归一 `writeError`;幂等冲突统一 422 对齐契约;修 P7 残留 404
- B2 `1bdb7fa`:httpapi god-file 1395→385 行,拆 routes_*.go 8 个域文件;**73 条路由前后 diff 完全一致**
- B6 `c7e3b88`:DTO 空值恒发 null(activeRevisionId 等),前端 schema optional→nullable
- B7 `e61c114`+`58f404f`:pN 文件 git mv 改域名(保留历史),gofmt 基线重生成
- B4 `464d61d`:TaskView/EventView 移出 task 包进 service/task_api.go,task/http_adapter.go 只留队列控制
- **B5(拆 service.API + Prism 移 platform)用户拍板暂缓**
- `5aacce2`:迁移 CRLF 校验和事故修复——4 个 SQL 转 LF + `.gitattributes` 钉 `*.sql text eol=lf`

### 2. 交互三项(用户报四个问题,搜索交互拍板不动)
- `06e1846`:provider.Version 加 datePublished,出口统一新→旧
- `e554985`:版本下拉兼容当前包的提前,默认选中最新兼容版
- `ae9fa11`:CF key 全链路——PUT/DELETE `/api/system/providers/curseforge/key`,保存前真实调用验证(无效 400/平台不通 502),secrets 表持久化,不回传,即存即效,env 优先;启动探测修"未探测"假象;矩阵 41→48

### 3. 双平台模组身份合并 + 版本镜像(`149120f`,用户批准方案)
- 搜索合并:身份表(本机学习+内置基线)优先,名称规范化(忽略大小写/空格/括号)兜底,**slug 否决**(AE2 反例);合并卡双平台标签、下载量求和
- 存储:pack_mods 一行+mirror 三列(迁移 0005);**添加即钉版,永不追新**(用户原话:追新版会让之前的调试失去意义,包大概率启动不了);查不到照常添加标"仅单平台";主版本换版镜像重钉
- 身份表双库:内置 `mod_identity_baseline.json` go:embed 只读随分发(种子 JEI/AE2 双平台实测核实);用户库 `mod_identity` 表只进不出;**打包红线** package.ps1 断言包内无 *.db/*.sqlite*
- 实测:添加 MR JEI 19.51.0.417 → 自动钉 CF 238222 fileId 8769490(jei-1.21.1-neoforge-19.51.0.417.jar,同版本号);矩阵 48→50

### 4. 兼容知识库(`3df1ce4`,用户拍板"自动但可见")
- `compat_knowledge_baseline.json` 内嵌只读,只放人工核实条目(source 必填),`knownIssues` 空数组等用户整理
- 添加模组自动扫描:有 install_mod 解法自动加装(origin=compat-fix,"兼容补丁"标签可移除,活动日志注明,深度封顶 2)
- 修不掉的 → resolve 时进冲突列表 kind=known_issue(fatal→error/其余 warning)
- `GET /api/packs/{id}/mod-recommendations` 按包 MC/loader 过滤,已装隐藏,MR 优先;前端推荐卡一键添加
- 种子实测核实:Polymorph(MR tagwiZkJ/CF 388800)、Sinytra Connector(MR u58R1TMW/CF 890127)
- `docs/compat-knowledge.md` 维护者整理指南;5 个单测;迁移 0006 origin 列;矩阵 50→53

### 5. 登记不实现
- `idea-client-telemetry-upload`:客户端采集兼容问题/配对确认上传服务端,用户明确"先知道有这么个事儿"

## 验收终态
`verify-contract.sh` 全绿:go test 9 包;curl **53/53**;E2E **22/22×2**(直连+代理);gofmt 基线制;前端 tsc 零错误。含真实网络的断言偶发假失败(平台超时),重跑即绿。

## 教训/坑(本会话新增)
- 迁移文件受 git autocrlf 影响会被 sha256 判篡改:.gitattributes 必须钉 SQL 为 LF
- 工具轮次被系统掐停时残留半成品(旧别名),恢复后必须先解释断点再收尾
- Windows Go http.Server 对超限 body 直接 RST,断言 413 要容忍连接重置
- UpdatePackMod 整行回写,新增字段必须进 SELECT 否则被静默清空
