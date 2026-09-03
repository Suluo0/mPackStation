//! 下载能力模块
//!
//! 负责：URL 重写、并发下载、断点续传、SHA1 校验
//! M1 里程碑实现

pub mod mirror;
pub mod item;
pub mod cache;
pub mod concurrent;
