//! 协议事件输出测试

use mpack_launcher::error::LauncherError;
use mpack_launcher::protocol::{phase, Protocol};
use serde_json::Value;
use std::io::Write;

#[test]
fn test_phase_constants() {
    assert_eq!(phase::RESOLVING_VERSION, "resolving_version");
    assert_eq!(phase::DOWNLOADING_LIBRARIES, "downloading_libraries");
    assert_eq!(phase::DOWNLOADING_ASSETS, "downloading_assets");
    assert_eq!(phase::INSTALLING_LOADER, "installing_loader");
    assert_eq!(phase::VERIFYING, "verifying");
    assert_eq!(phase::PREPARING, "preparing");
    assert_eq!(phase::LAUNCHING, "launching");
}

#[test]
fn test_phase_event_captured_output() {
    let mut buf: Vec<u8> = Vec::new();
    let event = Protocol::build_phase("downloading_assets", "正在下载资源文件");
    Protocol::emit_to(&event, &mut buf);

    let output = String::from_utf8(buf).unwrap();
    let parsed: Value = serde_json::from_str(output.trim()).unwrap();

    assert_eq!(parsed["type"], "phase");
    assert_eq!(parsed["phase"], "downloading_assets");
    assert_eq!(parsed["message"], "正在下载资源文件");
    // 确保只有一行
    assert_eq!(output.lines().count(), 1);
}

#[test]
fn test_success_event_captured_output() {
    let mut buf: Vec<u8> = Vec::new();
    let event = Protocol::build_success(serde_json::json!({"version_id": "1.20.1"}));
    Protocol::emit_to(&event, &mut buf);

    let output = String::from_utf8(buf).unwrap();
    let parsed: Value = serde_json::from_str(output.trim()).unwrap();

    assert_eq!(parsed["type"], "result");
    assert_eq!(parsed["success"], true);
    assert_eq!(parsed["data"]["version_id"], "1.20.1");
    assert!(parsed.get("error").is_none());
}

#[test]
fn test_failure_event_with_suggestion() {
    let error = LauncherError::DirectoryLocked("/tmp/test".into());
    let mut buf: Vec<u8> = Vec::new();
    let event = Protocol::build_failure(&error);
    Protocol::emit_to(&event, &mut buf);

    let output = String::from_utf8(buf).unwrap();
    let parsed: Value = serde_json::from_str(output.trim()).unwrap();

    assert_eq!(parsed["type"], "result");
    assert_eq!(parsed["success"], false);
    assert_eq!(parsed["error"], "DirectoryLocked");
    assert!(parsed["message"].as_str().unwrap().contains("目录已被锁定"));
    assert!(parsed["suggestion"].is_string());
}

#[test]
fn test_failure_event_without_suggestion() {
    let error = LauncherError::InvalidArgument("test".into());
    let event = Protocol::build_failure(&error);

    assert_eq!(event["type"], "result");
    assert_eq!(event["success"], false);
    assert_eq!(event["error"], "InvalidArgument");
    // InvalidArgument 没有 suggestion
    assert!(event.get("suggestion").is_none());
}

#[test]
fn test_multiple_events_one_per_line() {
    let mut buf: Vec<u8> = Vec::new();

    Protocol::emit_to(&Protocol::build_phase("phase1", "消息1"), &mut buf);
    Protocol::emit_to(&Protocol::build_phase("phase2", "消息2"), &mut buf);
    Protocol::emit_to(
        &Protocol::build_success(serde_json::json!({"ok": true})),
        &mut buf,
    );

    let output = String::from_utf8(buf).unwrap();
    let lines: Vec<&str> = output.lines().collect();
    assert_eq!(lines.len(), 3);

    let p1: Value = serde_json::from_str(lines[0]).unwrap();
    assert_eq!(p1["phase"], "phase1");

    let p2: Value = serde_json::from_str(lines[1]).unwrap();
    assert_eq!(p2["phase"], "phase2");

    let r: Value = serde_json::from_str(lines[2]).unwrap();
    assert_eq!(r["type"], "result");
    assert_eq!(r["success"], true);
}

#[test]
fn test_error_exit_codes() {
    assert_eq!(LauncherError::InvalidArgument("test".into()).exit_code(), 10);
    assert_eq!(LauncherError::VersionNotFound("1.0".into()).exit_code(), 11);
    assert_eq!(
        LauncherError::DownloadFailed {
            url: "http://x".into(),
            message: "fail".into()
        }
        .exit_code(),
        21
    );
    assert_eq!(
        LauncherError::ChecksumMismatch {
            file: "a.jar".into(),
            expected: "abc".into(),
            actual: "def".into()
        }
        .exit_code(),
        22
    );
    assert_eq!(
        LauncherError::DirectoryLocked("/tmp".into()).exit_code(),
        31
    );
    assert_eq!(
        LauncherError::InsufficientDiskSpace {
            needed: 100,
            available: 50
        }
        .exit_code(),
        32
    );
    assert_eq!(LauncherError::JavaNotFound { required: 17 }.exit_code(), 40);
    assert_eq!(LauncherError::NotLoggedIn.exit_code(), 50);
    assert_eq!(LauncherError::TokenRefreshFailed.exit_code(), 52);
    assert_eq!(
        LauncherError::LoaderIncompatible {
            loader: "forge".into(),
            mc_version: "1.20.1".into()
        }
        .exit_code(),
        60
    );
    assert_eq!(LauncherError::Internal("x".into()).exit_code(), 99);
}

#[test]
fn test_error_type_names() {
    assert_eq!(
        LauncherError::InvalidArgument("x".into()).error_type(),
        "InvalidArgument"
    );
    assert_eq!(LauncherError::NotLoggedIn.error_type(), "NotLoggedIn");
}

#[test]
fn test_error_display() {
    let e = LauncherError::VersionNotFound("1.20.1".into());
    assert_eq!(e.to_string(), "版本不存在: 1.20.1");
}
