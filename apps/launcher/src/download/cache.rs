//! 单文件下载：断点续传 + SHA1 校验 + 原子 rename + 多源竞速

use std::path::{Path, PathBuf};
use std::time::Duration;

use tokio::fs;
use tokio::io::AsyncWriteExt;

use crate::error::LauncherError;
use crate::Result;

use super::item::{sha1_file, DownloadItem, FileChecker};
use super::mirror::{get_download_urls, Mirror};

/// 下载超时
const DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(120);
/// 连接超时
const CONNECT_TIMEOUT: Duration = Duration::from_secs(15);

/// 下载单个文件
///
/// 流程：
/// 1. 预校验：文件已存在且 SHA1 匹配则跳过
/// 2. 多源竞速：auto 模式同时从官方和 BMCLAPI 下载
/// 3. 断点续传：.partial 文件存在时用 Range 续传
/// 4. 校验：下载完成后验证 SHA1
/// 5. 原子 rename：.partial → 目标路径
pub async fn download_file(item: &DownloadItem, mirror: Mirror) -> Result<()> {
    // 1. 预校验
    if FileChecker::should_skip(item) {
        tracing::debug!("文件已存在且校验通过，跳过: {}", item.label);
        return Ok(());
    }

    let urls = get_download_urls(&item.url, mirror);
    if urls.is_empty() {
        return Err(LauncherError::DownloadFailed {
            url: item.url.clone(),
            message: "无可用下载源".into(),
        });
    }

    // 2. 多源竞速或单源下载
    let result = if urls.len() == 1 {
        download_from_single_source(&urls[0], item).await
    } else {
        download_with_race(&urls, item).await
    };

    match result {
        Ok(()) => {
            // 3. 校验 SHA1
            if let Some(expected) = &item.sha1 {
                let actual = sha1_file(&item.destination).map_err(|e| {
                    LauncherError::Internal(format!("SHA1 计算失败: {}", e))
                })?;
                if actual != *expected {
                    // 校验失败，删除文件
                    let _ = fs::remove_file(&item.destination).await;
                    return Err(LauncherError::ChecksumMismatch {
                        file: item.destination.to_string_lossy().to_string(),
                        expected: expected.clone(),
                        actual,
                    });
                }
            }
            Ok(())
        }
        Err(e) => Err(e),
    }
}

/// 从单个源下载
async fn download_from_single_source(url: &str, item: &DownloadItem) -> Result<()> {
    let client = build_client()?;
    do_download(&client, url, item).await
}

/// 多源竞速：同时发起多个下载，第一个成功且校验通过的胜出
async fn download_with_race(urls: &[String], item: &DownloadItem) -> Result<()> {
    let client = build_client()?;
    let mut handles = Vec::new();

    for url in urls {
        let client = client.clone();
        let url = url.clone();
        let item = item.clone();
        // 每个源用不同的 .partial 文件名避免冲突
        let partial_path = item
            .destination
            .with_extension(format!("partial.{}", short_hash(&url)));
        handles.push(tokio::spawn(async move {
            do_download_to(&client, &url, &partial_path, &item.label).await
        }));
    }

    // 等待第一个成功的
    let mut last_error = None;
    for handle in handles {
        match handle.await {
            Ok(Ok(partial_path)) => {
                // 校验通过后 rename
                if let Some(expected) = &item.sha1 {
                    match sha1_file(&partial_path) {
                        Ok(actual) if actual == *expected => {
                            fs::rename(&partial_path, &item.destination)
                                .await
                                .map_err(|e| LauncherError::Internal(format!("rename 失败: {}", e)))?;
                            return Ok(());
                        }
                        _ => {
                            let _ = fs::remove_file(&partial_path).await;
                            last_error = Some(LauncherError::ChecksumMismatch {
                                file: partial_path.to_string_lossy().to_string(),
                                expected: expected.clone(),
                                actual: "校验失败".into(),
                            });
                            continue;
                        }
                    }
                } else {
                    fs::rename(&partial_path, &item.destination)
                        .await
                        .map_err(|e| LauncherError::Internal(format!("rename 失败: {}", e)))?;
                    return Ok(());
                }
            }
            Ok(Err(e)) => {
                last_error = Some(e);
            }
            Err(e) => {
                last_error = Some(LauncherError::Internal(format!("任务 panic: {}", e)));
            }
        }
    }

    Err(last_error.unwrap_or(LauncherError::DownloadFailed {
        url: item.url.clone(),
        message: "所有源均失败".into(),
    }))
}

/// 执行实际下载（支持断点续传）
async fn do_download(
    client: &reqwest::Client,
    url: &str,
    item: &DownloadItem,
) -> Result<()> {
    let partial_path = item.destination.with_extension("partial");
    do_download_to(client, url, &partial_path, &item.label).await?;
    fs::rename(&partial_path, &item.destination)
        .await
        .map_err(|e| LauncherError::Internal(format!("rename 失败: {}", e)))?;
    Ok(())
}

/// 下载到指定路径（支持断点续传）
async fn do_download_to(
    client: &reqwest::Client,
    url: &str,
    partial_path: &Path,
    label: &str,
) -> Result<PathBuf> {
    // 检查已下载的字节数（断点续传）
    let existing_bytes = if partial_path.is_file() {
        fs::metadata(partial_path)
            .await
            .map(|m| m.len())
            .unwrap_or(0)
    } else {
        0
    };

    // 构建请求
    let mut request = client.get(url);
    if existing_bytes > 0 {
        request = request.header("Range", format!("bytes={}-", existing_bytes));
    }

    let response = request
        .send()
        .await
        .map_err(|e| LauncherError::DownloadFailed {
            url: url.to_string(),
            message: format!("请求失败: {}", e),
        })?;

    if !response.status().is_success() && response.status() != reqwest::StatusCode::PARTIAL_CONTENT {
        return Err(LauncherError::DownloadFailed {
            url: url.to_string(),
            message: format!("HTTP {}", response.status()),
        });
    }

    // 打开文件（追加模式用于断点续传）
    if let Some(parent) = partial_path.parent() {
        fs::create_dir_all(parent).await.map_err(|e| {
            LauncherError::Internal(format!("创建目录失败: {}", e))
        })?;
    }

    let mut file = if existing_bytes > 0 {
        tokio::fs::OpenOptions::new()
            .append(true)
            .open(partial_path)
            .await
            .map_err(|e| LauncherError::Internal(format!("打开文件失败: {}", e)))?
    } else {
        tokio::fs::File::create(partial_path)
            .await
            .map_err(|e| LauncherError::Internal(format!("创建文件失败: {}", e)))?
    };

    // 流式写入
    let mut stream = response.bytes_stream();
    use futures_util::StreamExt;
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(|e| LauncherError::DownloadFailed {
            url: url.to_string(),
            message: format!("读取数据失败: {}", e),
        })?;
        file.write_all(&chunk)
            .await
            .map_err(|e| LauncherError::Internal(format!("写入文件失败: {}", e)))?;
    }

    file.flush()
        .await
        .map_err(|e| LauncherError::Internal(format!("flush 失败: {}", e)))?;

    tracing::debug!("下载完成: {}", label);
    Ok(partial_path.to_path_buf())
}

/// 构建 reqwest 客户端
fn build_client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .connect_timeout(CONNECT_TIMEOUT)
        .timeout(DOWNLOAD_TIMEOUT)
        .build()
        .map_err(|e| LauncherError::Internal(format!("创建 HTTP 客户端失败: {}", e)))
}

/// 生成 URL 的短哈希（用于区分不同源的 .partial 文件）
fn short_hash(s: &str) -> String {
    let mut hash: u32 = 5381;
    for b in s.bytes() {
        hash = hash.wrapping_mul(33).wrapping_add(b as u32);
    }
    format!("{:08x}", hash)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_short_hash() {
        let h1 = short_hash("https://example.com/a.jar");
        let h2 = short_hash("https://example.com/b.jar");
        assert_ne!(h1, h2);
        assert_eq!(h1.len(), 8);
    }
}
