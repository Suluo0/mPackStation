-- 0006: 模组来源标记。manual=用户手动添加; compat-fix=兼容知识库命中后
-- 自动加装的兼容补丁(只提供兼容不提供内容, 列表里标"兼容补丁", 可移除)。
ALTER TABLE pack_mods ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual';
