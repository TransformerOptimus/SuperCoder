use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};

/// Tool that saves the implementation plan to disk and yields to the frontend.
///
/// The agent calls this when the plan is complete. It:
/// 1. Writes the plan to `.agent/{filename}` in the working directory
/// 2. Yields control back to the frontend with the plan text
///
/// The frontend shows a PlanCard with an "Implement" button.
pub struct SavePlanTool;

#[async_trait]
impl Tool for SavePlanTool {
    fn name(&self) -> &str {
        "save_plan"
    }

    fn description(&self) -> &str {
        "Save your implementation plan and present it to the user for approval. \
         Call this when your plan is complete and ready for review. \
         You must provide a descriptive filename (e.g. 'plan-add-auth-middleware.md'). \
         The plan will be saved to .agent/{filename} and shown to the user with an Implement button."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["plan", "filename"],
            "properties": {
                "plan": {
                    "type": "string",
                    "description": "The complete implementation plan in markdown format"
                },
                "filename": {
                    "type": "string",
                    "description": "A descriptive filename for the plan, e.g. 'plan-add-auth-middleware.md' or 'plan-refactor-database-layer.md'. Must end in .md."
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let plan = args
            .get("plan")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();

        if plan.is_empty() {
            return Ok(ToolResult::error("Plan must not be empty."));
        }

        let filename = args
            .get("filename")
            .and_then(|v| v.as_str())
            .filter(|s| !s.is_empty())
            .unwrap_or("plan.md");

        // Sanitize filename — only allow alphanumeric, hyphens, underscores, dots
        let safe_filename: String = filename
            .chars()
            .map(|c| if c.is_alphanumeric() || c == '-' || c == '_' || c == '.' { c } else { '-' })
            .collect();
        let safe_filename = if safe_filename.ends_with(".md") { safe_filename } else { format!("{safe_filename}.md") };

        // Save plan to .agent/{filename}
        let agent_dir = crate::util::ensure_agent_dir(&ctx.working_dir).await;
        let plan_path = agent_dir.join(&safe_filename);
        if let Err(e) = tokio::fs::write(&plan_path, &plan).await {
            return Ok(ToolResult::error(format!("Failed to write plan file: {e}")));
        }

        log::info!(
            "[v1.0] save_plan: wrote {} bytes to {}",
            plan.len(),
            plan_path.display()
        );

        let yield_data = json!({
            "yield_type": "save_plan",
            "plan": plan,
            "plan_path": plan_path.to_string_lossy(),
        });

        Ok(ToolResult {
            output: format!("Plan saved to {}", plan_path.display()),
            is_error: false,
            yield_data: Some(yield_data),
            modified_files: Vec::new(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_ctx(dir: &std::path::Path) -> ToolContext {
        ToolContext::test_context(dir)
    }

    #[tokio::test]
    async fn test_save_plan_basic() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = SavePlanTool;
        let result = tool
            .execute(
                json!({"plan": "## Goal\nFix the bug\n\n## Steps\n1. Read file\n2. Edit file"}),
                &test_ctx(tmp.path()),
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.yield_data.is_some());

        let data = result.yield_data.unwrap();
        assert_eq!(data["yield_type"], "save_plan");
        assert!(data["plan"].as_str().unwrap().contains("Fix the bug"));

        // Verify file was written
        let plan_path = tmp.path().join(".agent").join("plan.md");
        assert!(plan_path.exists());
        let content = tokio::fs::read_to_string(&plan_path).await.unwrap();
        assert!(content.contains("Fix the bug"));
    }

    #[tokio::test]
    async fn test_save_plan_empty_error() {
        let tmp = tempfile::tempdir().unwrap();
        let tool = SavePlanTool;
        let result = tool
            .execute(json!({"plan": ""}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.yield_data.is_none());
    }
}
