//! mPackLauncher - Minecraft 启动器内核
//!
//! 为 mPackStation 提供 Minecraft 版本安装、启动、Java 管理、认证等能力。
//! 通过 CLI 被 Go 后端调用，stdout 输出 JSON Lines 协议事件，stderr 输出日志。

pub mod cli;
pub mod error;
pub mod install;
pub mod lock;
pub mod platform;
pub mod protocol;

pub mod download;
pub mod launch;
pub mod java;
pub mod auth;
pub mod loader;
