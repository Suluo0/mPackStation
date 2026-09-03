//! CLI 解析测试

use clap::Parser;
use mpack_launcher::cli::{Cli, Command};

#[test]
fn test_version_command() {
    let cli = Cli::parse_from(["mpack-launcher", "version"]);
    assert!(matches!(cli.command, Command::Version));
}

#[test]
fn test_install_command_basic() {
    let cli = Cli::parse_from([
        "mpack-launcher",
        "install",
        "--mc",
        "1.20.1",
        "--dir",
        "/tmp/mc",
    ]);
    match cli.command {
        Command::Install(args) => {
            assert_eq!(args.mc, "1.20.1");
            assert_eq!(args.loader, "vanilla");
            assert_eq!(args.loader_version, "latest");
            assert_eq!(args.mirror, "auto");
            assert_eq!(args.dir.to_str().unwrap(), "/tmp/mc");
        }
        _ => panic!("expected install command"),
    }
}

#[test]
fn test_install_command_with_loader() {
    let cli = Cli::parse_from([
        "mpack-launcher",
        "install",
        "--mc",
        "1.20.1",
        "--loader",
        "fabric",
        "--loader-version",
        "0.16.5",
        "--dir",
        "/tmp/mc",
        "--mirror",
        "bmclapi",
    ]);
    match cli.command {
        Command::Install(args) => {
            assert_eq!(args.loader, "fabric");
            assert_eq!(args.loader_version, "0.16.5");
            assert_eq!(args.mirror, "bmclapi");
        }
        _ => panic!("expected install command"),
    }
}

#[test]
fn test_launch_command() {
    let cli = Cli::parse_from([
        "mpack-launcher",
        "launch",
        "--version",
        "1.20.1",
        "--dir",
        "/tmp/mc",
        "--username",
        "Steve",
        "--wait",
    ]);
    match cli.command {
        Command::Launch(args) => {
            assert_eq!(args.version, "1.20.1");
            assert_eq!(args.username.as_deref(), Some("Steve"));
            assert!(args.wait);
            assert_eq!(args.account_type, "offline");
        }
        _ => panic!("expected launch command"),
    }
}

#[test]
fn test_auth_login_offline() {
    let cli = Cli::parse_from([
        "mpack-launcher",
        "auth",
        "login",
        "--provider",
        "offline",
        "--username",
        "Steve",
    ]);
    match cli.command {
        Command::Auth { action } => match action {
            mpack_launcher::cli::AuthCommand::Login {
                provider,
                username,
            } => {
                assert_eq!(provider, "offline");
                assert_eq!(username.as_deref(), Some("Steve"));
            }
            _ => panic!("expected login"),
        },
        _ => panic!("expected auth command"),
    }
}

#[test]
fn test_log_level_default() {
    let cli = Cli::parse_from(["mpack-launcher", "version"]);
    assert_eq!(cli.log_level, "info");
}

#[test]
fn test_log_level_custom() {
    let cli = Cli::parse_from(["mpack-launcher", "--log-level", "debug", "version"]);
    assert_eq!(cli.log_level, "debug");
}

#[test]
fn test_java_command_list() {
    let cli = Cli::parse_from(["mpack-launcher", "java", "list"]);
    match cli.command {
        Command::Java { action } => {
            assert!(matches!(
                action,
                mpack_launcher::cli::JavaCommand::List
            ));
        }
        _ => panic!("expected java command"),
    }
}

#[test]
fn test_list_command() {
    let cli = Cli::parse_from(["mpack-launcher", "list", "--dir", "/tmp/mc"]);
    match cli.command {
        Command::List { dir } => {
            assert_eq!(dir.to_str().unwrap(), "/tmp/mc");
        }
        _ => panic!("expected list command"),
    }
}
