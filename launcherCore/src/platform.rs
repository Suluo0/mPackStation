use std::path::PathBuf;

use sysinfo::{Disks, System};

use crate::error::{LauncherError, Result};

/// 操作系统类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Os {
    Windows,
    Linux,
    Macos,
    Unknown,
}

impl Os {
    pub fn current() -> Self {
        if cfg!(target_os = "windows") {
            Os::Windows
        } else if cfg!(target_os = "linux") {
            Os::Linux
        } else if cfg!(target_os = "macos") {
            Os::Macos
        } else {
            Os::Unknown
        }
    }

    pub fn as_str(&self) -> &'static str {
        match self {
            Os::Windows => "windows",
            Os::Linux => "linux",
            Os::Macos => "osx",
            Os::Unknown => "unknown",
        }
    }
}

/// CPU 架构
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Arch {
    X86_64,
    Aarch64,
    X86,
    Unknown,
}

impl Arch {
    pub fn current() -> Self {
        match std::env::consts::ARCH {
            "x86_64" => Arch::X86_64,
            "aarch64" => Arch::Aarch64,
            "x86" => Arch::X86,
            _ => Arch::Unknown,
        }
    }

    pub fn as_str(&self) -> &'static str {
        match self {
            Arch::X86_64 => "x86_64",
            Arch::Aarch64 => "aarch64",
            Arch::X86 => "x86",
            Arch::Unknown => "unknown",
        }
    }
}

/// 系统内存信息（字节）
pub struct MemoryInfo {
    pub total_bytes: u64,
    pub available_bytes: u64,
}

impl MemoryInfo {
    pub fn current() -> Self {
        let mut sys = System::new();
        sys.refresh_memory();
        MemoryInfo {
            total_bytes: sys.total_memory(),
            available_bytes: sys.available_memory(),
        }
    }

    /// 总内存（MB）
    pub fn total_mb(&self) -> u64 {
        self.total_bytes / (1024 * 1024)
    }

    /// 可用内存（MB）
    pub fn available_mb(&self) -> u64 {
        self.available_bytes / (1024 * 1024)
    }
}

/// 数据目录（存储 Java 运行时、认证信息等）
pub fn data_dir() -> Result<PathBuf> {
    let base = match Os::current() {
        Os::Windows => std::env::var("APPDATA")
            .map(PathBuf::from)
            .unwrap_or_else(|_| dirs_fallback()),
        Os::Macos => {
            let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
            PathBuf::from(home).join("Library").join("Application Support")
        }
        Os::Linux => {
            let xdg = std::env::var("XDG_DATA_HOME")
                .map(PathBuf::from)
                .unwrap_or_else(|_| {
                    let home = std::env::var("HOME").unwrap_or_else(|_| ".".to_string());
                    PathBuf::from(home).join(".local").join("share")
                });
            xdg
        }
        Os::Unknown => dirs_fallback(),
    };

    let dir = base.join("mpack-launcher");
    std::fs::create_dir_all(&dir)?;
    Ok(dir)
}

fn dirs_fallback() -> PathBuf {
    std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."))
}

/// 检查指定路径的可用磁盘空间（MB）
pub fn available_disk_space(path: &std::path::Path) -> Result<u64> {
    let disks = Disks::new_with_refreshed_list();
    let canonical = path.canonicalize().unwrap_or_else(|_| path.to_path_buf());

    for disk in &disks {
        if canonical.starts_with(disk.mount_point()) {
            return Ok(disk.available_space() / (1024 * 1024));
        }
    }

    // 找不到匹配的磁盘，返回第一个磁盘的可用空间
    if let Some(disk) = disks.first() {
        Ok(disk.available_space() / (1024 * 1024))
    } else {
        Err(LauncherError::Internal(
            "无法获取磁盘信息".to_string(),
        ))
    }
}

/// 磁盘空间预检：确保目标目录有足够空间
pub fn ensure_disk_space(path: &std::path::Path, needed_mb: u64) -> Result<()> {
    let available = available_disk_space(path)?;
    if available < needed_mb {
        return Err(LauncherError::InsufficientDiskSpace {
            needed: needed_mb,
            available,
        });
    }
    Ok(())
}

/// 去除 Windows 扩展路径前缀 \\?\
fn strip_verbatim_prefix(path: &std::path::Path) -> PathBuf {
    let s = path.to_string_lossy();
    if let Some(stripped) = s.strip_prefix(r"\\?\") {
        PathBuf::from(stripped)
    } else {
        path.to_path_buf()
    }
}

/// 路径安全检查：防止目录穿越
///
/// 规则：target 解析后的最终路径必须在 base 目录内。
/// 使用 components() 逐项检查，不依赖 canonicalize（目标文件可能尚不存在）。
pub fn validate_safe_path(base: &std::path::Path, target: &std::path::Path) -> Result<PathBuf> {
    // base 必须存在且可 canonicalize
    let canonical_base = base.canonicalize().map_err(|_| {
        LauncherError::UnsafePath(format!("base 路径不存在: {}", base.display()))
    })?;
    let base_normalized = strip_verbatim_prefix(&canonical_base);

    // 如果 target 是绝对路径，直接使用；否则拼接到 base 下
    let candidate = if target.is_absolute() {
        target.to_path_buf()
    } else {
        base_normalized.join(target)
    };

    // 用 components 规范化（处理 .. 和 .）
    let mut normalized: Vec<std::path::Component> = Vec::new();
    for component in candidate.components() {
        match component {
            std::path::Component::ParentDir => {
                normalized.pop();
            }
            std::path::Component::CurDir => {}
            other => normalized.push(other),
        }
    }
    let normalized_path: PathBuf = normalized.iter().collect();

    // 最终路径必须在 base 内
    if !normalized_path.starts_with(&base_normalized) {
        return Err(LauncherError::UnsafePath(
            normalized_path.to_string_lossy().to_string(),
        ));
    }

    Ok(normalized_path)
}
