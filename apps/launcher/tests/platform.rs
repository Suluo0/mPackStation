//! platform 模块测试

use mpack_launcher::platform::{validate_safe_path, Os, Arch};
use std::fs;
use std::path::PathBuf;

fn temp_base(name: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("mpack-platform-test-{}", name));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir).unwrap();
    dir
}

#[test]
fn test_os_current() {
    let os = Os::current();
    // 在 Windows 上应该是 Windows
    if cfg!(target_os = "windows") {
        assert_eq!(os, Os::Windows);
    }
    assert_ne!(os.as_str(), "unknown");
}

#[test]
fn test_arch_current() {
    let arch = Arch::current();
    assert_ne!(arch.as_str(), "unknown");
}

/// 比较路径是否在 base 内（Windows 不区分大小写，统一小写比较）
fn is_under_base(base: &std::path::Path, path: &std::path::Path) -> bool {
    let b = base.to_string_lossy().to_lowercase();
    let p = path.to_string_lossy().to_lowercase();
    p.starts_with(&b)
}

#[test]
fn test_safe_path_normal() {
    let base = temp_base("normal");
    let result = validate_safe_path(&base, &PathBuf::from("subdir/file.txt")).unwrap();
    assert!(is_under_base(&base, &result));
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_parent_dir_blocked() {
    let base = temp_base("parent");
    let result = validate_safe_path(&base, &PathBuf::from("../etc/passwd"));
    assert!(result.is_err());
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_deep_parent_blocked() {
    let base = temp_base("deep");
    let result = validate_safe_path(&base, &PathBuf::from("a/b/../../../etc/passwd"));
    assert!(result.is_err());
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_absolute_outside_blocked() {
    let base = temp_base("abs");
    // 尝试一个不在 base 内的绝对路径
    let outside = std::env::temp_dir().join("definitely_outside_mpack_test");
    let result = validate_safe_path(&base, &outside);
    assert!(result.is_err());
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_absolute_inside_allowed() {
    let base = temp_base("abs-inside");
    let inside = base.join("subdir").join("file.txt");
    let result = validate_safe_path(&base, &inside).unwrap();
    assert!(is_under_base(&base, &result));
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_current_dir_allowed() {
    let base = temp_base("cur");
    let result = validate_safe_path(&base, &PathBuf::from("./file.txt")).unwrap();
    assert!(is_under_base(&base, &result));
    let _ = fs::remove_dir_all(&base);
}

#[test]
fn test_safe_path_nonexistent_base_rejected() {
    let base = PathBuf::from("/nonexistent/path/for/sure");
    let result = validate_safe_path(&base, &PathBuf::from("file.txt"));
    assert!(result.is_err());
}

#[test]
fn test_available_disk_space_returns_positive() {
    let dir = std::env::temp_dir();
    let space = mpack_launcher::platform::available_disk_space(&dir).unwrap();
    assert!(space > 0);
}

#[test]
fn test_ensure_disk_space_small_need_passes() {
    let dir = std::env::temp_dir();
    // 需要 1MB 应该总能通过
    let result = mpack_launcher::platform::ensure_disk_space(&dir, 1);
    assert!(result.is_ok());
}
