-- Offline-first onboarding: add launcherReady (user acknowledges after
-- configuring the self-built mPackLauncher game dir + offline username).
-- prismAccount is no longer required — premium Microsoft login is optional.

CREATE TABLE onboarding_state_v18 (
  step TEXT PRIMARY KEY CHECK (step IN ('curseforgeKey','firstPack','firstMod','launcherReady')),
  acknowledged INTEGER NOT NULL DEFAULT 0 CHECK (acknowledged IN (0,1)),
  acknowledged_at INTEGER,
  updated_at INTEGER NOT NULL,
  CHECK ((acknowledged = 1 AND acknowledged_at IS NOT NULL) OR (acknowledged = 0 AND acknowledged_at IS NULL))
);

INSERT INTO onboarding_state_v18(step, acknowledged, acknowledged_at, updated_at)
SELECT step, acknowledged, acknowledged_at, updated_at FROM onboarding_state;

INSERT OR IGNORE INTO onboarding_state_v18(step, acknowledged, acknowledged_at, updated_at)
VALUES ('launcherReady', 0, NULL, unixepoch('now') * 1000);

DROP TABLE onboarding_state;

ALTER TABLE onboarding_state_v18 RENAME TO onboarding_state;
