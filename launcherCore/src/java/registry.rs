//! Java 运行时注册表：管理已检测到的 Java 版本，按版本查询

use std::collections::HashMap;

use super::detect::{deep_scan_java, scan_system_java, JavaRuntime};
use crate::error::LauncherError;
use crate::Result;

/// Java 运行时管理器
pub struct JavaRegistry {
    /// 已检测到的 Java 运行时列表
    runtimes: Vec<JavaRuntime>,
}

impl JavaRegistry {
    /// 创建空的注册表
    pub fn new() -> Self {
        Self {
            runtimes: Vec::new(),
        }
    }

    /// 扫描系统并创建注册表
    pub fn detect() -> Self {
        Self {
            runtimes: scan_system_java(),
        }
    }

    /// 深度扫描系统并创建注册表（全盘3层目录扫描，较慢但更全面）
    pub fn detect_deep() -> Self {
        Self {
            runtimes: deep_scan_java(),
        }
    }

    /// 获取所有已检测到的 Java 运行时
    pub fn list(&self) -> &[JavaRuntime] {
        &self.runtimes
    }

    /// 查找满足最低主版本要求的 Java
    ///
    /// 策略：选择 >= required_major 的最低版本（兼容性最好）。
    /// 如果没有 >= required 的，返回最高版本（可能不兼容）。
    pub fn find(&self, required_major: u32) -> Option<&JavaRuntime> {
        let mut candidates: Vec<&JavaRuntime> = self
            .runtimes
            .iter()
            .filter(|rt| rt.major_version >= required_major)
            .collect();

        if candidates.is_empty() {
            // 没有满足要求的，返回最高版本
            return self.runtimes.iter().max_by_key(|rt| rt.major_version);
        }

        // 选择满足要求的最低版本
        candidates.sort_by_key(|rt| rt.major_version);
        candidates.first().copied()
    }

    /// 查找精确主版本的 Java
    pub fn find_exact(&self, major: u32) -> Option<&JavaRuntime> {
        self.runtimes.iter().find(|rt| rt.major_version == major)
    }

    /// 按主版本分组
    pub fn by_major_version(&self) -> HashMap<u32, Vec<&JavaRuntime>> {
        let mut map: HashMap<u32, Vec<&JavaRuntime>> = HashMap::new();
        for rt in &self.runtimes {
            map.entry(rt.major_version).or_default().push(rt);
        }
        map
    }
}

impl Default for JavaRegistry {
    fn default() -> Self {
        Self::new()
    }
}

/// 根据 Minecraft 版本确定所需的 Java 主版本
///
/// 映射规则：
/// - 1.16.5 及更早 → Java 8
/// - 1.17 ~ 1.20.4 → Java 17
/// - 1.20.5+ → Java 21
pub fn mc_version_to_java(mc_version: &str) -> Result<u32> {
    let parts: Vec<&str> = mc_version.split('.').collect();
    if parts.len() < 2 {
        return Err(LauncherError::InvalidArgument(format!(
            "无效的 Minecraft 版本: {}",
            mc_version
        )));
    }

    let major: u32 = parts[0].parse().map_err(|_| {
        LauncherError::InvalidArgument(format!("无效的版本号: {}", mc_version))
    })?;
    let minor: u32 = parts[1].parse().map_err(|_| {
        LauncherError::InvalidArgument(format!("无效的版本号: {}", mc_version))
    })?;

    // 只处理 1.x 版本
    if major != 1 {
        return Ok(21); // 未来版本默认 Java 21
    }

    // 1.21+ → Java 21
    if minor >= 21 {
        return Ok(21);
    }

    // 1.20.5+ → Java 21（必须先于 minor <= 20 判断）
    if minor == 20 {
        let patch: u32 = parts.get(2).and_then(|s| s.parse().ok()).unwrap_or(0);
        if patch >= 5 {
            return Ok(21);
        }
        return Ok(17);
    }

    // 1.17 ~ 1.19 → Java 17
    if minor >= 17 {
        return Ok(17);
    }

    // 1.16.5 及更早 → Java 8
    Ok(8)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mc_version_to_java_1_16_5() {
        assert_eq!(mc_version_to_java("1.16.5").unwrap(), 8);
    }

    #[test]
    fn test_mc_version_to_java_1_12_2() {
        assert_eq!(mc_version_to_java("1.12.2").unwrap(), 8);
    }

    #[test]
    fn test_mc_version_to_java_1_17() {
        assert_eq!(mc_version_to_java("1.17").unwrap(), 17);
    }

    #[test]
    fn test_mc_version_to_java_1_18_2() {
        assert_eq!(mc_version_to_java("1.18.2").unwrap(), 17);
    }

    #[test]
    fn test_mc_version_to_java_1_20_1() {
        assert_eq!(mc_version_to_java("1.20.1").unwrap(), 17);
    }

    #[test]
    fn test_mc_version_to_java_1_20_4() {
        assert_eq!(mc_version_to_java("1.20.4").unwrap(), 17);
    }

    #[test]
    fn test_mc_version_to_java_1_20_5() {
        assert_eq!(mc_version_to_java("1.20.5").unwrap(), 21);
    }

    #[test]
    fn test_mc_version_to_java_1_21() {
        assert_eq!(mc_version_to_java("1.21").unwrap(), 21);
    }

    #[test]
    fn test_mc_version_to_java_1_21_1() {
        assert_eq!(mc_version_to_java("1.21.1").unwrap(), 21);
    }

    #[test]
    fn test_registry_find() {
        let mut registry = JavaRegistry::new();
        // 手动添加测试数据
        registry.runtimes = vec![
            JavaRuntime {
                executable: "/usr/lib/jvm/java-8/bin/java".into(),
                major_version: 8,
                version_string: "1.8.0_381".into(),
                vendor: None,
            },
            JavaRuntime {
                executable: "/usr/lib/jvm/java-17/bin/java".into(),
                major_version: 17,
                version_string: "17.0.8".into(),
                vendor: None,
            },
            JavaRuntime {
                executable: "/usr/lib/jvm/java-21/bin/java".into(),
                major_version: 21,
                version_string: "21.0.1".into(),
                vendor: None,
            },
        ];

        // 找 Java 17：应该返回 17（满足要求的最低版本）
        let rt = registry.find(17).unwrap();
        assert_eq!(rt.major_version, 17);

        // 找 Java 16：应该返回 17（>=16 的最低版本）
        let rt = registry.find(16).unwrap();
        assert_eq!(rt.major_version, 17);

        // 找 Java 25：没有满足的，返回最高版本 21
        let rt = registry.find(25).unwrap();
        assert_eq!(rt.major_version, 21);
    }
}
