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
const DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(300);
/// 连接超时
const CONNECT_TIMEOUT: Duration = Duration::from_secs(30);

/// 下载单个文件（带重试）
///
/// 流程：
/// 1. 预校验：文件已存在且 SHA1 匹配则跳过
/// 2. 多源竞速或依次尝试
/// 3. 失败时最多重试 3 次
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

    // 2. 重试下载
    let max_retries = 5;
    let mut last_error = None;

    for attempt in 0..max_retries {
        if attempt > 0 {
            tracing::debug!("重试下载 ({}/{}): {}", attempt + 1, max_retries, item.label);
            tokio::time::sleep(Duration::from_secs(1)).await;
        }

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
                        last_error = Some(LauncherError::ChecksumMismatch {
                            file: item.destination.to_string_lossy().to_string(),
                            expected: expected.clone(),
                            actual,
                        });
                        continue;
                    }
                }
                return Ok(());
            }
            Err(e) => {
                last_error = Some(e);
            }
        }
    }

    Err(last_error.unwrap_or(LauncherError::DownloadFailed {
        url: item.url.clone(),
        message: "下载失败".into(),
    }))
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

/// 构建 reqwest 客户端（自动检测系统代理，不可用则直连）
pub(crate) fn build_client() -> Result<reqwest::Client> {
    let mut builder = reqwest::Client::builder()
        .connect_timeout(CONNECT_TIMEOUT)
        .timeout(DOWNLOAD_TIMEOUT)
        .user_agent("mPackLauncher/0.1")
        .redirect(reqwest::redirect::Policy::limited(20));

    // 自动检测系统代理（Windows 注册表），不可用则直连
    if let Some(proxy_url) = detect_system_proxy() {
        if let Ok(proxy) = reqwest::Proxy::all(&proxy_url) {
            builder = builder.proxy(proxy);
            tracing::debug!("使用系统代理: {}", proxy_url);
        }
    }

    builder
        .build()
        .map_err(|e| LauncherError::Internal(format!("创建 HTTP 客户端失败: {}", e)))
}

/// 检测 Windows 系统代理设置（验证可用性）
#[cfg(windows)]
fn detect_system_proxy() -> Option<String> {
    use std::process::Command;
    let output = Command::new("reg")
        .args([
            "query",
            r"HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings",
            "/v",
            "ProxyEnable",
        ])
        .output()
        .ok()?;
    let stdout = String::from_utf8_lossy(&output.stdout);
    if !stdout.contains("0x1") {
        return None;
    }

    let output = Command::new("reg")
        .args([
            "query",
            r"HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings",
            "/v",
            "ProxyServer",
        ])
        .output()
        .ok()?;
    let stdout = String::from_utf8_lossy(&output.stdout);
    for line in stdout.lines() {
        if line.contains("ProxyServer") {
            if let Some(pos) = line.find("REG_SZ") {
                let addr = line[pos + 6..].trim();
                if !addr.is_empty() {
                    let proxy_url = format!("http://{}", addr);
                    // 验证代理是否可用（简单 TCP 连接测试）
                    if is_proxy_alive(addr) {
                        return Some(proxy_url);
                    } else {
                        tracing::debug!("系统代理 {} 不可用，使用直连", addr);
                        return None;
                    }
                }
            }
        }
    }
    None
}

/// 检测代理是否可用（TCP 连接测试）
#[cfg(windows)]
fn is_proxy_alive(addr: &str) -> bool {
    use std::net::TcpStream;
    use std::time::Duration;
    TcpStream::connect_timeout(
        &addr.parse().unwrap_or_else(|_| "127.0.0.1:7897".parse().unwrap()),
        Duration::from_secs(2),
    )
    .is_ok()
}

#[cfg(not(windows))]
fn detect_system_proxy() -> Option<String> {
    std::env::var("HTTPS_PROXY")
        .or_else(|_| std::env::var("https_proxy"))
        .ok()
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
