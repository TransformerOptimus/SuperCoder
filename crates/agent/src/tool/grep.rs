use async_trait::async_trait;
use serde_json::{json, Value};
use tokio::process::Command;

use crate::error::ToolError;
use crate::util::{resolve_path, truncate_str};
use super::{Tool, ToolContext, ToolResult};

const MAX_MATCHES: usize = 100;
const MAX_LINE_LENGTH: usize = 2000;

pub struct GrepTool;

#[async_trait]
impl Tool for GrepTool {
    fn name(&self) -> &str {
        "grep"
    }

    fn description(&self) -> &str {
        "Search file contents using regex patterns. \
         Returns matching lines with file paths and line numbers. \
         Use the 'include' parameter to filter by file type (e.g. '*.rs')."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["pattern"],
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Regex pattern to search for"
                },
                "path": {
                    "type": "string",
                    "description": "File or directory to search in (defaults to working directory)"
                },
                "include": {
                    "type": "string",
                    "description": "Glob pattern to filter files (e.g. '*.rs', '*.{ts,tsx}')"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let pattern = args
            .get("pattern")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: pattern".into()))?;

        let search_path = match args.get("path").and_then(|v| v.as_str()) {
            Some(p) => resolve_path(&ctx.working_dir, p),
            None => ctx.working_dir.clone(),
        };

        let include = args.get("include").and_then(|v| v.as_str());

        // Try rg first, fall back to grep
        let output = if rg_available() {
            run_rg(pattern, &search_path, include, &ctx.working_dir).await
        } else {
            run_grep(pattern, &search_path, include, &ctx.working_dir).await
        };

        match output {
            Ok(output) => {
                let exit_code = output.status.code().unwrap_or(-1);

                match exit_code {
                    0 => {
                        let stdout = String::from_utf8_lossy(&output.stdout);
                        let formatted = format_matches(&stdout);
                        Ok(ToolResult::success(formatted))
                    }
                    1 => {
                        // No matches (not an error for both rg and grep)
                        Ok(ToolResult::success(format!("No matches found for pattern '{pattern}'")))
                    }
                    _ => {
                        let stderr = String::from_utf8_lossy(&output.stderr);
                        Ok(ToolResult::error(format!("Grep error (exit code {exit_code}): {stderr}")))
                    }
                }
            }
            Err(e) => {
                Ok(ToolResult::error(format!("Failed to execute grep: {e}")))
            }
        }
    }
}

fn rg_available() -> bool {
    let mut cmd = std::process::Command::new("rg");
    cmd.arg("--version");
    git_ops::no_window::no_window_std(&mut cmd);
    cmd.output().map(|o| o.status.success()).unwrap_or(false)
}

async fn run_rg(
    pattern: &str,
    search_path: &std::path::Path,
    include: Option<&str>,
    working_dir: &std::path::Path,
) -> std::io::Result<std::process::Output> {
    let mut cmd = Command::new("rg");
    cmd.arg("-n")
        .arg("-H")
        .arg("--color").arg("never")
        .arg("--no-heading");

    if let Some(glob) = include {
        cmd.arg("--glob").arg(glob);
    }

    cmd.arg(pattern)
        .arg(search_path)
        .current_dir(working_dir)
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped())
        .kill_on_drop(true);
    git_ops::no_window::no_window_tokio(&mut cmd);

    cmd.output().await
}

async fn run_grep(
    pattern: &str,
    search_path: &std::path::Path,
    include: Option<&str>,
    working_dir: &std::path::Path,
) -> std::io::Result<std::process::Output> {
    let mut cmd = Command::new("grep");
    cmd.arg("-r")     // recursive
        .arg("-n")    // line numbers
        .arg("-H")    // always show filename
        .arg("-E");   // extended regex (closer to rg default)

    if let Some(glob) = include {
        cmd.arg("--include").arg(glob);
    }

    cmd.arg(pattern)
        .arg(search_path)
        .current_dir(working_dir)
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped())
        .kill_on_drop(true);
    git_ops::no_window::no_window_tokio(&mut cmd);

    cmd.output().await
}

fn format_matches(stdout: &str) -> String {
    let lines: Vec<&str> = stdout.lines().collect();
    let total = lines.len();

    let mut output = String::new();

    for (count, line) in lines.iter().enumerate() {
        if count >= MAX_MATCHES {
            break;
        }

        // Truncate long content lines (use truncate_str for UTF-8 safety)
        if line.len() > MAX_LINE_LENGTH {
            output.push_str(truncate_str(line, MAX_LINE_LENGTH));
            output.push_str("...");
        } else {
            output.push_str(line);
        }
        output.push('\n');
    }

    // Remove trailing newline
    if output.ends_with('\n') {
        output.pop();
    }

    if total > MAX_MATCHES {
        output.push_str(&format!("\n\nShowing {MAX_MATCHES} of {total} matches"));
    }

    output
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use std::fs;

    #[tokio::test]
    async fn test_grep_simple_regex() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("test.rs"), "fn main() {\n    println!(\"hello\");\n}\n").unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "fn main" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("fn main"));
        assert!(result.output.contains("test.rs"));
    }

    #[tokio::test]
    async fn test_grep_include_filter() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("code.rs"), "fn hello() {}\n").unwrap();
        fs::write(dir.path().join("code.py"), "def hello():\n    pass\n").unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "hello", "include": "*.rs" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("code.rs"));
        assert!(!result.output.contains("code.py"));
    }

    #[tokio::test]
    async fn test_grep_no_matches() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("test.txt"), "hello world\n").unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "nonexistent_pattern_xyz" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("No matches found"));
    }

    #[tokio::test]
    async fn test_grep_long_line_truncation() {
        let dir = tempdir().unwrap();
        let long_line = format!("match_here {}", "x".repeat(3000));
        fs::write(dir.path().join("long.txt"), &long_line).unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "match_here" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        for line in result.output.lines() {
            assert!(line.len() <= MAX_LINE_LENGTH + 10, "Line too long: {} chars", line.len());
        }
    }

    #[tokio::test]
    async fn test_grep_many_matches_truncated() {
        let dir = tempdir().unwrap();
        let content: String = (0..150).map(|i| format!("match_line_{i}\n")).collect();
        fs::write(dir.path().join("many.txt"), &content).unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "match_line" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Showing 100 of 150 matches"));
    }

    #[tokio::test]
    async fn test_grep_error_bad_regex() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("test.txt"), "hello\n").unwrap();

        let tool = GrepTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "[unclosed" }), &ctx)
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("Grep error") || result.output.contains("Invalid")
            || result.output.contains("Unmatched"));
    }

    #[test]
    fn test_format_matches_basic() {
        let stdout = "file.rs:1:fn main()\nfile.rs:5:fn test()\n";
        let result = format_matches(stdout);
        assert!(result.contains("file.rs:1:fn main()"));
        assert!(result.contains("file.rs:5:fn test()"));
    }

    #[test]
    fn test_format_matches_utf8_truncation_no_panic() {
        let prefix = "x".repeat(MAX_LINE_LENGTH - 2);
        let line = format!("file.txt:1:{prefix}🌍yyyy");
        let result = format_matches(&line);
        assert!(result.contains("..."));
        assert!(result.len() <= MAX_LINE_LENGTH + 20);
    }
}
