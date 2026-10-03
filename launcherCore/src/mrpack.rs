//! Modrinth `.mrpack`（v1）导入
//!
//! `.mrpack` 不是 jar 的压缩包，而是「manifest + overrides」：
//! `modrinth.index.json` 里 `files[]` 给路径/哈希/下载地址，模组字节由安装方按下。
//! 本模块把这条链补上，让流水线的最后一环（构建产物 -> 启动）能闭合。
//!
//! 字段名以真实产物为准：2026-09-30 实测现役整合包
//! （Fabulously Optimized v15.0.0-alpha.4.mrpack）顶层是
//! `formatVersion / game / versionId / name / files / dependencies`，
//! 旧稿里的 `manifestVersion` / `version` 仍按别名接受，避免历史产物读不动。

use std::collections::BTreeMap;
use std::io::Read;
use std::path::{Component, Path, PathBuf};

use serde::Deserialize;
use zip::ZipArchive;

use crate::download::concurrent::{ConcurrentDownloader, DownloadGroup};
use crate::download::item::DownloadItem;
use crate::download::Mirror;
use crate::error::LauncherError;
use crate::java::JavaRegistry;
use crate::loader::{LoaderInstaller, LoaderType};
use crate::protocol::{self, Protocol};
use crate::Result;

pub const MANIFEST_NAME: &str = "modrinth.index.json";
pub const OVERRIDES_DIR: &str = "overrides";
pub const CLIENT_OVERRIDES_DIR: &str = "client-overrides";

/// 只解析，不触网：manifest 结构。
#[derive(Debug, Clone, Deserialize)]
pub struct MrpackManifest {
    #[serde(alias = "formatVersion", alias = "manifestVersion")]
    pub format_version: u32,
    #[serde(default)]
    pub game: Option<String>,
    #[serde(alias = "versionId", alias = "version")]
    pub version_id: String,
    pub name: String,
    #[serde(default)]
    pub files: Vec<MrpackFile>,
    #[serde(default)]
    pub dependencies: BTreeMap<String, String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct MrpackFile {
    pub path: String,
    #[serde(default)]
    pub hashes: MrpackHashes,
    /// 规范里这个键就叫 `env`（实测真实产物同为 `env`）。
    #[serde(default)]
    pub env: Option<MrpackEnv>,
    #[serde(default)]
    pub downloads: Vec<String>,
    #[serde(rename = "fileSize", default)]
    pub file_size: u64,
    #[serde(default)]
    pub optional: bool,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct MrpackHashes {
    #[serde(default)]
    pub sha1: String,
    #[serde(default)]
    pub sha512: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct MrpackEnv {
    #[serde(default)]
    pub client: Option<String>,
    #[serde(default)]
    pub server: Option<String>,
}

/// 解析出的导入计划：装哪个加载器、下哪些模组、覆盖哪些文件。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ImportPlan {
    pub mc_version: String,
    /// None 表示纯原版
    pub loader: Option<LoaderType>,
    pub loader_version: Option<String>,
    /// 相对实例根目录的模组路径（manifest 的 `mods/xxx.jar`）
    pub mods: Vec<ModDownload>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ModDownload {
    pub relative_path: String,
    pub url: String,
    pub sha1: String,
    pub size: u64,
}

/// 从 manifest 推出导入计划。
///
/// 客户端不需要的模组（`env.client = "unsupported"`，或 `optional = true`）
/// 不进下载计划——这是「不询问用户」的确定性口径，缺省（没有 env 字段）按必装处理。
pub fn plan_from_manifest(manifest: &MrpackManifest) -> Result<ImportPlan> {
    if manifest.format_version != 1 {
        return Err(LauncherError::InvalidArgument(format!(
            "不支持的 mrpack 格式版本 {}（当前只支持 v1）",
            manifest.format_version
        )));
    }
    if let Some(game) = manifest.game.as_deref() {
        if game != "minecraft" {
            return Err(LauncherError::InvalidArgument(format!(
                "不支持的游戏: {game}"
            )));
        }
    }
    let mc_version = manifest
        .dependencies
        .get("minecraft")
        .cloned()
        .ok_or_else(|| {
            LauncherError::InvalidArgument("manifest 缺 dependencies.minecraft".to_string())
        })?;
    let (loader, loader_version) = loader_from_dependencies(&manifest.dependencies)?;
    let mut mods = Vec::new();
    for file in &manifest.files {
        if file.optional {
            continue;
        }
        if let Some(env) = &file.env {
            if env.client.as_deref() == Some("unsupported") {
                continue;
            }
        }
        let url = file.downloads.first().ok_or_else(|| {
            LauncherError::InvalidArgument(format!("模组 {} 没有下载地址", file.path))
        })?;
        // 路径必须先过穿越检查，再落到实例目录里
        let relative = safe_join_relative(Path::new(""), &file.path)?
            .to_string_lossy()
            .replace('\\', "/");
        mods.push(ModDownload {
            relative_path: relative,
            url: url.clone(),
            sha1: file.hashes.sha1.clone(),
            size: file.file_size,
        });
    }
    Ok(ImportPlan {
        mc_version,
        loader,
        loader_version,
        mods,
    })
}

/// 加载器依赖键 -> LoaderType + 版本。识别不了的键必须报错，
/// 不能悄悄按原版装——那会装出一个「构建成功但模组全不加载」的实例。
fn loader_from_dependencies(
    deps: &BTreeMap<String, String>,
) -> Result<(Option<LoaderType>, Option<String>)> {
    let mut found: Option<(LoaderType, String)> = None;
    for (key, value) in deps {
        let kind = match key.as_str() {
            "minecraft" => continue,
            "fabric-loader" => LoaderType::Fabric,
            "quilt-loader" => LoaderType::Quilt,
            "forge" => LoaderType::Forge,
            "neoforge" => LoaderType::NeoForge,
            other => {
                return Err(LauncherError::InvalidArgument(format!(
                    "不认识的加载器依赖 {other}（版本 {value}）"
                )))
            }
        };
        if found.is_some() {
            return Err(LauncherError::InvalidArgument(
                "manifest 同时声明了多个加载器".to_string(),
            ));
        }
        found = Some((kind, value.clone()));
    }
    Ok(match found {
        Some((kind, version)) => (Some(kind), Some(version)),
        None => (None, None),
    })
}

/// 拒绝绝对路径与 `..` 穿越，返回规范化相对路径。
pub fn safe_join_relative(root: &Path, raw: &str) -> Result<PathBuf> {
    if raw.is_empty() {
        return Err(LauncherError::UnsafePath(raw.to_string()));
    }
    let candidate = Path::new(raw);
    if candidate.is_absolute() {
        return Err(LauncherError::UnsafePath(raw.to_string()));
    }
    // Windows 形状的路径在 Unix 上不会 is_absolute()，反斜杠会被当成一个普通
    // 段名（`C:\\Windows\\x` 是一段，不是穿越），所以必须按字面显式拦掉。
    let first = raw.chars().next().unwrap_or(' ');
    if (first.is_ascii_alphabetic() && raw.get(1..2) == Some(":")) || raw.starts_with("\\\\") {
        return Err(LauncherError::UnsafePath(raw.to_string()));
    }
    // 逐段归一化：`..` 允许，但不允许越过实例根（depth 计数而不是靠 out.pop()，
    // 否则 root 本身的段会被 pop 掉而无人察觉）。
    let mut out = root.to_path_buf();
    let mut depth = 0usize;
    for part in candidate.components() {
        match part {
            Component::Normal(seg) => {
                out.push(seg);
                depth += 1;
            }
            Component::CurDir => {}
            Component::ParentDir => {
                if depth == 0 {
                    return Err(LauncherError::UnsafePath(raw.to_string()));
                }
                out.pop();
                depth -= 1;
            }
            // 绝对路径的根段、Windows 盘符前缀都出现在非首位 -> 拒绝
            _ => return Err(LauncherError::UnsafePath(raw.to_string())),
        }
    }
    if depth == 0 {
        // 归一后正好落在实例根（如 `a/..`）：它不是文件，写它会覆盖整个实例目录
        return Err(LauncherError::UnsafePath(raw.to_string()));
    }
    Ok(out)
}

/// 从 .mrpack 读出 manifest。
pub fn read_manifest(path: &Path) -> Result<MrpackManifest> {
    let file = std::fs::File::open(path)
        .map_err(|e| LauncherError::Internal(format!("打开 .mrpack 失败: {e}")))?;
    let mut zip = ZipArchive::new(file)
        .map_err(|e| LauncherError::Internal(format!("打开 .mrpack zip 失败: {e}")))?;
    let mut reader = zip
        .by_name(MANIFEST_NAME)
        .map_err(|_| LauncherError::InvalidArgument(format!(
            "{MANIFEST_NAME} 不在包内（不是有效的 .mrpack）"
        )))?;
    let mut buf = Vec::new();
    reader
        .read_to_end(&mut buf)
        .map_err(|e| LauncherError::Internal(format!("读取 manifest 失败: {e}")))?;
    serde_json::from_slice::<MrpackManifest>(&buf)
        .map_err(|e| LauncherError::InvalidArgument(format!("manifest 解析失败: {e}")))
}

/// 解压 overrides/ 与 client-overrides/ 到实例根目录。
///
/// 顺序固定：先 `overrides/` 后 `client-overrides/`，同路径文件后者胜出。
/// 不能按 zip 条目顺序处理——那会让落地结果随打包时的遍历顺序变化，
/// 同一个包两次安装可能得到不同内容。每个条目都过穿越检查，无关文件按原样落地。
pub fn extract_overrides(zip_path: &Path, instance_dir: &Path) -> Result<usize> {
    let file = std::fs::File::open(zip_path)
        .map_err(|e| LauncherError::Internal(format!("打开 .mrpack 失败: {e}")))?;
    let mut zip = ZipArchive::new(file)
        .map_err(|e| LauncherError::Internal(format!("打开 .mrpack zip 失败: {e}")))?;
    let written = write_override_group(&mut zip, instance_dir, OVERRIDES_DIR)?
        + write_override_group(&mut zip, instance_dir, CLIENT_OVERRIDES_DIR)?;
    Ok(written)
}

/// 把 `<group>/` 前缀下的条目落到实例目录，返回写入的文件数。
fn write_override_group<R: Read + std::io::Seek>(
    zip: &mut ZipArchive<R>,
    instance_dir: &Path,
    group: &str,
) -> Result<usize> {
    let prefix = format!("{group}/");
    let mut written = 0usize;
    for idx in 0..zip.len() {
        let mut entry = zip
            .by_index(idx)
            .map_err(|e| LauncherError::Internal(format!("读取 zip 条目失败: {e}")))?;
        let name = entry.name().to_string();
        let Some(raw) = name.strip_prefix(prefix.as_str()) else {
            continue;
        };
        let relative = raw.replace('\\', "/");
        if relative.is_empty() {
            continue;
        }
        let dest = safe_join_relative(instance_dir, &relative)?;
        if entry.is_dir() {
            std::fs::create_dir_all(&dest)
                .map_err(|e| LauncherError::Internal(format!("创建目录失败: {e}")))?;
            continue;
        }
        if let Some(parent) = dest.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| LauncherError::Internal(format!("创建目录失败: {e}")))?;
        }
        let mut buf = Vec::new();
        entry
            .read_to_end(&mut buf)
            .map_err(|e| LauncherError::Internal(format!("解压失败: {e}")))?;
        std::fs::write(&dest, &buf)
            .map_err(|e| LauncherError::Internal(format!("写入覆盖文件失败: {e}")))?;
        written += 1;
    }
    Ok(written)
}


/// 导入结果（由调用方按 JSON Lines 协议输出，模块自身不决定协议形状）
#[derive(Debug, Clone)]
pub struct ImportReport {
    pub version_id: String,
    pub pack_name: String,
    pub pack_version: String,
    pub mods: usize,
    pub overrides: usize,
}

/// 从 `.mrpack` 安装一个可启动实例。
///
/// 流程：解析 manifest -> 装加载器（含原版文件）-> 解 overrides -> 按 `files[]` 下模组。
/// 模组下载复用现有下载层（sha1 校验 + 已存在即跳过 + 并发）。
pub async fn install_from_mrpack(
    minecraft_dir: &Path,
    zip_path: &Path,
    mirror: Mirror,
    java_registry: &JavaRegistry,
) -> Result<ImportReport> {
    Protocol::phase(protocol::phase::RESOLVING_VERSION, "正在解析 .mrpack");
    let zip = zip_path.to_path_buf();
    let manifest = tokio::task::spawn_blocking(move || read_manifest(&zip))
        .await
        .map_err(|e| LauncherError::Internal(format!("解析任务失败: {e}")))??;
    let plan = plan_from_manifest(&manifest)?;
    tracing::info!(
        "导入 {} {}：mc {}，loader {:?} {:?}，模组 {} 个",
        manifest.name,
        manifest.version_id,
        plan.mc_version,
        plan.loader,
        plan.loader_version,
        plan.mods.len()
    );

    // 1. 加载器（或原版）
    Protocol::phase(protocol::phase::INSTALLING_LOADER, "正在安装加载器");
    let version_id = match plan.loader {
        Some(kind) => {
            LoaderInstaller::new(minecraft_dir, mirror)
                .install(
                    &plan.mc_version,
                    kind,
                    plan.loader_version.as_deref(),
                    java_registry,
                )
                .await?
        }
        // 原版包：install_vanilla 只发 phase，result 由 main.rs 统一打一条
        //（协议规定整场只有一条 result）。
        None => crate::install::install_vanilla(minecraft_dir, &plan.mc_version, mirror).await?,
    };

    // 2. overrides（版本隔离实例根目录）
    let instance_dir = minecraft_dir.join("versions").join(&version_id);
    std::fs::create_dir_all(&instance_dir)
        .map_err(|e| LauncherError::Internal(format!("创建实例目录失败: {e}")))?;
    Protocol::phase(protocol::phase::PREPARING, "正在写入 overrides");
    let zip_for_overrides = zip_path.to_path_buf();
    let target = instance_dir.clone();
    let override_count = tokio::task::spawn_blocking(move || {
        extract_overrides(&zip_for_overrides, &target)
    })
    .await
    .map_err(|e| LauncherError::Internal(format!("overrides 任务失败: {e}")))??;

    // 3. files[]
    if !plan.mods.is_empty() {
        Protocol::phase(protocol::phase::DOWNLOADING_LIBRARIES, "正在下载模组");
        let items: Vec<(DownloadItem, DownloadGroup)> = plan
            .mods
            .iter()
            .map(|m| {
                let dest = instance_dir.join(&m.relative_path);
                let mut item = DownloadItem::new(
                    m.url.clone(),
                    dest,
                    format!("mod {}", m.relative_path),
                );
                if !m.sha1.is_empty() {
                    item = item.with_sha1(m.sha1.clone());
                }
                if m.size > 0 {
                    item = item.with_size(m.size);
                }
                (item, DownloadGroup::Other)
            })
            .collect();
        ConcurrentDownloader::new(mirror).download_all(items).await?;
    }

    Ok(ImportReport {
        version_id,
        pack_name: manifest.name,
        pack_version: manifest.version_id,
        mods: plan.mods.len(),
        overrides: override_count,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 与 apps/server/internal/service/build_mrpack.go 的产物同形（字段名逐一对应）
    const OUR_MANIFEST: &str = r#"{
        "formatVersion": 1,
        "game": "minecraft",
        "versionId": "0.9.1",
        "name": "Chain ASM",
        "dependencies": {"fabric-loader": "0.16.14", "minecraft": "1.21.1"},
        "files": [
            {
                "path": "mods/jei-1.21.1-fabric-19.57.0.450.jar",
                "hashes": {"sha1": "853deece496debf2c04941db954fc881ee786d10",
                           "sha512": "7a5be8bb90df93861d74fd8e3092ccd36e82c166ad2686e85dbcdcfd5f2d423"},
                "env": {"client": "required", "server": "required"},
                "downloads": ["https://cdn.modrinth.com/data/u6dRKJwZ/versions/Dd2GGBzd/jei.jar"],
                "fileSize": 2293143
            }
        ]
    }"#;

    fn parse(raw: &str) -> MrpackManifest {
        serde_json::from_str(raw).expect("manifest 应能解析")
    }

    #[test]
    fn plan_reads_our_own_build_output() {
        let plan = plan_from_manifest(&parse(OUR_MANIFEST)).unwrap();
        assert_eq!(plan.mc_version, "1.21.1");
        assert_eq!(plan.loader, Some(LoaderType::Fabric));
        assert_eq!(plan.loader_version.as_deref(), Some("0.16.14"));
        assert_eq!(plan.mods.len(), 1);
        assert_eq!(plan.mods[0].relative_path, "mods/jei-1.21.1-fabric-19.57.0.450.jar");
        assert_eq!(plan.mods[0].sha1, "853deece496debf2c04941db954fc881ee786d10");
        assert_eq!(plan.mods[0].size, 2293143);
    }

    #[test]
    fn plan_accepts_legacy_field_names() {
        // 历史产物用 manifestVersion + version，必须仍然读得动
        let legacy = OUR_MANIFEST
            .replace("\"formatVersion\"", "\"manifestVersion\"")
            .replace("\"versionId\"", "\"version\"");
        let plan = plan_from_manifest(&parse(&legacy)).unwrap();
        assert_eq!(plan.mc_version, "1.21.1");
        assert_eq!(plan.mods.len(), 1);
    }

    #[test]
    fn plan_skips_unsupported_and_optional() {
        let raw = r#"{
            "formatVersion": 1, "versionId": "1", "name": "p",
            "dependencies": {"minecraft": "1.21.1"},
            "files": [
                {"path": "mods/a.jar", "downloads": ["https://x/a.jar"], "fileSize": 1,
                 "hashes": {"sha1": "aa"}, "env": {"client": "required", "server": "required"}},
                {"path": "mods/b.jar", "downloads": ["https://x/b.jar"], "fileSize": 1,
                 "hashes": {"sha1": "bb"}, "env": {"client": "unsupported", "server": "required"}},
                {"path": "mods/c.jar", "downloads": ["https://x/c.jar"], "fileSize": 1,
                 "hashes": {"sha1": "cc"}, "optional": true}
            ]
        }"#;
        let plan = plan_from_manifest(&parse(raw)).unwrap();
        // 只有 a 进下载计划；b 客户端不需要，c 是可选项（不询问用户的确定性口径）
        assert_eq!(plan.mods.iter().map(|m| m.relative_path.as_str()).collect::<Vec<_>>(),
                   vec!["mods/a.jar"]);
        assert_eq!(plan.loader, None);
    }

    #[test]
    fn unknown_loader_is_an_error_not_silent_vanilla() {
        // 按原版装会产出「装成功但模组全不加载」的实例，必须显式失败
        let raw = r#"{
            "formatVersion": 1, "versionId": "1", "name": "p",
            "dependencies": {"minecraft": "1.21.1", "rift-loader": "1.0"},
            "files": []
        }"#;
        let err = plan_from_manifest(&parse(raw)).unwrap_err();
        assert!(matches!(err, LauncherError::InvalidArgument(_)), "{err:?}");
        assert!(err.to_string().contains("rift-loader"), "{err}");
    }

    #[test]
    fn rejects_wrong_format_version_and_game() {
        let v2 = OUR_MANIFEST.replace("\"formatVersion\": 1", "\"formatVersion\": 2");
        assert!(plan_from_manifest(&parse(&v2)).is_err());
        let other = OUR_MANIFEST.replace("\"game\": \"minecraft\"", "\"game\": \"storment\"");
        assert!(plan_from_manifest(&parse(&other)).is_err());
        let nomc = OUR_MANIFEST
            .replace("\"fabric-loader\": \"0.16.14\", ", "")
            .replace("\"minecraft\": \"1.21.1\"", "\"fabric-loader\": \"0.16.14\"");
        assert!(plan_from_manifest(&parse(&nomc)).is_err());
    }

    #[test]
    fn safe_join_rejects_traversal() {
        let root = Path::new("/tmp/instance");
        assert_eq!(
            safe_join_relative(root, "mods/jei.jar").unwrap(),
            PathBuf::from("/tmp/instance/mods/jei.jar")
        );
        assert_eq!(
            safe_join_relative(root, "config/../options.txt").unwrap(),
            PathBuf::from("/tmp/instance/options.txt")
        );
        for bad in ["../evil.jar", "/etc/passwd", "mods/../../x", "C:\\Windows\\x", ""] {
            assert!(
                matches!(safe_join_relative(root, bad), Err(LauncherError::UnsafePath(_))),
                "{bad} 必须被拒绝"
            );
        }
    }


    /// 造一个真 zip：manifest + overrides/config/a.txt + client-overrides/config/a.txt
    /// + overrides/options.txt，验证读得出、解得开、冲突时 client-overrides 胜出。
    #[test]
    fn reads_and_extracts_a_real_mrpack_zip() {
        use std::io::Write;
        let dir = std::env::temp_dir().join(format!(
            "mpack-mrpack-test-{}-{}",
            std::process::id(),
            "fixture"
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let zip_path = dir.join("pack.mrpack");
        {
            let file = std::fs::File::create(&zip_path).unwrap();
            let mut writer = zip::ZipWriter::new(file);
            let opts: zip::write::FileOptions<'_, ()> =
                zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);
            writer.start_file(MANIFEST_NAME, opts).unwrap();
            writer.write_all(OUR_MANIFEST.as_bytes()).unwrap();
            writer.start_file("overrides/config/a.txt", opts).unwrap();
            writer.write_all(b"from-overrides").unwrap();
            writer.start_file("client-overrides/config/a.txt", opts).unwrap();
            writer.write_all(b"from-client-overrides").unwrap();
            writer.start_file("overrides/options.txt", opts).unwrap();
            writer.write_all(b"gameOptions").unwrap();
            writer.finish().unwrap();
        }

        let manifest = read_manifest(&zip_path).unwrap();
        assert_eq!(manifest.name, "Chain ASM");
        assert_eq!(manifest.files.len(), 1);

        let instance = dir.join("versions/fabric-loader-0.16.14-1.21.1");
        std::fs::create_dir_all(&instance).unwrap();
        let written = extract_overrides(&zip_path, &instance).unwrap();
        assert_eq!(written, 3);
        assert_eq!(
            std::fs::read_to_string(instance.join("config/a.txt")).unwrap(),
            "from-client-overrides"
        );
        assert_eq!(
            std::fs::read_to_string(instance.join("options.txt")).unwrap(),
            "gameOptions"
        );

        std::fs::remove_dir_all(&dir).ok();
    }

    #[test]
    fn manifest_missing_is_clear_error() {
        use std::io::Write;
        let dir = std::env::temp_dir().join(format!("mpack-mrpack-bad-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let zip_path = dir.join("not-a-pack.mrpack");
        {
            let file = std::fs::File::create(&zip_path).unwrap();
            let mut writer = zip::ZipWriter::new(file);
            let opts: zip::write::FileOptions<'_, ()> =
                zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);
            writer.start_file("readme.txt", opts).unwrap();
            writer.write_all(b"hello").unwrap();
            writer.finish().unwrap();
        }
        let err = read_manifest(&zip_path).unwrap_err();
        assert!(err.to_string().contains(MANIFEST_NAME), "{err}");
        std::fs::remove_dir_all(&dir).ok();
    }
}
