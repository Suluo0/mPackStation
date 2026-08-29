# 打包与发布 `/packs/:id/publish`

## 页面目的

让用户知道整合包是否可交付，并完成本地打包、导出或发布到 CurseForge / Modrinth。

## 布局

- 顶部包摘要和当前包版本。
- 左侧发布流程：检查、版本、说明、平台、打包。
- 主区“交付检查”清单：依赖、冲突、缺失文件、内容校验、版本号。
- 右侧交付卡：本地导出、CurseForge、Modrinth 三个目标，显示准备状态。
- 底部最近产物列表：文件名、大小、时间、状态、打开目录。

## 核心组件

`ReleaseStepper`、`DeliveryCheckList`、`TargetCard`、`BuildProgressCard`、`ArtifactRow`。

## 状态

可打包、存在阻塞项、打包中、打包成功、发布失败、发布成功。

## 视觉重点

这是结果页，留白比工作台多；成功状态用绿色但只出现在真实校验和任务结果上，主操作仍用 ember 橙。
