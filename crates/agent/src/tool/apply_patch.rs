use async_trait::async_trait;
use serde_json::{json, Value};

use crate::error::ToolError;
use crate::util::{is_path_within_working_dir, resolve_path};
use super::{Tool, ToolContext, ToolResult};

pub struct ApplyPatchTool;

#[async_trait]
impl Tool for ApplyPatchTool {
    fn name(&self) -> &str {
        "apply_patch"
    }

    fn description(&self) -> &str {
        "Apply a unified diff patch to one or more files. Use for coordinated multi-file changes. \
         Supports creating, modifying, and deleting files. Prefer `edit` for single-file changes."
    }

    fn parameters_schema(&self) -> Value {
        json!({
            "type": "object",
            "required": ["patch"],
            "properties": {
                "patch": {
                    "type": "string",
                    "description": "The patch content in unified diff format"
                }
            }
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> Result<ToolResult, ToolError> {
        let patch_str = args
            .get("patch")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ToolError("Missing required parameter: patch".into()))?;

        if patch_str.trim().is_empty() {
            return Ok(ToolResult::error("Patch content is empty."));
        }

        let file_patches = match parse_unified_diff(patch_str) {
            Ok(patches) => {
                log::info!("[v1.0] apply_patch: parsed {} file patches", patches.len());
                patches
            }
            Err(e) => return Ok(ToolResult::error(format!("Failed to parse patch: {e}"))),
        };

        if file_patches.is_empty() {
            return Ok(ToolResult::error(
                "No file patches found in the input. Expected unified diff format with --- and +++ headers.",
            ));
        }

        let mut modified = Vec::new();
        let mut created = Vec::new();
        let mut deleted = Vec::new();

        // ── Phase 1: Validate all patches and compute new contents in memory ──
        // This ensures we don't leave files in a partial state if a later patch fails.
        enum Staged {
            Write { content: String },
            Delete,
            CreateDirs { content: String },
        }
        let mut staged: Vec<(std::path::PathBuf, String, PatchOp, Staged)> = Vec::new();

        for fp in &file_patches {
            let path = resolve_path(&ctx.working_dir, &fp.path);

            if !is_path_within_working_dir(&ctx.working_dir, &path) {
                return Ok(ToolResult::error(format!(
                    "Path '{}' is outside the working directory. Cannot apply patch.",
                    fp.path
                )));
            }

            match fp.operation {
                PatchOp::Delete => {
                    staged.push((path, fp.path.clone(), PatchOp::Delete, Staged::Delete));
                }
                PatchOp::Create => {
                    let new_content = apply_hunks_to_empty(&fp.hunks)?;
                    staged.push((path, fp.path.clone(), PatchOp::Create, Staged::CreateDirs { content: new_content }));
                }
                PatchOp::Modify => {
                    let content = tokio::fs::read_to_string(&path)
                        .await
                        .map_err(|e| ToolError(format!("Failed to read {}: {e}", fp.path)))?;
                    let new_content = match apply_hunks(&content, &fp.hunks) {
                        Ok(c) => c,
                        Err(e) => {
                            return Ok(ToolResult::error(format!(
                                "Failed to apply patch to {}: {e}",
                                fp.path
                            )));
                        }
                    };
                    staged.push((path, fp.path.clone(), PatchOp::Modify, Staged::Write { content: new_content }));
                }
            }
        }

        // ── Phase 2: Apply all validated changes to disk ──
        for (path, name, _op, action) in &staged {
            match action {
                Staged::Delete => {
                    if path.exists() {
                        tokio::fs::remove_file(path)
                            .await
                            .map_err(|e| ToolError(format!("Failed to delete {}: {e}", name)))?;
                    }
                    deleted.push(name.clone());
                }
                Staged::CreateDirs { content } => {
                    if let Some(parent) = path.parent() {
                        tokio::fs::create_dir_all(parent)
                            .await
                            .map_err(|e| ToolError(format!("Failed to create dirs: {e}")))?;
                    }
                    tokio::fs::write(path, content)
                        .await
                        .map_err(|e| ToolError(format!("Failed to write {}: {e}", name)))?;
                    created.push(name.clone());
                }
                Staged::Write { content } => {
                    tokio::fs::write(path, content)
                        .await
                        .map_err(|e| ToolError(format!("Failed to write {}: {e}", name)))?;
                    modified.push(name.clone());
                }
            }
        }

        let mut parts = Vec::new();
        if !modified.is_empty() {
            parts.push(format!("modified {} file(s) ({})", modified.len(), modified.join(", ")));
        }
        if !created.is_empty() {
            parts.push(format!("created {} file(s) ({})", created.len(), created.join(", ")));
        }
        if !deleted.is_empty() {
            parts.push(format!("deleted {} file(s) ({})", deleted.len(), deleted.join(", ")));
        }

        let mut all_files: Vec<String> = Vec::new();
        all_files.extend(modified.iter().cloned());
        all_files.extend(created.iter().cloned());
        all_files.extend(deleted.iter().cloned());

        let mut result = ToolResult::success(format!("Applied patch: {}", parts.join(", ")));
        result.modified_files = all_files;
        Ok(result)
    }
}

// ── Patch parsing types ──

#[derive(Debug)]
enum PatchOp {
    Create,
    Modify,
    Delete,
}

#[derive(Debug)]
struct FilePatch {
    path: String,
    operation: PatchOp,
    hunks: Vec<Hunk>,
}

#[derive(Debug)]
struct Hunk {
    old_start: usize,
    lines: Vec<HunkLine>,
}

#[derive(Debug)]
enum HunkLine {
    Context(()),
    Remove(()), // content stored for potential future context matching
    Add(String),
}

// ── Unified diff parser ──

fn parse_unified_diff(patch: &str) -> Result<Vec<FilePatch>, String> {
    let lines: Vec<&str> = patch.lines().collect();
    let mut file_patches = Vec::new();
    let mut i = 0;

    while i < lines.len() {
        // Find --- header
        if lines[i].starts_with("--- ") {
            if i + 1 >= lines.len() || !lines[i + 1].starts_with("+++ ") {
                i += 1;
                continue;
            }

            let old_path = parse_file_path(lines[i], "--- ");
            let new_path = parse_file_path(lines[i + 1], "+++ ");
            i += 2;

            let operation = if old_path == "/dev/null" {
                PatchOp::Create
            } else if new_path == "/dev/null" {
                PatchOp::Delete
            } else {
                PatchOp::Modify
            };

            let path = match &operation {
                PatchOp::Create => new_path.clone(),
                _ => old_path.clone(),
            };

            // Parse hunks
            let mut hunks = Vec::new();
            while i < lines.len() && !lines[i].starts_with("--- ") && !lines[i].starts_with("diff ") {
                if lines[i].starts_with("@@ ") {
                    let (old_start, hunk_lines, next_i) = parse_hunk(&lines, i)?;
                    hunks.push(Hunk {
                        old_start,
                        lines: hunk_lines,
                    });
                    i = next_i;
                } else {
                    i += 1;
                }
            }

            file_patches.push(FilePatch {
                path,
                operation,
                hunks,
            });
        } else {
            i += 1;
        }
    }

    Ok(file_patches)
}

fn parse_file_path(line: &str, prefix: &str) -> String {
    let raw = line.strip_prefix(prefix).unwrap_or(line);
    // Strip a/ or b/ prefix (common in git diffs)
    let stripped = if raw.starts_with("a/") || raw.starts_with("b/") {
        &raw[2..]
    } else {
        raw
    };
    stripped.to_string()
}

fn parse_hunk(lines: &[&str], start: usize) -> Result<(usize, Vec<HunkLine>, usize), String> {
    let header = lines[start];
    // Parse @@ -old_start[,old_count] +new_start[,new_count] @@
    let parts: Vec<&str> = header.split("@@").collect();
    if parts.len() < 3 {
        return Err(format!("Invalid hunk header: {header}"));
    }
    let range_part = parts[1].trim();
    let old_range = range_part.split(' ').next().unwrap_or("-1");
    let old_start: usize = old_range
        .trim_start_matches('-')
        .split(',')
        .next()
        .and_then(|s| s.parse().ok())
        .unwrap_or(1);

    let mut hunk_lines = Vec::new();
    let mut i = start + 1;

    while i < lines.len() {
        let line = lines[i];
        if line.starts_with("@@ ") || line.starts_with("--- ") || line.starts_with("diff ") {
            break;
        }
        if line.starts_with('-') {
            hunk_lines.push(HunkLine::Remove(()));
        } else if let Some(content) = line.strip_prefix('+') {
            hunk_lines.push(HunkLine::Add(content.to_string()));
        } else if line.starts_with(' ') {
            hunk_lines.push(HunkLine::Context(()));
        } else if line == "\\ No newline at end of file" {
            // Skip this marker
        } else {
            // Treat unrecognized lines as context (bare lines without prefix)
            hunk_lines.push(HunkLine::Context(()));
        }
        i += 1;
    }

    Ok((old_start, hunk_lines, i))
}

// ── Hunk application ──

fn apply_hunks(content: &str, hunks: &[Hunk]) -> Result<String, String> {
    let mut lines: Vec<String> = content.lines().map(String::from).collect();
    let mut offset: isize = 0;

    for hunk in hunks {
        let raw = hunk.old_start as isize - 1 + offset;
        if raw < 0 {
            return Err(format!(
                "Hunk at old_start={} produces negative offset ({}) — patch is invalid",
                hunk.old_start, raw
            ));
        }
        let start = raw as usize;
        let mut pos = start;
        let mut removals = 0;
        let mut additions = 0;

        for hline in &hunk.lines {
            match hline {
                HunkLine::Context(_) => {
                    pos += 1;
                }
                HunkLine::Remove(_) => {
                    if pos < lines.len() {
                        lines.remove(pos);
                        removals += 1;
                    }
                }
                HunkLine::Add(text) => {
                    if pos <= lines.len() {
                        lines.insert(pos, text.clone());
                        pos += 1;
                        additions += 1;
                    }
                }
            }
        }

        offset += additions as isize - removals as isize;
    }

    // Preserve trailing newline if original had one
    let mut result = lines.join("\n");
    if content.ends_with('\n') && !result.ends_with('\n') {
        result.push('\n');
    }
    Ok(result)
}

fn apply_hunks_to_empty(hunks: &[Hunk]) -> Result<String, ToolError> {
    let mut lines: Vec<String> = Vec::new();
    for hunk in hunks {
        for hline in &hunk.lines {
            if let HunkLine::Add(text) = hline {
                lines.push(text.clone());
            }
        }
    }
    let mut result = lines.join("\n");
    if !result.is_empty() {
        result.push('\n');
    }
    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    fn test_ctx(dir: &std::path::Path) -> ToolContext {
        ToolContext::test_context(dir)
    }

    #[tokio::test]
    async fn test_apply_single_file_modify() {
        let tmp = tempdir().unwrap();
        std::fs::write(tmp.path().join("test.txt"), "line 1\nline 2\nline 3\n").unwrap();

        let patch = "\
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line 1
-line 2
+line 2 modified
 line 3
";
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": patch}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(!result.is_error, "Error: {}", result.output);
        let content = std::fs::read_to_string(tmp.path().join("test.txt")).unwrap();
        assert!(content.contains("line 2 modified"));
        assert!(!content.contains("\nline 2\n"));
    }

    #[tokio::test]
    async fn test_apply_new_file() {
        let tmp = tempdir().unwrap();

        let patch = "\
--- /dev/null
+++ b/new_file.txt
@@ -0,0 +1,3 @@
+line 1
+line 2
+line 3
";
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": patch}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(!result.is_error, "Error: {}", result.output);
        assert!(result.output.contains("created"));
        let content = std::fs::read_to_string(tmp.path().join("new_file.txt")).unwrap();
        assert!(content.contains("line 1"));
        assert!(content.contains("line 3"));
    }

    #[tokio::test]
    async fn test_apply_delete_file() {
        let tmp = tempdir().unwrap();
        std::fs::write(tmp.path().join("delete_me.txt"), "content\n").unwrap();

        let patch = "\
--- a/delete_me.txt
+++ /dev/null
@@ -1 +0,0 @@
-content
";
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": patch}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(!result.is_error, "Error: {}", result.output);
        assert!(result.output.contains("deleted"));
        assert!(!tmp.path().join("delete_me.txt").exists());
    }

    #[tokio::test]
    async fn test_apply_multi_file() {
        let tmp = tempdir().unwrap();
        std::fs::write(tmp.path().join("a.txt"), "aaa\n").unwrap();
        std::fs::write(tmp.path().join("b.txt"), "bbb\n").unwrap();

        let patch = "\
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-aaa
+aaa modified
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-bbb
+bbb modified
";
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": patch}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(!result.is_error, "Error: {}", result.output);
        assert!(std::fs::read_to_string(tmp.path().join("a.txt")).unwrap().contains("aaa modified"));
        assert!(std::fs::read_to_string(tmp.path().join("b.txt")).unwrap().contains("bbb modified"));
    }

    #[tokio::test]
    async fn test_apply_empty_patch() {
        let tmp = tempdir().unwrap();
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": ""}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("empty"));
    }

    #[tokio::test]
    async fn test_apply_invalid_format() {
        let tmp = tempdir().unwrap();
        let tool = ApplyPatchTool;
        let result = tool
            .execute(
                json!({"patch": "this is not a patch\njust random text\n"}),
                &test_ctx(tmp.path()),
            )
            .await
            .unwrap();

        assert!(result.is_error);
    }

    #[tokio::test]
    async fn test_apply_path_outside_working_dir() {
        let tmp = tempdir().unwrap();
        let patch = "\
--- a/../../../etc/passwd
+++ b/../../../etc/passwd
@@ -1 +1 @@
-root
+hacked
";
        let tool = ApplyPatchTool;
        let result = tool
            .execute(json!({"patch": patch}), &test_ctx(tmp.path()))
            .await
            .unwrap();

        assert!(result.is_error);
        assert!(result.output.contains("outside"));
    }

    // ── Parser unit tests ──

    #[test]
    fn test_parse_file_path_strips_prefix() {
        assert_eq!(parse_file_path("--- a/src/main.rs", "--- "), "src/main.rs");
        assert_eq!(parse_file_path("+++ b/src/main.rs", "+++ "), "src/main.rs");
        assert_eq!(parse_file_path("--- /dev/null", "--- "), "/dev/null");
    }

    #[test]
    fn test_parse_unified_diff_basic() {
        let patch = "--- a/test.txt\n+++ b/test.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n context\n";
        let result = parse_unified_diff(patch).unwrap();
        assert_eq!(result.len(), 1);
        assert_eq!(result[0].path, "test.txt");
        assert_eq!(result[0].hunks.len(), 1);
    }
}
