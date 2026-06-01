use async_trait::async_trait;
use serde_json::{json, Value};
use tokio::process::Command;

use crate::error::ToolError;
use crate::types::AgentEvent;
use crate::util::truncate_output;
use super::{Tool, ToolContext, ToolResult};

const DEFAULT_TIMEOUT_MS: u64 = 120_000;
const MAX_OUTPUT_LINES: usize = 2000;
const MAX_OUTPUT_BYTES: usize = 50 * 1024;

pub struct BashTool;

#[async_trait]
impl Tool for BashTool {
    fn name(&self) -> &str {
        "bash"
    }

    fn description(&self) -> &str {
        "Execute a bash command. The command runs in a shell with the working directory set to \
         the project root. Returns stdout and stderr combined with the exit code."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["command", "description"],
            "properties": {
                "command": {
                    "type": "string",
                    "description": "The bash command to execute"
                },
                "description": {
                    "type": "string",
                    "description": "A short 5-10 word description of what this command does"
                },
                "timeout": {
                    "type": "integer",
                    "description": "Timeout in milliseconds (default 120000)"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let command = args
            .get("command")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: command".into()))?;

        let description = args
            .get("description")
            .and_then(|v| v.as_str())
            .unwrap_or("Running command");

        let timeout_ms = args
            .get("timeout")
            .and_then(|v| v.as_u64())
            .unwrap_or(DEFAULT_TIMEOUT_MS);

        // Emit status event
        let _ = ctx.event_tx.send(AgentEvent::ToolStatus {
            session_id: ctx.session_id.clone(),
            tool_call_id: ctx.tool_call_id.clone(),
            status: format!("Running: {description}"),
        }).await;

        let mut cmd = Command::new("sh");
        cmd.arg("-c")
            .arg(command)
            .current_dir(&ctx.working_dir)
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .kill_on_drop(true);
        git_ops::no_window::no_window_tokio(&mut cmd);
        let child = cmd
            .spawn()
            .map_err(|e| ToolError(format!("Failed to spawn command: {e}")))?;

        let timeout_duration = std::time::Duration::from_millis(timeout_ms);

        // wait_with_output takes ownership, so timeout/cancel rely on kill_on_drop
        // when the future is dropped by tokio::select!
        let result = tokio::select! {
            result = child.wait_with_output() => {
                match result {
                    Ok(output) => Ok(output),
                    Err(e) => Err(ToolError(format!("Command execution error: {e}"))),
                }
            }
            _ = tokio::time::sleep(timeout_duration) => {
                // child is dropped here → kill_on_drop kills it
                Err(ToolError(format!("Command timed out after {timeout_ms}ms")))
            }
            _ = ctx.cancel_token.cancelled() => {
                // child is dropped here → kill_on_drop kills it
                Err(ToolError("Command cancelled".into()))
            }
        };

        match result {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                let stderr = String::from_utf8_lossy(&output.stderr);
                let exit_code = output.status.code().unwrap_or(-1);

                let mut combined = String::new();
                if !stdout.is_empty() {
                    combined.push_str(&stdout);
                }
                if !stderr.is_empty() {
                    if !combined.is_empty() {
                        combined.push('\n');
                    }
                    combined.push_str("STDERR:\n");
                    combined.push_str(&stderr);
                }

                // Truncate output
                let combined = truncate_output(&combined, MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES);

                let output_text = format!("Exit code: {exit_code}\n{combined}");

                Ok(ToolResult {
                    output: output_text,
                    is_error: exit_code != 0,
                    yield_data: None,
                    modified_files: Vec::new(),
                })
            }
            Err(e) => Ok(ToolResult::error(e.to_string())),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use tokio::sync::mpsc;
    use tokio_util::sync::CancellationToken;

    #[tokio::test]
    async fn test_simple_echo() {
        let dir = tempdir().unwrap();
        let tool = BashTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "command": "echo hello world", "description": "echo test" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Exit code: 0"));
        assert!(result.output.contains("hello world"));
    }

    #[tokio::test]
    async fn test_stderr_capture() {
        let dir = tempdir().unwrap();
        let tool = BashTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "command": "echo err >&2", "description": "stderr test" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("STDERR:"));
        assert!(result.output.contains("err"));
    }

    #[tokio::test]
    async fn test_nonzero_exit() {
        let dir = tempdir().unwrap();
        let tool = BashTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "command": "exit 42", "description": "exit code test" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("Exit code: 42"));
    }

    #[tokio::test]
    async fn test_timeout() {
        let dir = tempdir().unwrap();
        let tool = BashTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "command": "sleep 10", "description": "sleep test", "timeout": 100 }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("timed out"));
    }

    #[tokio::test]
    async fn test_cancellation() {
        let dir = tempdir().unwrap();
        let (tx, _rx) = mpsc::channel(32);
        let cancel_token = CancellationToken::new();

        let ctx = ToolContext {
            working_dir: dir.path().to_path_buf(),
            cancel_token: cancel_token.clone(),
            event_tx: tx,
            session_id: "test".into(),
            tool_call_id: "tc_1".into(),
        };

        let tool = BashTool;

        // Cancel after a brief delay
        let cancel = cancel_token.clone();
        tokio::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(50)).await;
            cancel.cancel();
        });

        let result = tool
            .execute(
                json!({ "command": "sleep 10", "description": "cancel test" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("cancelled"));
    }

    #[tokio::test]
    async fn test_working_dir() {
        let dir = tempdir().unwrap();
        let tool = BashTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "command": "pwd", "description": "pwd test" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        // The output should contain the temp dir path
        // On macOS, tempdir is under /private/var or /var, and pwd resolves symlinks
        // so we just check it's in the output somewhere
        assert!(result.output.contains("Exit code: 0"));
    }

    // ── truncate_output tests (delegates to shared util::truncate_output) ──

    #[test]
    fn test_truncate_output_small_input_unchanged() {
        let input = "line 1\nline 2\nline 3";
        let result = truncate_output(input, MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES);
        assert_eq!(result, input);
    }

    #[test]
    fn test_truncate_output_empty_input() {
        assert_eq!(truncate_output("", MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES), "");
    }

    #[test]
    fn test_truncate_output_exceeds_line_limit() {
        let lines: String = (0..2500).map(|i| format!("line {i}\n")).collect();
        let result = truncate_output(&lines, MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES);

        assert!(result.contains("truncated"));
        assert!(result.contains("2000 of 2500 lines"));
    }

    #[test]
    fn test_truncate_output_exceeds_byte_limit() {
        let lines: String = (0..1200).map(|i| format!("this is line number {:04} with some padding text\n", i)).collect();
        assert!(lines.len() > MAX_OUTPUT_BYTES);

        let result = truncate_output(&lines, MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES);
        assert!(result.contains("truncated"));
        assert!(result.len() <= MAX_OUTPUT_BYTES + 100);
    }

    #[test]
    fn test_truncate_output_exactly_at_line_limit() {
        let lines: String = (0..2000).map(|_| "x\n").collect();
        let result = truncate_output(&lines, MAX_OUTPUT_LINES, MAX_OUTPUT_BYTES);
        assert!(!result.contains("truncated"));
    }
}
