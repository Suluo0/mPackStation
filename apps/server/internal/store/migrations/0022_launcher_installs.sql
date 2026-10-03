-- 0022: 记录启动器内核实际装出来的版本目录 ID。
--
-- 装完之后要启动，用的不是包的 mc_version。带加载器时真实版本目录是
-- profile.id，缺省回退命名 fabric-loader-<加载器版本>-<MC 版本>
-- （launcherCore/src/loader/fabric.rs:43-45）。此前 service/launcher_task.go:80
-- 用 `_, err :=` 把 install 的返回结果整包丢掉，前端只能拿 mcVersion 去猜版本号，
-- 真实二进制必然 VersionNotFound —— 这就是终局链上除构建之外的第二处断点。
--
-- 键取 (minecraft_dir, version_id)：安装是「某个目录里的某个版本」，
-- 整合包只是发起上下文（可以没有），所以 pack_id 可空且随包删除置空，
-- 不给 0021 的删包级联清单再添一张表。

CREATE TABLE IF NOT EXISTS launcher_installs (
  id TEXT PRIMARY KEY,
  minecraft_dir TEXT NOT NULL,
  version_id TEXT NOT NULL,
  loader TEXT NOT NULL DEFAULT '',
  mc_version TEXT NOT NULL DEFAULT '',
  pack_id TEXT REFERENCES packs(id) ON DELETE SET NULL,
  task_id TEXT NOT NULL DEFAULT '',
  installed_at INTEGER NOT NULL,
  UNIQUE (minecraft_dir, version_id)
);

CREATE INDEX IF NOT EXISTS idx_launcher_installs_pack
  ON launcher_installs(pack_id, installed_at DESC);
