//! 安装编排：锁 + 磁盘预检 + 下载 + 启动

use std::path::{Path, PathBuf};

use mc_launcher_core::install::client::{fetch_vanilla_version, load_version_json, write_version_json};

use crate::download::{Downloader, Mirror};
use crate::error::LauncherError;
use crate::java::{mc_version_to_java, JavaRegistry};
use crate::launch::{build_command, spawn_detached, LaunchParams};
use crate::lock::DirectoryLock;
use crate::platform::{ensure_disk_space, validate_safe_path};
use crate::protocol::{self, Protocol};
use crate::Result;

/// 安装 Vanilla Minecraft
///
/// 流程：
/// 1. 获取目录锁
/// 2. 磁盘空间预检
/// 3. 解析版本元数据（fetch_vanilla_version）
/// 4. 写入版本 JSON
/// 5. 并发下载 client.jar + libraries + assets（支持 BMCLAPI 镜像）
/// 6. 校验
pub async fn install_vanilla(
    minecraft_dir: &Path,
    version_id: &str,
    mirror: Mirror,
) -> Result<String> {
    // 1. 获取目录锁
    let _lock = DirectoryLock::acquire(minecraft_dir)?;

    // 2. 磁盘空间预检（预估 500MB，实际可能更多）
    ensure_disk_space(minecraft_dir, 500 * 1024 * 1024)?;

    // 3. 解析版本元数据
    Protocol::phase(protocol::phase::RESOLVING_VERSION, "正在解析版本元数据");
    let version = fetch_vanilla_version(version_id).map_err(|e| {
        LauncherError::VersionNotFound(format!("{}: {}", version_id, e))
    })?;

    // 4. 写入版本 JSON
    write_version_json(minecraft_dir, &version).map_err(|e| {
        LauncherError::Internal(format!("写入版本 JSON 失败: {}", e))
    })?;

    // 5. 下载所有文件
    Protocol::phase(protocol::phase::DOWNLOADING_LIBRARIES, "正在下载游戏文件");
    let downloader = Downloader::new(mirror);
    downloader.download_version(&version, minecraft_dir).await?;

    // 6. 校验完成
    Protocol::phase(protocol::phase::VERIFYING, "安装完成");
    Protocol::success(serde_json::json!({
        "version_id": version_id,
        "message": "安装成功"
    }));

    Ok(version_id.to_string())
}

/// 启动 Minecraft
///
/// 流程：
/// 1. 加载版本 JSON
/// 2. 检测系统 Java，选择合适版本
/// 3. 构建启动命令
/// 4. 以 detach 模式启动游戏
pub fn launch_game(
    minecraft_dir: &Path,
    version_id: &str,
    username: &str,
    java_path: Option<PathBuf>,
    max_memory_mb: Option<u32>,
    detach: bool,
) -> Result<u32> {
    // 1. 加载版本 JSON
    Protocol::phase(protocol::phase::PREPARING, "正在准备启动");
    let version = load_version_json(minecraft_dir, version_id).map_err(|e| {
        LauncherError::VersionNotFound(format!("{}: {}", version_id, e))
    })?;

    // 2. 选择 Java
    let java_executable = match java_path {
        Some(path) => {
            if !path.is_file() {
                return Err(LauncherError::JavaNotFound { required: 0 });
            }
            path
        }
        None => {
            let required_java = mc_version_to_java(version_id)?;
            let registry = JavaRegistry::detect();
            match registry.find(required_java) {
                Some(rt) => rt.executable.clone(),
                None => {
                    return Err(LauncherError::JavaNotFound {
                        required: required_java,
                    });
                }
            }
        }
    };

    // 3. 构建启动命令
    Protocol::phase(protocol::phase::LAUNCHING, "正在启动游戏");
    let mut params = LaunchParams::offline(username, java_executable);
    params.max_memory_mb = max_memory_mb;
    let command = build_command(&version, minecraft_dir, &params)?;

    // 4. 启动
    if detach {
        let pid = spawn_detached(&command, version_id)?;
        Protocol::success(serde_json::json!({
            "pid": pid,
            "version_id": version_id,
            "mode": "detached"
        }));
        Ok(pid)
    } else {
        use crate::launch::GameProcess;
        let process = GameProcess::spawn(&command, version_id)?;
        let pid = process.id();
        Protocol::success(serde_json::json!({
            "pid": pid,
            "version_id": version_id,
            "mode": "foreground"
        }));
        // 前台模式等待退出
        let status = process.wait()?;
        if !status.success() {
            return Err(LauncherError::Internal(format!(
                "游戏异常退出，code={:?}",
                status.code()
            )));
        }
        Ok(pid)
    }
}

/// 列出已安装的版本
pub fn list_installed_versions(minecraft_dir: &Path) -> Result<Vec<String>> {
    let versions_dir = minecraft_dir.join("versions");
    if !versions_dir.is_dir() {
        return Ok(Vec::new());
    }

    let mut versions = Vec::new();
    for entry in std::fs::read_dir(&versions_dir).map_err(|e| {
        LauncherError::Internal(format!("读取 versions 目录失败: {}", e))
    })? {
        let entry = entry.map_err(|e| LauncherError::Internal(format!("读取目录项失败: {}", e)))?;
        let path = entry.path();
        if path.is_dir() {
            let version_id = path.file_name().unwrap().to_string_lossy().to_string();
            // 检查是否有 version JSON
            let json_path = path.join(format!("{}.json", version_id));
            if json_path.is_file() {
                versions.push(version_id);
            }
        }
    }

    versions.sort();
    Ok(versions)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_list_installed_empty_dir() {
        let dir = std::env::temp_dir().join("mpack-test-empty");
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let versions = list_installed_versions(&dir).unwrap();
        assert!(versions.is_empty());
        let _ = std::fs::remove_dir_all(&dir);
    }
}
