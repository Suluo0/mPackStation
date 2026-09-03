//! 启动命令构建：封装 mc-launcher-core 的命令构建

use std::path::{Path, PathBuf};

use mc_launcher_core::account::Account;
use mc_launcher_core::command::builder::{LaunchCommand, LaunchOptions};
use mc_launcher_core::compatibility::CompatibilityPolicy;
use mc_launcher_core::core::version::VersionJson;
use mc_launcher_core::launcher::Launcher;

use crate::error::LauncherError;
use crate::Result;

/// 启动参数
#[derive(Debug, Clone)]
pub struct LaunchParams {
    /// 玩家用户名（离线模式）
    pub username: String,
    /// Java 可执行文件路径
    pub java_executable: PathBuf,
    /// 游戏目录（None 则使用版本隔离目录）
    pub game_directory: Option<PathBuf>,
    /// 最大内存（MB），如 4096
    pub max_memory_mb: Option<u32>,
    /// 自定义窗口大小 (width, height)
    pub resolution: Option<(u32, u32)>,
    /// 额外 JVM 参数
    pub extra_jvm_args: Vec<String>,
}

impl LaunchParams {
    /// 创建离线模式启动参数
    pub fn offline(username: impl Into<String>, java_executable: impl Into<PathBuf>) -> Self {
        Self {
            username: username.into(),
            java_executable: java_executable.into(),
            game_directory: None,
            max_memory_mb: None,
            resolution: None,
            extra_jvm_args: Vec::new(),
        }
    }
}

/// 构建 Minecraft 启动命令
///
/// 使用 mc-launcher-core 构建命令，然后注入自定义 JVM 参数（如 -Xmx）。
pub fn build_command(
    version: &VersionJson,
    minecraft_dir: &Path,
    params: &LaunchParams,
) -> Result<LaunchCommand> {
    let launcher = Launcher::new(minecraft_dir);

    let options = LaunchOptions {
        account: Account::offline(&params.username),
        java_executable: Some(params.java_executable.clone()),
        game_directory: params.game_directory.clone(),
        launcher_name: "mPackLauncher".to_string(),
        launcher_version: env!("CARGO_PKG_VERSION").to_string(),
        custom_resolution: params.resolution,
        compatibility: CompatibilityPolicy::Auto,
        ..Default::default()
    };

    let mut command = launcher
        .build_launch_command_from_version(version, options)
        .map_err(|e| LauncherError::Internal(format!("构建启动命令失败: {}", e)))?;

    // 注入 -Xmx 参数（在 main class 之前的 JVM 参数中）
    if let Some(max_mem) = params.max_memory_mb {
        // 找到 main class 的位置，在它之前插入 -Xmx
        // mc-launcher-core 的 args 顺序：JVM args + main class + game args
        let main_class = version.main_class.clone().unwrap_or_else(|| {
            "net.minecraft.client.main.Main".to_string()
        });
        if let Some(pos) = command.args.iter().position(|a| a == &main_class) {
            command.args.insert(pos, format!("-Xmx{}m", max_mem));
        } else {
            // 找不到 main class，插到最前面
            command.args.insert(0, format!("-Xmx{}m", max_mem));
        }
    }

    // 注入额外 JVM 参数
    if !params.extra_jvm_args.is_empty() {
        let main_class = version.main_class.clone().unwrap_or_else(|| {
            "net.minecraft.client.main.Main".to_string()
        });
        if let Some(pos) = command.args.iter().position(|a| a == &main_class) {
            for (i, arg) in params.extra_jvm_args.iter().enumerate() {
                command.args.insert(pos + i, arg.clone());
            }
        }
    }

    Ok(command)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_launch_params_offline() {
        let params = LaunchParams::offline("Steve", "/usr/bin/java");
        assert_eq!(params.username, "Steve");
        assert_eq!(params.java_executable, PathBuf::from("/usr/bin/java"));
    }
}
