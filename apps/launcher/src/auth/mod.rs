//! 认证模块
//!
//! 负责：离线账号、微软 OAuth device flow、token 加密存储
//! M3 里程碑实现

pub mod offline;
pub mod microsoft;
pub mod store;
