use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::util::resolve_path;
use super::{Tool, ToolContext, ToolResult};

pub struct WriteTool;

#[async_trait]
impl Tool for WriteTool {
    fn name(&self) -> &str {
        "write"
    }

    fn description(&self) -> &str {
        "Write content to a file. Creates the file and any parent directories if they don't exist. \
         Overwrites the file if it already exists."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["filePath", "content"],
            "properties": {
                "filePath": {
                    "type": "string",
                    "description": "Absolute or relative path to the file to write"
                },
                "content": {
                    "type": "string",
                    "description": "The content to write to the file"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let file_path = args
            .get("filePath")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: filePath".into()))?;

        let content = args
            .get("content")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: content".into()))?;

        let path = resolve_path(&ctx.working_dir, file_path);

        // Note: path traversal check (outside working dir) is handled by the agent loop
        // before execute() is called, with user approval flow.

        // Create parent directories if needed
        if let Some(parent) = path.parent() {
            tokio::fs::create_dir_all(parent)
                .await
                .map_err(|e| ToolError(format!("Failed to create parent directories: {e}")))?;
        }

        // Back up the file's prior contents before overwriting (per-turn undo).
        ctx.checkpoint(&path).await;

        let byte_count = content.len();
        tokio::fs::write(&path, content)
            .await
            .map_err(|e| ToolError(format!("Failed to write file: {e}")))?;

        Ok(ToolResult::success(format!("Wrote {byte_count} bytes to {}", path.display())))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use tokio::sync::mpsc;
    use tokio_util::sync::CancellationToken;

    /// End-to-end: a `write` then `edit` through the real tool path with a checkpoint
    /// dir set must back up prior contents so `restore_to` reverts the agent's edits.
    #[tokio::test]
    async fn test_write_edit_then_restore_roundtrip() {
        let proj = tempdir().unwrap();
        let ckpt = tempdir().unwrap();
        let file = proj.path().join("src/lib.rs");
        std::fs::create_dir_all(file.parent().unwrap()).unwrap();
        std::fs::write(&file, "original\n").unwrap();

        let (tx, _rx) = mpsc::channel(32);
        let ctx = ToolContext {
            working_dir: proj.path().to_path_buf(),
            cancel_token: CancellationToken::new(),
            event_tx: tx,
            session_id: "sess".into(),
            tool_call_id: "tc_1".into(),
            checkpoint_dir: Some(ckpt.path().to_path_buf()),
            checkpoint_turn: 1,
        };

        // write overwrites the file (backs up "original\n") ...
        WriteTool
            .execute(json!({ "filePath": "src/lib.rs", "content": "rewritten\n" }), &ctx)
            .await
            .unwrap();
        // ... and edit mutates it again within the same turn (no-op backup; first wins).
        crate::tool::edit::EditTool
            .execute(
                json!({ "filePath": "src/lib.rs", "oldString": "rewritten", "newString": "edited" }),
                &ctx,
            )
            .await
            .unwrap();
        assert_eq!(std::fs::read_to_string(&file).unwrap(), "edited\n");

        // Restore to before turn 1 -> back to the turn's starting contents.
        git_ops::restore_to(ckpt.path(), "sess", 0, proj.path()).await.unwrap();
        assert_eq!(std::fs::read_to_string(&file).unwrap(), "original\n");
    }

    #[tokio::test]
    async fn test_write_new_file() {
        let dir = tempdir().unwrap();
        let tool = WriteTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "filePath": "hello.txt", "content": "hello world" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("11 bytes"));

        let written = std::fs::read_to_string(dir.path().join("hello.txt")).unwrap();
        assert_eq!(written, "hello world");
    }

    #[tokio::test]
    async fn test_write_creates_parent_dirs() {
        let dir = tempdir().unwrap();
        let tool = WriteTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "filePath": "a/b/c/deep.txt", "content": "deep content" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        let written = std::fs::read_to_string(dir.path().join("a/b/c/deep.txt")).unwrap();
        assert_eq!(written, "deep content");
    }

    #[tokio::test]
    async fn test_write_overwrites_existing() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("existing.txt");
        std::fs::write(&file_path, "old content").unwrap();

        let tool = WriteTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({ "filePath": file_path.to_str().unwrap(), "content": "new content" }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        let written = std::fs::read_to_string(&file_path).unwrap();
        assert_eq!(written, "new content");
    }
}
