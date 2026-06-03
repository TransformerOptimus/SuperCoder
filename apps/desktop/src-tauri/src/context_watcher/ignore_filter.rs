use ignore::gitignore::{Gitignore, GitignoreBuilder};
use std::path::{Path, PathBuf};

const BLOCKED_DIRS: &[&str] = &[
    ".git",
    "node_modules",
    "target",
    "dist",
    "build",
    "__pycache__",
    ".next",
    "vendor",
    ".agent-worktrees",
    // Credential / config dirs — never sync to the context engine
    ".ssh",
    ".aws",
    ".gnupg",
];

const BLOCKED_EXTENSIONS: &[&str] = &[
    "lock", "png", "jpg", "jpeg", "gif", "svg", "ico", "webp", "woff", "woff2", "ttf", "eot",
    "mp3", "mp4", "avi", "mov", "zip", "tar", "gz", "bz2", "7z", "rar", "jar", "war", "exe",
    "dll", "so", "dylib", "o", "a", "pyc", "class", "wasm", "map",
    // Credential / secret file extensions — never sync to the context engine
    "pem", "key", "cer", "crt", "p12", "pfx", "jks", "keystore", "asc", "gpg",
];

/// Compound extensions that need special matching (contain dots).
const BLOCKED_COMPOUND_EXTENSIONS: &[&str] = &["min.js", "min.css"];

/// Exact filenames (case-insensitive) that are always excluded — common secret files
/// that don't have a recognizable extension. This is a safety net on top of .gitignore,
/// which many users forget to update for local secret files.
const BLOCKED_FILENAMES: &[&str] = &[
    ".env",
    ".env.local",
    ".env.development",
    ".env.production",
    ".env.staging",
    ".env.test",
    "credentials",
    "credentials.json",
    "secrets.yaml",
    "secrets.yml",
    "secrets.json",
    "id_rsa",
    "id_dsa",
    "id_ecdsa",
    "id_ed25519",
    ".netrc",
    ".pgpass",
    ".npmrc",
    ".pypirc",
];

/// Filename prefixes that indicate a secret file (e.g., `.env.anything`).
const BLOCKED_FILENAME_PREFIXES: &[&str] = &[".env."];

/// Max file size to sync (1 MB).
const MAX_FILE_SIZE: u64 = 1_048_576;

pub struct IgnoreFilter {
    gitignore: Option<Gitignore>,
    repo_root: PathBuf,
}

impl IgnoreFilter {
    /// Build filter for a repo. Loads `.gitignore` if present.
    pub fn new(repo_root: &Path) -> Self {
        let gitignore_path = repo_root.join(".gitignore");
        let gitignore = if gitignore_path.exists() {
            let mut builder = GitignoreBuilder::new(repo_root);
            builder.add(gitignore_path);
            builder.build().ok()
        } else {
            None
        };

        Self {
            gitignore,
            repo_root: repo_root.to_path_buf(),
        }
    }

    /// Returns `true` if a directory should be walked into (not blocked).
    pub fn should_walk_dir(&self, dir: &Path) -> bool {
        !self.is_blocked_dir(dir)
    }

    /// Returns `true` if the file should be synced (not ignored).
    pub fn should_include(&self, path: &Path) -> bool {
        // Check blocked directory components
        if self.is_blocked_dir(path) {
            return false;
        }

        // Check blocked filenames (secrets like .env, credentials, id_rsa)
        if self.is_blocked_filename(path) {
            return false;
        }

        // Check blocked extensions
        if self.has_blocked_extension(path) {
            return false;
        }

        // Check .gitignore
        if let Some(ref gi) = self.gitignore {
            let relative = path.strip_prefix(&self.repo_root).unwrap_or(path);
            let is_dir = path.is_dir();
            if gi.matched(relative, is_dir).is_ignore() {
                return false;
            }
        }

        // Check file size (only for existing files)
        if let Ok(meta) = std::fs::metadata(path) {
            if meta.is_file() && meta.len() > MAX_FILE_SIZE {
                return false;
            }
        }

        true
    }

    /// Check if any path component is a blocked directory.
    fn is_blocked_dir(&self, path: &Path) -> bool {
        for component in path.components() {
            if let std::path::Component::Normal(name) = component {
                if let Some(name_str) = name.to_str() {
                    if BLOCKED_DIRS.contains(&name_str) {
                        return true;
                    }
                }
            }
        }
        false
    }

    /// Check if the file's base name matches a known secret file
    /// (case-insensitive exact match or prefix match).
    fn is_blocked_filename(&self, path: &Path) -> bool {
        let file_name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n,
            None => return false,
        };
        let lower = file_name.to_ascii_lowercase();

        if BLOCKED_FILENAMES.iter().any(|&n| n == lower) {
            return true;
        }
        if BLOCKED_FILENAME_PREFIXES.iter().any(|&p| lower.starts_with(p)) {
            return true;
        }
        false
    }

    /// Check if the file has a blocked extension.
    fn has_blocked_extension(&self, path: &Path) -> bool {
        let file_name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n,
            None => return false,
        };

        // Check compound extensions (e.g., "foo.min.js" but NOT "admin.js")
        for ext in BLOCKED_COMPOUND_EXTENSIONS {
            if file_name.ends_with(&format!(".{ext}")) {
                return true;
            }
        }

        // Check simple extensions
        if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
            if BLOCKED_EXTENSIONS.contains(&ext) {
                return true;
            }
        }

        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    fn make_filter(dir: &Path) -> IgnoreFilter {
        IgnoreFilter::new(dir)
    }

    #[test]
    fn test_blocked_dirs() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        assert!(!filter.should_include(&tmp.path().join("node_modules/foo.js")));
        assert!(!filter.should_include(&tmp.path().join(".git/config")));
        assert!(!filter.should_include(&tmp.path().join("target/debug/agent")));
        assert!(!filter.should_include(&tmp.path().join("__pycache__/mod.pyc")));
        assert!(!filter.should_include(&tmp.path().join(".agent-worktrees/ws1/file.rs")));
        // Credential dirs
        assert!(!filter.should_include(&tmp.path().join(".ssh/id_rsa")));
        assert!(!filter.should_include(&tmp.path().join(".aws/credentials")));
        assert!(!filter.should_include(&tmp.path().join(".gnupg/private-keys-v1.d/x")));
    }

    #[test]
    fn test_blocked_secret_filenames() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        // Create the files so the metadata check passes
        fs::write(tmp.path().join(".env"), "SECRET=1").unwrap();
        fs::write(tmp.path().join(".env.local"), "SECRET=1").unwrap();
        fs::write(tmp.path().join(".env.production"), "SECRET=1").unwrap();
        fs::write(tmp.path().join("credentials.json"), "{}").unwrap();
        fs::write(tmp.path().join("secrets.yaml"), "x: y").unwrap();
        fs::write(tmp.path().join("id_rsa"), "----- BEGIN -----").unwrap();
        fs::write(tmp.path().join(".netrc"), "machine x").unwrap();

        assert!(!filter.should_include(&tmp.path().join(".env")));
        assert!(!filter.should_include(&tmp.path().join(".env.local")));
        assert!(!filter.should_include(&tmp.path().join(".env.production")));
        // Arbitrary .env.* suffix still blocked (prefix match)
        fs::write(tmp.path().join(".env.custom"), "x=1").unwrap();
        assert!(!filter.should_include(&tmp.path().join(".env.custom")));
        assert!(!filter.should_include(&tmp.path().join("credentials.json")));
        assert!(!filter.should_include(&tmp.path().join("secrets.yaml")));
        assert!(!filter.should_include(&tmp.path().join("id_rsa")));
        assert!(!filter.should_include(&tmp.path().join(".netrc")));

        // Case-insensitive
        fs::write(tmp.path().join(".ENV"), "x=1").unwrap();
        assert!(!filter.should_include(&tmp.path().join(".ENV")));
    }

    #[test]
    fn test_blocked_cert_key_extensions() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        for ext in &["pem", "key", "p12", "pfx", "crt", "cer", "gpg"] {
            let p = tmp.path().join(format!("cert.{ext}"));
            fs::write(&p, "x").unwrap();
            assert!(
                !filter.should_include(&p),
                "cert.{ext} should be excluded",
            );
        }
    }

    #[test]
    fn test_blocked_extensions() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        // Create actual files so metadata check works
        let png = tmp.path().join("image.png");
        fs::write(&png, "x").unwrap();
        assert!(!filter.should_include(&png));

        let woff = tmp.path().join("font.woff");
        fs::write(&woff, "x").unwrap();
        assert!(!filter.should_include(&woff));

        let zip = tmp.path().join("archive.zip");
        fs::write(&zip, "x").unwrap();
        assert!(!filter.should_include(&zip));

        let exe = tmp.path().join("binary.exe");
        fs::write(&exe, "x").unwrap();
        assert!(!filter.should_include(&exe));
    }

    #[test]
    fn test_lock_files() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        // .lock extension → blocked
        let cargo_lock = tmp.path().join("Cargo.lock");
        fs::write(&cargo_lock, "content").unwrap();
        assert!(!filter.should_include(&cargo_lock));

        let yarn_lock = tmp.path().join("yarn.lock");
        fs::write(&yarn_lock, "content").unwrap();
        assert!(!filter.should_include(&yarn_lock));

        // package-lock.json has .json extension — NOT blocked by extension
        // (it would be blocked by .gitignore in real repos, not our filter)
        let pkg_lock = tmp.path().join("package-lock.json");
        fs::write(&pkg_lock, "{}").unwrap();
        assert!(filter.should_include(&pkg_lock));
    }

    #[test]
    fn test_source_files_included() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        let rs = tmp.path().join("src/main.rs");
        fs::create_dir_all(rs.parent().unwrap()).unwrap();
        fs::write(&rs, "fn main() {}").unwrap();
        assert!(filter.should_include(&rs));

        let ts = tmp.path().join("index.ts");
        fs::write(&ts, "export {}").unwrap();
        assert!(filter.should_include(&ts));

        let go = tmp.path().join("lib/auth.go");
        fs::create_dir_all(go.parent().unwrap()).unwrap();
        fs::write(&go, "package lib").unwrap();
        assert!(filter.should_include(&go));
    }

    #[test]
    fn test_gitignore_respected() {
        let tmp = TempDir::new().unwrap();

        // Create .gitignore
        fs::write(tmp.path().join(".gitignore"), "*.log\n").unwrap();

        let filter = make_filter(tmp.path());

        let log_file = tmp.path().join("app.log");
        fs::write(&log_file, "log data").unwrap();
        assert!(!filter.should_include(&log_file));

        // Non-ignored file should pass
        let txt = tmp.path().join("readme.txt");
        fs::write(&txt, "hello").unwrap();
        assert!(filter.should_include(&txt));
    }

    #[test]
    fn test_max_file_size() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        // Create file > 1 MB
        let big = tmp.path().join("big.txt");
        let data = vec![b'x'; MAX_FILE_SIZE as usize + 1];
        fs::write(&big, &data).unwrap();
        assert!(!filter.should_include(&big));

        // File exactly at limit should be included
        let ok = tmp.path().join("ok.txt");
        let data = vec![b'x'; MAX_FILE_SIZE as usize];
        fs::write(&ok, &data).unwrap();
        assert!(filter.should_include(&ok));
    }

    #[test]
    fn test_should_walk_dir() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        assert!(!filter.should_walk_dir(&tmp.path().join("node_modules")));
        assert!(!filter.should_walk_dir(&tmp.path().join(".git")));
        assert!(!filter.should_walk_dir(&tmp.path().join("target")));
        assert!(filter.should_walk_dir(&tmp.path().join("src")));
        assert!(filter.should_walk_dir(&tmp.path().join("lib")));
    }

    #[test]
    fn test_nested_blocked_dir() {
        let tmp = TempDir::new().unwrap();
        let filter = make_filter(tmp.path());

        assert!(!filter.should_include(&tmp.path().join("foo/bar/node_modules/baz.js")));
        assert!(!filter.should_include(&tmp.path().join("deep/nested/.git/objects/pack")));
    }
}
