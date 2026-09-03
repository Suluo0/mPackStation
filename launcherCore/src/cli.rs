use clap::{Parser, Subcommand};
use std::path::PathBuf;

/// mPackLauncher - Minecraft 启动器内核
#[derive(Parser, Debug)]
#[command(name = "mpack-launcher", version, about, long_about = None)]
pub struct Cli {
    /// 日志级别（error/warn/info/debug/trace）
    #[arg(long, env = "MPACK_LOG", default_value = "info")]
    pub log_level: String,

    #[command(subcommand)]
    pub command: Command,
}

#[derive(Subcommand, Debug)]
pub enum Command {
    /// 安装 Minecraft 版本
    Install(InstallArgs),

    /// 启动 Minecraft
    Launch(LaunchArgs),

    /// 认证管理
    Auth {
        #[command(subcommand)]
        action: AuthCommand,
    },

    /// Java 运行时管理
    Java {
        #[command(subcommand)]
        action: JavaCommand,
    },

    /// 列出已安装版本
    List {
        /// Minecraft 实例目录
        #[arg(long, short = 'd')]
        dir: PathBuf,
    },

    /// 查看内核版本
    Version,
}

#[derive(Parser, Debug)]
pub struct InstallArgs {
    /// Minecraft 版本（如 1.20.1）
    #[arg(long)]
    pub mc: String,

    /// 加载器类型（vanilla/fabric/forge/neoforge/quilt）
    #[arg(long, default_value = "vanilla")]
    pub loader: String,

    /// 加载器版本（如 0.16.5，latest 表示最新）
    #[arg(long, default_value = "latest")]
    pub loader_version: String,

    /// Minecraft 实例目录
    #[arg(long, short = 'd')]
    pub dir: PathBuf,

    /// 镜像源（auto/mojang/bmclapi）
    #[arg(long, default_value = "auto")]
    pub mirror: String,

    /// Java 可执行文件路径（留空自动检测/下载）
    #[arg(long)]
    pub java: Option<PathBuf>,
}

#[derive(Parser, Debug)]
pub struct LaunchArgs {
    /// 版本 ID（如 1.20.1 或 1.20.1-fabric-0.16.5）
    #[arg(long)]
    pub version: String,

    /// Minecraft 实例目录
    #[arg(long, short = 'd')]
    pub dir: PathBuf,

    /// 用户名（离线模式）
    #[arg(long)]
    pub username: Option<String>,

    /// 账号类型（offline/microsoft）
    #[arg(long, default_value = "offline")]
    pub account_type: String,

    /// 最大内存（如 2G、4G）
    #[arg(long)]
    pub xmx: Option<String>,

    /// 最小内存（如 512M）
    #[arg(long)]
    pub xms: Option<String>,

    /// 额外 JVM 参数
    #[arg(long, value_delimiter = ' ')]
    pub jvm_args: Vec<String>,

    /// 额外游戏参数
    #[arg(long, value_delimiter = ' ')]
    pub game_args: Vec<String>,

    /// 等待游戏退出（默认 detach 模式）
    #[arg(long)]
    pub wait: bool,

    /// 游戏日志输出文件
    #[arg(long)]
    pub log_file: Option<PathBuf>,

    /// Java 可执行文件路径（留空自动检测）
    #[arg(long)]
    pub java: Option<PathBuf>,
}

#[derive(Subcommand, Debug)]
pub enum AuthCommand {
    /// 登录
    Login {
        /// 登录方式（offline/microsoft）
        #[arg(long, default_value = "offline")]
        provider: String,

        /// 用户名（offline 模式必填）
        #[arg(long)]
        username: Option<String>,
    },

    /// 查看当前登录状态
    Status,

    /// 登出
    Logout,
}

#[derive(Subcommand, Debug)]
pub enum JavaCommand {
    /// 列出检测到的 Java 运行时
    List {
        /// 深度扫描（全盘3层目录，较慢但更全面）
        #[arg(long)]
        deep: bool,
    },

    /// 下载指定版本的 Java
    Install {
        /// Java 大版本（8/17/21）
        #[arg(long)]
        version: u32,
        /// 镜像源（auto/mojang/bmclapi）
        #[arg(long, default_value = "auto")]
        mirror: String,
    },
}
