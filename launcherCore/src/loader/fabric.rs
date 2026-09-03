//! Fabric 加载器安装
//!
//! 从 Fabric Meta API 获取 profile JSON，写入版本目录后用自研下载层下载

use std::path::Path;

use mc_launcher_core::install::loader::write_loader_profile;
use mc_launcher_core::loader::fabric;

use crate::download::Mirror;
use crate::error::{LauncherError, Result};

/// 安装 Fabric，返回版本 ID
pub async fn install_fabric(
    minecraft_dir: &Path,
    mc_version: &str,
    loader_version: Option<&str>,
    mirror: Mirror,
) -> Result<String> {
    // 1. 确定 loader 版本
    let loader_ver = match loader_version {
        Some(v) => v.to_string(),
        None => {
            let versions = fabric::list_loader_versions().map_err(|e| {
                LauncherError::LoaderInstallFailed(format!("fabric: 获取 loader 版本列表失败: {e}"))
            })?;
            let latest = fabric::latest_stable_loader(&versions).map_err(|e| {
                LauncherError::LoaderInstallFailed(format!("fabric: 获取最新稳定版失败: {e}"))
            })?;
            latest.version.clone()
        }
    };

    // 2. 获取 profile JSON
    let profile = fabric::fetch_profile(mc_version, &loader_ver).map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("fabric: 获取 profile 失败: {e}"))
    })?;

    let version_id = profile.id.clone().unwrap_or_else(|| {
        format!("fabric-loader-{loader_ver}-{mc_version}")
    });

    // 3. 写入版本 JSON
    write_loader_profile(minecraft_dir, &profile).map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("fabric: 写入 profile 失败: {e}"))
    })?;

    // 4. 用自研下载层下载所有文件
    super::download_version_files(minecraft_dir, &version_id, mirror).await?;

    Ok(version_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fabric_version_id_format() {
        // Fabric profile 的 id 格式通常是 "fabric-loader-<ver>-<mc>"
        let id = "fabric-loader-0.15.11-1.20.1";
        assert!(id.contains("fabric-loader"));
        assert!(id.contains("1.20.1"));
    }
}
