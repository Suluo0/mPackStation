//! 离线账号：基于用户名生成标准 UUID v3
//!
//! Minecraft 离线账号的 UUID 生成规则：
//! - UUID v3，namespace 为全零 UUID
//! - name 为 "OfflinePlayer:<username>"
//! 这样同一用户名在所有启动器中生成相同的 UUID，保证服务器玩家数据一致。

use uuid::Uuid;

/// 离线账号
#[derive(Debug, Clone)]
pub struct OfflineAccount {
    pub username: String,
    pub uuid: String,
}

impl OfflineAccount {
    /// 创建离线账号，生成标准 UUID v3
    pub fn new(username: impl Into<String>) -> Self {
        let username = username.into();
        let uuid = generate_offline_uuid(&username);
        Self { username, uuid }
    }
}

/// 生成离线账号的标准 UUID v3
///
/// 与 HMCL/PCL/Prism 等主流启动器保持一致。
pub fn generate_offline_uuid(username: &str) -> String {
    let namespace = Uuid::nil(); // 全零 UUID
    let name = format!("OfflinePlayer:{}", username);
    let uuid = Uuid::new_v3(&namespace, name.as_bytes());
    uuid.to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_offline_uuid_deterministic() {
        // 同一用户名应生成相同 UUID
        let uuid1 = generate_offline_uuid("TestPlayer");
        let uuid2 = generate_offline_uuid("TestPlayer");
        assert_eq!(uuid1, uuid2);
    }

    #[test]
    fn test_different_usernames_different_uuid() {
        let uuid1 = generate_offline_uuid("Player1");
        let uuid2 = generate_offline_uuid("Player2");
        assert_ne!(uuid1, uuid2);
    }

    #[test]
    fn test_offline_uuid_format() {
        let uuid = generate_offline_uuid("Test");
        // UUID 格式：8-4-4-4-12 十六进制字符
        assert_eq!(uuid.len(), 36);
        assert!(uuid.chars().filter(|c| *c == '-').count() == 4);
    }

    #[test]
    fn test_offline_account_creation() {
        let account = OfflineAccount::new("Steve");
        assert_eq!(account.username, "Steve");
        assert_eq!(account.uuid, generate_offline_uuid("Steve"));
    }
}
