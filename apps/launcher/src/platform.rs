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

/// 路径安全检查：防止目录穿越
pub fn validate_safe_path(base: &std::path::Path, target: &std::path::Path) -> Result<PathBuf> {
    let canonical_base = base.canonicalize().unwrap_or_else(|_| base.to_path_buf());
    let canonical_target = target.canonicalize().unwrap_or_else(|_| target.to_path_buf());

    if !canonical_target.starts_with(&canonical_base) {
        return Err(LauncherError::UnsafePath(
            canonical_target.to_string_lossy().to_string(),
        ));
    }
    Ok(canonical_target)
}
