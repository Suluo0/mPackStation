use serde_json::{json, Value};
use std::io::{self, Write};

use crate::error::LauncherError;

/// stdout JSON 事件协议输出
///
/// 与日志（tracing → stderr）完全分离。
/// 只有两种事件：phase（阶段变化）和 result（最终结果）。
pub struct Protocol;

impl Protocol {
    /// 输出阶段变化事件
    pub fn phase(phase: &str, message: &str) {
        let event = json!({
            "type": "phase",
            "phase": phase,
            "message": message,
        });
        Self::emit(&event);
    }

    /// 输出成功结果
    pub fn success(data: Value) {
        let event = json!({
            "type": "result",
            "success": true,
            "data": data,
        });
        Self::emit(&event);
    }

    /// 输出失败结果
    pub fn failure(error: &LauncherError) {
        let mut event = json!({
            "type": "result",
            "success": false,
            "error": error.error_type(),
            "message": error.to_string(),
        });
        if let Some(s) = error.suggestion() {
            event["suggestion"] = json!(s);
        }
        Self::emit(&event);
    }

    fn emit(event: &Value) {
        let stdout = io::stdout();
        let mut lock = stdout.lock();
        // stdout 写入失败意味着进程已无法继续输出，直接忽略
        let _ = writeln!(lock, "{}", serde_json::to_string(event).unwrap_or_default());
    }
}

/// 阶段标识常量
pub mod phase {
    pub const RESOLVING_VERSION: &str = "resolving_version";
    pub const DOWNLOADING_LIBRARIES: &str = "downloading_libraries";
    pub const DOWNLOADING_ASSETS: &str = "downloading_assets";
    pub const INSTALLING_LOADER: &str = "installing_loader";
    pub const VERIFYING: &str = "verifying";
    pub const PREPARING: &str = "preparing";
    pub const LAUNCHING: &str = "launching";
}
