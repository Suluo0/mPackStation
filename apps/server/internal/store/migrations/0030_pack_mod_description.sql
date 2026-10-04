-- 0030: pack_mods 增加用户自定义描述。
--
-- 平台元数据的描述（platform_projects.description，0029）是只读的"这是什么模组"；
-- 用户经常要补一句自己的备忘（"为什么把它加进来""这是给哪个玩法用的"），
-- 这句话属于每个包里的模组成员，不是平台事实，所以落在 pack_mods 上。
-- 展示口径：用户描述优先，为空时回退平台描述（service.enrichModDTOs）。
-- 纯展示字段：改它不触发目录失效（catalogRelevantModChange 不在列，0028 同口径）。

ALTER TABLE pack_mods ADD COLUMN description TEXT NOT NULL DEFAULT '';
