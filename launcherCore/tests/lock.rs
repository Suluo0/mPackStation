//! 文件锁测试

use mpack_launcher::lock::DirectoryLock;
use std::fs;
use std::path::PathBuf;

fn temp_dir(name: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("mpack-launcher-test-{}", name));
    let _ = fs::remove_dir_all(&dir);
    fs::create_dir_all(&dir).unwrap();
    dir
}

#[test]
fn test_lock_acquire_and_release() {
    let dir = temp_dir("lock-basic");
    let lock = DirectoryLock::acquire(&dir).unwrap();
    assert!(lock.lock_path().exists());
    drop(lock);
    // 释放后应该能重新获取
    let _lock2 = DirectoryLock::acquire(&dir).unwrap();
    let _ = fs::remove_dir_all(&dir);
}

#[test]
fn test_lock_conflict() {
    let dir = temp_dir("lock-conflict");
    let _lock1 = DirectoryLock::acquire(&dir).unwrap();
    // 第二个锁应该失败
    let result = DirectoryLock::acquire(&dir);
    assert!(result.is_err());
    let _ = fs::remove_dir_all(&dir);
}

#[test]
fn test_lock_creates_directory() {
    let dir = temp_dir("lock-create");
    let _ = fs::remove_dir_all(&dir);
    assert!(!dir.exists());
    let _lock = DirectoryLock::acquire(&dir).unwrap();
    assert!(dir.exists());
    let _ = fs::remove_dir_all(&dir);
}
