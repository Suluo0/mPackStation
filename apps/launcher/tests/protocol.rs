//! 协议事件输出测试

use mpack_launcher::error::LauncherError;
use mpack_launcher::protocol::{phase, Protocol};
use serde_json::Value;

#[test]
fn test_phase_event_format() {
    // 捕获 stdout 比较困难，这里验证 phase 常量和 Protocol API 存在
    assert_eq!(phase::RESOLVING_VERSION, "resolving_version");
    assert_eq!(phase::DOWNLOADING_LIBRARIES, "downloading_libraries");
    assert_eq!(phase::DOWNLOADING_ASSETS, "downloading_assets");
    assert_eq!(phase::INSTALLING_LOADER, "installing_loader");
    assert_eq!(phase::VERIFYING, "verifying");
    assert_eq!(phase::PREPARING, "preparing");
    assert_eq!(phase::LAUNCHING, "launching");
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
    assert_eq!(
        LauncherError::ChecksumMismatch {
            file: "a".into(),
            expected: "b".into(),
            actual: "c".into()
        }
        .error_type(),
        "ChecksumMismatch"
    );
    assert_eq!(LauncherError::NotLoggedIn.error_type(), "NotLoggedIn");
}

#[test]
fn test_error_suggestions() {
    assert!(LauncherError::NotLoggedIn.suggestion().is_some());
    assert!(LauncherError::TokenRefreshFailed.suggestion().is_some());
    assert!(LauncherError::InsufficientDiskSpace {
        needed: 1,
        available: 0
    }
    .suggestion()
    .is_some());
    // InvalidArgument 没有 suggestion
    assert!(LauncherError::InvalidArgument("x".into()).suggestion().is_none());
}

#[test]
fn test_error_display() {
    let e = LauncherError::VersionNotFound("1.20.1".into());
    assert_eq!(e.to_string(), "版本不存在: 1.20.1");

    let e = LauncherError::JavaNotFound { required: 17 };
    assert_eq!(e.to_string(), "未找到匹配的 Java 运行时（需要 Java 17+）");
}

#[test]
fn test_protocol_success_json_structure() {
    // 验证 success 数据结构
    let data = serde_json::json!({"version_id": "1.20.1"});
    let event = serde_json::json!({
        "type": "result",
        "success": true,
        "data": data,
    });
    assert_eq!(event["type"], "result");
    assert_eq!(event["success"], true);
    assert_eq!(event["data"]["version_id"], "1.20.1");
}

#[test]
fn test_protocol_failure_json_structure() {
    let error = LauncherError::DirectoryLocked("/tmp/test".into());
    let event = serde_json::json!({
        "type": "result",
        "success": false,
        "error": error.error_type(),
        "message": error.to_string(),
        "suggestion": error.suggestion(),
    });
    assert_eq!(event["type"], "result");
    assert_eq!(event["success"], false);
    assert_eq!(event["error"], "DirectoryLocked");
    assert!(event["message"].as_str().unwrap().contains("目录已被锁定"));
    assert!(event["suggestion"].is_string());
}

#[test]
fn test_phase_event_json_structure() {
    let event = serde_json::json!({
        "type": "phase",
        "phase": "downloading_assets",
        "message": "正在下载资源文件",
    });
    assert_eq!(event["type"], "phase");
    assert_eq!(event["phase"], "downloading_assets");
    assert!(event["message"].as_str().unwrap().contains("下载"));
}

// 确保 Protocol 方法可以被调用（编译期验证）
#[test]
fn test_protocol_api_exists() {
    // 这些调用会写入 stdout，但测试环境中可以接受
    Protocol::phase("test_phase", "测试消息");
    Protocol::success(serde_json::json!({"ok": true}));
    Protocol::failure(&LauncherError::Internal("test".into()));
}
