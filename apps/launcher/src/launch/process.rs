//! 游戏进程管理：spawn + detach + 等待 + 终止

use std::path::Path;
use std::process::{Child, Command, Stdio};

use mc_launcher_core::command::builder::LaunchCommand;

use crate::error::LauncherError;
use crate::Result;

/// 运行中的游戏进程
pub struct GameProcess {
    child: Child,
    version_id: String,
}

impl GameProcess {
    /// 启动游戏进程
    pub fn spawn(command: &LaunchCommand, version_id: impl Into<String>) -> Result<Self> {
        let mut cmd = Command::new(&command.executable);
        cmd.args(&command.args)
            .current_dir(&command.working_dir)
            .stdin(Stdio::null());

        // 设置环境变量
        for (key, value) in &command.env {
            cmd.env(key, value);
        }

        let child = cmd
            .spawn()
            .map_err(|e| LauncherError::Internal(format!("启动游戏进程失败: {}", e)))?;

        Ok(Self {
            child,
            version_id: version_id.into(),
        })
    }

    /// 获取进程 ID
    pub fn id(&self) -> u32 {
        self.child.id()
    }

    /// 获取版本 ID
    pub fn version_id(&self) -> &str {
        &self.version_id
    }

    /// 等待进程退出（阻塞）
    pub fn wait(mut self) -> Result<std::process::ExitStatus> {
        let status = self
            .child
            .wait()
            .map_err(|e| LauncherError::Internal(format!("等待进程失败: {}", e)))?;
        Ok(status)
    }

    /// 非阻塞检查进程是否仍在运行
    pub fn try_wait(&mut self) -> Result<Option<std::process::ExitStatus>> {
        let status = self
            .child
            .try_wait()
            .map_err(|e| LauncherError::Internal(format!("检查进程状态失败: {}", e)))?;
        Ok(status)
    }

    /// 终止进程
    pub fn kill(mut self) -> Result<()> {
        self.child
            .kill()
            .map_err(|e| LauncherError::Internal(format!("终止进程失败: {}", e)))?;
        Ok(())
    }
}

/// 以 detach 模式启动游戏（父进程退出后游戏继续运行）
///
/// Windows 上通过 CREATE_NEW_PROCESS_GROUP 实现；
/// Unix 上通过 setsid 实现（需要额外处理，当前使用 std::process 基本 detach）。
pub fn spawn_detached(command: &LaunchCommand, version_id: impl Into<String>) -> Result<u32> {
    let mut cmd = Command::new(&command.executable);
    cmd.args(&command.args)
        .current_dir(&command.working_dir)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());

    for (key, value) in &command.env {
        cmd.env(key, value);
    }

    // Windows: 创建新进程组，实现 detach
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NEW_PROCESS_GROUP: u32 = 0x00000200;
        const DETACHED_PROCESS: u32 = 0x00000008;
        cmd.creation_flags(CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS);
    }

    let child = cmd
        .spawn()
        .map_err(|e| LauncherError::Internal(format!("启动游戏进程失败: {}", e)))?;

    let pid = child.id();
    // 不等待，直接返回 PID（detach 模式）
    std::mem::forget(child);

    tracing::info!("游戏已 detach 启动，PID={}, version={}", pid, version_id.into());
    Ok(pid)
}

/// 检查指定 PID 的进程是否仍在运行
pub fn is_process_running(pid: u32) -> bool {
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        let output = Command::new("tasklist")
            .args(["/FI", &format!("PID eq {}", pid), "/NH"])
            .output();
        match output {
            Ok(o) => String::from_utf8_lossy(&o.stdout).contains(&pid.to_string()),
            Err(_) => false,
        }
    }
    #[cfg(unix)]
    {
        // 发送信号 0 检查进程是否存在
        unsafe { libc_kill(pid as i32, 0) == 0 }
    }
    #[cfg(not(any(windows, unix)))]
    {
        false
    }
}

#[cfg(unix)]
extern "C" {
    fn libc_kill(pid: i32, sig: i32) -> i32;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_process_running_invalid_pid() {
        // 一个不可能存在的 PID
        assert!(!is_process_running(99999));
    }
}
