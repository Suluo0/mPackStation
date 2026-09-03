//! 安装编排层
//!
//! 协调 download/ 和 loader/ 模块完成完整的安装流程：
//! 1. 解析版本元数据
//! 2. 下载 libraries + assets + client.jar
//! 3. 安装加载器（Fabric/Forge/NeoForge/Quilt）
//! 4. 校验所有文件

use std::path::PathBuf;

use crate::error::Result;
use crate::lock::DirectoryLock;
use crate::platform;
use crate::protocol::{phase, Protocol};

/// 安装参数
pub struct InstallOptions {
    pub mc_version: String,
    pub loader: String,
    pub loader_version: String,
    pub dir: PathBuf,
    pub mirror: String,
    pub java: Option<PathBuf>,
}

/// 执行安装流程
pub async fn install(opts: InstallOptions) -> Result<String> {
    // 1. 获取目录锁，防止并发安装
    let _lock = DirectoryLock::acquire(&opts.dir)?;

    // 2. 磁盘空间预检（预估 1.5GB，实际按需调整）
    platform::ensure_disk_space(&opts.dir, 1536)?;

    // 3. 解析版本
    Protocol::phase(phase::RESOLVING_VERSION, "正在解析版本信息");

    // TODO M1: 调用 mc-launcher-core 解析 version manifest
    // TODO M1: 调用 download/ 下载 libraries + assets + client.jar
    // TODO M2: 调用 loader/ 安装加载器

    let version_id = if opts.loader == "vanilla" {
        opts.mc_version.clone()
    } else {
        format!(
            "{}-{}-{}",
            opts.mc_version, opts.loader, opts.loader_version
        )
    };

    Ok(version_id)
}
