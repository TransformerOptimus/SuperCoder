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
