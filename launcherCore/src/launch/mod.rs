//! 启动模块：命令构建 + 进程管理

pub mod command;
pub mod process;

pub use command::{build_command, LaunchParams};
pub use process::{spawn_detached, GameProcess};
