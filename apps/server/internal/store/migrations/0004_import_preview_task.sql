-- 0004: record which task consumed an import preview.
--
-- The import contract returns the original task when a consumed preview is
-- confirmed again with the same input (409 import_preview_consumed is only
-- used when the original task can no longer be located). Storing the task id
-- on the preview row makes that replay possible.

ALTER TABLE import_previews ADD COLUMN consumed_task_id TEXT NOT NULL DEFAULT '';
