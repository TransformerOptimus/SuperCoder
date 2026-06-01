use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::util::resolve_path;
use super::{Tool, ToolContext, ToolResult};

const MAX_RESULTS: usize = 100;

pub struct GlobTool;

#[async_trait]
impl Tool for GlobTool {
    fn name(&self) -> &str {
        "glob"
    }

    fn description(&self) -> &str {
        "Find files matching a glob pattern. Respects .gitignore. \
         Returns absolute paths sorted by modification time (most recent first). \
         Example patterns: '**/*.rs', 'src/**/*.ts', '*.json'"
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["pattern"],
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Glob pattern to match files against (e.g. '**/*.rs')"
                },
                "path": {
                    "type": "string",
                    "description": "Base directory to search in (defaults to working directory)"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let pattern = args
            .get("pattern")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: pattern".into()))?;

        let base_path = match args.get("path").and_then(|v| v.as_str()) {
            Some(p) => resolve_path(&ctx.working_dir, p),
            None => ctx.working_dir.clone(),
        };

        let pattern = pattern.to_string();
        let base = base_path.clone();

        let result = tokio::task::spawn_blocking(move || {
            glob_search(&base, &pattern)
        })
        .await
        .map_err(|e| ToolError(format!("Glob task failed: {e}")))?;

        match result {
            Ok(output) => Ok(ToolResult::success(output)),
            Err(e) => Ok(ToolResult::error(e)),
        }
    }
}

fn glob_search(base_path: &std::path::Path, pattern: &str) -> Result<String, String> {
    let matcher = globset::GlobBuilder::new(pattern)
        .literal_separator(true) // * doesn't match /, only ** does
        .build()
        .map_err(|e| format!("Invalid glob pattern '{pattern}': {e}"))?
        .compile_matcher();

    let walker = ignore::WalkBuilder::new(base_path)
        .standard_filters(true)
        .hidden(false) // don't skip hidden files (let .gitignore handle it)
        .build();

    let mut entries: Vec<(std::path::PathBuf, std::time::SystemTime)> = Vec::new();

    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(_) => continue,
        };

        // Skip directories
        if entry.file_type().is_none_or(|ft| ft.is_dir()) {
            continue;
        }

        let abs_path = match entry.path().canonicalize() {
            Ok(p) => p,
            Err(_) => entry.path().to_path_buf(),
        };

        // Match against relative path from base
        let rel_path = match entry.path().strip_prefix(base_path) {
            Ok(r) => r,
            Err(_) => entry.path(),
        };

        if matcher.is_match(rel_path) {
            let mtime = entry
                .metadata()
                .ok()
                .and_then(|m| m.modified().ok())
                .unwrap_or(std::time::SystemTime::UNIX_EPOCH);
            entries.push((abs_path, mtime));
        }
    }

    if entries.is_empty() {
        return Ok(format!("No files found matching pattern '{pattern}'"));
    }

    let total = entries.len();

    // Sort by mtime descending (most recent first)
    entries.sort_by(|a, b| b.1.cmp(&a.1));

    // Cap at MAX_RESULTS
    let truncated = total > MAX_RESULTS;
    entries.truncate(MAX_RESULTS);

    let mut output: String = entries
        .iter()
        .map(|(p, _)| p.display().to_string())
        .collect::<Vec<_>>()
        .join("\n");

    if truncated {
        output.push_str(&format!("\n\nShowing {MAX_RESULTS} of {total} results"));
    }

    Ok(output)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    use std::fs;

    #[tokio::test]
    async fn test_glob_rs_files() {
        let dir = tempdir().unwrap();
        fs::create_dir_all(dir.path().join("src/nested")).unwrap();
        fs::write(dir.path().join("src/main.rs"), "fn main() {}").unwrap();
        fs::write(dir.path().join("src/nested/lib.rs"), "mod lib;").unwrap();
        fs::write(dir.path().join("readme.md"), "# Readme").unwrap();

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "**/*.rs" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("main.rs"));
        assert!(result.output.contains("lib.rs"));
        assert!(!result.output.contains("readme.md"));
    }

    #[tokio::test]
    async fn test_glob_root_only() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("root.txt"), "root").unwrap();
        fs::create_dir_all(dir.path().join("sub")).unwrap();
        fs::write(dir.path().join("sub/nested.txt"), "nested").unwrap();

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "*.txt" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("root.txt"));
        // *.txt should not match nested files (no **)
        assert!(!result.output.contains("nested.txt"));
    }

    #[tokio::test]
    async fn test_glob_gitignore_respected() {
        let dir = tempdir().unwrap();
        // The ignore crate requires a .git dir to honor .gitignore
        fs::create_dir(dir.path().join(".git")).unwrap();
        fs::write(dir.path().join(".gitignore"), "ignored.txt\n").unwrap();
        fs::write(dir.path().join("kept.txt"), "kept").unwrap();
        fs::write(dir.path().join("ignored.txt"), "ignored").unwrap();

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "**/*.txt" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("kept.txt"));
        assert!(!result.output.contains("ignored.txt"));
    }

    #[tokio::test]
    async fn test_glob_no_matches() {
        let dir = tempdir().unwrap();
        fs::write(dir.path().join("test.txt"), "test").unwrap();

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "**/*.xyz" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("No files found"));
    }

    #[tokio::test]
    async fn test_glob_truncation() {
        let dir = tempdir().unwrap();
        // Create 110 files
        for i in 0..110 {
            fs::write(dir.path().join(format!("file_{i:03}.txt")), "content").unwrap();
        }

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "**/*.txt" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Showing 100 of 110 results"));
    }

    #[tokio::test]
    async fn test_glob_sorted_by_mtime() {
        let dir = tempdir().unwrap();

        // Create files with different modification times
        fs::write(dir.path().join("old.txt"), "old").unwrap();
        // Small sleep to ensure different mtime
        std::thread::sleep(std::time::Duration::from_millis(50));
        fs::write(dir.path().join("new.txt"), "new").unwrap();

        let tool = GlobTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(json!({ "pattern": "**/*.txt" }), &ctx)
            .await
            .unwrap();

        assert!(!result.is_error);
        // Most recent first
        let lines: Vec<&str> = result.output.lines().collect();
        assert!(lines.len() >= 2);
        assert!(lines[0].contains("new.txt"), "new.txt should be first (most recent)");
        assert!(lines[1].contains("old.txt"), "old.txt should be second");
    }
}
