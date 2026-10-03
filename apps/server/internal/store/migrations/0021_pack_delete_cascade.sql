-- 0021: 让「删除整合包」真正可达。
-- 前端到后端链路测试(docs/tests/chain-test-*)复现:DELETE /api/packs/{id} 必失败。
-- 两个独立成因:
--   1) minecraft_pack_mod_delete_guard 是无条件 BEFORE DELETE 守卫。删包时
--      packs→pack_mods 的 ON DELETE CASCADE 也会触发它,内置 minecraft 成员行
--      删不掉,整条语句以 SQLITE_CONSTRAINT_FOREIGNKEY(19) 回滚 → HTTP 500。
--      守卫的本意是"活着的包不许移除内置成员",不该拦住包自身的销毁,故加
--      "父包仍存在"条件。
--   2) catalog_tag_members / catalog_definition_decisions / catalog_text_resolutions /
--      catalog_icon_inputs / catalog_icons 对 catalog_entries、content_definitions 是
--      RESTRICT / NO ACTION,且自身没有到 packs 的级联路径:级联删完父行后它们留下
--      孤儿行,COMMIT 时的外键校验再报 (19)。这些行改由 Repository 在删包事务里
--      显式按 pack_id 清理(见 pack_repo.DeletePack)。
-- 另外为级联要扫的子列补索引,避免删包事务把单连接(SetMaxOpenConns=1)的库占住数秒。

DROP TRIGGER IF EXISTS minecraft_pack_mod_delete_guard;

CREATE TRIGGER minecraft_pack_mod_delete_guard
BEFORE DELETE ON pack_mods
WHEN OLD.mod_id='minecraft' AND EXISTS (
  SELECT 1 FROM packs p WHERE p.id=OLD.pack_id
)
BEGIN SELECT RAISE(ABORT, 'minecraft is a required builtin pack member'); END;

CREATE INDEX IF NOT EXISTS idx_catalog_tag_members_pack ON catalog_tag_members(pack_id);
CREATE INDEX IF NOT EXISTS idx_catalog_definition_decisions_pack ON catalog_definition_decisions(pack_id);
CREATE INDEX IF NOT EXISTS idx_catalog_text_resolutions_pack ON catalog_text_resolutions(pack_id);
CREATE INDEX IF NOT EXISTS idx_catalog_icon_inputs_pack ON catalog_icon_inputs(pack_id);
CREATE INDEX IF NOT EXISTS idx_catalog_icons_pack ON catalog_icons(pack_id);
CREATE INDEX IF NOT EXISTS idx_catalog_entries_pack ON catalog_entries(pack_id);
CREATE INDEX IF NOT EXISTS idx_content_definitions_pack ON content_definitions(pack_id);
CREATE INDEX IF NOT EXISTS idx_outbox_events_pack ON outbox_events(pack_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_pack ON audit_events(pack_id);
CREATE INDEX IF NOT EXISTS idx_tasks_pack ON tasks(pack_id);
