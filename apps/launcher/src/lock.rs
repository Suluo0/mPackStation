use std::fs::{File, OpenOptions};
use std::path::{Path, PathBuf};

use fs2::FileExt;

use crate::error::{LauncherError, Result};

/// 目录级文件锁，防止多进程并发操作同一 Minecraft 目录
///
/// 锁文件位于 `<dir>/.mpack-launcher.lock`
/// 进程退出时自动释放（OS 级文件锁）。
pub struct DirectoryLock {
    _file: File,
    path: PathBuf,
}

impl DirectoryLock {
    /// 尝试获取目录锁，失败立即返回错误（不阻塞）
    pub fn acquire(dir: &Path) -> Result<Self> {
        let lock_path = dir.join(".mpack-launcher.lock");

        // 确保目录存在
        std::fs::create_dir_all(dir)?;

        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .open(&lock_path)?;

        // 尝试非阻塞排他锁
        file.try_lock_exclusive().map_err(|_| {
            LauncherError::DirectoryLocked(dir.to_string_lossy().to_string())
        })?;

        Ok(DirectoryLock {
            _file: file,
            path: lock_path,
        })
    }

    /// 锁文件路径
    pub fn lock_path(&self) -> &Path {
        &self.path
    }
}

impl Drop for DirectoryLock {
    fn drop(&mut self) {
        // File drop 时自动释放锁，fs2 的 FileExt 在 drop 时解锁
        // 锁文件本身保留（不删除，避免竞态）
    }
}
