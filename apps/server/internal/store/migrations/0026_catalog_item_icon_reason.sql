-- 0026: record WHY an item has no icon (R0 of the icon renderer plan 2026-10-03).
--
-- 图标渲染器现在能说出「为什么没有」：unsupported_model / missing_texture /
-- requires_tint / loader:xxx / runtime_generated_model（builtin/entity 一类）。
-- 之前原因算完就丢，界面只能给一个没有解释的占位格。
ALTER TABLE pack_catalog_items ADD COLUMN icon_reason TEXT NOT NULL DEFAULT '';
