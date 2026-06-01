use std::path::Path;
use std::process::Command;
use std::sync::OnceLock;

/// Cached IDE detection — runs `which cursor` at most once per process lifetime,
/// avoiding repeated blocking subprocess calls from the async runtime.
static DETECTED_IDE: OnceLock<String> = OnceLock::new();

/// Detect the available IDE. Tries `cursor` first, falls back to `code`.
/// Result is cached after the first call.
fn detect_ide() -> &'static str {
    DETECTED_IDE.get_or_init(|| {
        if Command::new("which")
            .arg("cursor")
            .output()
            .map(|o| o.status.success())
            .unwrap_or(false)
        {
            "cursor".to_string()
        } else {
            "code".to_string()
        }
    })
}

/// Open a file in the IDE. Fire-and-forget.
pub fn open_file(file_path: &Path, ide: Option<&str>) {
    let ide = ide.unwrap_or_else(|| detect_ide());
    if let Err(e) = Command::new(ide).arg(file_path).spawn() {
        log::debug!("Failed to open file in {ide}: {e}");
    }
}

/// Open a diff between two files in the IDE. Fire-and-forget.
pub fn open_diff(file_a: &Path, file_b: &Path, ide: Option<&str>) {
    let ide = ide.unwrap_or_else(|| detect_ide());
    if let Err(e) = Command::new(ide).arg("--diff").arg(file_a).arg(file_b).spawn() {
        log::debug!("Failed to open diff in {ide}: {e}");
    }
}

/// Open a project directory in the IDE. Fire-and-forget.
pub fn open_project(dir_path: &Path, ide: Option<&str>) {
    let ide = ide.unwrap_or_else(|| detect_ide());
    if let Err(e) = Command::new(ide).arg(dir_path).spawn() {
        log::debug!("Failed to open project in {ide}: {e}");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_ide_returns_something() {
        let ide = detect_ide();
        assert!(ide == "cursor" || ide == "code", "Got: {ide}");
    }

    #[test]
    fn test_detect_ide_is_cached() {
        // Calling twice should return the same pointer (cached)
        let first = detect_ide();
        let second = detect_ide();
        assert!(std::ptr::eq(first, second));
    }

    #[test]
    #[ignore] // Launches a process
    fn test_open_file() {
        open_file(Path::new("/tmp/test.txt"), Some("echo"));
    }

    #[test]
    #[ignore] // Launches a process
    fn test_open_diff() {
        open_diff(
            Path::new("/tmp/a.txt"),
            Path::new("/tmp/b.txt"),
            Some("echo"),
        );
    }

    #[test]
    #[ignore] // Launches a process
    fn test_open_project() {
        open_project(Path::new("/tmp"), Some("echo"));
    }
}
