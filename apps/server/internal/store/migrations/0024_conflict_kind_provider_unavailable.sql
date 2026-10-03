-- 0024: allow conflict kind 'provider_unavailable'.
--
-- service/mods.go 在 resolve 时给「平台适配器不可用 / 模组元数据拉不下来」写冲突记录，
-- kind 传的是 provider_unavailable，但 0002/0023 的 CHECK 里没有这个值。此前它靠
-- conflict() 里的白名单把未知 kind 悄悄改写成 'dependency' 才落库 —— 于是「Modrinth
-- 此刻打不开」在库里长得像「这个包有依赖问题」：error 级、pending，被构建闸门（O20）
-- 当成致命缺陷一直拦着构建，用户既看不懂也修不了，只能反复 resolve 碰运气。
--
-- 现在把 kind 补进枚举，调用方保留真实 kind，严重级由服务端按语义给（网络态 = warning）。
-- SQLite 不能原地改 CHECK，照 0007/0023 的做法重建表并补回索引。

CREATE TABLE conflicts_v24 (
  id TEXT PRIMARY KEY,
  pack_id TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
  fingerprint TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL CHECK (kind IN ('dependency','version','loader','duplicate','crash','known_issue','provider_unavailable')),
  severity TEXT NOT NULL DEFAULT 'error' CHECK (severity IN ('error','warning')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resolved','ignored')),
  summary TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
  detail_path TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  resolved_at INTEGER
);

INSERT INTO conflicts_v24
  (id, pack_id, fingerprint, kind, severity, status, summary, detail, detail_path,
   created_at, updated_at, resolved_at)
SELECT id, pack_id, fingerprint,
       CASE WHEN kind = 'dependency' AND summary IN ('Provider unavailable', 'Metadata unavailable')
            THEN 'provider_unavailable' ELSE kind END,
       CASE WHEN kind = 'dependency' AND summary IN ('Provider unavailable', 'Metadata unavailable')
            THEN 'warning' ELSE severity END,
       status, summary, detail, detail_path,
       created_at, updated_at, resolved_at
FROM conflicts;

DROP TABLE conflicts;
ALTER TABLE conflicts_v24 RENAME TO conflicts;

CREATE INDEX IF NOT EXISTS idx_conflicts_pack ON conflicts(pack_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_conflicts_fingerprint
  ON conflicts(pack_id, fingerprint) WHERE fingerprint <> '';
