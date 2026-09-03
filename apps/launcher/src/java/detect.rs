//! Java 运行时检测：扫描系统中的 Java 安装

use std::path::{Path, PathBuf};
use std::process::Command;

/// 已检测到的 Java 运行时
#[derive(Debug, Clone)]
pub struct JavaRuntime {
    /// java 可执行文件路径
    pub executable: PathBuf,
    /// 主版本号（如 8, 17, 21）
    pub major_version: u32,
    /// 完整版本字符串（如 "17.0.8"）
    pub version_string: String,
    /// 供应商（如 "Microsoft", "Oracle", "Adoptium"）
    pub vendor: Option<String>,
}

impl JavaRuntime {
    /// 检测指定 java 可执行文件的版本信息
    pub fn detect_from(executable: &Path) -> Option<Self> {
        let output = Command::new(executable)
            .arg("-version")
            .output()
            .ok()?;

        // java -version 输出到 stderr
        let stderr = String::from_utf8_lossy(&output.stderr);
        let version_info = parse_java_version_output(&stderr)?;

        Some(Self {
            executable: executable.to_path_buf(),
            major_version: version_info.major,
            version_string: version_info.full,
            vendor: version_info.vendor,
        })
    }
}

/// 解析后的 Java 版本信息
struct ParsedVersion {
    major: u32,
    full: String,
    vendor: Option<String>,
}

/// 解析 `java -version` 输出
///
/// 典型输出：
/// ```text
/// openjdk version "17.0.8" 2023-07-18
/// OpenJDK Runtime Environment Microsoft-8192769 (build 17.0.8+7-LTS)
/// OpenJDK 64-Bit Server VM Microsoft-8192769 (build 17.0.8+7-LTS, mixed mode)
/// ```
fn parse_java_version_output(output: &str) -> Option<ParsedVersion> {
    let first_line = output.lines().next()?;

    // 提取版本号（引号内）
    let version_str = first_line
        .split('"')
        .nth(1)
            .unwrap_or("")
            .to_string();

    if version_str.is_empty() {
        return None;
    }

    // 解析主版本号
    let major = if version_str.starts_with("1.") {
        // Java 8 及之前: 1.8.0_381
        version_str
            .split('.')
            .nth(1)
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    } else {
        // Java 9+: 17.0.8
        version_str
            .split('.')
            .next()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    };

    // 提取供应商（第二行）
    let vendor = output
        .lines()
        .nth(1)
        .and_then(|line| {
            // "OpenJDK Runtime Environment Microsoft-8192769 ..."
            if line.contains("Microsoft") {
                Some("Microsoft".to_string())
            } else if line.contains("Oracle") {
                Some("Oracle".to_string())
            } else if line.contains("Adoptium") || line.contains("Eclipse") {
                Some("Adoptium".to_string())
            } else if line.contains("Amazon") {
                Some("Amazon Corretto".to_string())
            } else if line.contains("Azul") {
                Some("Azul Zulu".to_string())
            } else {
                None
            }
        });

    Some(ParsedVersion {
        major,
        full: version_str,
        vendor,
    })
}

/// 扫描系统中所有可用的 Java 运行时
///
/// 扫描顺序：
/// 1. JAVA_HOME 环境变量
/// 2. PATH 中的 java
/// 3. 常见安装路径（Windows: Program Files/Java, macOS: /Library/Java, Linux: /usr/lib/jvm）
pub fn scan_system_java() -> Vec<JavaRuntime> {
    let mut runtimes = Vec::new();
    let mut seen = std::collections::HashSet::new();

    // 1. JAVA_HOME
    if let Ok(java_home) = std::env::var("JAVA_HOME") {
        let java_exe = PathBuf::from(&java_home).join("bin").join(java_executable_name());
        if let Some(rt) = check_and_add(&java_exe, &mut seen) {
            runtimes.push(rt);
        }
    }

    // 2. PATH 中的 java
    if let Ok(path) = std::env::var("PATH") {
        for dir in std::env::split_paths(&path) {
            let java_exe = dir.join(java_executable_name());
            if let Some(rt) = check_and_add(&java_exe, &mut seen) {
                runtimes.push(rt);
            }
        }
    }

    // 3. 常见安装路径
    for dir in common_java_dirs() {
        if dir.is_dir() {
            if let Ok(entries) = std::fs::read_dir(&dir) {
                for entry in entries.flatten() {
                    let java_exe = entry.path().join("bin").join(java_executable_name());
                    if let Some(rt) = check_and_add(&java_exe, &mut seen) {
                        runtimes.push(rt);
                    }
                }
            }
        }
    }

    runtimes
}

/// 检查 java 可执行文件并添加到结果中（去重）
fn check_and_add(
    exe: &Path,
    seen: &mut std::collections::HashSet<PathBuf>,
) -> Option<JavaRuntime> {
    if !exe.is_file() {
        return None;
    }
    let canonical = exe.canonicalize().unwrap_or_else(|_| exe.to_path_buf());
    if !seen.insert(canonical) {
        return None;
    }
    JavaRuntime::detect_from(exe)
}

/// 当前平台的 java 可执行文件名
fn java_executable_name() -> &'static str {
    if cfg!(windows) {
        "java.exe"
    } else {
        "java"
    }
}

/// 当前平台的常见 Java 安装目录
fn common_java_dirs() -> Vec<PathBuf> {
    let mut dirs = Vec::new();

    if cfg!(windows) {
        if let Ok(program_files) = std::env::var("ProgramFiles") {
            dirs.push(PathBuf::from(&program_files).join("Java"));
            dirs.push(PathBuf::from(&program_files).join("Eclipse Adoptium"));
            dirs.push(PathBuf::from(&program_files).join("Microsoft"));
            dirs.push(PathBuf::from(&program_files).join("Amazon Corretto"));
        }
        if let Ok(program_files_x86) = std::env::var("ProgramFiles(x86)") {
            dirs.push(PathBuf::from(&program_files_x86).join("Java"));
        }
    } else if cfg!(target_os = "macos") {
        dirs.push(PathBuf::from("/Library/Java/JavaVirtualMachines"));
        dirs.push(PathBuf::from("/System/Library/Java/JavaVirtualMachines"));
    } else {
        dirs.push(PathBuf::from("/usr/lib/jvm"));
        dirs.push(PathBuf::from("/opt/java"));
        dirs.push(PathBuf::from("/usr/local/java"));
    }

    dirs
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_java_17() {
        let output = r#"openjdk version "17.0.8" 2023-07-18
OpenJDK Runtime Environment Microsoft-8192769 (build 17.0.8+7-LTS)
OpenJDK 64-Bit Server VM Microsoft-8192769 (build 17.0.8+7-LTS, mixed mode)"#;
        let parsed = parse_java_version_output(output).unwrap();
        assert_eq!(parsed.major, 17);
        assert_eq!(parsed.full, "17.0.8");
        assert_eq!(parsed.vendor.as_deref(), Some("Microsoft"));
    }

    #[test]
    fn test_parse_java_8() {
        let output = r#"java version "1.8.0_381"
Java(TM) SE Runtime Environment (build 1.8.0_381-b09)
Java HotSpot(TM) 64-Bit Server VM (build 25.381-b09, mixed mode)"#;
        let parsed = parse_java_version_output(output).unwrap();
        assert_eq!(parsed.major, 8);
        assert_eq!(parsed.full, "1.8.0_381");
    }

    #[test]
    fn test_parse_java_21() {
        let output = r#"openjdk version "21.0.1" 2023-10-17 LTS
OpenJDK Runtime Environment Temurin-21.0.1+12 (build 21.0.1+12-LTS)
OpenJDK 64-Bit Server VM Temurin-21.0.1+12 (build 21.0.1+12-LTS, mixed mode)"#;
        let parsed = parse_java_version_output(output).unwrap();
        assert_eq!(parsed.major, 21);
    }

    #[test]
    fn test_scan_system_java_not_empty() {
        // 系统应该至少有一个 Java（CI 环境可能没有，所以只验证不 panic）
        let runtimes = scan_system_java();
        // 不断言数量，因为测试环境不确定
        let _ = runtimes;
    }
}
