/// 启动器统一错误类型
#[derive(Debug, thiserror::Error)]
pub enum LauncherError {
    // --- 参数错误 (1x) ---
    #[error("参数错误: {0}")]
    InvalidArgument(String),

    #[error("版本不存在: {0}")]
    VersionNotFound(String),

    // --- 网络错误 (2x) ---
    #[error("网络错误: {0}")]
    Network(#[from] reqwest::Error),

    #[error("下载失败: {url} - {message}")]
    DownloadFailed { url: String, message: String },

    #[error("校验失败: {file} - 期望 {expected}, 实际 {actual}")]
    ChecksumMismatch {
        file: String,
        expected: String,
        actual: String,
    },

    // --- 文件系统错误 (3x) ---
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),

    #[error("目录已被锁定: {0}（另一个进程正在操作此目录）")]
    DirectoryLocked(String),

    #[error("磁盘空间不足: 需要 {needed} MB, 可用 {available} MB")]
    InsufficientDiskSpace { needed: u64, available: u64 },

    #[error("路径不安全: {0}（可能包含目录穿越）")]
    UnsafePath(String),

    // --- Java 错误 (4x) ---
    #[error("未找到匹配的 Java 运行时（需要 Java {required}+）")]
    JavaNotFound { required: u32 },

    #[error("Java 版本不兼容: 找到 {found}, 需要 {required}+")]
    JavaVersionIncompatible { found: String, required: u32 },

    // --- 认证错误 (5x) ---
    #[error("未登录，请先执行 auth login")]
    NotLoggedIn,

    #[error("认证失败: {0}")]
    AuthFailed(String),

    #[error("Token 已过期且刷新失败")]
    TokenRefreshFailed,

    // --- 加载器错误 (6x) ---
    #[error("加载器 {loader} 不支持 Minecraft {mc_version}")]
    LoaderIncompatible { loader: String, mc_version: String },

    #[error("加载器安装失败: {0}")]
    LoaderInstallFailed(String),

    // --- 启动错误 (7x) ---
    #[error("游戏进程启动失败: {0}")]
    LaunchFailed(String),

    #[error("游戏异常退出（退出码 {exit_code}）")]
    GameCrashed { exit_code: i32 },

    // --- 内部错误 (9x) ---
    #[error("内部错误: {0}")]
    Internal(String),
}

impl LauncherError {
    /// 映射到退出码
    pub fn exit_code(&self) -> i32 {
        match self {
            Self::InvalidArgument(_) => 10,
            Self::VersionNotFound(_) => 11,
            Self::Network(_) => 20,
            Self::DownloadFailed { .. } => 21,
            Self::ChecksumMismatch { .. } => 22,
            Self::Io(_) => 30,
            Self::DirectoryLocked(_) => 31,
            Self::InsufficientDiskSpace { .. } => 32,
            Self::UnsafePath(_) => 33,
            Self::JavaNotFound { .. } => 40,
            Self::JavaVersionIncompatible { .. } => 41,
            Self::NotLoggedIn => 50,
            Self::AuthFailed(_) => 51,
            Self::TokenRefreshFailed => 52,
            Self::LoaderIncompatible { .. } => 60,
            Self::LoaderInstallFailed(_) => 61,
            Self::LaunchFailed(_) => 71,
            Self::GameCrashed { .. } => 72,
            Self::Internal(_) => 99,
        }
    }

    /// 用户可操作的建议
    pub fn suggestion(&self) -> Option<&'static str> {
        match self {
            Self::Network(_) | Self::DownloadFailed { .. } => {
                Some("网络不稳定，可尝试 --mirror bmclapi 使用国内镜像")
            }
            Self::ChecksumMismatch { .. } => {
                Some("文件校验失败，可能是下载不完整，删除目标目录后重试")
            }
            Self::DirectoryLocked(_) => {
                Some("另一个启动器进程正在操作此目录，请等待其完成或关闭后重试")
            }
            Self::InsufficientDiskSpace { .. } => Some("磁盘空间不足，请清理磁盘空间后重试"),
            Self::JavaNotFound { .. } => {
                Some("未找到 Java 运行时，请安装 Java 或使用 --java 指定路径")
            }
            Self::JavaVersionIncompatible { .. } => {
                Some("Java 版本不兼容，请安装对应版本的 Java 或使用 --java 指定")
            }
            Self::NotLoggedIn => Some("请先执行 mpack-launcher auth login 登录"),
            Self::TokenRefreshFailed => Some("Token 刷新失败，请重新登录"),
            Self::LoaderIncompatible { .. } => {
                Some("该加载器版本不支持此 Minecraft 版本，请检查版本组合")
            }
            Self::GameCrashed { .. } => Some("游戏崩溃，可能是内存不足，尝试增大 --xmx"),
            _ => None,
        }
    }

    /// 错误类型标识（用于 JSON 输出的 error 字段）
    pub fn error_type(&self) -> &'static str {
        match self {
            Self::InvalidArgument(_) => "InvalidArgument",
            Self::VersionNotFound(_) => "VersionNotFound",
            Self::Network(_) => "Network",
            Self::DownloadFailed { .. } => "DownloadFailed",
            Self::ChecksumMismatch { .. } => "ChecksumMismatch",
            Self::Io(_) => "Io",
            Self::DirectoryLocked(_) => "DirectoryLocked",
            Self::InsufficientDiskSpace { .. } => "InsufficientDiskSpace",
            Self::UnsafePath(_) => "UnsafePath",
            Self::JavaNotFound { .. } => "JavaNotFound",
            Self::JavaVersionIncompatible { .. } => "JavaVersionIncompatible",
            Self::NotLoggedIn => "NotLoggedIn",
            Self::AuthFailed(_) => "AuthFailed",
            Self::TokenRefreshFailed => "TokenRefreshFailed",
            Self::LoaderIncompatible { .. } => "LoaderIncompatible",
            Self::LoaderInstallFailed(_) => "LoaderInstallFailed",
            Self::LaunchFailed(_) => "LaunchFailed",
            Self::GameCrashed { .. } => "GameCrashed",
            Self::Internal(_) => "Internal",
        }
    }
}

/// 便捷类型别名
pub type Result<T> = std::result::Result<T, LauncherError>;
