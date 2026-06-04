//! Apply the Windows `CREATE_NO_WINDOW` flag to spawned child processes so the
//! Tauri GUI app doesn't flash a console window when running git/grep/bash etc.
//! No-op on macOS and Linux — code is `#[cfg(windows)]`-gated.

#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x08000000;

/// Apply `CREATE_NO_WINDOW` to a `tokio::process::Command` on Windows.
/// Returns the command for fluent chaining.
#[cfg(windows)]
pub fn no_window_tokio(cmd: &mut tokio::process::Command) -> &mut tokio::process::Command {
    cmd.creation_flags(CREATE_NO_WINDOW)
}

/// No-op on non-Windows platforms.
#[cfg(not(windows))]
pub fn no_window_tokio(cmd: &mut tokio::process::Command) -> &mut tokio::process::Command {
    cmd
}

/// Apply `CREATE_NO_WINDOW` to a `std::process::Command` on Windows.
/// Returns the command for fluent chaining.
#[cfg(windows)]
pub fn no_window_std(cmd: &mut std::process::Command) -> &mut std::process::Command {
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(CREATE_NO_WINDOW)
}

/// No-op on non-Windows platforms.
#[cfg(not(windows))]
pub fn no_window_std(cmd: &mut std::process::Command) -> &mut std::process::Command {
    cmd
}
