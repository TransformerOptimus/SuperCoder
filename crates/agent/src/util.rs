use std::path::{Path, PathBuf};

/// Ensure the `.agent/` directory exists and is excluded from git via `info/exclude`.
/// Called by any tool that writes to `.agent/` (save_plan, todo_write, truncate_and_persist).
/// Works in both the main repo and worktrees.
pub(crate) async fn ensure_agent_dir(working_dir: &Path) -> PathBuf {
    let agent_dir = working_dir.join(".agent");
    let _ = tokio::fs::create_dir_all(&agent_dir).await;

    // Add .agent/ to git's info/exclude (invisible to user, not tracked)
    ensure_git_exclude(working_dir, ".agent/").await;

    agent_dir
}

/// Add an entry to git's `info/exclude` for the repo at `working_dir`.
/// Uses `git rev-parse --absolute-git-dir` to find the correct gitdir.
/// Best-effort idempotent — concurrent calls may produce duplicate entries
/// (harmless, git deduplicates). Never modifies user-visible files.
pub(crate) async fn ensure_git_exclude(working_dir: &Path, entry: &str) {
    let mut cmd = tokio::process::Command::new("git");
    cmd.args(["rev-parse", "--absolute-git-dir"])
        .current_dir(working_dir);
    git_ops::no_window::no_window_tokio(&mut cmd);
    let output = cmd.output().await;

    let git_dir = match output {
        Ok(o) if o.status.success() => {
            PathBuf::from(String::from_utf8_lossy(&o.stdout).trim())
        }
        _ => return, // Not a git repo — skip silently
    };

    let exclude_dir = git_dir.join("info");
    let exclude_path = exclude_dir.join("exclude");

    let needs_append = if exclude_path.exists() {
        tokio::fs::read_to_string(&exclude_path)
            .await
            .map(|c| !c.lines().any(|line| line.trim() == entry))
            .unwrap_or(true)
    } else {
        true
    };

    if needs_append {
        let _ = tokio::fs::create_dir_all(&exclude_dir).await;
        use tokio::io::AsyncWriteExt;
        if let Ok(mut file) = tokio::fs::OpenOptions::new()
            .create(true)
            .append(true)
            .open(&exclude_path)
            .await
        {
            let _ = file.write_all(format!("\n{entry}\n").as_bytes()).await;
        }
    }
}

/// Resolve a file path against a working directory.
/// Returns the path as-is if absolute, otherwise joins with `working_dir`.
pub(crate) fn resolve_path(working_dir: &std::path::Path, file_path: &str) -> PathBuf {
    let p = PathBuf::from(file_path);
    if p.is_absolute() {
        p
    } else {
        working_dir.join(p)
    }
}

/// Check if a resolved path is inside the working directory.
/// Returns `true` if the path is inside (safe), `false` if outside (needs approval).
///
/// For existing paths, uses `canonicalize()` to resolve symlinks.
/// For new paths (don't exist yet), checks the nearest existing ancestor.
pub(crate) fn is_path_within_working_dir(working_dir: &Path, resolved: &Path) -> bool {
    let canonical_wd = match working_dir.canonicalize() {
        Ok(p) => p,
        Err(_) => return false,
    };

    // For existing files/dirs, canonicalize resolves symlinks
    if resolved.exists() {
        return resolved.canonicalize()
            .map(|p| p.starts_with(&canonical_wd))
            .unwrap_or(false);
    }

    // For new files, check the nearest existing ancestor
    let mut ancestor = resolved.to_path_buf();
    loop {
        if ancestor.exists() {
            return ancestor.canonicalize()
                .map(|p| p.starts_with(&canonical_wd))
                .unwrap_or(false);
        }
        if !ancestor.pop() {
            return false;
        }
    }
}

/// Find the largest byte offset <= `target` that is a valid UTF-8 char boundary.
pub(crate) fn floor_char_boundary(s: &str, target: usize) -> usize {
    if target >= s.len() {
        return s.len();
    }
    let mut end = target;
    while end > 0 && !s.is_char_boundary(end) {
        end -= 1;
    }
    end
}

/// Safely truncate a string to at most `max_bytes` bytes without splitting
/// a multi-byte character.
pub(crate) fn truncate_str(s: &str, max_bytes: usize) -> &str {
    &s[..floor_char_boundary(s, max_bytes)]
}

/// Truncate output that exceeds line or byte limits.
/// Used by both bash tool output and the global tool output truncation in the agent loop.
pub(crate) fn truncate_output(output: &str, max_lines: usize, max_bytes: usize) -> String {
    let total_lines = output.lines().count();
    let total_bytes = output.len();

    if total_lines <= max_lines && total_bytes <= max_bytes {
        return output.to_string();
    }

    let mut result = String::new();
    let mut byte_count = 0;

    for (line_count, line) in output.lines().enumerate() {
        if line_count >= max_lines || byte_count + line.len() + 1 > max_bytes {
            result.push_str(&format!(
                "\n... truncated ({line_count} of {total_lines} lines, {byte_count} of {total_bytes} bytes)"
            ));
            return result;
        }
        if line_count > 0 {
            result.push('\n');
            byte_count += 1;
        }
        result.push_str(line);
        byte_count += line.len();
    }

    result
}

/// Truncate tool output and persist the full content to a temp file when truncated.
///
/// When output exceeds limits, saves the full untruncated output to
/// `{working_dir}/.agent/tool-output/{tool_call_id}.txt` so the LLM can
/// read specific sections later using the read tool.
pub(crate) async fn truncate_and_persist(
    output: &str,
    max_lines: usize,
    max_bytes: usize,
    working_dir: &Path,
    tool_call_id: &str,
) -> String {
    let total_lines = output.lines().count();
    let total_bytes = output.len();

    // No truncation needed
    if total_lines <= max_lines && total_bytes <= max_bytes {
        return output.to_string();
    }

    // Truncate first
    let truncated = truncate_output(output, max_lines, max_bytes);

    // Save full output to temp file
    let base_agent_dir = ensure_agent_dir(working_dir).await;
    let agent_dir = base_agent_dir.join("tool-output");
    if let Ok(()) = tokio::fs::create_dir_all(&agent_dir).await {
        let temp_path = agent_dir.join(format!("{tool_call_id}.txt"));
        if let Ok(()) = tokio::fs::write(&temp_path, output).await {
            return format!(
                "{truncated}\nFull output saved to {}. Use the read tool to view specific sections.",
                temp_path.display()
            );
        }
    }

    // Fallback: return truncated without file reference
    truncated
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    // ── resolve_path tests ──

    #[test]
    fn test_resolve_absolute_path() {
        let result = resolve_path(Path::new("/working"), "/absolute/file.rs");
        assert_eq!(result, PathBuf::from("/absolute/file.rs"));
    }

    #[test]
    fn test_resolve_relative_path() {
        let result = resolve_path(Path::new("/working"), "relative/file.rs");
        assert_eq!(result, PathBuf::from("/working/relative/file.rs"));
    }

    // ── truncate_str tests ──

    #[test]
    fn test_truncate_str_ascii() {
        assert_eq!(truncate_str("hello world", 5), "hello");
    }

    #[test]
    fn test_truncate_str_short_string() {
        assert_eq!(truncate_str("hi", 10), "hi");
    }

    #[test]
    fn test_truncate_str_empty() {
        assert_eq!(truncate_str("", 5), "");
    }

    #[test]
    fn test_truncate_str_emoji() {
        let s = "Hello 🌍";
        assert_eq!(s.len(), 10);
        let result = truncate_str(s, 7);
        assert_eq!(result, "Hello ");
    }

    #[test]
    fn test_truncate_str_cjk() {
        let s = "中文";
        assert_eq!(s.len(), 6);
        let result = truncate_str(s, 4);
        assert_eq!(result, "中");
    }

    #[test]
    fn test_truncate_str_exact_boundary() {
        assert_eq!(truncate_str("abcdef", 3), "abc");
    }

    // ── truncate_output tests ──

    #[test]
    fn test_truncate_output_small_passes_through() {
        let input = "line 1\nline 2\nline 3";
        assert_eq!(truncate_output(input, 2000, 50 * 1024), input);
    }

    #[test]
    fn test_truncate_output_exceeds_line_limit() {
        let input: String = (0..2500).map(|i| format!("line {i}")).collect::<Vec<_>>().join("\n");
        let result = truncate_output(&input, 2000, 50 * 1024);
        assert!(result.contains("truncated"));
        assert!(result.contains("2000 of 2500 lines"));
    }

    #[test]
    fn test_truncate_output_exceeds_byte_limit() {
        let input: String = (0..100).map(|i| format!("line {:04} {}", i, "x".repeat(600))).collect::<Vec<_>>().join("\n");
        assert!(input.len() > 50 * 1024);
        let result = truncate_output(&input, 2000, 50 * 1024);
        assert!(result.contains("truncated"));
    }

    #[test]
    fn test_truncate_output_empty() {
        assert_eq!(truncate_output("", 2000, 50 * 1024), "");
    }

    #[test]
    fn test_truncate_output_exactly_at_limit() {
        let input: String = (0..2000).map(|_| "x\n").collect();
        let result = truncate_output(&input, 2000, 50 * 1024);
        assert!(!result.contains("truncated"));
    }

    // ── floor_char_boundary tests ──

    #[test]
    fn test_floor_char_boundary_ascii() {
        assert_eq!(floor_char_boundary("hello", 3), 3);
    }

    #[test]
    fn test_floor_char_boundary_at_string_end() {
        assert_eq!(floor_char_boundary("hello", 100), 5);
    }

    #[test]
    fn test_floor_char_boundary_snaps_back_on_multibyte() {
        let s = "abc🌍def"; // 🌍 is 4 bytes at positions 3-6
        assert_eq!(floor_char_boundary(s, 5), 3);
        assert_eq!(floor_char_boundary(s, 7), 7);
    }

    #[test]
    fn test_floor_char_boundary_zero() {
        assert_eq!(floor_char_boundary("hello", 0), 0);
    }

    // ── truncate_and_persist tests ──

    #[tokio::test]
    async fn test_truncate_and_persist_no_truncation() {
        let dir = tempfile::tempdir().unwrap();
        let result = truncate_and_persist("short output", 2000, 50 * 1024, dir.path(), "tc_1").await;
        assert_eq!(result, "short output");
        // No .agent directory should be created
        assert!(!dir.path().join(".agent").exists());
    }

    #[tokio::test]
    async fn test_truncate_and_persist_saves_full_output() {
        let dir = tempfile::tempdir().unwrap();
        let long_output: String = (0..100).map(|i| format!("line {i}")).collect::<Vec<_>>().join("\n");
        // Use a small line limit to trigger truncation
        let result = truncate_and_persist(&long_output, 10, 50 * 1024, dir.path(), "tc_42").await;

        assert!(result.contains("truncated"));
        assert!(result.contains("tc_42.txt"));
        assert!(result.contains("Use the read tool"));

        // Verify the full output was saved to the temp file
        let saved_path = dir.path().join(".agent").join("tool-output").join("tc_42.txt");
        assert!(saved_path.exists());
        let saved_content = std::fs::read_to_string(&saved_path).unwrap();
        assert_eq!(saved_content, long_output);
    }

    #[tokio::test]
    async fn test_truncate_and_persist_byte_limit() {
        let dir = tempfile::tempdir().unwrap();
        // Create output that exceeds byte limit
        let long_output: String = (0..100).map(|i| format!("line {:04} {}", i, "x".repeat(600))).collect::<Vec<_>>().join("\n");
        assert!(long_output.len() > 50 * 1024);

        let result = truncate_and_persist(&long_output, 2000, 50 * 1024, dir.path(), "tc_bytes").await;

        assert!(result.contains("truncated"));
        let saved_path = dir.path().join(".agent").join("tool-output").join("tc_bytes.txt");
        assert!(saved_path.exists());
        let saved_content = std::fs::read_to_string(&saved_path).unwrap();
        assert_eq!(saved_content, long_output);
    }
}
