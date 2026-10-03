-- 0023: allow conflict kind 'known_issue'.
--
-- 兼容知识库命中写出的冲突 kind='known_issue'（service/mods.go 的 knownIssues 分支），
-- 但 0002 建表时 CHECK 只列了 dependency/version/loader/duplicate/crash：知识库一旦
-- 有条目命中，CreateLock 就在 INSERT 上撞约束，整个 resolve 事务回滚、接口 500。
-- 现在库里 knownIssues 还是空数组，所以这条路没被踩到——属于埋着的雷。
--
-- SQLite 不能原地改 CHECK，照 0007 的做法重建表并补回索引。

CREATE TABLE conflicts_v23 (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  fingerprint TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL CHECK (kind IN ('dependency','version','loader','duplicate','crash','known_issue')),
  severity TEXT NOT NULL DEFAULT 'error' CHECK (severity IN ('error','warning')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resolved','ignored')),
  summary TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  detail_path TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  resolved_at INTEGER
);

INSERT INTO conflicts_v23
  (id, pack_id, fingerprint, kind, severity, status, summary, detail, detail_path,
   created_at, updated_at, resolved_at)
SELECT id, pack_id, fingerprint, kind, severity, status, summary, detail, detail_path,
       created_at, updated_at, resolved_at
FROM conflicts;

DROP TABLE conflicts;
ALTER TABLE conflicts_v23 RENAME TO conflicts;

CREATE INDEX IF NOT EXISTS idx_conflicts_pack ON conflicts(pack_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_conflicts_fingerprint
  ON conflicts(pack_id, fingerprint) WHERE fingerprint <> '';
