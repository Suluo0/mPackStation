-- mPackStation 初始 schema（v1）
-- 纪律：
--   1. 单一数据库 data/mpackstation.db，所有业务表按 pack_id 分域，不做每包一库。
--   2. jar 数据层索引（jar_index）按 sha1 跨包共享，多个包引用同一文件不重复解析。
--   3. 原始 JSON（CurseForge/Modrinth 响应、jar 内 mods.toml 等）不落库，
--      只存 data/ 下的文件路径（*_path 字段），库里只留结构化索引。

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- 整合包
CREATE TABLE IF NOT EXISTS packs (
    id              TEXT PRIMARY KEY,            -- ulid/uuid
    name            TEXT NOT NULL UNIQUE,
    mc_version      TEXT NOT NULL DEFAULT '',
    loader          TEXT NOT NULL DEFAULT '',    -- forge | neoforge | fabric | quilt
    loader_version  TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    icon_path       TEXT NOT NULL DEFAULT '',    -- data/icons/...
    created_at      INTEGER NOT NULL,            -- unix ms
    updated_at      INTEGER NOT NULL,
    last_edited_at  INTEGER NOT NULL
);

-- 包内模组清单（唯一权威的"已选择/已安装"来源；本地不再维护全局模组清单）
CREATE TABLE IF NOT EXISTS pack_mods (
    id                TEXT PRIMARY KEY,
    pack_id           TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    source            TEXT NOT NULL,             -- curseforge | modrinth | local
    project_id        TEXT NOT NULL DEFAULT '',  -- 平台项目 id；本地导入为空
    version_id        TEXT NOT NULL DEFAULT '',  -- 平台文件/version id；锁定版本
    display_name      TEXT NOT NULL,
    file_name         TEXT NOT NULL DEFAULT '',
    sha1              TEXT NOT NULL DEFAULT '',  -- 关联 jar_index.sha1；未下载为空
    status            TEXT NOT NULL DEFAULT 'pending', -- pending | installed | removed
    required          INTEGER NOT NULL DEFAULT 1,      -- 0=可选模组
    added_at          INTEGER NOT NULL,
    resolved_meta_path TEXT NOT NULL DEFAULT ''  -- 平台原始响应 JSON 的落盘路径
);
CREATE INDEX IF NOT EXISTS idx_pack_mods_pack ON pack_mods(pack_id, status);
CREATE INDEX IF NOT EXISTS idx_pack_mods_sha1 ON pack_mods(sha1);

-- jar 数据层索引：按文件 sha1 全局共享（跨包去重）
CREATE TABLE IF NOT EXISTS jar_index (
    sha1          TEXT PRIMARY KEY,
    file_path     TEXT NOT NULL,         -- data/jars/...
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    mod_ids       TEXT NOT NULL DEFAULT '[]',  -- JSON array，jar 内声明的 modid 列表
    loaders       TEXT NOT NULL DEFAULT '[]',  -- JSON array
    mc_versions   TEXT NOT NULL DEFAULT '[]',  -- JSON array
    raw_meta_path TEXT NOT NULL DEFAULT '',    -- mods.toml/fabric.mod.json 原文落盘路径
    parsed_at     INTEGER NOT NULL
);

-- 后台任务（打包 / 索引 / 同步等，看板任务面板数据源）
CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT PRIMARY KEY,
    pack_id       TEXT REFERENCES packs(id) ON DELETE SET NULL,
    kind          TEXT NOT NULL,           -- pack | index | sync | cache
    title         TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'queued', -- queued | running | success | failed | canceled
    progress      REAL NOT NULL DEFAULT 0, -- 0..100
    message       TEXT NOT NULL DEFAULT '',
    payload_path  TEXT NOT NULL DEFAULT '', -- 任务大字段/日志的落盘路径
    error         TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    started_at    INTEGER,
    finished_at   INTEGER
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status, created_at DESC);

-- 冲突 / 问题记录（看板信号：待解决 / 已解决）
CREATE TABLE IF NOT EXISTS conflicts (
    id           TEXT PRIMARY KEY,
    pack_id      TEXT NOT NULL REFERENCES packs(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,            -- dependency | version | loader | duplicate | crash
    severity     TEXT NOT NULL DEFAULT 'error', -- error | warning
    status       TEXT NOT NULL DEFAULT 'pending', -- pending | resolved | ignored
    summary      TEXT NOT NULL,
    detail_path  TEXT NOT NULL DEFAULT '', -- 详情 JSON 落盘路径
    created_at   INTEGER NOT NULL,
    resolved_at  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_conflicts_pack ON conflicts(pack_id, status);

-- 动态（看板"最近动态"数据源）
CREATE TABLE IF NOT EXISTS activities (
    id          TEXT PRIMARY KEY,
    pack_id     TEXT REFERENCES packs(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,             -- conflict | edit | pack | task
    text        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activities_time ON activities(created_at DESC);

-- 设置（CF API Key、存储目录等）
CREATE TABLE IF NOT EXISTS settings (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL DEFAULT ''
);

-- 远端缓存登记（CF/Modrinth 响应文件；内容在磁盘，这里只记路径与 TTL）
CREATE TABLE IF NOT EXISTS remote_cache (
    cache_key    TEXT PRIMARY KEY,         -- 如 mr:search:<hash>
    payload_path TEXT NOT NULL,
    fetched_at   INTEGER NOT NULL,
    ttl_seconds  INTEGER NOT NULL DEFAULT 3600
);
