//! Java 自动下载（M3 实现，当前为骨架）

use crate::error::LauncherError;
use crate::Result;

/// 下载指定主版本的 Java 运行时
///
/// M3 实现：从 Adoptium API 下载对应版本的 JRE/JDK。
/// 当前未实现，返回错误提示使用系统 Java。
pub async fn download_java(_major_version: u32) -> Result<std::path::PathBuf> {
    Err(LauncherError::Internal(
        "Java 自动下载尚未实现，请使用系统已安装的 Java".into(),
    ))
}
