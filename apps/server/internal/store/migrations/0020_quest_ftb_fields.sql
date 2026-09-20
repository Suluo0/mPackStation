-- 0020: FTB quest book experience — optional extension payload as JSON meta.
-- Existing draft rows keep meta='{}'; service layer maps extended fields through these blobs.
-- Never edit this migration after it has been applied.

ALTER TABLE quest_nodes ADD COLUMN meta TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(meta));
ALTER TABLE quest_chapters ADD COLUMN meta TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(meta));
ALTER TABLE quest_revisions ADD COLUMN meta TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(meta));
