#[derive(Debug, thiserror::Error)]
pub enum GitOpsError {
    #[error("git command failed (exit {exit_code}): {stderr}")]
    GitCommand {
        exit_code: i32,
        stderr: String,
        stdout: String,
    },

    #[error("git not found: {0}")]
    GitNotFound(std::io::Error),

    #[error("invalid repo path: {0}")]
    InvalidRepoPath(String),

    #[error("GitHub API error (status {status}): {body}")]
    GitHubApi { status: u16, body: String },

    #[error("no GitHub auth token found")]
    NoAuthToken,

    #[error("invalid remote URL: {0}")]
    InvalidRemoteUrl(String),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("HTTP error: {0}")]
    Http(#[from] reqwest::Error),

    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),
}
