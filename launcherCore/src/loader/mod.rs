//! 加载器安装模块
//!
//! 支持 Fabric / Quilt / Forge / NeoForge 四种加载器
//! Fabric/Quilt: 从 meta API 获取 profile JSON，写入后用自研下载层下载
//! Forge/NeoForge: 下载 installer.jar 并执行，生成版本 JSON 后补全下载

use std::path::{Path, PathBuf};

use mc_launcher_core::launcher::Launcher;
use mc_launcher_core::loader::LoaderKind;

use crate::download::Downloader;
use crate::error::{LauncherError, Result};
use crate::java::JavaRegistry;

pub mod fabric;
pub mod forge;
pub mod neoforge;
pub mod quilt;

/// 加载器类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LoaderType {
    Fabric,
    Quilt,
    Forge,
    NeoForge,
}

impl LoaderType {
    pub fn as_str(&self) -> &'static str {
        match self {
            LoaderType::Fabric => "fabric",
            LoaderType::Quilt => "quilt",
            LoaderType::Forge => "forge",
            LoaderType::NeoForge => "neoforge",
        }
    }

    pub fn to_mc_kind(&self) -> LoaderKind {
        match self {
            LoaderType::Fabric => LoaderKind::Fabric,
            LoaderType::Quilt => LoaderKind::Quilt,
            LoaderType::Forge => LoaderKind::Forge,
            LoaderType::NeoForge => LoaderKind::NeoForge,
        }
    }
}

/// 加载器安装器
pub struct LoaderInstaller {
    minecraft_dir: PathBuf,
    mirror: crate::download::Mirror,
}

impl LoaderInstaller {
    pub fn new(minecraft_dir: impl Into<PathBuf>, mirror: crate::download::Mirror) -> Self {
        Self {
            minecraft_dir: minecraft_dir.into(),
            mirror,
        }
    }

    /// 安装加载器，返回安装后的版本 ID
    pub async fn install(
        &self,
        mc_version: &str,
        loader: LoaderType,
        loader_version: Option<&str>,
        java_registry: &JavaRegistry,
    ) -> Result<String> {
        let version_id = match loader {
            LoaderType::Fabric => {
                fabric::install_fabric(&self.minecraft_dir, mc_version, loader_version, self.mirror).await?
            }
            LoaderType::Quilt => {
                quilt::install_quilt(&self.minecraft_dir, mc_version, loader_version, self.mirror).await?
            }
            LoaderType::Forge => {
                forge::install_forge(&self.minecraft_dir, mc_version, loader_version, self.mirror, java_registry).await?
            }
            LoaderType::NeoForge => {
                neoforge::install_neoforge(&self.minecraft_dir, mc_version, loader_version, self.mirror, java_registry).await?
            }
        };
        Ok(version_id)
    }
}

/// 用自研下载层下载指定版本的所有文件（加载器安装后调用）
async fn download_version_files(
    minecraft_dir: &Path,
    version_id: &str,
    mirror: crate::download::Mirror,
) -> Result<()> {
    let launcher = Launcher::new(minecraft_dir);
    let version = launcher.load_version(version_id).map_err(|e| {
        LauncherError::LoaderInstallFailed(format!("加载版本 JSON 失败: {e}"))
    })?;

    let downloader = Downloader::new(mirror);
    downloader.download_version(&version, minecraft_dir).await?;
    Ok(())
}
