//! Java 运行时自动下载
//!
//! 从 Mojang/BMCLAPI 获取 Java runtime 清单，下载到 runtime/{component}/ 目录。
//! 下载后可被 JavaRegistry 自动检测到（runtime 目录在扫描路径中）。

use std::path::{Path, PathBuf};

use serde::Deserialize;

use crate::download::cache::download_file;
use crate::download::item::DownloadItem;
use crate::download::mirror::Mirror;
use crate::error::LauncherError;
use crate::Result;
/// Mojang Java runtime 清单（all.json）
#[derive(Deserialize)]
struct JavaRuntimeIndex {
    #[serde(flatten)]
    platforms: std::collections::HashMap<String, std::collections::HashMap<String, Vec<RuntimeComponent>>>,
}

#[derive(Deserialize)]
struct RuntimeComponent {
    #[serde(rename = "manifest")]
    manifest: ManifestRef,
    version: RuntimeVersion,
}

#[derive(Deserialize)]
struct ManifestRef {
    #[allow(dead_code)]
    sha1: String,
    #[allow(dead_code)]
    size: u64,
    url: String,
}

#[derive(Deserialize)]
struct RuntimeVersion {
    name: String,
}

/// Java runtime manifest（单个 component 的文件清单）
#[derive(Deserialize)]
struct JavaManifest {
    files: std::collections::HashMap<String, ManifestFile>,
}

#[derive(Deserialize)]
struct ManifestFile {
    #[serde(rename = "type")]
    file_type: String,
    #[serde(default)]
    downloads: Option<ManifestDownloads>,
    #[allow(dead_code)]
    #[serde(default)]
    executable: Option<bool>,
}

#[derive(Deserialize)]
struct ManifestDownloads {
    raw: RawDownload,
}

#[derive(Deserialize)]
struct RawDownload {
    sha1: String,
    size: u64,
    url: String,
}

/// 下载指定主版本的 Java 运行时
///
/// 返回下载后的 java 可执行文件路径。
/// 下载目录：runtime_dir/{component_name}/
pub async fn download_java(
    major_version: u32,
    mirror: Mirror,
    runtime_dir: &Path,
) -> Result<PathBuf> {
    let platform = current_platform_key();
    tracing::info!("开始下载 Java {}，平台: {}", major_version, platform);

    // 1. 获取 runtime 清单
    let index = fetch_runtime_index(mirror).await?;

    // 2. 找到匹配的 component
    let platform_components = index
        .platforms
        .get(&platform)
        .ok_or_else(|| LauncherError::Internal(format!("平台 {} 无可用 Java runtime", platform)))?;

    let (component_name, component) = find_matching_component(platform_components, major_version)?;
    tracing::info!("选中 component: {} (版本 {})", component_name, component.version.name);

    // 3. 下载 manifest
    let manifest = fetch_manifest(&component.manifest.url, mirror).await?;

    // 4. 收集所有需要下载的文件
    let component_dir = runtime_dir.join(&component_name);
    std::fs::create_dir_all(&component_dir)?;

    let mut items: Vec<DownloadItem> = Vec::new();
    for (rel_path, file_info) in &manifest.files {
        if file_info.file_type != "file" {
            continue;
        }
        let downloads = match &file_info.downloads {
            Some(d) => d,
            None => continue,
        };

        // PCL2 #3827: 跳过 3 个无意义大量重复文件（LICENSE 等），BMCLAPI 对这些文件限流严重
        const SKIP_SHA1S: &[&str] = &[
            "12976a6c2b227cbac58969c1455444596c894656",
            "c80e4bab46e34d02826eab226a4441d0970f2aba",
            "84d2102ad171863db04e7ee22a259d1f6c5de4a5",
        ];
        if SKIP_SHA1S.contains(&downloads.raw.sha1.as_str()) {
            tracing::debug!("跳过重复文件: {}", rel_path);
            continue;
        }

        let dest = component_dir.join(rel_path);
        if let Some(parent) = dest.parent() {
            std::fs::create_dir_all(parent)?;
        }

        let item = DownloadItem::new(
            downloads.raw.url.clone(),
            dest,
            format!("java/{}/{}", component_name, rel_path),
        )
        .with_sha1(&downloads.raw.sha1)
        .with_size(downloads.raw.size);
        items.push(item);
    }

    // 5. 并发下载（限制 8 并发）
    let file_count = items.len();
    tracing::info!("开始下载 {} 个 Java 文件", file_count);
    let semaphore = std::sync::Arc::new(tokio::sync::Semaphore::new(2));
    let mut handles = Vec::new();
    for item in items {
        let sem = semaphore.clone();
        handles.push(tokio::spawn(async move {
            let _permit = sem.acquire().await.unwrap();
            download_file(&item, mirror).await
        }));
    }

    for handle in handles {
        handle.await.map_err(|e| LauncherError::Internal(format!("Java 下载任务 panic: {e}")))?
            .map_err(|e| LauncherError::Internal(format!("下载 Java 文件失败: {e}")))?;
    }

    tracing::info!("Java {} 下载完成，共 {} 个文件", major_version, file_count);

    // 5. 返回 java 可执行文件路径
    let java_exe = component_dir.join("bin").join(java_executable_name());
    if !java_exe.is_file() {
        return Err(LauncherError::Internal(format!(
            "Java 下载完成但未找到可执行文件: {}",
            java_exe.display()
        )));
    }

    Ok(java_exe)
}

/// 获取 runtime 清单（all.json）
async fn fetch_runtime_index(mirror: Mirror) -> Result<JavaRuntimeIndex> {
    let url = match mirror {
        Mirror::Mojang | Mirror::Auto => {
            "https://piston-meta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"
        }
        Mirror::Bmclapi => {
            "https://bmclapi2.bangbang93.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"
        }
    };

    let resp = reqwest::get(url).await?;

    let index: JavaRuntimeIndex = resp
        .json()
        .await
        .map_err(|e| LauncherError::Internal(format!("解析 Java runtime 清单失败: {e}")))?;

    Ok(index)
}

/// 获取 component 的 manifest
async fn fetch_manifest(url: &str, mirror: Mirror) -> Result<JavaManifest> {
    // BMCLAPI 镜像重写
    let actual_url = match mirror {
        Mirror::Bmclapi => url.replace(
            "piston-meta.mojang.com",
            "bmclapi2.bangbang93.com",
        ),
        _ => url.to_string(),
    };

    let resp = reqwest::get(&actual_url).await?;

    let manifest: JavaManifest = resp
        .json()
        .await
        .map_err(|e| LauncherError::Internal(format!("解析 Java manifest 失败: {e}")))?;

    Ok(manifest)
}

/// 从平台 components 中找到主版本匹配的 component
///
/// 优先级：精确匹配主版本 > 非snapshot版本 > snapshot版本
fn find_matching_component<'a>(
    components: &'a std::collections::HashMap<String, Vec<RuntimeComponent>>,
    major_version: u32,
) -> Result<(&'a String, &'a RuntimeComponent)> {
    // 收集所有精确匹配的 component
    let mut exact_matches: Vec<(&String, &RuntimeComponent)> = Vec::new();
    for (name, versions) in components {
        if name == "minecraft-java-exe" {
            continue;
        }
        if let Some(first) = versions.first() {
            let comp_major = parse_java_major(&first.version.name);
            if comp_major == major_version {
                exact_matches.push((name, first));
            }
        }
    }

    if !exact_matches.is_empty() {
        // 优先非 snapshot
        exact_matches.sort_by_key(|(name, _)| name.contains("snapshot"));
        return Ok(exact_matches[0]);
    }

    // 没有精确匹配，找最接近的（>= required 的最低版本）
    let mut best: Option<(&String, &RuntimeComponent, u32)> = None;
    for (name, versions) in components {
        if name == "minecraft-java-exe" {
            continue;
        }
        if let Some(first) = versions.first() {
            let comp_major = parse_java_major(&first.version.name);
            if comp_major >= major_version {
                match best {
                    None => best = Some((name, first, comp_major)),
                    Some((_, _, best_major)) if comp_major < best_major => {
                        best = Some((name, first, comp_major))
                    }
                    _ => {}
                }
            }
        }
    }

    match best {
        Some((name, comp, _)) => Ok((name, comp)),
        None => Err(LauncherError::Internal(format!(
            "未找到 Java {} 的 runtime component",
            major_version
        ))),
    }
}

/// 从版本字符串解析主版本号
/// "17.0.8" → 17, "1.8.0_381" → 8, "8u51-..." → 8
fn parse_java_major(version: &str) -> u32 {
    if version.starts_with("1.") {
        version
            .split('.')
            .nth(1)
            .and_then(|s| s.split('_').next())
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    } else if version.contains('u') {
        version
            .split('u')
            .next()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    } else {
        version
            .split('.')
            .next()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    }
}

/// 当前平台的 runtime 清单 key
fn current_platform_key() -> String {
    let arch = if cfg!(target_arch = "x86_64") {
        "x64"
    } else if cfg!(target_arch = "aarch64") {
        "arm64"
    } else {
        "x86"
    };

    if cfg!(windows) {
        format!("windows-{}", arch)
    } else if cfg!(target_os = "macos") {
        if arch == "arm64" {
            "mac-os-arm64".to_string()
        } else {
            "mac-os".to_string()
        }
    } else {
        "linux".to_string()
    }
}

fn java_executable_name() -> &'static str {
    if cfg!(windows) {
        "java.exe"
    } else {
        "java"
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_java_major_17() {
        assert_eq!(parse_java_major("17.0.8"), 17);
    }

    #[test]
    fn test_parse_java_major_8() {
        assert_eq!(parse_java_major("1.8.0_381"), 8);
    }

    #[test]
    fn test_parse_java_major_21() {
        assert_eq!(parse_java_major("21.0.1"), 21);
    }

    #[test]
    fn test_parse_java_major_8u() {
        assert_eq!(parse_java_major("8u51-cacert"), 8);
    }

    #[test]
    fn test_platform_key_windows() {
        if cfg!(windows) {
            assert!(current_platform_key().starts_with("windows-"));
        }
    }
}
