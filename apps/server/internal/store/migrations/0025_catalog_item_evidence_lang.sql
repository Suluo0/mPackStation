-- 0025: allow catalog item evidence 'lang'.
--
-- 目录构建器修复（第九轮）：物品的权威来源从「模型文件」改为「语言文件的键」
-- （item.minecraft.apple / block.minecraft.stone）。模型文件此前把原版资产里的
-- 动画帧模型（clock_01..64 / compass_01..64 / bow_pulling_*）也当成了物品，
-- 产生 421 条不在注册表、任何语言都没有名字的残渣条目；修复后模型只作为
-- 图标来源（model_path），不再产生物品条目，新物品的证据值是 'lang'。
-- 0013 建表时 CHECK 只列了 model/reference，'lang' 会撞约束。
--
-- 迁移在事务里执行、不能关 foreign_keys，父表重建必须连三张子表一起换
-- （0023 重建的 conflicts 没有子表，这个先例在这里不适用）。
-- 顺序：建 v25 四表 → 拷数据 → 先 drop 子表再 drop 父表（避免级联清空）→ 重命名。

CREATE TABLE pack_catalog_items_v25 (
 pack_id TEXT NOT NULL REFERENCES pack_catalog_state(pack_id) ON DELETE CASCADE,
 item_id TEXT NOT NULL CHECK(instr(item_id,':')>1),
 evidence TEXT NOT NULL CHECK(evidence IN ('lang','model','reference')),
 source TEXT NOT NULL,
 model_path TEXT NOT NULL DEFAULT '',
 icon_status TEXT NOT NULL DEFAULT 'pending' CHECK(icon_status IN ('pending','ready','missing','dynamic','failed')),
 PRIMARY KEY(pack_id,item_id)
);
INSERT INTO pack_catalog_items_v25 (pack_id,item_id,evidence,source,model_path,icon_status)
SELECT pack_id,item_id,evidence,source,model_path,icon_status FROM pack_catalog_items;

CREATE TABLE pack_catalog_item_blocks_v25 (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL, block_id TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id,block_id),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items_v25(pack_id,item_id) ON DELETE CASCADE,
 FOREIGN KEY(pack_id,block_id) REFERENCES pack_catalog_blocks(pack_id,block_id) ON DELETE CASCADE
);
INSERT INTO pack_catalog_item_blocks_v25 (pack_id,item_id,block_id)
SELECT pack_id,item_id,block_id FROM pack_catalog_item_blocks;

CREATE TABLE pack_catalog_item_names_v25 (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL, locale TEXT NOT NULL CHECK(length(locale)>0),
 name TEXT NOT NULL, translation_key TEXT NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id,locale),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items_v25(pack_id,item_id) ON DELETE CASCADE
);
INSERT INTO pack_catalog_item_names_v25 (pack_id,item_id,locale,name,translation_key,source)
SELECT pack_id,item_id,locale,name,translation_key,source FROM pack_catalog_item_names;

CREATE TABLE pack_catalog_item_icons_v25 (
 pack_id TEXT NOT NULL, item_id TEXT NOT NULL,
 mime TEXT NOT NULL CHECK(mime='image/png'), data BLOB NOT NULL CHECK(length(data)>0 AND length(data)<=1048576),
 width INTEGER NOT NULL CHECK(width>0 AND width<=256), height INTEGER NOT NULL CHECK(height>0 AND height<=256),
 source TEXT NOT NULL,
 PRIMARY KEY(pack_id,item_id),
 FOREIGN KEY(pack_id,item_id) REFERENCES pack_catalog_items_v25(pack_id,item_id) ON DELETE CASCADE
);
INSERT INTO pack_catalog_item_icons_v25 (pack_id,item_id,mime,data,width,height,source)
SELECT pack_id,item_id,mime,data,width,height,source FROM pack_catalog_item_icons;

DROP TABLE pack_catalog_item_blocks;
DROP TABLE pack_catalog_item_names;
DROP TABLE pack_catalog_item_icons;
DROP TABLE pack_catalog_items;
ALTER TABLE pack_catalog_items_v25 RENAME TO pack_catalog_items;
ALTER TABLE pack_catalog_item_blocks_v25 RENAME TO pack_catalog_item_blocks;
ALTER TABLE pack_catalog_item_names_v25 RENAME TO pack_catalog_item_names;
ALTER TABLE pack_catalog_item_icons_v25 RENAME TO pack_catalog_item_icons;
CREATE INDEX IF NOT EXISTS catalog_name_search ON pack_catalog_item_names(pack_id,locale,name);
