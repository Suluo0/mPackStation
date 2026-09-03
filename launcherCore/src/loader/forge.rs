//! Forge 加载器安装
//!
//! 下载 installer.jar 并用 Java 执行，生成版本 JSON 后用自研下载层补全

use std::path::Path;

use mc_launcher_core::install::loader::{run_loader_installer, InstallerInvocation};
use mc_launcher_core::loader::forge;
use mc_launcher_core::loader::LoaderKind;

use crate::download::{cache, item::DownloadItem, Mirror};
use crate::error::{LauncherError, Result};
use crate::java::{JavaRegistry, mc_version_to_java};

/// 安装 Forge，返回版本 ID
pub async fn install_forge(
    minecraft_dir: &Path,
    mc_version: &str,
    loader_version: Option<&str>,
    mirror: Mirror,
    java_registry: &JavaRegistry,
) -> Result<String> {
    // 1. 确定 Forge 版本
    let forge_ver = match loader_version {
        Some(v) => v.to_string(),
        None => {
            let versions = tokio::task::spawn_blocking(|| forge::list_forge_versions())
                .await
                .map_err(|e| LauncherError::LoaderInstallFailed(format!("forge: 获取版本列表join失败: {e}")))?
                .map_err(|e| LauncherError::LoaderInstallFailed(format!("forge: 获取版本列表失败: {e}")))?;
            let mc = mc_version.to_string();
            let latest = forge::latest_for_minecraft(&versions, &mc).map_err(|e| {
                LauncherError::LoaderInstallFailed(format!("forge: 获取最新版失败: {e}"))
            })?;
            latest.to_string()
        }
    };

    // 2. 下载 installer.jar
    let installer_url = forge::installer_url(&forge_ver);
    let installer_path = minecraft_dir.join("cache").join(format!("forge-{forge_ver}-installer.jar"));
    if let Some(parent) = installer_path.parent() {
        std::fs::create_dir_all(parent)?;
    }

    let item = DownloadItem::new(
        installer_url,
        installer_path.clone(),
        format!("forge-{forge_ver}-installer.jar"),
    );
    cache::download_file(&item, mirror).await.map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("forge: 下载 installer 失败: {e}"))
    })?;

    // 3. 确定 Java 可执行文件
    let required_java = mc_version_to_java(mc_version)?;
    let java_path = java_registry
        .find(required_java)
        .map(|j| j.executable.clone())
        .ok_or(LauncherError::JavaNotFound { required: required_java })?;

    // 4. 执行 installer（同步阻塞，需spawn_blocking）
    let invocation = InstallerInvocation {
        loader: LoaderKind::Forge,
        java_executable: java_path,
        installer_path: installer_path.clone(),
        minecraft_dir: minecraft_dir.to_path_buf(),
    };
    tokio::task::spawn_blocking(move || run_loader_installer(&invocation))
        .await
        .map_err(|e| LauncherError::LoaderInstallFailed(format!("forge: installer join失败: {e}")))?
        .map_err(|e| LauncherError::LoaderInstallFailed(format!("forge: 执行 installer 失败: {e}")))?;

    // 5. 获取安装后的版本 ID
    let version_id = forge::forge_installed_version_id(&forge_ver).map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("forge: 解析版本 ID 失败: {e}"))
    })?;

    // 6. 用自研下载层补全文件
    super::download_version_files(minecraft_dir, &version_id, mirror).await?;

    Ok(version_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_forge_installer_url() {
        let url = forge::installer_url("1.20.1-47.2.0");
        assert!(url.contains("maven.minecraftforge.net"));
        assert!(url.contains("forge-1.20.1-47.2.0-installer.jar"));
    }

    #[test]
    fn test_forge_version_id() {
        let id = forge::forge_installed_version_id("1.20.1-47.2.0").unwrap();
        assert_eq!(id, "1.20.1-forge-47.2.0");
    }
}
