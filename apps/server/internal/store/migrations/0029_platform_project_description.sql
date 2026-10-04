-- 0029: platform_projects 增加一句话描述。
--
-- 模组树展开的第一行要显示「这个模组是干嘛的」的一句话描述，数据源是平台
-- 元数据的短描述（Modrinth 的 description / CurseForge 的 summary，对应
-- provider.Project.Summary）。历史行留空串：服务层在读取模组清单时对缺失的
-- 行按需补拉一次并回写（service.enrichModDTOs），网络失败静默降级，
-- 不阻塞清单返回。

ALTER TABLE platform_projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
