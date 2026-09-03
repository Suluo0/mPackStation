use serde_json::{json, Value};
use std::io::{self, Write};

use crate::error::LauncherError;

/// stdout JSON 事件协议输出
///
/// 与日志（tracing → stderr）完全分离。
/// 只有两种事件：phase（阶段变化）和 result（最终结果）。
pub struct Protocol;

impl Protocol {
    /// 输出阶段变化事件到 stdout
    pub fn phase(phase: &str, message: &str) {
        let event = json!({
            "type": "phase",
            "phase": phase,
            "message": message,
        });
        Self::emit_to(&event, &mut io::stdout().lock());
    }

    /// 输出成功结果到 stdout
    pub fn success(data: Value) {
        let event = json!({
            "type": "result",
            "success": true,
            "data": data,
        });
        Self::emit_to(&event, &mut io::stdout().lock());
    }

    /// 输出失败结果到 stdout
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
        Self::emit_to(&event, &mut io::stdout().lock());
    }

    /// 写入事件到指定 writer（用于测试捕获）
    pub fn emit_to<W: Write>(event: &Value, writer: &mut W) {
        let _ = writeln!(writer, "{}", serde_json::to_string(event).unwrap_or_default());
    }

    /// 构建 phase 事件（不输出，用于测试）
    pub fn build_phase(phase: &str, message: &str) -> Value {
        json!({
            "type": "phase",
            "phase": phase,
            "message": message,
        })
    }

    /// 构建 success 事件（不输出，用于测试）
    pub fn build_success(data: Value) -> Value {
        json!({
            "type": "result",
            "success": true,
            "data": data,
        })
    }

    /// 构建 failure 事件（不输出，用于测试）
    pub fn build_failure(error: &LauncherError) -> Value {
        let mut event = json!({
            "type": "result",
            "success": false,
            "error": error.error_type(),
            "message": error.to_string(),
        });
        if let Some(s) = error.suggestion() {
            event["suggestion"] = json!(s);
        }
        event
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
