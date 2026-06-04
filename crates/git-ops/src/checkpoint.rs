//! App-managed file-snapshot checkpoint layer.
//!
//! Replaces the old git-ref checkpoint system. The coding agent edits the user's
//! project directory IN PLACE; before a file-mutating tool (write/edit/apply_patch)
//! touches a file, it calls [`backup_file`] to stash the file's prior contents into
//! an out-of-tree checkpoint directory keyed by `(session, turn)`. [`restore_to`]
//! reverse-applies those backups to undo a range of turns.
//!
//! Shell/bash-driven changes are intentionally NOT captured — only the tools that
//! call `backup_file` are covered (same limitation as Claude Code / Cursor).

use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use crate::error::GitOpsError;
use crate::types::DiffOutput;

/// One backed-up file within a turn.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BackupEntry {
    /// Absolute path of the file in the user's project.
    pub path: String,
    /// `false` => the agent created this file (no prior state); restore deletes it.
    pub existed_before: bool,
    /// Blob filename under `blobs/` holding the prior bytes. `None` iff `!existed_before`.
    pub blob: Option<String>,
}

/// Per-turn manifest, persisted as `manifest.json`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TurnManifest {
    pub version: u32,
    pub session_id: String,
    pub turn: u32,
    pub entries: Vec<BackupEntry>,
}

/// Summary of one available turn (replacement for the old `CheckpointInfo`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TurnInfo {
    pub turn: u32,
    pub file_count: usize,
    /// Absolute paths touched in this turn (from the manifest).
    pub paths: Vec<String>,
}

const MANIFEST_VERSION: u32 = 1;

// ---------------------------------------------------------------------------
// Path / layout helpers
// ---------------------------------------------------------------------------

/// Encode a session id into a single filesystem-safe directory segment.
/// Chars outside `[A-Za-z0-9._-]` are percent-encoded byte-wise (reversible,
/// collision-free). The raw id is also stored inside each manifest.
fn sanitize_session_id(session_id: &str) -> String {
    let mut out = String::with_capacity(session_id.len());
    for &b in session_id.as_bytes() {
        let safe = b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'-');
        if safe {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

fn session_root(checkpoint_dir: &Path, session_id: &str) -> PathBuf {
    checkpoint_dir.join(sanitize_session_id(session_id))
}

fn turn_dir(session_root: &Path, turn: u32) -> PathBuf {
    session_root.join(format!("turn-{turn:04}"))
}

/// Parse the turn number out of a `turn-NNNN` directory name.
fn parse_turn_from_dirname(name: &str) -> Option<u32> {
    name.strip_prefix("turn-")?.parse().ok()
}

async fn read_manifest(turn_dir: &Path) -> Result<Option<TurnManifest>, GitOpsError> {
    let manifest_path = turn_dir.join("manifest.json");
    match tokio::fs::read(&manifest_path).await {
        Ok(bytes) => {
            let manifest = serde_json::from_slice(&bytes).map_err(|e| GitOpsError::CorruptManifest {
                path: manifest_path.to_string_lossy().to_string(),
                reason: e.to_string(),
            })?;
            Ok(Some(manifest))
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(e) => Err(GitOpsError::Io(e)),
    }
}

/// Write the manifest atomically (`manifest.json.tmp` + rename).
async fn write_manifest_atomic(turn_dir: &Path, manifest: &TurnManifest) -> Result<(), GitOpsError> {
    tokio::fs::create_dir_all(turn_dir).await?;
    let json = serde_json::to_vec_pretty(manifest)?;
    let tmp = turn_dir.join("manifest.json.tmp");
    let final_path = turn_dir.join("manifest.json");
    tokio::fs::write(&tmp, &json).await?;
    tokio::fs::rename(&tmp, &final_path).await?;
    Ok(())
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Back up a file's prior state BEFORE a mutating tool writes to it.
///
/// - The FIRST backup of a given path within a turn wins (idempotent per path/turn):
///   this guarantees [`restore_to`] reverts to the turn's *starting* state even if a
///   file is edited multiple times in one turn.
/// - If the file does not currently exist, records `existed_before = false` (no blob)
///   so restore knows to delete the agent-created file.
/// - `file_path` must be absolute.
pub async fn backup_file(
    checkpoint_dir: &Path,
    session_id: &str,
    turn: u32,
    file_path: &Path,
) -> Result<(), GitOpsError> {
    if !file_path.is_absolute() {
        return Err(GitOpsError::NonAbsolutePath(
            file_path.to_string_lossy().to_string(),
        ));
    }

    let root = session_root(checkpoint_dir, session_id);
    let tdir = turn_dir(&root, turn);
    let path_key = file_path.to_string_lossy().to_string();

    // Load (or start) this turn's manifest.
    let mut manifest = read_manifest(&tdir).await?.unwrap_or(TurnManifest {
        version: MANIFEST_VERSION,
        session_id: session_id.to_string(),
        turn,
        entries: Vec::new(),
    });

    // First backup of this path in this turn wins.
    if manifest.entries.iter().any(|e| e.path == path_key) {
        return Ok(());
    }

    // Capture prior state.
    let existed_before = match tokio::fs::metadata(file_path).await {
        Ok(m) => m.is_file(),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => false,
        Err(e) => return Err(GitOpsError::Io(e)),
    };

    let blob = if existed_before {
        let bytes = tokio::fs::read(file_path).await?;
        let blobs_dir = tdir.join("blobs");
        tokio::fs::create_dir_all(&blobs_dir).await?;
        let blob_name = format!("{:04}.blob", manifest.entries.len());
        tokio::fs::write(blobs_dir.join(&blob_name), &bytes).await?;
        Some(blob_name)
    } else {
        None
    };

    manifest.entries.push(BackupEntry {
        path: path_key,
        existed_before,
        blob,
    });
    write_manifest_atomic(&tdir, &manifest).await?;
    Ok(())
}

/// Restore project state to the end of `target_turn` by reverse-applying every turn
/// strictly greater than `target_turn` (newest-first). The edits captured *in*
/// `target_turn` are kept. Returns the number of files rewritten/created/deleted.
///
/// This is a pure inverse — it does NOT prune the undone turns. Callers that want to
/// discard them should follow with [`delete_from`]`(.., target_turn + 1)`.
pub async fn restore_to(
    checkpoint_dir: &Path,
    session_id: &str,
    target_turn: u32,
    project_root: &Path,
) -> Result<usize, GitOpsError> {
    let root = session_root(checkpoint_dir, session_id);
    if tokio::fs::metadata(&root).await.is_err() {
        return Ok(0);
    }

    // Collect turns to undo: turn > target_turn, newest-first.
    let mut turns = list_turn_numbers(&root).await?;
    turns.retain(|&t| t > target_turn);
    turns.sort_unstable_by(|a, b| b.cmp(a)); // descending

    let mut changed = 0usize;
    for t in turns {
        let tdir = turn_dir(&root, t);
        let manifest = match read_manifest(&tdir).await? {
            Some(m) => m,
            None => continue,
        };
        for entry in &manifest.entries {
            let path = PathBuf::from(&entry.path);
            // Manifests live in an app-managed dir; if one were tampered with, an
            // entry path could escape the project. Refuse to write/delete outside.
            if !is_within_root(project_root, &path) {
                log::warn!(
                    "checkpoint restore: skipping out-of-project path {}",
                    entry.path
                );
                continue;
            }
            if !entry.existed_before {
                // Agent created this file in turn t -> remove it.
                match tokio::fs::remove_file(&path).await {
                    Ok(()) => {}
                    Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                    Err(e) => return Err(GitOpsError::Io(e)),
                }
                changed += 1;
            } else {
                // File existed before turn t (edited or deleted) -> rewrite prior bytes.
                let blob_name = entry.blob.as_ref().ok_or_else(|| GitOpsError::CorruptManifest {
                    path: tdir.join("manifest.json").to_string_lossy().to_string(),
                    reason: format!("entry {} has existed_before=true but no blob", entry.path),
                })?;
                let bytes = tokio::fs::read(tdir.join("blobs").join(blob_name)).await?;
                if let Some(parent) = path.parent() {
                    tokio::fs::create_dir_all(parent).await?;
                }
                tokio::fs::write(&path, &bytes).await?;
                changed += 1;
            }
        }
    }

    Ok(changed)
}

/// True if `candidate` stays within `root` after lexically resolving `.`/`..`
/// (no filesystem access, so it works for not-yet-existing paths). Defends
/// `restore_to` against tampered-manifest path traversal and absolute escapes.
fn is_within_root(root: &Path, candidate: &Path) -> bool {
    use std::path::Component;
    let mut normalized = PathBuf::new();
    for comp in candidate.components() {
        match comp {
            Component::ParentDir => {
                if !normalized.pop() {
                    return false;
                }
            }
            Component::CurDir => {}
            other => normalized.push(other.as_os_str()),
        }
    }
    normalized.starts_with(root)
}

/// Human-readable diff (prior vs current on-disk) for a single turn.
/// Generated in-process — there is no git repo to diff against.
pub async fn diff_turn(
    checkpoint_dir: &Path,
    session_id: &str,
    turn: u32,
) -> Result<DiffOutput, GitOpsError> {
    let root = session_root(checkpoint_dir, session_id);
    let tdir = turn_dir(&root, turn);
    let manifest = read_manifest(&tdir)
        .await?
        .ok_or_else(|| GitOpsError::CheckpointNotFound {
            session_id: session_id.to_string(),
            turn,
        })?;

    let mut diff = String::new();
    let mut files_changed = 0u32;
    let mut insertions = 0u32;
    let mut deletions = 0u32;

    for entry in &manifest.entries {
        // Prior bytes: blob contents, or empty for agent-created files.
        let prior: Vec<u8> = match &entry.blob {
            Some(name) => tokio::fs::read(tdir.join("blobs").join(name)).await?,
            None => Vec::new(),
        };
        // Current bytes: on-disk contents, or empty if the file is now absent.
        let current: Vec<u8> = match tokio::fs::read(&entry.path).await {
            Ok(b) => b,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Vec::new(),
            Err(e) => return Err(GitOpsError::Io(e)),
        };

        if prior == current {
            continue;
        }
        files_changed += 1;
        unified_file_diff(&entry.path, &prior, &current, &mut diff, &mut insertions, &mut deletions);
    }

    let stat = format!("{files_changed} files changed, {insertions} insertions(+), {deletions} deletions(-)");
    Ok(DiffOutput {
        diff,
        files_changed,
        insertions,
        deletions,
        stat,
    })
}

/// List available turns for a session, ascending by turn number.
pub async fn list(checkpoint_dir: &Path, session_id: &str) -> Result<Vec<TurnInfo>, GitOpsError> {
    let root = session_root(checkpoint_dir, session_id);
    if tokio::fs::metadata(&root).await.is_err() {
        return Ok(Vec::new());
    }

    let mut turns = list_turn_numbers(&root).await?;
    turns.sort_unstable();

    let mut out = Vec::with_capacity(turns.len());
    for t in turns {
        if let Some(manifest) = read_manifest(&turn_dir(&root, t)).await? {
            out.push(TurnInfo {
                turn: t,
                file_count: manifest.entries.len(),
                paths: manifest.entries.iter().map(|e| e.path.clone()).collect(),
            });
        }
    }
    Ok(out)
}

/// Delete all turn directories with `turn >= from_turn`. Returns the count removed.
pub async fn delete_from(
    checkpoint_dir: &Path,
    session_id: &str,
    from_turn: u32,
) -> Result<u32, GitOpsError> {
    let root = session_root(checkpoint_dir, session_id);
    if tokio::fs::metadata(&root).await.is_err() {
        return Ok(0);
    }

    let turns = list_turn_numbers(&root).await?;
    let mut deleted = 0u32;
    for t in turns {
        if t >= from_turn {
            tokio::fs::remove_dir_all(turn_dir(&root, t)).await?;
            deleted += 1;
        }
    }
    Ok(deleted)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/// Enumerate the turn numbers present under a session root.
async fn list_turn_numbers(root: &Path) -> Result<Vec<u32>, GitOpsError> {
    let mut turns = Vec::new();
    let mut rd = tokio::fs::read_dir(root).await?;
    while let Some(entry) = rd.next_entry().await? {
        if let Some(name) = entry.file_name().to_str() {
            if let Some(t) = parse_turn_from_dirname(name) {
                turns.push(t);
            }
        }
    }
    Ok(turns)
}

/// Append a per-file unified-style diff to `out`, trimming common prefix/suffix lines.
fn unified_file_diff(
    path: &str,
    prior: &[u8],
    current: &[u8],
    out: &mut String,
    insertions: &mut u32,
    deletions: &mut u32,
) {
    match (std::str::from_utf8(prior), std::str::from_utf8(current)) {
        (Ok(p), Ok(c)) => {
            let p_lines: Vec<&str> = p.lines().collect();
            let c_lines: Vec<&str> = c.lines().collect();

            // Common prefix.
            let mut pre = 0;
            while pre < p_lines.len() && pre < c_lines.len() && p_lines[pre] == c_lines[pre] {
                pre += 1;
            }
            // Common suffix (not overlapping the prefix).
            let mut suf = 0;
            while suf < p_lines.len() - pre
                && suf < c_lines.len() - pre
                && p_lines[p_lines.len() - 1 - suf] == c_lines[c_lines.len() - 1 - suf]
            {
                suf += 1;
            }

            out.push_str(&format!("--- a/{path}\n+++ b/{path}\n"));
            for line in &p_lines[pre..p_lines.len() - suf] {
                out.push('-');
                out.push_str(line);
                out.push('\n');
                *deletions += 1;
            }
            for line in &c_lines[pre..c_lines.len() - suf] {
                out.push('+');
                out.push_str(line);
                out.push('\n');
                *insertions += 1;
            }
        }
        _ => {
            out.push_str(&format!("Binary files a/{path} and b/{path} differ\n"));
            *insertions += 1;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use tempfile::tempdir;

    /// Create (project dir, checkpoint dir) temp pair. Returned handles must be kept
    /// alive for the duration of the test.
    fn dirs() -> (tempfile::TempDir, tempfile::TempDir) {
        (tempdir().unwrap(), tempdir().unwrap())
    }

    fn write(path: &PathBuf, content: &str) {
        if let Some(p) = path.parent() {
            std::fs::create_dir_all(p).unwrap();
        }
        std::fs::write(path, content).unwrap();
    }

    fn read(path: &PathBuf) -> String {
        std::fs::read_to_string(path).unwrap()
    }

    #[test]
    fn test_sanitize_session_id_roundtrip_and_no_collision() {
        assert_eq!(sanitize_session_id("plain-id_1.2"), "plain-id_1.2");
        // '/' and ':' get encoded to a single segment with no path separators.
        let s = sanitize_session_id("a/b:c");
        assert!(!s.contains('/'));
        assert!(!s.contains(':'));
        // Distinct ids never collide.
        assert_ne!(sanitize_session_id("a/b"), sanitize_session_id("a-b"));
    }

    #[tokio::test]
    async fn test_backup_edit_then_restore_roundtrip() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("src/main.rs");
        write(&f, "v1");

        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "v2");

        let changed = restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert_eq!(changed, 1);
        assert_eq!(read(&f), "v1");
    }

    #[tokio::test]
    async fn test_backup_created_file_restore_deletes_it() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("new.rs");
        // File does not exist yet — backup records did-not-exist.
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "created by agent");
        assert!(f.exists());

        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert!(!f.exists(), "agent-created file should be removed on restore");
    }

    #[tokio::test]
    async fn test_backup_deleted_file_restore_recreates_it() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("orig.rs");
        write(&f, "orig");

        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        std::fs::remove_file(&f).unwrap();
        assert!(!f.exists());

        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert!(f.exists());
        assert_eq!(read(&f), "orig");
    }

    #[tokio::test]
    async fn test_double_edit_same_turn_reverts_to_turn_start() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "A");

        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "B");
        // Second backup in the same turn must be a no-op (first wins).
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "C");

        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert_eq!(read(&f), "A");
    }

    #[tokio::test]
    async fn test_first_backup_wins_one_entry_and_turn_start_blob() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "start");

        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "mid");
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();

        let turns = list(ckpt.path(), "s1").await.unwrap();
        assert_eq!(turns.len(), 1);
        assert_eq!(turns[0].file_count, 1);

        // The single blob holds the turn-start content.
        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert_eq!(read(&f), "start");
    }

    #[tokio::test]
    async fn test_bash_change_not_covered() {
        let (proj, ckpt) = dirs();
        let tracked = proj.path().join("tracked.txt");
        let untracked = proj.path().join("untracked.txt");
        write(&tracked, "t0");
        backup_file(ckpt.path(), "s1", 1, &tracked).await.unwrap();
        write(&tracked, "t1");

        // A change made WITHOUT going through backup_file (simulating bash).
        write(&untracked, "bash-made");

        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        // tracked reverts; the bash-made file is untouched.
        assert_eq!(read(&tracked), "t0");
        assert_eq!(read(&untracked), "bash-made");
    }

    #[tokio::test]
    async fn test_restore_across_multiple_turns() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "A");
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "B");
        backup_file(ckpt.path(), "s1", 2, &f).await.unwrap();
        write(&f, "C");
        backup_file(ckpt.path(), "s1", 3, &f).await.unwrap();
        write(&f, "D");

        // Restoring to turn 2 undoes only turn 3 -> back to "C".
        restore_to(ckpt.path(), "s1", 2, proj.path()).await.unwrap();
        assert_eq!(read(&f), "C");
    }

    #[tokio::test]
    async fn test_restore_to_target_turn_keeps_its_own_edits() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "A");
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "B"); // change made during turn 1

        // restore_to(1) keeps turn 1's edit (only undoes turn > 1).
        let changed = restore_to(ckpt.path(), "s1", 1, proj.path()).await.unwrap();
        assert_eq!(changed, 0);
        assert_eq!(read(&f), "B");
    }

    #[tokio::test]
    async fn test_restore_created_then_edited_across_turns() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        // Created in turn 1.
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "v1");
        // Edited in turn 2.
        backup_file(ckpt.path(), "s1", 2, &f).await.unwrap();
        write(&f, "v2");

        // Restoring to turn 0: newest-first => turn 2 rewrites "v1", then turn 1 deletes.
        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert!(!f.exists(), "file created then edited should end up absent");
    }

    #[tokio::test]
    async fn test_list_returns_turns_ascending() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "x");
        for t in [0u32, 1, 2] {
            backup_file(ckpt.path(), "s1", t, &f).await.unwrap();
            write(&f, &format!("x{t}"));
        }
        let turns = list(ckpt.path(), "s1").await.unwrap();
        let nums: Vec<u32> = turns.iter().map(|t| t.turn).collect();
        assert_eq!(nums, vec![0, 1, 2]);
        assert!(turns.iter().all(|t| t.file_count == 1));
        assert_eq!(turns[0].paths, vec![f.to_string_lossy().to_string()]);
    }

    #[tokio::test]
    async fn test_delete_from_prunes_future_turns() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "x");
        for t in 0..4u32 {
            backup_file(ckpt.path(), "s1", t, &f).await.unwrap();
            write(&f, &format!("x{t}"));
        }
        let deleted = delete_from(ckpt.path(), "s1", 2).await.unwrap();
        assert_eq!(deleted, 2);
        let nums: Vec<u32> = list(ckpt.path(), "s1").await.unwrap().iter().map(|t| t.turn).collect();
        assert_eq!(nums, vec![0, 1]);
    }

    #[tokio::test]
    async fn test_diff_turn_shows_prior_vs_current() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        write(&f, "old line\n");
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "new line\n");

        let d = diff_turn(ckpt.path(), "s1", 1).await.unwrap();
        assert!(d.diff.contains("old line"));
        assert!(d.diff.contains("new line"));
        assert!(d.insertions > 0);
        assert!(d.deletions > 0);
        assert_eq!(d.files_changed, 1);
    }

    #[tokio::test]
    async fn test_diff_turn_created_file() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("a.txt");
        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        write(&f, "line1\nline2\n");

        let d = diff_turn(ckpt.path(), "s1", 1).await.unwrap();
        assert!(d.diff.contains("line1"));
        assert!(d.insertions >= 2);
        assert_eq!(d.deletions, 0);
    }

    #[tokio::test]
    async fn test_diff_turn_unknown_turn_errors() {
        let (_proj, ckpt) = dirs();
        let err = diff_turn(ckpt.path(), "s1", 5).await;
        assert!(matches!(err, Err(GitOpsError::CheckpointNotFound { .. })));
    }

    #[tokio::test]
    async fn test_restore_nonexistent_session_is_noop() {
        let (_proj, ckpt) = dirs();
        let changed = restore_to(ckpt.path(), "never", 0, _proj.path()).await.unwrap();
        assert_eq!(changed, 0);
        assert!(list(ckpt.path(), "never").await.unwrap().is_empty());
    }

    #[tokio::test]
    async fn test_binary_bytes_roundtrip() {
        let (proj, ckpt) = dirs();
        let f = proj.path().join("blob.bin");
        let original: Vec<u8> = vec![0u8, 159, 146, 150, 255, 1, 2, 3];
        std::fs::write(&f, &original).unwrap();

        backup_file(ckpt.path(), "s1", 1, &f).await.unwrap();
        std::fs::write(&f, [9u8, 9, 9]).unwrap();

        restore_to(ckpt.path(), "s1", 0, proj.path()).await.unwrap();
        assert_eq!(std::fs::read(&f).unwrap(), original);
    }

    #[tokio::test]
    async fn test_backup_rejects_relative_path() {
        let (_proj, ckpt) = dirs();
        let err = backup_file(ckpt.path(), "s1", 1, Path::new("relative/path.txt")).await;
        assert!(matches!(err, Err(GitOpsError::NonAbsolutePath(_))));
    }
}
