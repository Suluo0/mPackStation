# 2026-09-07 物品图标修复与原版资源补齐

## 结果

修正旧方块投影的 Y 轴方向、可见面和 UV；由直接 parent 特判改为完整路径的父模型递归解析。item 与 block 模型不再混用身份。支持逐面纹理、GUI 变换、elements/旋转、深度遮挡、半砖/楼梯/墙、多层生成物品与常见动画条首帧。

新增图标 resolve 接口，从已有数据库源模型重新生成展示图标，绕开旧 item_icon 快照。按当前 MC 版本下载经校验的 Mojang client JAR，共享缓存。原版铁锭/红石粉/金锭/钻石/工作台在真实资源图像核对中可用。支持真实标签引用；1.21.1 的铁锭/红石粉/钻石通用标签使用官方核实的代表成员，界面保留 # 标签语义。

## 验证证据

- 前端真实 PNG 响应 fixture 通过实际 modContentIconsSchema 校验：`node scripts/contract/icon-fixture-check.cjs`。
- 项目状态补齐三个历史启动器任务缺失的 kind/updated_at 字段，保留原功能状态。
- 当前工作树：全量 go test ./... 与 go vet ./... 通过；npm run build 通过。既有大 chunk 警告保留。
- 回归：方块顶部朝上、正确可见面、UV 上下方向、半砖几何、父模型/纹理继承、循环引用、透明图层、动画首帧、原版离线缓存、跨包拒绝、HTTP token 校验、下载来源/校验和、标签循环与 replace。
- 修正历史 Provider 测试 fixture：旧测试把实际下载指向 cdn.example，现以本地测试服务返回 JAR 字节并检查不泄漏 API 凭据；保留原断言。
- 真实 MC 1.21.1 + 数据库中 AE2 19.2.17 源模型：1766 个图标，其中 AE2 349；29 个 AE2 item 模型未生成。统计仅枚举 models/item 路径，不等同于注册物品数量，不能与旧报告混合 item/block 口径直接计算覆盖率。
- 已用只读数据库验证工具生成 .tmp/icon-check.json 与 .tmp/icon-check.html，浏览器检查方块、楼梯、半砖、充能器、基础素材的图像。
- 正式本机 API 实测 200、warnings=[]；tagIcons 中铁锭和红石粉分别指向 minecraft:iron_ingot / minecraft:redstone。
- 浏览器实际打开 AE2 高级卡配方，确认钻石/铁锭/红石粉标签格和输出卡片图标显示。
- 未重新执行整套旧四层网络验收矩阵；本次以全量 Go、构建、专项回归及真实接口/UI 为验证边界。用户验收待定，未提交/推送。

代码证据指纹（下列源文件按路径与内容拼接 SHA-256）：`d411445bb155afc452d82d9218576165cd699c1a64524eed52b102567fd456f8`

- apps/server/internal/service/item_icon_render.go
- apps/server/internal/service/item_model_resources.go
- apps/server/internal/service/item_icon_tags.go
- apps/server/internal/service/mod_content_icons.go
- apps/server/internal/service/mod_content_extract.go
- apps/server/internal/provider/minecraft_assets.go
- apps/server/internal/httpapi/routes_mod_content.go
- apps/web/src/api/modContent.ts
- apps/web/src/pages/ModContentPage.tsx

## 剩余范围

自定义 loader、运行时染色/实体渲染未实现；复杂动画只取常见纵向条首帧，不保证任意 .mcmeta 排列。仅组合原版和当前模组资源，未融合其他模组和用户资源包；通用标签种子仅核实了 1.21.1 的三项，其他标签按实际数据解析。1.21.4+ 新 client item 选择规则另行支持。

仍无法生成的 AE2 item 模型（含辅助模型）：

- ae2:annihilation_plane
- ae2:cable_anchor
- ae2:cable_bus
- ae2:color_applicator
- ae2:color_applicator_colored
- ae2:conversion_monitor
- ae2:covered_cable_base
- ae2:covered_dense_cable_base
- ae2:crafting_terminal
- ae2:dark_monitor
- ae2:display_base
- ae2:facade
- ae2:formation_plane
- ae2:glass_cable_base
- ae2:matrix_frame
- ae2:memory_card
- ae2:meteorite_compass
- ae2:monitor
- ae2:paint
- ae2:part_base
- ae2:pattern_access_terminal
- ae2:pattern_encoding_terminal
- ae2:security_station
- ae2:semi_dark_monitor
- ae2:smart_cable_base
- ae2:smart_dense_cable_base
- ae2:storage_monitor
- ae2:terminal
- ae2:wrapped_generic_stack
