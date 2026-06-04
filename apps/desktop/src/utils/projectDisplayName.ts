/**
 * Derive a clean project-name for UI display from a folder path.
 *
 * Coding-thread worktrees live at `<repo>/.agent-worktrees/<thread_id>-coding`.
 * The naive basename is the worktree's `<thread_id>-coding` UUID — ugly for
 * the user. Walk up past `.agent-worktrees` so the chip shows the actual repo
 * name instead.
 *
 * Examples:
 *   /Users/me/boilerplate/.agent-worktrees/d05a37fc-...-coding  →  boilerplate
 *   /Users/me/boilerplate                                       →  boilerplate
 *   /                                                           →  /
 */
export function projectDisplayName(folderPath: string): string {
  if (!folderPath) return folderPath;
  const parts = folderPath.split("/").filter(Boolean);
  const worktreeIdx = parts.lastIndexOf(".agent-worktrees");
  if (worktreeIdx > 0) {
    // Directory containing .agent-worktrees is the repo root.
    return parts[worktreeIdx - 1]!;
  }
  return parts[parts.length - 1] ?? folderPath;
}
