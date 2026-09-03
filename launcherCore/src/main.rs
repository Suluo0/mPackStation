use clap::Parser;
use mpack_launcher::cli::{Cli, Command};
use mpack_launcher::download::Mirror;
use mpack_launcher::error::LauncherError;
use mpack_launcher::java::JavaRegistry;
use mpack_launcher::protocol::Protocol;
use serde_json::json;

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    // 初始化日志（stderr，与 stdout 协议分离）
    tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_env_filter(format!(
            "mpack_launcher={}",
            cli.log_level.to_lowercase()
        ))
        .init();

    let result = match cli.command {
        Command::Version => {
            Protocol::success(json!({
                "name": "mpack-launcher",
                "version": env!("CARGO_PKG_VERSION"),
            }));
            Ok(())
        }
        Command::Install(args) => {
            let mirror = Mirror::from_str(&args.mirror);
            if args.loader != "vanilla" {
                Err(LauncherError::Internal(format!(
                    "加载器 {} 将在 M2 里程碑实现",
                    args.loader
                )))
            } else {
                match mpack_launcher::install::install_vanilla(
                    &args.dir,
                    &args.mc,
                    mirror,
                )
                .await
                {
                    Ok(version_id) => {
                        Protocol::success(json!({ "version_id": version_id }));
                        Ok(())
                    }
                    Err(e) => Err(e),
                }
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
            mpack_launcher::cli::JavaCommand::List => {
                let registry = JavaRegistry::detect();
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
            mpack_launcher::cli::JavaCommand::Install { version } => {
                Err(LauncherError::Internal(format!(
                    "Java 自动下载将在 M3 里程碑实现（请求版本: {}）",
                    version
                )))
            }
        },
        Command::Auth { .. } => Err(LauncherError::Internal(
            "auth 命令将在 M3 里程碑实现".to_string(),
        )),
    };

    if let Err(e) = result {
        let exit_code = e.exit_code();
        Protocol::failure(&e);
        tracing::error!("{}", e);
        std::process::exit(exit_code);
    }
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
