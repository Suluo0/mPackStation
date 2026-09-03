//! 并发下载调度：assets 32 并发 / libraries 16 并发

use std::sync::Arc;

use tokio::sync::Semaphore;

use super::cache::download_file;
use super::item::DownloadItem;
use super::mirror::Mirror;

/// 下载文件类型（决定使用哪个并发组）
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DownloadGroup {
    /// 资源文件（assets），高并发
    Assets,
    /// 支持库（libraries），中并发
    Libraries,
    /// 其他（client.jar 等），中并发
    Other,
}

/// 并发下载器
pub struct ConcurrentDownloader {
    /// assets 组信号量（32 并发）
    assets_sem: Arc<Semaphore>,
    /// libraries 组信号量（16 并发）
    libraries_sem: Arc<Semaphore>,
    /// 镜像模式
    mirror: Mirror,
}

impl ConcurrentDownloader {
    /// 创建新的并发下载器
    pub fn new(mirror: Mirror) -> Self {
        Self {
            assets_sem: Arc::new(Semaphore::new(32)),
            libraries_sem: Arc::new(Semaphore::new(16)),
            mirror,
        }
    }

    /// 并发下载一批文件
    ///
    /// 按 DownloadGroup 分配到不同的并发组。
    /// 全部完成后返回；任何一个失败则返回第一个错误。
    pub async fn download_all(&self, items: Vec<(DownloadItem, DownloadGroup)>) -> crate::Result<()> {
        let mut handles = Vec::new();

        for (item, group) in items {
            let sem = match group {
                DownloadGroup::Assets => self.assets_sem.clone(),
                DownloadGroup::Libraries | DownloadGroup::Other => self.libraries_sem.clone(),
            };
            let mirror = self.mirror;

            handles.push(tokio::spawn(async move {
                let _permit = sem
                    .acquire()
                    .await
                    .expect("semaphore 不应被关闭");
                download_file(&item, mirror).await
            }));
        }

        // 等待所有任务完成，收集第一个错误
        let mut first_error = None;
        for handle in handles {
            match handle.await {
                Ok(Ok(())) => {}
                Ok(Err(e)) => {
                    if first_error.is_none() {
                        first_error = Some(e);
                    }
                }
                Err(e) => {
                    if first_error.is_none() {
                        first_error = Some(crate::error::LauncherError::Internal(format!(
                            "下载任务 panic: {}",
                            e
                        )));
                    }
                }
            }
        }

        match first_error {
            Some(e) => Err(e),
            None => Ok(()),
        }
    }

    /// 获取当前并发配置
    pub fn concurrency_info(&self) -> (usize, usize) {
        (32, 16) // assets, libraries
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_concurrency_info() {
        let downloader = ConcurrentDownloader::new(Mirror::Auto);
        let (assets, libs) = downloader.concurrency_info();
        assert_eq!(assets, 32);
        assert_eq!(libs, 16);
    }
}
