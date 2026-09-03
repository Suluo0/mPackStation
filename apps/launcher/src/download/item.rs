//! 下载项 + 文件预校验

use std::path::{Path, PathBuf};

/// 单个文件下载项
#[derive(Debug, Clone)]
pub struct DownloadItem {
    /// 原始下载 URL
    pub url: String,
    /// 目标文件路径
    pub destination: PathBuf,
    /// 期望的 SHA1 校验值（无则不校验）
    pub sha1: Option<String>,
    /// 文件大小（字节，用于预估和日志）
    pub size: Option<u64>,
    /// 人类可读标签
    pub label: String,
}

impl DownloadItem {
    /// 创建新的下载项
    pub fn new(url: impl Into<String>, destination: impl Into<PathBuf>, label: impl Into<String>) -> Self {
        Self {
            url: url.into(),
            destination: destination.into(),
            sha1: None,
            size: None,
            label: label.into(),
        }
    }

    /// 设置 SHA1 校验值
    pub fn with_sha1(mut self, sha1: impl Into<String>) -> Self {
        self.sha1 = Some(sha1.into());
        self
    }

    /// 设置文件大小
    pub fn with_size(mut self, size: u64) -> Self {
        self.size = Some(size);
        self
    }
}

/// 文件预校验器：检查目标文件是否已存在且校验通过
pub struct FileChecker;

impl FileChecker {
    /// 检查文件是否已存在且 SHA1 匹配
    ///
    /// 返回 true 表示文件已存在且校验通过，可以跳过下载。
    /// 无 SHA1 时，如果文件存在也返回 true（信任已存在文件）。
    pub fn should_skip(item: &DownloadItem) -> bool {
        if !item.destination.is_file() {
            return false;
        }

        match &item.sha1 {
            Some(expected) => {
                // 计算现有文件的 SHA1
                match sha1_file(&item.destination) {
                    Ok(actual) => actual == *expected,
                    Err(_) => false,
                }
            }
            None => true, // 无校验值，文件存在即跳过
        }
    }
}

/// 计算文件的 SHA1 哈希（十六进制字符串）
pub fn sha1_file(path: &Path) -> std::io::Result<String> {
    use std::fs::File;
    use std::io::Read;

    let mut file = File::open(path)?;
    let mut hasher = Sha1Hasher::new();
    let mut buffer = [0u8; 8192];
    loop {
        let n = file.read(&mut buffer)?;
        if n == 0 {
            break;
        }
        hasher.update(&buffer[..n]);
    }
    Ok(hasher.finalize())
}

/// 简单的 SHA1 实现（避免额外依赖，mc-launcher-core 已带 sha1）
struct Sha1Hasher {
    state: [u32; 5],
    buffer: Vec<u8>,
    length: u64,
}

impl Sha1Hasher {
    fn new() -> Self {
        Self {
            state: [0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0],
            buffer: Vec::new(),
            length: 0,
        }
    }

    fn update(&mut self, data: &[u8]) {
        self.buffer.extend_from_slice(data);
        self.length += data.len() as u64;
        while self.buffer.len() >= 64 {
            let block: [u8; 64] = self.buffer[..64].try_into().unwrap();
            self.process_block(&block);
            self.buffer.drain(..64);
        }
    }

    fn finalize(mut self) -> String {
        // 填充
        let bit_len = self.length * 8;
        self.buffer.push(0x80);
        while self.buffer.len() % 64 != 56 {
            self.buffer.push(0);
        }
        self.buffer.extend_from_slice(&bit_len.to_be_bytes());

        // 处理剩余块（先复制数据，避免借用冲突）
        let buffer = std::mem::take(&mut self.buffer);
        for chunk in buffer.chunks(64) {
            let mut block = [0u8; 64];
            block[..chunk.len()].copy_from_slice(chunk);
            self.process_block(&block);
        }

        // 输出十六进制
        self.state
            .iter()
            .map(|w| format!("{:08x}", w))
            .collect()
    }

    fn process_block(&mut self, block: &[u8; 64]) {
        let mut w = [0u32; 80];
        for i in 0..16 {
            w[i] = u32::from_be_bytes(block[i * 4..(i + 1) * 4].try_into().unwrap());
        }
        for i in 16..80 {
            w[i] = (w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16]).rotate_left(1);
        }

        let [mut a, mut b, mut c, mut d, mut e] = self.state;

        for i in 0..80 {
            let (f, k) = match i {
                0..=19 => ((b & c) | ((!b) & d), 0x5A827999),
                20..=39 => (b ^ c ^ d, 0x6ED9EBA1),
                40..=59 => ((b & c) | (b & d) | (c & d), 0x8F1BBCDC),
                _ => (b ^ c ^ d, 0xCA62C1D6),
            };

            let temp = a
                .rotate_left(5)
                .wrapping_add(f)
                .wrapping_add(e)
                .wrapping_add(k)
                .wrapping_add(w[i]);
            e = d;
            d = c;
            c = b.rotate_left(30);
            b = a;
            a = temp;
        }

        self.state[0] = self.state[0].wrapping_add(a);
        self.state[1] = self.state[1].wrapping_add(b);
        self.state[2] = self.state[2].wrapping_add(c);
        self.state[3] = self.state[3].wrapping_add(d);
        self.state[4] = self.state[4].wrapping_add(e);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sha1_empty() {
        let hasher = Sha1Hasher::new();
        assert_eq!(
            hasher.finalize(),
            "da39a3ee5e6b4b0d3255bfef95601890afd80709"
        );
    }

    #[test]
    fn test_sha1_abc() {
        let mut hasher = Sha1Hasher::new();
        hasher.update(b"abc");
        assert_eq!(
            hasher.finalize(),
            "a9993e364706816aba3e25717850c26c9cd0d89d"
        );
    }

    #[test]
    fn test_file_checker_skip_nonexistent() {
        let item = DownloadItem::new("http://x", "/nonexistent/path/file.jar", "test");
        assert!(!FileChecker::should_skip(&item));
    }
}
