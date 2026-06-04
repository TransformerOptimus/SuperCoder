use serde::{Deserialize, Serialize};

/// Raw output from a git command.
#[derive(Debug, Clone)]
pub struct GitOutput {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

/// A file's status in git.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileStatus {
    pub path: String,
    /// One of "M", "A", "D", "R", "?" etc.
    pub status: String,
}

/// Output of `git status`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StatusOutput {
    pub branch: String,
    pub ahead: u32,
    pub behind: u32,
    pub staged: Vec<FileStatus>,
    pub unstaged: Vec<FileStatus>,
    pub untracked: Vec<FileStatus>,
    pub raw: String,
}

/// Output of `git diff`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiffOutput {
    pub diff: String,
    pub files_changed: u32,
    pub insertions: u32,
    pub deletions: u32,
    pub stat: String,
}

/// Output of `git commit`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommitOutput {
    pub sha: String,
    pub message: String,
    pub files_changed: u32,
}

/// Output of `git push`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PushOutput {
    pub remote: String,
    pub branch: String,
    pub raw: String,
}

/// A git branch.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BranchInfo {
    pub name: String,
    pub is_current: bool,
    pub upstream: Option<String>,
}

/// A git log entry.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEntry {
    pub sha: String,
    pub message: String,
    pub author: String,
    pub date: String,
}

/// Output of PR creation.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PrOutput {
    pub url: String,
    pub number: u64,
}
