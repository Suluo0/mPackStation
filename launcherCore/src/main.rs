use clap::Parser;
use mpack_launcher::cli::{Cli, Command};
use mpack_launcher::download::Mirror;
use mpack_launcher::error::LauncherError;
use mpack_launcher::auth::{OfflineAccount, StoredAccount, save_account};
use mpack_launcher::java::{JavaRegistry, download_java};
use mpack_launcher::protocol::Protocol;
use serde_json::json;

#[tokio::main]
async fn main() -> Result<(), LauncherError> {
    let cli = Cli::parse();

    // 初始化日志（stderr，与 stdout 协议分离）
    tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_env_filter(format!(
            "mpack_launcher={}",
            cli.log_level.to_lowercase()
        ))
        .init();

    let result: Result<(), LauncherError> = match cli.command {
        Command::Version => {
            Protocol::success(json!({
                "name": "mpack-launcher",
                "version": env!("CARGO_PKG_VERSION"),
            }));
            Ok(())
        }
        Command::Install(args) => {
            match run_install(&args).await {
                Ok(version_id) => {
                    Protocol::success(json!({ "version_id": version_id }));
                    Ok(())
                }
                Err(e) => Err(e),
            }
        }
        Command::Launch(args) => {
            let username = args.username.unwrap_or_else(|| "Player".to_string());
            let max_memory_mb = args.xmx.as_deref().and_then(parse_memory_mb);
            let detach = !args.wait;

            match mpack_launcher::install::launch_game(
                &args.dir,
                &args.version,
                &username,
                args.java,
                max_memory_mb,
                detach,
            ) {
                Ok(pid) => {
                    Protocol::success(json!({ "pid": pid }));
                    Ok(())
                }
                Err(e) => Err(e),
            }
        }
        Command::List { dir } => {
            match mpack_launcher::install::list_installed_versions(&dir) {
                Ok(versions) => {
                    Protocol::success(json!({ "versions": versions }));
                    Ok(())
                }
                Err(e) => Err(e),
            }
        }
        Command::Java { action } => match action {
            mpack_launcher::cli::JavaCommand::List { deep } => {
                let registry = if deep {
                    JavaRegistry::detect_deep()
                } else {
                    JavaRegistry::detect()
                };
                let runtimes: Vec<_> = registry
                    .list()
                    .iter()
                    .map(|rt| {
                        json!({
                            "executable": rt.executable,
                            "major_version": rt.major_version,
                            "version": rt.version_string,
                            "vendor": rt.vendor,
                        })
                    })
                    .collect();
                Protocol::success(json!({ "runtimes": runtimes }));
                Ok(())
            }
            mpack_launcher::cli::JavaCommand::Install { version, mirror } => {
                let mirror = Mirror::from_str(&mirror);
                Protocol::phase("downloading", &format!("正在下载 Java {}", version));
                let runtime_dir = std::env::current_exe()
                        .ok()
                        .and_then(|p| p.parent().map(|p| p.join("runtime")))
                        .unwrap_or_else(|| std::path::PathBuf::from("runtime"));
                let java_path = download_java(version, mirror, &runtime_dir).await?;
                Protocol::success(json!({
                    "version": version,
                    "path": java_path,
                    "runtime_dir": runtime_dir,
                }));
                Ok(())
            }
        },
        Command::Auth { action } => match action {
            mpack_launcher::cli::AuthCommand::Login { provider, username } => {
                match provider.as_str() {
                    "offline" => {
                        let username = username.ok_or_else(|| LauncherError::InvalidArgument(
                            "offline 登录需要 --username 参数".into(),
                        ))?;
                        let account = OfflineAccount::new(&username);
                        let stored = StoredAccount::offline(&account.username, &account.uuid);
                        save_account(&stored)?;
                        Protocol::success(json!({
                            "provider": "offline",
                            "username": account.username,
                            "uuid": account.uuid,
                        }));
                        Ok(())
                    }
                    "microsoft" => {
                        // 1. 请求 device code
                        Protocol::phase("authenticating", "正在获取设备登录码...");
                        let device_code = mpack_launcher::auth::request_device_code().await?;
                        Protocol::phase("await_user", &format!(
                            "请打开 {} 并输入代码: {}",
                            device_code.verification_uri, device_code.user_code
                        ));
                        // 输出 device code 信息供调用方读取
                        Protocol::success(json!({
                            "step": "device_code",
                            "user_code": device_code.user_code,
                            "verification_uri": device_code.verification_uri,
                            "message": format!("请打开 {} 输入代码 {}", device_code.verification_uri, device_code.user_code),
                        }));
                        // 2. 轮询 token（阻塞等待用户登录）
                        let ms_account = mpack_launcher::auth::poll_token(&device_code).await?;
                        let stored = StoredAccount::microsoft(
                            &ms_account.username,
                            &ms_account.uuid,
                            &ms_account.minecraft_access_token,
                            &ms_account.microsoft_refresh_token,
                        );
                        save_account(&stored)?;
                        Protocol::phase("authenticated", &format!("登录成功: {}", ms_account.username));
                        Protocol::success(json!({
                            "provider": "microsoft",
                            "username": ms_account.username,
                            "uuid": ms_account.uuid,
                        }));
                        Ok(())
                    }
                    other => Err(LauncherError::InvalidArgument(format!(
                        "不支持的登录方式: {}（支持 offline/microsoft）", other
                    ))),
                }
            }
            mpack_launcher::cli::AuthCommand::Status => {
                // keyring 不支持枚举，返回提示
                Protocol::success(json!({
                    "message": "使用 auth login 登录，auth logout --username <name> 登出",
                    "note": "keyring 不支持枚举所有账号，请指定用户名查询",
                }));
                Ok(())
            }
            mpack_launcher::cli::AuthCommand::Logout => {
                // 简化：不指定用户名时无法删除（keyring 不支持枚举）
                Protocol::success(json!({
                    "message": "请使用 keyring 管理器手动删除，或指定用户名调用 auth logout --username",
                }));
                Ok(())
            }
        },
    };

    if let Err(e) = result {
        let exit_code = e.exit_code();
        Protocol::failure(&e);
        tracing::error!("{}", e);
        std::process::exit(exit_code);
    }
    Ok(())
}

/// 执行安装（Vanilla 或加载器）
async fn run_install(args: &mpack_launcher::cli::InstallArgs) -> Result<String, LauncherError> {
    use mpack_launcher::loader::{LoaderInstaller, LoaderType};
    let mirror = Mirror::from_str(&args.mirror);
    if args.loader == "vanilla" {
        return mpack_launcher::install::install_vanilla(&args.dir, &args.mc, mirror).await;
    }
    let loader_type = match args.loader.as_str() {
        "fabric" => LoaderType::Fabric,
        "quilt" => LoaderType::Quilt,
        "forge" => LoaderType::Forge,
        "neoforge" => LoaderType::NeoForge,
        other => return Err(LauncherError::Internal(format!("不支持的加载器: {}", other))),
    };
    let java_registry = JavaRegistry::detect();
    let installer = LoaderInstaller::new(&args.dir, mirror);
    let loader_ver = if args.loader_version == "latest" {
        None
    } else {
        Some(args.loader_version.as_str())
    };
    installer
        .install(&args.mc, loader_type, loader_ver, &java_registry)
        .await
}

/// 解析内存字符串为 MB（如 "2G" → 2048, "512M" → 512）
fn parse_memory_mb(s: &str) -> Option<u32> {
    let s = s.trim().to_uppercase();
    if let Some(gb) = s.strip_suffix('G') {
        gb.parse::<u32>().ok().map(|v| v * 1024)
    } else if let Some(mb) = s.strip_suffix('M') {
        mb.parse().ok()
    } else {
        s.parse::<u32>().ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_memory_gb() {
        assert_eq!(parse_memory_mb("2G"), Some(2048));
        assert_eq!(parse_memory_mb("4g"), Some(4096));
    }

    #[test]
    fn test_parse_memory_mb() {
        assert_eq!(parse_memory_mb("512M"), Some(512));
        assert_eq!(parse_memory_mb("4096m"), Some(4096));
    }

    #[test]
    fn test_parse_memory_plain() {
        assert_eq!(parse_memory_mb("2048"), Some(2048));
    }

    #[test]
    fn test_parse_memory_invalid() {
        assert_eq!(parse_memory_mb("abc"), None);
    }
}
