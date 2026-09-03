use clap::Parser;
use mpack_launcher::cli::{Cli, Command};
use mpack_launcher::error::LauncherError;
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
            match mpack_launcher::install::install(mpack_launcher::install::InstallOptions {
                mc_version: args.mc,
                loader: args.loader,
                loader_version: args.loader_version,
                dir: args.dir,
                mirror: args.mirror,
                java: args.java,
            })
            .await
            {
                Ok(version_id) => {
                    Protocol::success(json!({ "version_id": version_id }));
                    Ok(())
                }
                Err(e) => Err(e),
            }
        }
        Command::Launch(_) => Err(LauncherError::Internal(
            "launch 命令将在 M1 里程碑实现".to_string(),
        )),
        Command::Auth { .. } => Err(LauncherError::Internal(
            "auth 命令将在 M3 里程碑实现".to_string(),
        )),
        Command::Java { .. } => Err(LauncherError::Internal(
            "java 命令将在 M1/M3 里程碑实现".to_string(),
        )),
        Command::List { .. } => Err(LauncherError::Internal(
            "list 命令将在 M1 里程碑实现".to_string(),
        )),
    };

    if let Err(e) = result {
        let exit_code = e.exit_code();
        Protocol::failure(&e);
        tracing::error!("{}", e);
        std::process::exit(exit_code);
    }
}
