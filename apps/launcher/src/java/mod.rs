//! Java 运行时模块
//!
//! 负责：系统 Java 检测、自动下载、多版本管理
//! M1 里程碑实现

pub mod detect;
pub mod registry;
pub mod install;
