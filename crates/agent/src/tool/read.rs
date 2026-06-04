use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::util::{resolve_path, truncate_str};
use super::{Tool, ToolContext, ToolResult};

const DEFAULT_LIMIT: usize = 2000;
const MAX_LINE_LENGTH: usize = 2000;
const MAX_BYTES: usize = 50 * 1024;

pub struct ReadTool;

#[async_trait]
impl Tool for ReadTool {
    fn name(&self) -> &str {
        "read"
    }

    fn description(&self) -> &str {
        "Read a file's contents or list a directory. Returns line-numbered output for files. \
         Supports offset and limit parameters for reading specific sections of large files."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["filePath"],
            "properties": {
                "filePath": {
                    "type": "string",
                    "description": "Absolute or relative path to the file or directory to read"
                },
                "offset": {
                    "type": "integer",
                    "description": "1-indexed line number to start reading from"
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of lines to read (default 2000)"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let file_path = args
            .get("filePath")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: filePath".into()))?;

        let offset = args.get("offset").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
        let limit = args.get("limit").and_then(|v| v.as_u64()).unwrap_or(DEFAULT_LIMIT as u64) as usize;

        let path = resolve_path(&ctx.working_dir, file_path);

        if !path.exists() {
            return Ok(ToolResult::error(format!("Error: Path does not exist: {}", path.display())));
        }

        if path.is_dir() {
            return read_directory(&path, offset, limit).await;
        }

        read_file(&path, offset, limit).await
    }
}

async fn read_directory(path: &std::path::Path, offset: usize, limit: usize) -> Result<ToolResult, ToolError> {
    let mut entries: Vec<String> = Vec::new();

    let mut read_dir = tokio::fs::read_dir(path)
        .await
        .map_err(|e| ToolError(format!("Failed to read directory: {e}")))?;

    while let Some(entry) = read_dir.next_entry().await.map_err(|e| ToolError(format!("Error reading entry: {e}")))? {
        let name = entry.file_name().to_string_lossy().to_string();
        let metadata = entry.metadata().await.ok();
        let suffix = if metadata.as_ref().is_some_and(|m| m.is_dir()) {
            "/"
        } else {
            ""
        };
        entries.push(format!("{name}{suffix}"));
    }

    entries.sort();

    // Apply offset and limit
    let start = if offset > 0 { offset - 1 } else { 0 };
    let listing: String = entries
        .iter()
        .skip(start)
        .take(limit)
        .map(|e| e.as_str())
        .collect::<Vec<_>>()
        .join("\n");

    let output = format!("Directory: {}\n\n{}", path.display(), listing);

    Ok(ToolResult::success(output))
}

async fn read_file(path: &std::path::Path, offset: usize, limit: usize) -> Result<ToolResult, ToolError> {
    // Read raw bytes for binary detection
    let raw_bytes = tokio::fs::read(path)
        .await
        .map_err(|e| ToolError(format!("Failed to read file: {e}")))?;

    // Binary detection: check for null bytes in first 4KB
    let check_len = raw_bytes.len().min(4096);
    if raw_bytes[..check_len].contains(&0) {
        return Ok(ToolResult::error(format!("Error: {} appears to be a binary file", path.display())));
    }

    let content = String::from_utf8_lossy(&raw_bytes);
    let lines: Vec<&str> = content.lines().collect();

    // Apply offset (1-indexed) and limit
    let start = if offset > 0 { (offset - 1).min(lines.len()) } else { 0 };
    let end = (start + limit).min(lines.len());
    let selected_lines = &lines[start..end];

    // Build line-numbered output, respecting MAX_BYTES
    let mut output = String::new();
    let mut total_bytes = 0;

    for (i, line) in selected_lines.iter().enumerate() {
        let line_num = start + i + 1;
        let truncated_line = if line.len() > MAX_LINE_LENGTH {
            truncate_str(line, MAX_LINE_LENGTH)
        } else {
            line
        };
        let formatted = format!("{line_num}\t{truncated_line}\n");

        total_bytes += formatted.len();
        if total_bytes > MAX_BYTES {
            output.push_str(&format!(
                "\n... truncated (file too large, showing {i} of {} selected lines)",
                selected_lines.len()
            ));
            break;
        }

        output.push_str(&formatted);
    }

    // Add info about total lines if we're showing a subset
    if start > 0 || end < lines.len() {
        output.push_str(&format!(
            "\n(showing lines {}-{} of {} total)",
            start + 1,
            end,
            lines.len()
        ));
    }

    Ok(ToolResult::success(output))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_read_text_file() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("test.txt");
        std::fs::write(&file_path, "line one\nline two\nline three\n").unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("1\tline one"));
        assert!(result.output.contains("2\tline two"));
        assert!(result.output.contains("3\tline three"));
    }

    #[tokio::test]
    async fn test_read_with_offset_and_limit() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("test.txt");
        let content: String = (1..=100).map(|i| format!("line {i}\n")).collect();
        std::fs::write(&file_path, &content).unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(
                json!({ "filePath": file_path.to_str().unwrap(), "offset": 10, "limit": 5 }),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("10\tline 10"));
        assert!(result.output.contains("14\tline 14"));
        assert!(!result.output.contains("15\tline 15"));
    }

    #[tokio::test]
    async fn test_read_directory() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("alpha.txt"), "").unwrap();
        std::fs::write(dir.path().join("beta.txt"), "").unwrap();
        std::fs::create_dir(dir.path().join("subdir")).unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": dir.path().to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("alpha.txt"));
        assert!(result.output.contains("beta.txt"));
        assert!(result.output.contains("subdir/"));
    }

    #[tokio::test]
    async fn test_read_binary_file() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("binary.bin");
        std::fs::write(&file_path, b"\x00\x01\x02\x03binary data").unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("binary file"));
    }

    #[tokio::test]
    async fn test_read_nonexistent_file() {
        let dir = tempdir().unwrap();
        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": "/nonexistent/file.txt" }), &ctx)
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("does not exist"));
    }

    #[tokio::test]
    async fn test_read_line_truncation() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("long.txt");
        let long_line = "x".repeat(3000);
        std::fs::write(&file_path, &long_line).unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        // The line should be truncated at MAX_LINE_LENGTH
        // Line format: "1\t" + content, so content portion should be <= 2000
        let line = result.output.lines().next().unwrap();
        let content_part = line.splitn(2, '\t').nth(1).unwrap();
        assert_eq!(content_part.len(), MAX_LINE_LENGTH);
    }

    #[tokio::test]
    async fn test_read_relative_path() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("relative.txt");
        std::fs::write(&file_path, "content here").unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": "relative.txt" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("content here"));
    }

    #[tokio::test]
    async fn test_read_max_bytes_truncation() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("large.txt");
        // Create a file larger than MAX_BYTES (50KB)
        // Each line is ~80 chars, so ~700 lines will exceed 50KB
        let content: String = (0..800)
            .map(|i| format!("this is line {:04} with enough text to make it around eighty characters long xxxx\n", i))
            .collect();
        assert!(content.len() > MAX_BYTES);
        std::fs::write(&file_path, &content).unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("truncated"));
        // Output should be roughly bounded by MAX_BYTES
        assert!(result.output.len() <= MAX_BYTES + 200);
    }

    #[tokio::test]
    async fn test_read_empty_file() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("empty.txt");
        std::fs::write(&file_path, "").unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
    }

    #[tokio::test]
    async fn test_read_unicode_line_truncation() {
        let dir = tempdir().unwrap();
        let file_path = dir.path().join("unicode.txt");
        // Create a line of emoji that exceeds MAX_LINE_LENGTH in bytes
        // '🌍' is 4 bytes, so 600 emojis = 2400 bytes > 2000
        let emoji_line: String = "🌍".repeat(600);
        std::fs::write(&file_path, &emoji_line).unwrap();

        let tool = ReadTool;
        let ctx = ToolContext::test_context(dir.path());
        let result = tool
            .execute(json!({ "filePath": file_path.to_str().unwrap() }), &ctx)
            .await
            .unwrap();

        // Should not panic (the whole point of the truncate_str fix)
        assert!(!result.is_error);
        // Line should be truncated — fewer than 600 emoji in the output
        let output_emoji_count = result.output.matches('🌍').count();
        assert!(output_emoji_count < 600);
        assert!(output_emoji_count == MAX_LINE_LENGTH / 4); // 500 emoji at 4 bytes each
    }
}
