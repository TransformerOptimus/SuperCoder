use std::path::Path;

use crate::error::GitOpsError;
use crate::exec::run_git;
use crate::types::PrOutput;

/// Parse a GitHub remote URL into (owner, repo).
/// Handles HTTPS and SSH formats:
/// - `https://github.com/owner/repo.git`
/// - `https://github.com/owner/repo`
/// - `git@github.com:owner/repo.git`
/// - `git@github.com:owner/repo`
pub fn parse_remote_url(url: &str) -> Result<(String, String), GitOpsError> {
    let url = url.trim();

    // SSH format: git@github.com:owner/repo.git
    if let Some(rest) = url.strip_prefix("git@github.com:") {
        let rest = rest.strip_suffix(".git").unwrap_or(rest);
        let parts: Vec<&str> = rest.splitn(2, '/').collect();
        if parts.len() == 2 && !parts[0].is_empty() && !parts[1].is_empty() {
            return Ok((parts[0].to_string(), parts[1].to_string()));
        }
        return Err(GitOpsError::InvalidRemoteUrl(url.to_string()));
    }

    // HTTPS format: https://github.com/owner/repo.git
    // Use strip_prefix for an exact match — a substring check would accept
    // crafted URLs like "https://evil.com/redirect?to=github.com/owner/repo".
    let https_prefix = url
        .strip_prefix("https://github.com/")
        .or_else(|| url.strip_prefix("http://github.com/"));
    if let Some(after) = https_prefix {
        let after = after.strip_suffix(".git").unwrap_or(after);
        let parts: Vec<&str> = after.splitn(2, '/').collect();
        if parts.len() == 2 && !parts[0].is_empty() && !parts[1].is_empty() {
            return Ok((parts[0].to_string(), parts[1].to_string()));
        }
    }

    Err(GitOpsError::InvalidRemoteUrl(url.to_string()))
}

/// Resolve the `gh` CLI binary path.
/// GUI apps often don't inherit the user's shell PATH, so package-manager-installed
/// binaries like `gh` won't be found via a bare `Command::new("gh")`.
/// We check well-known install locations per platform before falling back to a bare name.
fn resolve_gh() -> String {
    #[cfg(target_os = "windows")]
    let candidates: &[&str] = &[
        r"C:\Program Files\GitHub CLI\gh.exe",
        r"C:\Program Files (x86)\GitHub CLI\gh.exe",
    ];

    #[cfg(not(target_os = "windows"))]
    let candidates: &[&str] = &[
        "/opt/homebrew/bin/gh",      // macOS ARM Homebrew
        "/usr/local/bin/gh",         // macOS Intel Homebrew / Linux linuxbrew
        "/usr/bin/gh",               // Linux system package managers (apt, dnf, etc.)
        "/snap/bin/gh",              // Ubuntu snap
        "/home/linuxbrew/.linuxbrew/bin/gh", // Linux Homebrew (default prefix)
    ];

    for path in candidates {
        if std::path::Path::new(path).exists() {
            return path.to_string();
        }
    }

    // Also check user-local linuxbrew on Linux
    #[cfg(target_os = "linux")]
    if let Ok(home) = std::env::var("HOME") {
        let linuxbrew = format!("{home}/.linuxbrew/bin/gh");
        if std::path::Path::new(&linuxbrew).exists() {
            return linuxbrew;
        }
    }

    // Fallback — may work if PATH is set correctly (e.g. launched from terminal)
    #[cfg(target_os = "windows")]
    return "gh.exe".to_string();

    #[cfg(not(target_os = "windows"))]
    return "gh".to_string();
}

/// Get GitHub auth token. Tries:
/// 1. `GITHUB_TOKEN` environment variable
/// 2. `gh auth token` CLI command
pub async fn auth_token() -> Result<String, GitOpsError> {
    // Try env var first
    if let Ok(token) = std::env::var("GITHUB_TOKEN") {
        if !token.is_empty() {
            return Ok(token);
        }
    }

    // Try gh CLI
    let gh = resolve_gh();
    let mut cmd = tokio::process::Command::new(&gh);
    cmd.args(["auth", "token"]);
    crate::no_window::no_window_tokio(&mut cmd);
    let output = cmd.output().await;

    match output {
        Ok(out) if out.status.success() => {
            let token = String::from_utf8_lossy(&out.stdout).trim().to_string();
            if !token.is_empty() {
                return Ok(token);
            }
            Err(GitOpsError::NoAuthToken)
        }
        _ => Err(GitOpsError::NoAuthToken),
    }
}

/// Check whether a branch exists on the remote.
pub async fn branch_exists_on_remote(
    repo_path: &Path,
    branch: &str,
    remote: &str,
) -> Result<bool, GitOpsError> {
    let output = run_git(repo_path, &["ls-remote", "--heads", remote, branch]).await?;
    // ls-remote prints one line per matching ref; empty output means no match
    Ok(!output.stdout.trim().is_empty())
}

/// Create a pull request on GitHub.
pub async fn create(
    repo_path: &Path,
    title: &str,
    body: &str,
    branch: &str,
    base: &str,
) -> Result<PrOutput, GitOpsError> {
    // Get remote URL
    let remote_output = run_git(repo_path, &["remote", "get-url", "origin"]).await?;
    let remote_url = remote_output.stdout.trim();

    let (owner, repo) = parse_remote_url(remote_url)?;

    // Verify branch is pushed before making the API call
    if !branch_exists_on_remote(repo_path, branch, "origin").await? {
        return Err(GitOpsError::GitCommand {
            exit_code: 1,
            stderr: format!(
                "Branch '{branch}' not found on remote 'origin'. Push it first with: git push -u origin {branch}"
            ),
            stdout: String::new(),
        });
    }

    let token = auth_token().await?;

    let client = reqwest::Client::new();
    let url = format!("https://api.github.com/repos/{owner}/{repo}/pulls");

    let body_json = serde_json::json!({
        "title": title,
        "body": body,
        "head": branch,
        "base": base,
    });

    let response = client
        .post(&url)
        .header("Authorization", format!("token {token}"))
        .header("Accept", "application/vnd.github.v3+json")
        .header("User-Agent", "git-ops")
        .json(&body_json)
        .send()
        .await?;

    let status = response.status().as_u16();
    let response_body: serde_json::Value = response.json().await?;

    if status == 422 {
        // PR already exists — try to find the existing PR URL
        let existing = find_existing_pr(repo_path, branch, &owner, &repo, &token).await;
        if let Some(pr) = existing {
            return Ok(pr);
        }
        return Err(GitOpsError::GitHubApi {
            status,
            body: response_body.to_string(),
        });
    }

    if status != 201 {
        return Err(GitOpsError::GitHubApi {
            status,
            body: response_body.to_string(),
        });
    }

    let pr_url = response_body["html_url"]
        .as_str()
        .unwrap_or("")
        .to_string();
    let number = response_body["number"].as_u64().unwrap_or(0);

    Ok(PrOutput {
        url: pr_url,
        number,
    })
}

/// Find an existing open PR for a branch.
async fn find_existing_pr(
    _repo_path: &Path,
    branch: &str,
    owner: &str,
    repo: &str,
    token: &str,
) -> Option<PrOutput> {
    let client = reqwest::Client::new();
    let url = format!("https://api.github.com/repos/{owner}/{repo}/pulls?head={owner}:{branch}&state=open");

    let response = client
        .get(&url)
        .header("Authorization", format!("token {token}"))
        .header("Accept", "application/vnd.github.v3+json")
        .header("User-Agent", "git-ops")
        .send()
        .await
        .ok()?;

    let prs: serde_json::Value = response.json().await.ok()?;
    let first = prs.as_array()?.first()?;

    Some(PrOutput {
        url: first["html_url"].as_str()?.to_string(),
        number: first["number"].as_u64().unwrap_or(0),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    /// Mutex to serialize tests that mutate the GITHUB_TOKEN env var.
    /// Rust runs tests in parallel within a process, so concurrent
    /// set_var / remove_var on the same env var causes data races.
    static ENV_MUTEX: Mutex<()> = Mutex::new(());

    #[test]
    fn test_parse_https_url() {
        let (owner, repo) = parse_remote_url("https://github.com/acme/myrepo.git").unwrap();
        assert_eq!(owner, "acme");
        assert_eq!(repo, "myrepo");
    }

    #[test]
    fn test_parse_https_url_no_git_suffix() {
        let (owner, repo) = parse_remote_url("https://github.com/acme/myrepo").unwrap();
        assert_eq!(owner, "acme");
        assert_eq!(repo, "myrepo");
    }

    #[test]
    fn test_parse_ssh_url() {
        let (owner, repo) = parse_remote_url("git@github.com:acme/myrepo.git").unwrap();
        assert_eq!(owner, "acme");
        assert_eq!(repo, "myrepo");
    }

    #[test]
    fn test_parse_ssh_url_no_git_suffix() {
        let (owner, repo) = parse_remote_url("git@github.com:acme/myrepo").unwrap();
        assert_eq!(owner, "acme");
        assert_eq!(repo, "myrepo");
    }

    #[test]
    fn test_parse_invalid_url() {
        let result = parse_remote_url("not-a-url");
        assert!(matches!(result, Err(GitOpsError::InvalidRemoteUrl(_))));
    }

    #[test]
    fn test_parse_empty_url() {
        let result = parse_remote_url("");
        assert!(matches!(result, Err(GitOpsError::InvalidRemoteUrl(_))));
    }

    #[test]
    fn test_parse_github_url_missing_repo() {
        let result = parse_remote_url("https://github.com/acme/");
        assert!(matches!(result, Err(GitOpsError::InvalidRemoteUrl(_))));
    }

    #[test]
    fn test_parse_spoofed_url_rejected() {
        // A URL that merely contains "github.com/" as a substring should NOT match
        let result = parse_remote_url("https://evil.com/redirect?to=github.com/owner/repo");
        assert!(matches!(result, Err(GitOpsError::InvalidRemoteUrl(_))));
    }

    #[test]
    fn test_parse_subdomain_spoofed_url_rejected() {
        let result = parse_remote_url("https://not-github.com/github.com/owner/repo");
        assert!(matches!(result, Err(GitOpsError::InvalidRemoteUrl(_))));
    }

    #[tokio::test]
    async fn test_auth_token_from_env() {
        // Hold mutex to prevent other tests from seeing our env var mutation
        let _lock = ENV_MUTEX.lock().unwrap();

        let original = std::env::var("GITHUB_TOKEN").ok();
        std::env::set_var("GITHUB_TOKEN", "test-token-123");

        let token = auth_token().await.unwrap();
        assert_eq!(token, "test-token-123");

        // Restore
        match original {
            Some(val) => std::env::set_var("GITHUB_TOKEN", val),
            None => std::env::remove_var("GITHUB_TOKEN"),
        }
    }

    #[tokio::test]
    async fn test_branch_exists_on_remote_no_remote() {
        // In a repo with no remote, ls-remote should fail
        let dir = tempfile::tempdir().unwrap();
        crate::test_util::init_repo_with_commit(dir.path()).await;

        let result = branch_exists_on_remote(dir.path(), "main", "origin").await;
        // Should error because there's no remote named "origin"
        assert!(result.is_err());
    }

    #[tokio::test]
    #[ignore] // Requires real GitHub token and repo
    async fn test_create_pr() {
        // This test would need a real repo with a remote
    }
}
