use std::path::Path;
use std::fs;
use tokio::process::Command;

pub async fn init_repo(dir: &Path) {
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
}

pub async fn init_repo_with_commit(dir: &Path) {
    init_repo(dir).await;
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
