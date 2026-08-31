-- 0005: 双平台镜像源 + 模组身份对应表
-- pack_mods 一行仍是一个"逻辑模组": source/project_id/version_id 是用户添加时的
-- 主源; mirror_* 是另一平台上钉死的对应文件(添加时立即解析, 之后不追新),
-- 供多平台格式导出(mrpack 用主源/MR 链接, CF 格式用 mirror 的 CF fileId)。
ALTER TABLE pack_mods ADD COLUMN mirror_source TEXT NOT NULL DEFAULT '';
ALTER TABLE pack_mods ADD COLUMN mirror_project_id TEXT;
ALTER TABLE pack_mods ADD COLUMN mirror_version_id TEXT;

-- 模组身份对应(用户库内为"本机已确认"的映射; 内置知识库是同结构的只读基线,
-- 运行时两层叠加, 新确认只写这里)。两个平台的项目 ID 各自唯一。
CREATE TABLE IF NOT EXISTS mod_identity (
  mr_project_id  TEXT NOT NULL UNIQUE,
  cf_project_id  TEXT NOT NULL UNIQUE,
  display_name   TEXT NOT NULL DEFAULT '',
  confirmed_at   INTEGER NOT NULL,
  PRIMARY KEY (mr_project_id, cf_project_id)
);
