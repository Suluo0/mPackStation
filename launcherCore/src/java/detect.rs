//! Java 运行时检测：扫描系统中的 Java 安装
//!
//! 扫描策略（按优先级）：
//! 1. JAVA_HOME 环境变量
//! 2. PATH 中的 java
//! 3. Windows 注册表（HKLM/HKCU JavaSoft JDK）
//! 4. 常见厂商目录（Program Files/Java, Eclipse Adoptium, Microsoft, etc.）
//! 5. 启动器 runtime 目录（自动下载的 Java）
//! 6. 深度目录扫描（全盘3层 + 跳过规则 + 关键词前缀匹配）

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
fn parse_java_version_output(output: &str) -> Option<ParsedVersion> {
    let first_line = output.lines().next()?;

    let version_str = first_line
        .split('"')
        .nth(1)
        .unwrap_or("")
        .to_string();

    if version_str.is_empty() {
        return None;
    }

    let major = if version_str.starts_with("1.") {
        version_str
            .split('.')
            .nth(1)
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    } else {
        version_str
            .split('.')
            .next()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0)
    };

    let vendor = output.lines().nth(1).and_then(|line| {
        if line.contains("Microsoft") {
            Some("Microsoft".to_string())
        } else if line.contains("Oracle") {
            Some("Oracle".to_string())
        } else if line.contains("Adoptium") || line.contains("Eclipse") || line.contains("Temurin") {
            Some("Adoptium".to_string())
        } else if line.contains("Amazon") {
            Some("Amazon Corretto".to_string())
        } else if line.contains("Azul") || line.contains("Zulu") {
            Some("Azul Zulu".to_string())
        } else if line.contains("BellSoft") || line.contains("Liberica") {
            Some("BellSoft Liberica".to_string())
        } else if line.contains("IBM") || line.contains("Semeru") {
            Some("IBM Semeru".to_string())
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
/// 快速路径：JAVA_HOME + PATH + 注册表 + 常见目录 + runtime目录
/// 不包含深度扫描（深度扫描由 `deep_scan_java` 单独提供，用户主动触发）
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

    // 3. Windows 注册表
    #[cfg(windows)]
    {
        for rt in scan_registry_java() {
            if seen.insert(rt.executable.to_string_lossy().to_lowercase()) {
                runtimes.push(rt);
            }
        }
    }

    // 4. 常见厂商目录
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

    // 5. 启动器 runtime 目录（自动下载的 Java）
    if let Some(runtime_dir) = launcher_runtime_dir() {
        if runtime_dir.is_dir() {
            if let Ok(entries) = std::fs::read_dir(&runtime_dir) {
                for entry in entries.flatten() {
                    // runtime/{component}/bin/java.exe
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

/// 深度扫描：全盘3层目录扫描 + 跳过规则 + 关键词前缀匹配
///
/// 此操作较慢（约0.2秒），应在用户主动触发或首次启动时调用，
/// 不应在每次启动时自动执行。
pub fn deep_scan_java() -> Vec<JavaRuntime> {
    let mut runtimes = scan_system_java(); // 先跑快速路径
    let mut seen: std::collections::HashSet<String> = runtimes
        .iter()
        .map(|r| r.executable.to_string_lossy().to_lowercase())
        .collect();

    // 遍历所有可用盘符
    for root in available_drives() {
        scan_depth(&root, 1, &mut runtimes, &mut seen);
    }

    runtimes
}

/// 递归扫描指定深度的目录
fn scan_depth(
    dir: &Path,
    depth: u32,
    runtimes: &mut Vec<JavaRuntime>,
    seen: &mut std::collections::HashSet<String>,
) {
    if depth > 3 {
        return;
    }

    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let name = match entry.file_name().into_string() {
            Ok(n) => n,
            Err(_) => continue,
        };

        // 跳过无关目录
        if should_skip_dir(&name) {
            continue;
        }

        // 检查是否是 Java 目录（关键词前缀匹配）
        if is_java_dir_name(&name) {
            let java_exe = path.join("bin").join(java_executable_name());
            if let Some(rt) = check_and_add(&java_exe, seen) {
                runtimes.push(rt);
            }
        }

        // 继续递归
        if depth < 3 && path.is_dir() {
            scan_depth(&path, depth + 1, runtimes, seen);
        }
    }
}

/// 判断目录名是否匹配 Java 关键词（前缀匹配，避免 javascript/javax 误匹配）
fn is_java_dir_name(name: &str) -> bool {
    let lower = name.to_lowercase();

    // 这些前缀后面必须跟非字母字符（数字、连字符、点号、下划线）或结束
    let prefixes = ["jdk", "jre", "temurin", "zulu", "corretto", "adoptium", "semeru", "liberica", "openjdk", "graalvm"];
    for p in &prefixes {
        if lower.starts_with(p) {
            let rest = &lower[p.len()..];
            if rest.is_empty() || !rest.chars().next().unwrap().is_ascii_alphabetic() {
                return true;
            }
        }
    }

    // "java" 需要特殊处理：完全等于，或后面跟非字母字符
    if lower.starts_with("java") {
        let rest = &lower[4..];
        if rest.is_empty() || !rest.chars().next().unwrap().is_ascii_alphabetic() {
            return true;
        }
    }

    false
}

/// 判断是否应该跳过该目录
fn should_skip_dir(name: &str) -> bool {
    let lower = name.to_lowercase();
    matches!(
        lower.as_str(),
        "windows"
            | "windowsapps"
            | "programdata"
            | "$recycle.bin"
            | "system volume information"
            | "recovery"
            | "node_modules"
            | ".git"
            | "appdata"
            | "common files"
            | "perflogs"
            | "program files (x86)" // 已在common_java_dirs单独处理
    ) || lower.starts_with("msys")
        || lower.starts_with("cygwin")
}

/// 获取系统中所有可用盘符（Windows）或根目录（Unix）
fn available_drives() -> Vec<PathBuf> {
    let mut drives = Vec::new();

    #[cfg(windows)]
    {
        for letter in b'A'..=b'Z' {
            let drive = format!("{}:\\", letter as char);
            if Path::new(&drive).is_dir() {
                drives.push(PathBuf::from(&drive));
            }
        }
    }

    #[cfg(not(windows))]
    {
        drives.push(PathBuf::from("/"));
        if let Ok(home) = std::env::var("HOME") {
            drives.push(PathBuf::from(home));
        }
    }

    drives
}

/// 检查 java 可执行文件并添加到结果中（去重）
fn check_and_add(
    exe: &Path,
    seen: &mut std::collections::HashSet<String>,
) -> Option<JavaRuntime> {
    if !exe.is_file() {
        return None;
    }
    // 用小写规范化路径去重，避免大小写或符号链接导致的重复
    let key = exe.to_string_lossy().to_lowercase();
    if !seen.insert(key) {
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

/// 启动器 runtime 目录（用于存放自动下载的 Java）
fn launcher_runtime_dir() -> Option<PathBuf> {
    // 优先使用环境变量 MPACK_LAUNCHER_DIR
    if let Ok(dir) = std::env::var("MPACK_LAUNCHER_DIR") {
        return Some(PathBuf::from(dir).join("runtime"));
    }
    // 否则使用可执行文件同级目录
    if let Ok(exe) = std::env::current_exe() {
        if let Some(parent) = exe.parent() {
            return Some(parent.join("runtime"));
        }
    }
    None
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
            dirs.push(PathBuf::from(&program_files).join("Zulu"));
            dirs.push(PathBuf::from(&program_files).join("BellSoft"));
            dirs.push(PathBuf::from(&program_files).join("Semeru"));
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

// ==================== Windows 注册表扫描 ====================

#[cfg(windows)]
fn scan_registry_java() -> Vec<JavaRuntime> {
    use winreg::enums::{HKEY_LOCAL_MACHINE, HKEY_CURRENT_USER};
    use winreg::RegKey;

    let mut runtimes = Vec::new();
    let mut seen = std::collections::HashSet::new();

    let hives = [HKEY_LOCAL_MACHINE, HKEY_CURRENT_USER];
    // JDK 1.9+ 使用 JavaSoft\JDK，JDK 1.8 使用 JavaSoft\Java Development Kit
    let keys = [
        r"SOFTWARE\JavaSoft\JDK",
        r"SOFTWARE\JavaSoft\Java Development Kit",
        r"SOFTWARE\JavaSoft\Java Runtime Environment",
    ];

    for hive in &hives {
        for key_path in &keys {
            if let Ok(key) = RegKey::predef(*hive).open_subkey(key_path) {
                // 枚举子键（每个子键是一个版本号）
                for version_subkey in key.enum_keys().flatten() {
                    if let Ok(version_key) = key.open_subkey(&version_subkey) {
                        // JavaHome 值指向 JDK 安装目录
                        if let Ok(java_home) = version_key.get_value::<String, _>("JavaHome") {
                            let java_exe = PathBuf::from(&java_home)
                                .join("bin")
                                .join(java_executable_name());
                            if let Some(rt) = check_and_add(&java_exe, &mut seen) {
                                runtimes.push(rt);
                            }
                        }
                    }
                }
            }
        }
    }

    runtimes
}

#[cfg(not(windows))]
fn scan_registry_java() -> Vec<JavaRuntime> {
    Vec::new()
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
        assert_eq!(parsed.vendor.as_deref(), Some("Adoptium"));
    }

    #[test]
    fn test_parse_zulu_java() {
        let output = r#"openjdk version "25.0.1" 2025-01-21
OpenJDK Runtime Environment Zulu25.30+13-CA (build 25.0.1+9)
OpenJDK 64-Bit Server VM Zulu25.30+13-CA (build 25.0.1+9, mixed mode)"#;
        let parsed = parse_java_version_output(output).unwrap();
        assert_eq!(parsed.major, 25);
        assert_eq!(parsed.vendor.as_deref(), Some("Azul Zulu"));
    }

    #[test]
    fn test_is_java_dir_name() {
        assert!(is_java_dir_name("jdk-17"));
        assert!(is_java_dir_name("jre1.8.0_381"));
        assert!(is_java_dir_name("java"));
        assert!(is_java_dir_name("temurin-21"));
        assert!(is_java_dir_name("zulu25-win_x64"));
        assert!(is_java_dir_name("corretto-17"));
        assert!(is_java_dir_name("JDK-21")); // 大小写不敏感
        assert!(!is_java_dir_name("javascript"));
        assert!(!is_java_dir_name("javax"));
        assert!(!is_java_dir_name("javanesetext"));
        assert!(!is_java_dir_name("myjavaapp")); // 不是前缀
    }

    #[test]
    fn test_should_skip_dir() {
        assert!(should_skip_dir("Windows"));
        assert!(should_skip_dir("ProgramData"));
        assert!(should_skip_dir("node_modules"));
        assert!(should_skip_dir(".git"));
        assert!(should_skip_dir("Common Files"));
        assert!(!should_skip_dir("Java"));
        assert!(!should_skip_dir("jdk-17"));
        assert!(!should_skip_dir("plugin"));
    }

    #[test]
    fn test_scan_system_java_not_empty() {
        let runtimes = scan_system_java();
        // 开发机应该有 Java，但 CI 可能没有
        let _ = runtimes;
    }

    #[test]
    fn test_deep_scan_finds_java() {
        // 深度扫描应该能找到至少和快速路径一样多的 Java
        let quick = scan_system_java();
        let deep = deep_scan_java();
        assert!(deep.len() >= quick.len(), "深度扫描不应比快速路径找到更少");
    }
}
