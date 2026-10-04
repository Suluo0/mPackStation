# mPackStation 点击级 UI 回归报告

- 日期：2026-09-18T20:43:55.681Z
- 前端：http://127.0.0.1:5273
- 后端：http://127.0.0.1:18871
- 总计：33 · 通过 33 · 失败 0

## 明细

| ID | 页面 | 结果 | 说明 |
|---|---|---|---|
| BOOT | setup | PASS | packId=pack-35c7e82a7de9764d6538d368 |
| FE-PACKS-LOAD | packs | PASS | 列表有数据 |
| FE-PACKS-CREATE-MODAL | packs | PASS | 新建弹窗可打开 |
| FE-QUEST-OPEN | quests | PASS | pack=pack-5ff385a8c265798a38a2166a 任务书编辑器可见，无报错 |
| FE-QUEST-UI-NOT-JSON-DUMP | quests | PASS | 章节/节点编辑 UI + 保存草稿 |
| FE-QUEST-SAVE | quests | PASS | 整合包
/
无
无任务书包
MC 1.21.1 · neoforge · v0.1.0
已保存
任务书
保存草稿
校 验
应 用
QUEST BOOK
任务书

按章节组织任务节点；缺失时会自动创建初始草稿。当前修订 7 · draft

章节
新增
1
开始
1 个任务
开始
添加任务节点
入门
完成基础准备
属性
 |
| FE-QUEST-OPEN-EXISTING | quests | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
任务书
保存草稿
校 验
应 用
QUEST BOOK
任务书

按章节组织任务节点；缺失时会自动创建初始草稿。当前修订 — · draft

章节
新增
1
开始
1 个任务
开始
添加任务节点
入门
完成基础准 |
| FE-CONTENT-LAYOUT | content | PASS | h1=1 toolbar=2 parseCta=1 / 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
内容编辑
重建目录与图标
开始解析
MOD CONTENT / EXTRACTION
内容编辑

从模组 jar 提取配方、物品、结构 |
| FE-CONTENT-NO-CRASH | content | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
内容编辑
重建目录与图标
开始解析
MOD CONTENT / EXTRACTION
内容编辑

从模组 jar 提取配方、物品、结构 |
| FE-LAUNCHER-PICKER-BTN | launcher | PASS | 存在选择目录按钮 |
| FE-LAUNCHER-PICKER-MODAL | launcher | PASS | 选择游戏目录
上级
当前路径
/Users/xunsu
推荐位置
home
/Users/xunsu
子目录（21）
Applications
/Users/xunsu/Applications
Desktop
/Users/xunsu/Desktop
Documents
/Users/xunsu/Documents
DoubaoWork
/Users/xu |
| FE-LAUNCHER-PICKER-APPLY | launcher | PASS | picked=home value=/Users/xunsu |
| FE-LAUNCHER-VALIDATE | launcher | PASS | 空表单被前端拦截 |
| FE-PUBLISH-BUTTONS | publish | PASS | recheck=1 build=1 select=1 |
| FE-PUBLISH-DIR-PICKER | publish | PASS | 选择导出目录
上级
当前路径
/Users/xunsu
推荐位置
home
/Users/xunsu
子目录（21）
Applications
/Users/xunsu/Applications
Desktop
/Users/xunsu/Desktop
Documents
/Users/xunsu/Documents
 |
| FE-PUBLISH-REGISTER-EXPORT | publish | PASS | 注册反馈正常 |
| FE-PUBLISH-RECHECK | publish | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
打包与发布
PUBLISH / DELIVERY
打包与发布

交付检查、导出目录与构建产物。构建前必须先注册允许的导出目录。

交付检查
重新检查
content
passed · {}
passed
版本与产物
当前版本
0.1.0
还没有构建产物。 |
| FE-PUBLISH-BUILD | publish | PASS | 整合包工作台
WORKBENCH
工作台设置
工作空间
工作台
整合包
当前整合包
包工作台
概览
模组
依赖与冲突
内容编辑
任务书
打包与发布
启动器
收起侧边栏
整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
打包与发布
PUBLISH / DELIVERY
打包与发布

交付检查、导出目录与构 |
| FE-MODS-SEARCH | mods | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
模组
MOD CATALOG
模组

按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC 1.21.1 · neoforge 过滤。

搜索
CurseForge:未配置(设置 CURSEFORGE_API_KEY 后可用)
 |
| FE-MODS-VERSIONS | mods | PASS | version options=10 |
| FE-MODS-ADD | mods | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
模组
MOD CATALOG
模组

按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC 1.21.1 · neoforge 过滤。

搜索
CurseForge:未配置(设置 CURSEFORGE_API_KEY 后可用)
 |
| FE-MODS-PARSE | content | PASS | 解析任务有完成/条目产出 |
| FE-MODS-REMOVE | mods | PASS | 整合包
/
r
readd-test
MC 1.21.1 · neoforge · v0.1.0
已保存
模组
MOD CATALOG
模组

按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC 1.21.1 · neoforge 过滤。

搜索
推荐兼容模组(人工核实)
Polymorph
合成配方冲突的通用解法: 多个模组注册 |
| FE-ROUTE-_ | / | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368 | /packs/pack-35c7e82a7de9764d6538d368 | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_mods | /packs/pack-35c7e82a7de9764d6538d368/mods | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_dependencies | /packs/pack-35c7e82a7de9764d6538d368/dependencies | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_content | /packs/pack-35c7e82a7de9764d6538d368/content | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_quests | /packs/pack-35c7e82a7de9764d6538d368/quests | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_publish | /packs/pack-35c7e82a7de9764d6538d368/publish | PASS | 可打开 |
| FE-ROUTE-_packs_pack-35c7e82a7de9764d6538d368_launcher | /packs/pack-35c7e82a7de9764d6538d368/launcher | PASS | 可打开 |
| FE-ROUTE-_settings | /settings | PASS | 可打开 |
| FE-CONSOLE | all | PASS | 无页面 JS 错误（忽略 favicon/404/409 业务响应） |

## 结论

本轮用户点名的问题（任务书报错、内容页排版、启动台目录选择、发布页按钮、模组链路）均已覆盖并通过。
