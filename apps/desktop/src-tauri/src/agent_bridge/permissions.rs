use rusqlite::Connection;
use serde::{Deserialize, Serialize};

// ── Types ──────────────────────────────────────────────────────────────────

/// Permission level for agent tool execution.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PermissionLevel {
    /// All tools run without approval.
    AutoApproveAll,
    /// Destructive tools require approval (default).
    ApproveDestructive,
    /// All tools require approval.
    ApproveEverything,
}

/// Per-tool overrides that take priority over the permission level.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolOverrides {
    /// Tools that always run without approval regardless of level.
    pub auto_approve: Vec<String>,
    /// Tools that always require approval regardless of level.
    pub always_ask: Vec<String>,
}

/// Complete permission configuration for a project (or global default).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PermissionConfig {
    /// None = global default, Some = project-specific.
    pub project_path: Option<String>,
    /// Permission level.
    pub level: PermissionLevel,
    /// Per-tool overrides.
    pub tool_overrides: Option<ToolOverrides>,
}

/// Tools considered destructive (can modify filesystem or external state).
const DESTRUCTIVE_TOOLS: &[&str] = &["write", "edit", "bash", "git", "create_pr", "apply_patch"];

/// System tools that never require approval, even in ApproveEverything mode.
/// These are internal/UI tools that don't affect the codebase.
const SYSTEM_TOOLS: &[&str] = &["todo_write", "ask_user", "save_plan", "edit_plan"];

// ── Table management ───────────────────────────────────────────────────────

/// Create the agent_permissions table in the existing settings DB if it doesn't exist.
pub fn ensure_permissions_table(conn: &Connection) -> Result<(), String> {
    conn.execute_batch(
        "CREATE TABLE IF NOT EXISTS agent_permissions (
            project_path TEXT PRIMARY KEY,
            level TEXT NOT NULL DEFAULT 'approve_destructive',
            tool_overrides TEXT
        );",
    )
    .map_err(|e| e.to_string())
}

// ── CRUD ───────────────────────────────────────────────────────────────────

/// Get permission config for a project path. Falls back to global default if no
/// project-specific config exists. Returns the default (ApproveDestructive) if
/// no config exists at all.
pub fn get_permission(conn: &Connection, project_path: Option<&str>) -> PermissionConfig {
    let key = project_path.unwrap_or("__global__");

    let result: Option<(String, Option<String>)> = conn
        .query_row(
            "SELECT level, tool_overrides FROM agent_permissions WHERE project_path = ?",
            [key],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .ok();

    match result {
        Some((level_str, overrides_str)) => {
            PermissionConfig {
                project_path: project_path.map(String::from),
                level: str_to_level(&level_str),
                tool_overrides: overrides_str
                    .and_then(|s| serde_json::from_str(&s).ok()),
            }
        },
        None => {
            if let Some(path_str) = project_path {
                // Walk up parent directories to find a matching config.
                // This handles worktree paths like /project/.agent-worktrees/session-id
                // matching a config saved for /project.
                let mut search_path = std::path::Path::new(path_str);
                while let Some(parent) = search_path.parent() {
                    if parent.as_os_str().is_empty() {
                        break;
                    }
                    let parent_str = parent.to_string_lossy();
                    let parent_result: Option<(String, Option<String>)> = conn
                        .query_row(
                            "SELECT level, tool_overrides FROM agent_permissions WHERE project_path = ?",
                            [parent_str.as_ref()],
                            |row| Ok((row.get(0)?, row.get(1)?)),
                        )
                        .ok();

                    if let Some((level_str, overrides_str)) = parent_result {
                        return PermissionConfig {
                            project_path: project_path.map(String::from),
                            level: str_to_level(&level_str),
                            tool_overrides: overrides_str
                                .and_then(|s| serde_json::from_str(&s).ok()),
                        };
                    }
                    search_path = parent;
                }

                // No ancestor match — fall back to global
                let global = get_permission(conn, None);
                PermissionConfig {
                    project_path: project_path.map(String::from),
                    level: global.level,
                    tool_overrides: global.tool_overrides,
                }
            } else {
                // No global config — return default
                PermissionConfig {
                    project_path: None,
                    level: PermissionLevel::ApproveDestructive,
                    tool_overrides: None,
                }
            }
        }
    }
}

/// Save a permission config. Uses project_path or "__global__" as key.
pub fn set_permission(conn: &Connection, config: &PermissionConfig) -> Result<(), String> {
    let key = config
        .project_path
        .as_deref()
        .unwrap_or("__global__");
    let level_str = level_to_str(config.level);
    let overrides_str = config
        .tool_overrides
        .as_ref()
        .map(|o| serde_json::to_string(o).map_err(|e| e.to_string()))
        .transpose()?;

    conn.execute(
        "INSERT OR REPLACE INTO agent_permissions (project_path, level, tool_overrides) VALUES (?, ?, ?)",
        rusqlite::params![key, level_str, overrides_str],
    )
    .map_err(|e| e.to_string())?;
    Ok(())
}

// ── Approval logic ─────────────────────────────────────────────────────────

/// Check if a tool requires user approval given the current permission config.
pub fn needs_approval(config: &PermissionConfig, tool_name: &str) -> bool {
    // Check overrides first — they take priority
    if let Some(ref overrides) = config.tool_overrides {
        if overrides.auto_approve.iter().any(|t| t == tool_name) {
            return false;
        }
        if overrides.always_ask.iter().any(|t| t == tool_name) {
            return true;
        }
    }

    // System tools never need approval
    if SYSTEM_TOOLS.contains(&tool_name) {
        return false;
    }

    // Fall back to level
    match config.level {
        PermissionLevel::AutoApproveAll => false,
        PermissionLevel::ApproveEverything => true,
        PermissionLevel::ApproveDestructive => is_destructive(tool_name),
    }
}

fn is_destructive(tool_name: &str) -> bool {
    DESTRUCTIVE_TOOLS.contains(&tool_name)
}

fn level_to_str(level: PermissionLevel) -> &'static str {
    match level {
        PermissionLevel::AutoApproveAll => "auto_approve_all",
        PermissionLevel::ApproveDestructive => "approve_destructive",
        PermissionLevel::ApproveEverything => "approve_everything",
    }
}

fn str_to_level(s: &str) -> PermissionLevel {
    match s {
        "auto_approve_all" => PermissionLevel::AutoApproveAll,
        "approve_everything" => PermissionLevel::ApproveEverything,
        _ => PermissionLevel::ApproveDestructive,
    }
}

// ── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn setup_db() -> Connection {
        let conn = Connection::open_in_memory().unwrap();
        ensure_permissions_table(&conn).unwrap();
        conn
    }

    #[test]
    fn test_ensure_permissions_table() {
        let conn = setup_db();
        // Should be able to query the table
        let count: i64 = conn
            .query_row("SELECT COUNT(*) FROM agent_permissions", [], |row| {
                row.get(0)
            })
            .unwrap();
        assert_eq!(count, 0);
    }

    #[test]
    fn test_get_default_permission() {
        let conn = setup_db();
        let config = get_permission(&conn, None);
        assert_eq!(config.level, PermissionLevel::ApproveDestructive);
        assert!(config.tool_overrides.is_none());
        assert!(config.project_path.is_none());
    }

    #[test]
    fn test_set_and_get_permission() {
        let conn = setup_db();
        let config = PermissionConfig {
            project_path: None,
            level: PermissionLevel::AutoApproveAll,
            tool_overrides: None,
        };
        set_permission(&conn, &config).unwrap();

        let loaded = get_permission(&conn, None);
        assert_eq!(loaded.level, PermissionLevel::AutoApproveAll);
    }

    #[test]
    fn test_project_specific_permission() {
        let conn = setup_db();

        // Set global to AutoApproveAll
        set_permission(
            &conn,
            &PermissionConfig {
                project_path: None,
                level: PermissionLevel::AutoApproveAll,
                tool_overrides: None,
            },
        )
        .unwrap();

        // Set project-specific to ApproveEverything
        set_permission(
            &conn,
            &PermissionConfig {
                project_path: Some("/home/user/project".into()),
                level: PermissionLevel::ApproveEverything,
                tool_overrides: None,
            },
        )
        .unwrap();

        let project_config = get_permission(&conn, Some("/home/user/project"));
        assert_eq!(project_config.level, PermissionLevel::ApproveEverything);

        // Other project falls back to global
        let other_config = get_permission(&conn, Some("/home/user/other"));
        assert_eq!(other_config.level, PermissionLevel::AutoApproveAll);
    }

    #[test]
    fn test_needs_approval_auto_approve_all() {
        let config = PermissionConfig {
            project_path: None,
            level: PermissionLevel::AutoApproveAll,
            tool_overrides: None,
        };
        assert!(!needs_approval(&config, "read"));
        assert!(!needs_approval(&config, "write"));
        assert!(!needs_approval(&config, "bash"));
        assert!(!needs_approval(&config, "edit"));
    }

    #[test]
    fn test_needs_approval_approve_everything() {
        let config = PermissionConfig {
            project_path: None,
            level: PermissionLevel::ApproveEverything,
            tool_overrides: None,
        };
        assert!(needs_approval(&config, "read"));
        assert!(needs_approval(&config, "glob"));
        assert!(needs_approval(&config, "write"));
        assert!(needs_approval(&config, "bash"));
        // System tools bypass even ApproveEverything
        assert!(!needs_approval(&config, "todo_write"));
        assert!(!needs_approval(&config, "ask_user"));
        assert!(!needs_approval(&config, "save_plan"));
        assert!(!needs_approval(&config, "edit_plan"));
    }

    #[test]
    fn test_needs_approval_destructive_only() {
        let config = PermissionConfig {
            project_path: None,
            level: PermissionLevel::ApproveDestructive,
            tool_overrides: None,
        };
        // Read-only tools pass
        assert!(!needs_approval(&config, "read"));
        assert!(!needs_approval(&config, "glob"));
        assert!(!needs_approval(&config, "grep"));
        // Destructive tools need approval
        assert!(needs_approval(&config, "write"));
        assert!(needs_approval(&config, "edit"));
        assert!(needs_approval(&config, "bash"));
        assert!(needs_approval(&config, "git"));
        assert!(needs_approval(&config, "create_pr"));
    }

    #[test]
    fn test_tool_overrides() {
        let config = PermissionConfig {
            project_path: None,
            level: PermissionLevel::ApproveDestructive,
            tool_overrides: Some(ToolOverrides {
                auto_approve: vec!["bash".into()],
                always_ask: vec!["read".into()],
            }),
        };
        // bash is destructive but overridden to auto_approve
        assert!(!needs_approval(&config, "bash"));
        // read is non-destructive but overridden to always_ask
        assert!(needs_approval(&config, "read"));
        // write is destructive with no override — follows level
        assert!(needs_approval(&config, "write"));
        // glob is non-destructive with no override — follows level
        assert!(!needs_approval(&config, "glob"));
    }
}
