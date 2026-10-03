//! 下载模块：从 VersionJson 生成下载计划 + 并发编排
//!
//! 职责：
//! - 从 mc-launcher-core 的 VersionJson 提取需要下载的文件
//! - 分类为 assets / libraries / other
//! - 调用 ConcurrentDownloader 并发下载
//! - 处理 asset index（先下载索引，再解析出所有资源文件）

pub mod cache;
pub mod concurrent;
pub mod item;
pub mod mirror;

pub use mirror::Mirror;

use std::collections::HashMap;
use std::path::Path;

use mc_launcher_core::core::maven::MavenCoordinate;
use mc_launcher_core::core::rules::{evaluate_rules, FeatureSet};
use mc_launcher_core::core::version::VersionJson;
use mc_launcher_core::platform::Platform;
use serde::Deserialize;

use crate::error::LauncherError;
use crate::Result;

use self::concurrent::{ConcurrentDownloader, DownloadGroup};
use self::item::DownloadItem;

/// 下载器：封装版本文件下载流程
pub struct Downloader {
    mirror: Mirror,
}

impl Downloader {
    /// 创建新的下载器
    pub fn new(mirror: Mirror) -> Self {
        Self { mirror }
    }

    /// 下载一个版本的所有文件（client.jar + libraries + assets）
    ///
    /// 流程：
    /// 1. 生成 client.jar 和 libraries 的下载计划
    /// 2. 并发下载
    /// 3. 下载并解析 asset index
    /// 4. 并发下载所有 asset 文件
    pub async fn download_version(
        &self,
        version: &VersionJson,
        minecraft_dir: &Path,
    ) -> Result<()> {
        let platform = Platform::current();
        let features = FeatureSet::default();

        // 1. 生成 client.jar + libraries 下载计划
        let items = plan_libraries_and_client(version, minecraft_dir, platform, &features)?;

        // 2. 下载 client.jar + libraries
        if !items.is_empty() {
            tracing::info!("开始下载 {} 个库文件和 client.jar", items.len());
            let downloader = ConcurrentDownloader::new(self.mirror);
            downloader.download_all(items).await?;
        }

        // 3. 下载并解析 asset index，然后下载 assets
        if let Some(asset_index) = &version.asset_index {
            let assets_dir = minecraft_dir.join("assets");
            let index_path = assets_dir.join("indexes").join(format!("{}.json", asset_index.id));

            // 下载 asset index
            let index_item = DownloadItem::new(
                &asset_index.url,
                &index_path,
                format!("asset index {}", asset_index.id),
            )
            .with_sha1(&asset_index.sha1)
            .with_size(asset_index.size as u64);

            cache::download_file(&index_item, self.mirror).await?;

            // 解析 asset index
            let index_content = std::fs::read_to_string(&index_path).map_err(|e| {
                LauncherError::Internal(format!("读取 asset index 失败: {}", e))
            })?;
            let asset_index_data: AssetIndexData =
                serde_json::from_str(&index_content).map_err(|e| {
                    LauncherError::Internal(format!("解析 asset index 失败: {}", e))
                })?;

            // 生成 asset 下载计划
            let mut asset_items = Vec::new();
            for (_path, obj) in &asset_index_data.objects {
                let hash = &obj.hash;
                let prefix = &hash[..2.min(hash.len())];
                let url = format!(
                    "https://resources.download.minecraft.net/{}/{}",
                    prefix, hash
                );
                let destination = assets_dir
                    .join("objects")
                    .join(prefix)
                    .join(hash);

                asset_items.push((
                    DownloadItem::new(url, destination, format!("asset {}", &hash[..8]))
                        .with_sha1(hash)
                        .with_size(obj.size),
                    DownloadGroup::Assets,
                ));
            }

            // 4. 并发下载 assets
            if !asset_items.is_empty() {
                tracing::info!("开始下载 {} 个资源文件", asset_items.len());
                let downloader = ConcurrentDownloader::new(self.mirror);
                downloader.download_all(asset_items).await?;
            }
        }

        Ok(())
    }
}

/// 从 VersionJson 生成 client.jar + libraries 下载计划
fn plan_libraries_and_client(
    version: &VersionJson,
    minecraft_dir: &Path,
    platform: Platform,
    features: &FeatureSet,
) -> Result<Vec<(DownloadItem, DownloadGroup)>> {
    let mut items = Vec::new();

    // client.jar
    if let Some(client) = version.downloads.get("client") {
        let version_id = version.id.as_deref().unwrap_or("unknown");
        let destination = minecraft_dir
            .join("versions")
            .join(version_id)
            .join(format!("{}.jar", version_id));
        items.push((
            DownloadItem::new(&client.url, destination, format!("client.jar {}", version_id))
                .with_sha1(&client.sha1)
                .with_size(client.size as u64),
            DownloadGroup::Other,
        ));
    }

    // libraries
    for library in &version.libraries {
        if !evaluate_rules(&library.rules, platform, features) {
            continue;
        }

        // 主 artifact
        if let Some(downloads) = &library.downloads {
            if let Some(artifact) = &downloads.artifact {
                let destination = minecraft_dir.join("libraries").join(&artifact.path);
                items.push((
                    DownloadItem::new(
                        &artifact.url,
                        destination,
                        format!("lib {}", library.name),
                    )
                    .with_sha1(&artifact.sha1)
                    .with_size(artifact.size as u64),
                    DownloadGroup::Libraries,
                ));
            }

            // natives（classifiers）
            if let Some(natives) = &library.natives {
                let os_name = platform.minecraft_os_name();
                if let Some(classifier_template) = natives.get(os_name) {
                    // ${arch} 在 64 位系统上替换为 "64"，32 位上替换为 ""
                    let arch_suffix = if platform.is_32_bit() { "" } else { "64" };
                    let classifier = classifier_template.replace("${arch}", arch_suffix);
                    if let Some(native_artifact) = downloads.classifiers.get(&classifier) {
                        let destination = minecraft_dir
                            .join("libraries")
                            .join(&native_artifact.path);
                        items.push((
                            DownloadItem::new(
                                &native_artifact.url,
                                destination,
                                format!("native {}", library.name),
                            )
                            .with_sha1(&native_artifact.sha1)
                            .with_size(native_artifact.size as u64),
                            DownloadGroup::Libraries,
                        ));
                    }
                }
            }
        } else if library.natives.is_none() {
            // Fabric/Quilt 风格的 profile 里 library 只有 name + url，没有 downloads
            // 块（实测 fabric-loader-0.16.14-1.21.1.json 的 8 条库全是 downloads:null）。
            // 只认 downloads.artifact 的话，加载器本体和 intermediary 一个都不会下载，
            // 装完能落库、任务回 success，但启动时 java 直接
            // ClassNotFoundException: net.fabricmc.loader.impl.launch.knot.KnotClient。
            // 这里按 maven 坐标自己拼路径与仓库 URL，和上游
            // install/libraries.rs:52-73、core/classpath.rs:63 的算法保持一致，
            // 否则下载落点与启动 classpath 对不上。
            let coordinate = MavenCoordinate::parse(&library.name)
                .map_err(|e| LauncherError::InvalidArgument(format!("library 坐标非法 {}: {e}", library.name)))?;
            let path = coordinate.artifact_path();
            let base = library
                .url
                .as_deref()
                .map(str::trim)
                .filter(|u| !u.is_empty())
                .unwrap_or("https://maven.fabricmc.net/");
            let url = format!(
                "{}/{}",
                base.trim_end_matches('/'),
                path.to_string_lossy().replace('\\', "/")
            );
            items.push((
                DownloadItem::new(url, minecraft_dir.join("libraries").join(&path),
                    format!("lib {}", library.name)),
                DownloadGroup::Libraries,
            ));
        }
    }

    Ok(items)
}

/// asset index JSON 结构
#[derive(Debug, Deserialize)]
struct AssetIndexData {
    objects: HashMap<String, AssetObject>,
}

#[derive(Debug, Deserialize)]
struct AssetObject {
    hash: String,
    size: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 真实 fabric profile 的形状：library 只有 name+url，没有 downloads。
    /// 这条用例锁住 O16 —— 缺兜底时加载器 jar 不下载，游戏必然启动失败。
    const FABRIC_PROFILE: &str = r#"{
        "id": "fabric-loader-0.16.14-1.21.1",
        "inheritsFrom": "1.21.1",
        "mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
        "libraries": [
          {"name": "net.fabricmc:fabric-loader:0.16.14", "url": "https://maven.fabricmc.net/", "rules": []},
          {"name": "net.fabricmc:intermediary:1.21.1", "url": "https://maven.fabricmc.net/", "rules": []},
          {"name": "com.mojang:logging:1.1.1", "rules": [], "downloads": {"artifact": {
              "path": "com/mojang/logging/1.1.1/logging-1.1.1.jar",
              "url": "https://libraries.minecraft.net/com/mojang/logging/1.1.1/logging-1.1.1.jar",
              "sha1": "aa", "size": 10}}}
        ]
    }"#;

    #[test]
    fn test_plan_downloads_url_only_libraries() {
        let version: VersionJson = serde_json::from_str(FABRIC_PROFILE).expect("profile 应可解析");
        let items = plan_libraries_and_client(
            &version,
            Path::new("/root/.minecraft"),
            Platform::current(),
            &FeatureSet::default(),
        )
        .expect("计划不应失败");

        let loader = items
            .iter()
            .find(|(i, _)| i.label.contains("fabric-loader"))
            .expect("fabric-loader 必须进下载计划（此前被静默跳过）");
        assert_eq!(
            loader.0.url,
            "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.14/fabric-loader-0.16.14.jar"
        );
        assert!(loader.0.destination.ends_with(
            "libraries/net/fabricmc/fabric-loader/0.16.14/fabric-loader-0.16.14.jar"
        ), "落点必须与上游 classpath 的 maven 路径一致: {:?}", loader.0.destination);
        assert!(loader.0.sha1.is_none(), "url 风格库没有 sha1，不能伪造");

        // 有 downloads 的库仍走原路径，且带 sha1
        let mojang = items.iter().find(|(i, _)| i.label.contains("com.mojang:logging")).unwrap();
        assert_eq!(mojang.0.url, "https://libraries.minecraft.net/com/mojang/logging/1.1.1/logging-1.1.1.jar");
        assert_eq!(mojang.0.sha1.as_deref(), Some("aa"));
        // intermediary 也要在
        assert!(items.iter().any(|(i, _)| i.label.contains("intermediary")));
    }

    /// url 缺失时用 fabric maven 兜底；natives-only（老 forge/lwjgl）不该被当成主库下载。
    #[test]
    fn test_plan_url_defaults_and_skips_natives_only() {
        let json = r#"{
            "id": "x", "libraries": [
              {"name": "org.quiltmc:quilt-loader:0.23.1", "rules": []},
              {"name": "org.lwjgl:lwjgl:3.3.1:natives-macos", "rules": [],
               "natives": {"osx": "natives-macos"}}
            ]
        }"#;
        let version: VersionJson = serde_json::from_str(json).expect("应可解析");
        let items = plan_libraries_and_client(
            &version, Path::new("/m"), Platform::current(), &FeatureSet::default())
            .expect("计划");
        let quilt = items.iter().find(|(i, _)| i.label.contains("quilt-loader")).expect("缺 url 时按 fabric maven 兜底");
        assert!(quilt.0.url.starts_with("https://maven.fabricmc.net/org/quiltmc/quilt-loader/0.23.1/"));
        assert!(!items.iter().any(|(i, _)| i.label.contains("lwjgl:3.3.1:natives-macos")
            && !i.destination.to_string_lossy().contains("natives")),
            "natives-only 的库不该按主 jar 下载");
    }

    #[test]
    fn test_mirror_from_str() {
        assert_eq!(Mirror::from_str("mojang"), Mirror::Mojang);
        assert_eq!(Mirror::from_str("bmclapi"), Mirror::Bmclapi);
        assert_eq!(Mirror::from_str("auto"), Mirror::Auto);
        assert_eq!(Mirror::from_str("unknown"), Mirror::Auto);
    }
}
