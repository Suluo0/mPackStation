//! 微软 OAuth device flow 登录 + Minecraft 凭证获取
//!
//! 登录流程：
//! 1. 请求 device code → 返回 user_code + verification_uri
//! 2. 用户在浏览器打开 verification_uri，输入 user_code 并登录
//! 3. 轮询 token 端点，获取微软 access_token + refresh_token
//! 4. 用微软 token 获取 Xbox Live token
//! 5. 用 XBL token 获取 XSTS token
//! 6. 用 XSTS token 获取 Minecraft access_token
//! 7. 获取 Minecraft profile（username + UUID）

use serde::Deserialize;

use crate::error::LauncherError;
use crate::Result;

/// Minecraft 官方启动器的 Azure client_id（公开，第三方启动器通用）
const CLIENT_ID: &str = "00000000402b5328";
const SCOPE: &str = "XboxLive.signin offline_access";

/// Device code 响应
#[derive(Debug, Deserialize)]
pub struct DeviceCodeResponse {
    pub user_code: String,
    pub device_code: String,
    pub verification_uri: String,
    pub expires_in: u64,
    pub interval: u64,
}

/// Token 响应
#[derive(Debug, Deserialize)]
struct TokenResponse {
    access_token: String,
    #[serde(default)]
    refresh_token: Option<String>,
    #[serde(default)]
    expires_in: Option<u64>,
}

/// Xbox Live 认证响应
#[derive(Debug, Deserialize)]
struct XblAuthResponse {
    #[serde(rename = "Token")]
    token: String,
    #[serde(rename = "DisplayClaims")]
    display_claims: XblDisplayClaims,
}

#[derive(Debug, Deserialize)]
struct XblDisplayClaims {
    xui: Vec<XblXui>,
}

#[derive(Debug, Deserialize)]
struct XblXui {
    uhs: String,
}

/// XSTS 认证响应
#[derive(Debug, Deserialize)]
struct XstsAuthResponse {
    #[serde(rename = "Token")]
    token: String,
    #[serde(rename = "DisplayClaims")]
    display_claims: XblDisplayClaims,
}

/// Minecraft 登录响应
#[derive(Debug, Deserialize)]
struct MinecraftLoginResponse {
    access_token: String,
    #[serde(default)]
    expires_in: Option<u64>,
}

/// Minecraft profile 响应
#[derive(Debug, Deserialize)]
pub struct MinecraftProfile {
    pub id: String,
    pub name: String,
}

/// 登录结果
#[derive(Debug, Clone)]
pub struct MicrosoftAccount {
    pub username: String,
    pub uuid: String,
    pub minecraft_access_token: String,
    pub microsoft_refresh_token: String,
}

/// 第一步：请求 device code
///
/// 返回 device code 信息，调用方需要展示给用户：
/// - user_code: 用户需要输入的代码
/// - verification_uri: 用户需要打开的网址
pub async fn request_device_code() -> Result<DeviceCodeResponse> {
    let params = [
        ("client_id", CLIENT_ID),
        ("scope", SCOPE),
    ];

    let resp = reqwest::Client::new()
        .post("https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode")
        .form(&params)
        .send()
        .await?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        return Err(LauncherError::Internal(format!(
            "请求 device code 失败 ({}): {}",
            status, body
        )));
    }

    let device_code: DeviceCodeResponse = resp.json().await?;
    tracing::info!(
        "获取 device code 成功: {} (有效期 {}s)",
        device_code.user_code,
        device_code.expires_in
    );
    Ok(device_code)
}

/// 第二步：轮询获取微软 token
///
/// 按 device_code.interval 秒轮询，直到用户完成登录或过期。
/// 此函数会阻塞，建议在异步任务中调用。
pub async fn poll_token(device_code: &DeviceCodeResponse) -> Result<MicrosoftAccount> {
    let interval = device_code.interval.max(5); // 至少 5 秒
    let max_attempts = device_code.expires_in / interval;

    for _ in 0..max_attempts {
        tokio::time::sleep(std::time::Duration::from_secs(interval)).await;

        match request_token(&device_code.device_code).await {
            Ok(token) => {
                tracing::info!("获取微软 token 成功");
                let account = exchange_minecraft_token(&token.access_token, token.refresh_token.as_deref()).await?;
                return Ok(account);
            }
            Err(LauncherError::Internal(msg)) if msg.contains("authorization_pending") => {
                // 用户还没完成登录，继续轮询
                tracing::debug!("等待用户登录...");
                continue;
            }
            Err(e) => return Err(e),
        }
    }

    Err(LauncherError::Internal("device code 已过期，请重新登录".into()))
}

/// 请求微软 token（device code grant）
async fn request_token(device_code: &str) -> Result<TokenResponse> {
    let params = [
        ("client_id", CLIENT_ID),
        ("grant_type", "urn:ietf:params:oauth:grant-type:device_code"),
        ("device_code", device_code),
    ];

    let resp = reqwest::Client::new()
        .post("https://login.microsoftonline.com/consumers/oauth2/v2.0/token")
        .form(&params)
        .send()
        .await?;

    let status = resp.status();
    let body = resp.text().await?;

    if !status.is_success() {
        // 检查是否是 authorization_pending（正常轮询中的状态）
        if body.contains("authorization_pending") {
            return Err(LauncherError::Internal("authorization_pending".into()));
        }
        return Err(LauncherError::Internal(format!(
            "获取 token 失败 ({}): {}",
            status, body
        )));
    }

    let token: TokenResponse = serde_json::from_str(&body)
        .map_err(|e| LauncherError::Internal(format!("解析 token 响应失败: {e}")))?;
    Ok(token)
}

/// 用微软 token 交换 Minecraft 凭证（XBL → XSTS → Minecraft）
async fn exchange_minecraft_token(
    microsoft_access_token: &str,
    microsoft_refresh_token: Option<&str>,
) -> Result<MicrosoftAccount> {
    // 1. Xbox Live 认证
    let xbl = authenticate_xbox_live(microsoft_access_token).await?;
    let uhs = xbl
        .display_claims
        .xui
        .first()
        .ok_or_else(|| LauncherError::Internal("XBL 响应缺少 uhs".into()))?
        .uhs
        .clone();

    // 2. XSTS 认证
    let xsts = authenticate_xsts(&xbl.token).await?;

    // 3. Minecraft 登录
    let mc_token = login_minecraft(&uhs, &xsts.token).await?;

    // 4. 获取 profile
    let profile = get_minecraft_profile(&mc_token.access_token).await?;

    Ok(MicrosoftAccount {
        username: profile.name,
        uuid: profile.id,
        minecraft_access_token: mc_token.access_token,
        microsoft_refresh_token: microsoft_refresh_token
            .unwrap_or("")
            .to_string(),
    })
}

/// Xbox Live 认证
async fn authenticate_xbox_live(access_token: &str) -> Result<XblAuthResponse> {
    let body = serde_json::json!({
        "Properties": {
            "AuthMethod": "RPS",
            "SiteName": "user.auth.xboxlive.com",
            "RpsTicket": format!("d={}", access_token)
        },
        "RelyingParty": "http://auth.xboxlive.com",
        "TokenType": "JWT"
    });

    let resp = reqwest::Client::new()
        .post("https://user.auth.xboxlive.com/user/authenticate")
        .json(&body)
        .send()
        .await?;

    if !resp.status().is_success() {
        return Err(LauncherError::Internal(format!(
            "XBL 认证失败: {}",
            resp.status()
        )));
    }

    Ok(resp.json().await?)
}

/// XSTS 认证
async fn authenticate_xsts(xbl_token: &str) -> Result<XstsAuthResponse> {
    let body = serde_json::json!({
        "Properties": {
            "SandboxId": "RETAIL",
            "UserTokens": [xbl_token]
        },
        "RelyingParty": "rp://api.minecraftservices.com/",
        "TokenType": "JWT"
    });

    let resp = reqwest::Client::new()
        .post("https://xsts.auth.xboxlive.com/xsts/authorize")
        .json(&body)
        .send()
        .await?;

    if !resp.status().is_success() {
        return Err(LauncherError::Internal(format!(
            "XSTS 认证失败: {}",
            resp.status()
        )));
    }

    Ok(resp.json().await?)
}

/// Minecraft 登录（用 XSTS token 换 Minecraft access token）
async fn login_minecraft(uhs: &str, xsts_token: &str) -> Result<MinecraftLoginResponse> {
    let body = serde_json::json!({
        "identityToken": format!("XBL3.0 x={};{}", uhs, xsts_token)
    });

    let resp = reqwest::Client::new()
        .post("https://api.minecraftservices.com/authentication/login_with_xbox")
        .json(&body)
        .send()
        .await?;

    if !resp.status().is_success() {
        return Err(LauncherError::Internal(format!(
            "Minecraft 登录失败: {}",
            resp.status()
        )));
    }

    Ok(resp.json().await?)
}

/// 获取 Minecraft profile
async fn get_minecraft_profile(access_token: &str) -> Result<MinecraftProfile> {
    let resp = reqwest::Client::new()
        .get("https://api.minecraftservices.com/minecraft/profile")
        .bearer_auth(access_token)
        .send()
        .await?;

    if !resp.status().is_success() {
        return Err(LauncherError::Internal(format!(
            "获取 Minecraft profile 失败: {}",
            resp.status()
        )));
    }

    Ok(resp.json().await?)
}

/// 用 refresh token 刷新微软 token，然后重新获取 Minecraft 凭证
pub async fn refresh_microsoft_account(refresh_token: &str) -> Result<MicrosoftAccount> {
    let params = [
        ("client_id", CLIENT_ID),
        ("grant_type", "refresh_token"),
        ("refresh_token", refresh_token),
        ("scope", SCOPE),
    ];

    let resp = reqwest::Client::new()
        .post("https://login.microsoftonline.com/consumers/oauth2/v2.0/token")
        .form(&params)
        .send()
        .await?;

    if !resp.status().is_success() {
        return Err(LauncherError::Internal(format!(
            "刷新 token 失败: {}",
            resp.status()
        )));
    }

    let token: TokenResponse = resp.json().await?;
    exchange_minecraft_token(&token.access_token, token.refresh_token.as_deref()).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_device_code_response_deserialize() {
        let json = r#"{
            "user_code": "ABCD-1234",
            "device_code": "long_device_code",
            "verification_uri": "https://microsoft.com/link",
            "expires_in": 900,
            "interval": 5
        }"#;
        let parsed: DeviceCodeResponse = serde_json::from_str(json).unwrap();
        assert_eq!(parsed.user_code, "ABCD-1234");
        assert_eq!(parsed.interval, 5);
    }

    #[test]
    fn test_minecraft_profile_deserialize() {
        let json = r#"{"id": "abc123", "name": "TestPlayer"}"#;
        let parsed: MinecraftProfile = serde_json::from_str(json).unwrap();
        assert_eq!(parsed.id, "abc123");
        assert_eq!(parsed.name, "TestPlayer");
    }
}
