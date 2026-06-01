use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};
use super::edit::replace;

/// Tool that makes targeted edits to a plan file using find-and-replace.
///
/// Reuses the multi-strategy fuzzy matching from the `edit` tool.
/// The plan file is either specified explicitly or auto-discovered
/// from `.agent/plan-*.md` in the working directory.
pub struct EditPlanTool;

#[async_trait]
impl Tool for EditPlanTool {
    fn name(&self) -> &str {
        "edit_plan"
    }

    fn description(&self) -> &str {
        "Make targeted edits to a plan file using find-and-replace. \
         If file_path is omitted, auto-discovers the plan file from .agent/plan-*.md. \
         Use for small revisions. For complete rewrites, use save_plan instead."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["old_text", "new_text"],
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Optional relative path to the plan file (e.g. '.agent/plan-add-auth.md'). If omitted, auto-discovers from .agent/plan-*.md."
                },
                "old_text": {
                    "type": "string",
                    "description": "The exact text to find in the plan file"
                },
                "new_text": {
                    "type": "string",
                    "description": "The replacement text"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let old_text = match args.get("old_text").and_then(|v| v.as_str()) {
            Some(s) if !s.is_empty() => s,
            _ => return Ok(ToolResult::error("'old_text' is required and must be non-empty.")),
        };

        let new_text = match args.get("new_text").and_then(|v| v.as_str()) {
            Some(s) => s,
            _ => return Ok(ToolResult::error("'new_text' is required.")),
        };

        if old_text == new_text {
            return Ok(ToolResult::error("old_text and new_text are identical."));
        }

        // Resolve the plan file path
        let plan_path = match args.get("file_path").and_then(|v| v.as_str()).filter(|s| !s.is_empty()) {
            Some(fp) => ctx.working_dir.join(fp),
            None => match discover_plan_file(&ctx.working_dir).await {
                Ok(path) => path,
                Err(msg) => return Ok(ToolResult::error(msg)),
            },
        };

        // Read the plan file
        let content = match tokio::fs::read_to_string(&plan_path).await {
            Ok(c) => c,
            Err(e) => return Ok(ToolResult::error(format!(
                "Failed to read plan file {}: {e}", plan_path.display()
            ))),
        };

        // Apply replacement using the edit tool's multi-strategy matching
        let updated = match replace(&content, old_text, new_text, false) {
            Ok(result) => result,
            Err(err) => return Ok(ToolResult::error(format!(
                "Edit failed in {}: {err}", plan_path.display()
            ))),
        };

        // Write back
        if let Err(e) = tokio::fs::write(&plan_path, &updated).await {
            return Ok(ToolResult::error(format!(
                "Failed to write plan file {}: {e}", plan_path.display()
            )));
        }

        let relative_path = plan_path.strip_prefix(&ctx.working_dir)
            .unwrap_or(&plan_path)
            .display()
            .to_string();

        log::info!("[v1.0] edit_plan: edited {}", relative_path);

        Ok(ToolResult {
            output: format!("Plan updated: {}\n\n{}", relative_path, updated),
            is_error: false,
            yield_data: None,
            modified_files: vec![plan_path.to_string_lossy().to_string()],
        })
    }
}

/// Discover the plan file from `.agent/plan-*.md` in the working directory.
async fn discover_plan_file(working_dir: &std::path::Path) -> Result<std::path::PathBuf, String> {
    let agent_dir = working_dir.join(".agent");
    if !agent_dir.exists() {
        return Err("No .agent/ directory found. No plan files to edit.".to_string());
    }

    let mut plan_files = Vec::new();
    let mut entries = match tokio::fs::read_dir(&agent_dir).await {
        Ok(e) => e,
        Err(e) => return Err(format!("Failed to read .agent/ directory: {e}")),
    };

    while let Ok(Some(entry)) = entries.next_entry().await {
        let name = entry.file_name();
        let name_str = name.to_string_lossy();
        if name_str.starts_with("plan") && name_str.ends_with(".md") {
            plan_files.push(entry.path());
        }
    }

    match plan_files.len() {
        0 => Err("No plan files found in .agent/. Use save_plan to create one first.".to_string()),
        1 => Ok(plan_files.into_iter().next().unwrap()),
        _ => {
            let names: Vec<String> = plan_files.iter()
                .filter_map(|p| p.file_name().map(|n| n.to_string_lossy().to_string()))
                .collect();
            Err(format!(
                "Multiple plan files found: {}. Specify file_path to choose one.",
                names.join(", ")
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_ctx(dir: &std::path::Path) -> ToolContext {
        ToolContext::test_context(dir)
    }

    async fn write_plan(dir: &std::path::Path, filename: &str, content: &str) {
        let agent_dir = dir.join(".agent");
        tokio::fs::create_dir_all(&agent_dir).await.unwrap();
        tokio::fs::write(agent_dir.join(filename), content).await.unwrap();
    }

    #[tokio::test]
    async fn test_edit_plan_basic() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-test.md", "## Steps\n1. Do foo\n2. Do bar\n").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "Do foo",
            "new_text": "Do baz"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(!result.is_error, "Unexpected error: {}", result.output);
        assert!(result.output.contains("Do baz"));

        // Verify file on disk
        let content = tokio::fs::read_to_string(tmp.path().join(".agent/plan-test.md")).await.unwrap();
        assert!(content.contains("Do baz"));
        assert!(!content.contains("Do foo"));
    }

    #[tokio::test]
    async fn test_edit_plan_with_explicit_path() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-a.md", "Step A").await;
        write_plan(tmp.path(), "plan-b.md", "Step B").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "file_path": ".agent/plan-b.md",
            "old_text": "Step B",
            "new_text": "Step B revised"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Step B revised"));

        // plan-a untouched
        let a = tokio::fs::read_to_string(tmp.path().join(".agent/plan-a.md")).await.unwrap();
        assert_eq!(a, "Step A");
    }

    #[tokio::test]
    async fn test_edit_plan_auto_discover_single() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-feature.md", "old content").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "old content",
            "new_text": "new content"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("new content"));
    }

    #[tokio::test]
    async fn test_edit_plan_multiple_files_no_path_errors() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-a.md", "A").await;
        write_plan(tmp.path(), "plan-b.md", "B").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "A",
            "new_text": "A2"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("Multiple plan files found"));
        assert!(result.output.contains("plan-a.md"));
        assert!(result.output.contains("plan-b.md"));
    }

    #[tokio::test]
    async fn test_edit_plan_no_files_errors() {
        let tmp = tempfile::tempdir().unwrap();
        // No .agent/ directory at all

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "foo",
            "new_text": "bar"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("No .agent/ directory"));
    }

    #[tokio::test]
    async fn test_edit_plan_old_text_not_found() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-test.md", "## Steps\n1. Do foo\n").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "nonexistent text",
            "new_text": "replacement"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("Edit failed") || result.output.contains("No match"));
    }

    #[tokio::test]
    async fn test_edit_plan_identical_text_errors() {
        let tmp = tempfile::tempdir().unwrap();
        write_plan(tmp.path(), "plan-test.md", "content").await;

        let tool = EditPlanTool;
        let result = tool.execute(json!({
            "old_text": "same",
            "new_text": "same"
        }), &test_ctx(tmp.path())).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("identical"));
    }
}
