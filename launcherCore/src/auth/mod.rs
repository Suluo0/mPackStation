//! 认证模块
//!
//! 负责：离线账号、微软 OAuth device flow、token 加密存储

pub mod microsoft;
pub mod offline;
pub mod store;

pub use microsoft::{
    DeviceCodeResponse, MicrosoftAccount, MinecraftProfile, poll_token, refresh_microsoft_account,
    request_device_code,
};
pub use offline::OfflineAccount;
pub use store::{StoredAccount, delete_account, list_accounts, load_account, save_account};
