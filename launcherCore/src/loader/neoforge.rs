//! NeoForge 加载器安装
//!
//! 下载 installer.jar 并用 Java 执行，生成版本 JSON 后用自研下载层补全

use std::path::Path;

use mc_launcher_core::install::loader::{run_loader_installer, InstallerInvocation};
use mc_launcher_core::loader::neoforge;
use mc_launcher_core::loader::LoaderKind;

use crate::download::{cache, item::DownloadItem, Mirror};
use crate::error::{LauncherError, Result};
use crate::java::{JavaRegistry, mc_version_to_java};

/// 安装 NeoForge，返回版本 ID
pub async fn install_neoforge(
    minecraft_dir: &Path,
    mc_version: &str,
    loader_version: Option<&str>,
    mirror: Mirror,
    java_registry: &JavaRegistry,
) -> Result<String> {
    // 1. 确定 NeoForge 版本
    let neoforge_ver = match loader_version {
        Some(v) => v.to_string(),
        None => {
            let versions = neoforge::list_neoforge_versions().map_err(|e| {
                LauncherError::LoaderInstallFailed(format!("neoforge: 获取版本列表失败: {e}"))
            })?;
            let latest = neoforge::latest_for_minecraft(&versions, mc_version).map_err(|e| {
                LauncherError::LoaderInstallFailed(format!("neoforge: 获取最新版失败: {e}"))
            })?;
            latest.to_string()
        }
    };

    // 2. 下载 installer.jar
    let installer_url = neoforge::installer_url(&neoforge_ver);
    let installer_path = minecraft_dir
        .join("cache")
        .join(format!("neoforge-{neoforge_ver}-installer.jar"));
    if let Some(parent) = installer_path.parent() {
        std::fs::create_dir_all(parent)?;
    }

    let item = DownloadItem::new(
        installer_url,
        installer_path.clone(),
        format!("neoforge-{neoforge_ver}-installer.jar"),
    );
    cache::download_file(&item, mirror).await.map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("neoforge: 下载 installer 失败: {e}"))
    })?;

    // 3. 确定 Java 可执行文件
    let required_java = mc_version_to_java(mc_version)?;
    let java_path = java_registry
        .find(required_java)
        .map(|j| j.executable.clone())
        .ok_or(LauncherError::JavaNotFound { required: required_java })?;

    // 4. 执行 installer
    let invocation = InstallerInvocation {
        loader: LoaderKind::NeoForge,
        java_executable: java_path,
        installer_path: installer_path.clone(),
        minecraft_dir: minecraft_dir.to_path_buf(),
    };
    run_loader_installer(&invocation).map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("neoforge: 执行 installer 失败: {e}"))
    })?;

    // 5. 获取安装后的版本 ID
    let version_id = neoforge::neoforge_installed_version_id(mc_version, &neoforge_ver);

    // 6. 用自研下载层补全文件
    super::download_version_files(minecraft_dir, &version_id, mirror).await?;

    Ok(version_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_neoforge_installer_url() {
        let url = neoforge::installer_url("20.4.80-beta");
        assert!(url.contains("maven.neoforged.net"));
        assert!(url.contains("neoforge-20.4.80-beta-installer.jar"));
    }

    #[test]
    fn test_neoforge_version_id() {
        let id = neoforge::neoforge_installed_version_id("1.20.4", "20.4.80-beta");
        assert_eq!(id, "neoforge-20.4.80-beta");
    }
}
