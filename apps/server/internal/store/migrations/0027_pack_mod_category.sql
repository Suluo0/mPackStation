-- 0027: user-defined mod categories (来源树分组，2026-10-03 用户需求)。
-- 玩家可以按自己的想法给模组分类（"优化"、"科技"…），来源树按类目分组展示。
-- 空串 = 未分类。分类是包数据，进库而不是前端本地状态。
ALTER TABLE pack_mods ADD COLUMN category TEXT NOT NULL DEFAULT '';
