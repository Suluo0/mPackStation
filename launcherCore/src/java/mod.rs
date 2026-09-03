//! Java 运行时管理模块
//!
//! 职责：
//! - 检测系统中已安装的 Java
//! - 按 Minecraft 版本选择合适的 Java
//! - 未来支持自动下载 Java（M3）

pub mod detect;
pub mod install;
pub mod registry;

pub use detect::JavaRuntime;
pub use install::download_java;
pub use registry::{mc_version_to_java, JavaRegistry};
