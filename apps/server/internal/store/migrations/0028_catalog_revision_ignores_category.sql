-- 0028: 分类（pack_mods.category）不该让物品目录作废。
--
-- 0013 里的 catalog_pack_mods_UPDATE 是 `AFTER UPDATE ON pack_mods`（不带列名），
-- 于是给模组改一个纯显示用的分类标签，也会把 source_revision +1 并把 build_status
-- 打回 pending —— 用户只是把 JEI 从「优化」搬到「科技」，整个索引页立刻变成
-- 409 catalog_stale「包内容已变化，物品目录需要重建」，而重建要重新解析
-- 13000+ 个文件。category 跟「目录里有哪些物品」毫无关系：重建读的是
-- status / sha1 / current_selection_id 和解析批次（见 service.RebuildItemCatalog
-- 开头的过滤：mod.Status != "installed" 跳过、run.SHA1 != mod.SHA1 跳过）。
--
-- 收窄成只在这几列上开火。另外 SQLite 的 `UPDATE OF a,b` 是「语句提到这几列就开火」，
-- 哪怕值没变也开火，所以再加 WHEN 守卫做最后一道闸 —— 否则
-- `UPDATE pack_mods SET status=status` 这种空写也会把目录打成 pending。

DROP TRIGGER IF EXISTS catalog_pack_mods_UPDATE;

CREATE TRIGGER catalog_pack_mods_UPDATE
AFTER UPDATE OF status, sha1, current_selection_id, version_id, mirror_version_id,
                mirror_source, mirror_project_id, file_name, project_id, mod_id, origin
ON pack_mods
WHEN old.status IS NOT new.status
  OR old.sha1 IS NOT new.sha1
  OR old.current_selection_id IS NOT new.current_selection_id
  OR old.version_id IS NOT new.version_id
  OR old.mirror_version_id IS NOT new.mirror_version_id
  OR old.mirror_source IS NOT new.mirror_source
  OR old.mirror_project_id IS NOT new.mirror_project_id
  OR old.file_name IS NOT new.file_name
  OR old.project_id IS NOT new.project_id
  OR old.mod_id IS NOT new.mod_id
  OR old.origin IS NOT new.origin
BEGIN
  UPDATE pack_catalog_state
     SET source_revision = source_revision + 1, build_status = 'pending', last_error = ''
   WHERE pack_id = new.pack_id;
END;
