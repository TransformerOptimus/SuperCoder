use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use super::{Tool, ToolContext, ToolResult};

/// Tool that initiates a coding session from ask mode.
/// In ask mode, this signals the agent loop to yield with `AgentResult::StartSession`.
pub struct StartSessionTool;

#[async_trait]
impl Tool for StartSessionTool {
    fn name(&self) -> &str {
        "start_session"
    }

    fn description(&self) -> &str {
        "Start a coding session to make changes to the codebase. Use this when you need to write, edit, or execute code. \
         This creates a new git worktree and branch for isolated work."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["project_path", "task_summary"],
            "properties": {
                "project_path": {
                    "type": "string",
                    "description": "Absolute path to the project root directory"
                },
                "branch": {
                    "type": "string",
                    "description": "Optional branch name. If not provided, one will be generated from the task summary."
                },
                "task_summary": {
                    "type": "string",
                    "description": "Brief description of what this coding session will accomplish"
                }
            }
        })
    }

    async fn execute(&self, args: Value, _ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let project_path = args
            .get("project_path")
            .and_then(|v| v.as_str())
            .unwrap_or_default()
            .to_string();
        let branch = args
            .get("branch")
            .and_then(|v| v.as_str())
            .map(String::from);
        let task_summary = args
            .get("task_summary")
            .and_then(|v| v.as_str())
            .unwrap_or_default()
            .to_string();

        // Validate project_path exists
        let path = std::path::Path::new(&project_path);
        if project_path.is_empty() || !path.exists() {
            return Ok(ToolResult::error(format!(
                "Cannot start coding session: project path '{}' does not exist. \
                 Ask the user to select a project folder first.",
                project_path
            )));
        }

        // Validate it's a git repository (required for worktree creation)
        let git_dir = path.join(".git");
        if !git_dir.exists() {
            return Ok(ToolResult::error(format!(
                "Cannot start coding session: '{}' is not a git repository. \
                 Tell the user to please select a project folder with git initialized using the folder picker.",
                project_path
            )));
        }

        let yield_data = json!({
            "yield_type": "start_session",
            "project_path": project_path,
            "task_summary": task_summary,
            "branch": branch,
        });

        Ok(ToolResult {
            output: format!("Starting coding session: {task_summary}"),
            is_error: false,
            yield_data: Some(yield_data),
            modified_files: Vec::new(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tool::ToolContext;

    fn make_git_repo() -> tempfile::TempDir {
        let dir = tempfile::tempdir().unwrap();
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .unwrap();
        dir
    }

    #[tokio::test]
    async fn test_start_session_basic() {
        let dir = make_git_repo();
        let tool = StartSessionTool;
        let args = json!({
            "project_path": dir.path().to_string_lossy(),
            "task_summary": "Fix the login bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Fix the login bug"));
        assert!(result.yield_data.is_some());

        let data = result.yield_data.unwrap();
        assert_eq!(data["project_path"], dir.path().to_string_lossy().as_ref());
        assert_eq!(data["task_summary"], "Fix the login bug");
        assert!(data.get("branch").is_none() || data["branch"].is_null());
    }

    #[tokio::test]
    async fn test_start_session_with_branch() {
        let dir = make_git_repo();
        let tool = StartSessionTool;
        let args = json!({
            "project_path": dir.path().to_string_lossy(),
            "branch": "fix/login-bug",
            "task_summary": "Fix the login bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        let data = result.yield_data.unwrap();
        assert_eq!(data["branch"], "fix/login-bug");
    }

    #[tokio::test]
    async fn test_start_session_empty_path() {
        let tool = StartSessionTool;
        let args = json!({
            "project_path": "",
            "task_summary": "Fix a bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("does not exist"));
        assert!(result.yield_data.is_none());
    }

    #[tokio::test]
    async fn test_start_session_nonexistent_path() {
        let tool = StartSessionTool;
        let args = json!({
            "project_path": "/this/path/does/not/exist/at/all",
            "task_summary": "Fix a bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("does not exist"));
        assert!(result.yield_data.is_none());
    }

    #[tokio::test]
    async fn test_start_session_not_a_git_repo() {
        let dir = tempfile::tempdir().unwrap();
        let tool = StartSessionTool;
        let args = json!({
            "project_path": dir.path().to_string_lossy(),
            "task_summary": "Fix a bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("not a git repository"));
        assert!(result.yield_data.is_none());
    }

    #[tokio::test]
    async fn test_start_session_valid_git_repo() {
        let dir = tempfile::tempdir().unwrap();
        // Initialize a git repo
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(dir.path())
            .output()
            .unwrap();

        let tool = StartSessionTool;
        let args = json!({
            "project_path": dir.path().to_string_lossy(),
            "task_summary": "Fix the login bug"
        });
        let ctx = ToolContext::test_context(std::path::Path::new("/tmp"));
        let result = tool.execute(args, &ctx).await.unwrap();

        assert!(!result.is_error);
        assert!(result.yield_data.is_some());
        assert!(result.output.contains("Fix the login bug"));
    }
}
