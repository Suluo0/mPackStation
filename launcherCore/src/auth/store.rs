//! 凭证存储：基于 keyring 的安全存储
//!
//! 存储 Minecraft 账号的 access_token / refresh_token / username / uuid。
//! Windows 上使用 Windows Credential Manager，macOS 使用 Keychain，Linux 使用 Secret Service。

use serde::{Deserialize, Serialize};

use crate::error::LauncherError;
use crate::Result;

const KEYRING_SERVICE: &str = "mPackLauncher";

/// 存储的账号凭证
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredAccount {
    /// 用户名（Minecraft 玩家名）
    pub username: String,
    /// Minecraft UUID（无连字符格式）
    pub uuid: String,
    /// Minecraft access token
    pub access_token: String,
    /// 微软 refresh token（用于刷新 access token）
    pub refresh_token: Option<String>,
    /// 账号类型（microsoft / offline）
    pub account_type: String,
}

impl StoredAccount {
    /// 创建微软账号凭证
    pub fn microsoft(
        username: impl Into<String>,
        uuid: impl Into<String>,
        access_token: impl Into<String>,
        refresh_token: impl Into<String>,
    ) -> Self {
        Self {
            username: username.into(),
            uuid: uuid.into(),
            access_token: access_token.into(),
            refresh_token: Some(refresh_token.into()),
            account_type: "microsoft".to_string(),
        }
    }

    /// 创建离线账号凭证
    pub fn offline(username: impl Into<String>, uuid: impl Into<String>) -> Self {
        Self {
            username: username.into(),
            uuid: uuid.into(),
            access_token: String::new(),
            refresh_token: None,
            account_type: "offline".to_string(),
        }
    }
}

/// 保存账号凭证到 keyring
pub fn save_account(account: &StoredAccount) -> Result<()> {
    let entry = keyring::Entry::new(KEYRING_SERVICE, &account.username)
        .map_err(|e| LauncherError::Internal(format!("keyring 创建失败: {e}")))?;

    let json = serde_json::to_string(account)
        .map_err(|e| LauncherError::Internal(format!("序列化凭证失败: {e}")))?;

    entry
        .set_password(&json)
        .map_err(|e| LauncherError::Internal(format!("keyring 写入失败: {e}")))?;

    tracing::info!("账号凭证已保存: {} ({})", account.username, account.account_type);
    Ok(())
}

/// 从 keyring 加载指定用户名的账号凭证
pub fn load_account(username: &str) -> Result<Option<StoredAccount>> {
    let entry = keyring::Entry::new(KEYRING_SERVICE, username)
        .map_err(|e| LauncherError::Internal(format!("keyring 创建失败: {e}")))?;

    match entry.get_password() {
        Ok(json) => {
            let account: StoredAccount = serde_json::from_str(&json)
                .map_err(|e| LauncherError::Internal(format!("解析凭证失败: {e}")))?;
            Ok(Some(account))
        }
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(e) => Err(LauncherError::Internal(format!("keyring 读取失败: {e}"))),
    }
}

/// 删除指定用户名的账号凭证
pub fn delete_account(username: &str) -> Result<()> {
    let entry = keyring::Entry::new(KEYRING_SERVICE, username)
        .map_err(|e| LauncherError::Internal(format!("keyring 创建失败: {e}")))?;

    match entry.delete_credential() {
        Ok(()) => {
            tracing::info!("账号凭证已删除: {}", username);
            Ok(())
        }
        Err(keyring::Error::NoEntry) => Ok(()), // 不存在也算成功
        Err(e) => Err(LauncherError::Internal(format!("keyring 删除失败: {e}"))),
    }
}

/// 列出所有已保存的账号用户名
///
/// 注意：keyring crate 不支持枚举所有条目，此函数返回空列表。
/// 实际使用中，应在本地配置文件中记录已登录的用户名列表。
pub fn list_accounts() -> Result<Vec<String>> {
    // keyring 不支持枚举，返回空
    // 调用方应维护一个本地的账号列表配置
    Ok(Vec::new())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_stored_account_serialization() {
        let account = StoredAccount::offline("TestPlayer", "abc123");
        let json = serde_json::to_string(&account).unwrap();
        let parsed: StoredAccount = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.username, "TestPlayer");
        assert_eq!(parsed.uuid, "abc123");
        assert_eq!(parsed.account_type, "offline");
    }

    #[test]
    fn test_microsoft_account() {
        let account = StoredAccount::microsoft("Player", "uuid", "access", "refresh");
        assert_eq!(account.account_type, "microsoft");
        assert_eq!(account.refresh_token.as_deref(), Some("refresh"));
    }

    #[test]
    #[ignore = "依赖系统 keyring，需手动运行"]
    fn test_keyring_save_load_delete() {
        // 使用测试用户名避免冲突
        let test_user = "mpack_test_user_xyz";
        let account = StoredAccount::offline(test_user, "test-uuid");

        // 保存
        save_account(&account).expect("save should work");

        // 加载
        let loaded = load_account(test_user).expect("load should work");
        assert!(loaded.is_some());
        assert_eq!(loaded.unwrap().username, test_user);

        // 删除
        delete_account(test_user).expect("delete should work");

        // 确认已删除
        let loaded = load_account(test_user).expect("load after delete should work");
        assert!(loaded.is_none());
    }
}
