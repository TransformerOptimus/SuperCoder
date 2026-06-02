// ============================================================
// Git Operations — TypeScript mirrors of src-tauri/git-ops/src/types.rs
// ============================================================

export interface FileStatus {
  path: string;
  status: string;
}

export interface StatusOutput {
  branch: string;
  ahead: number;
  behind: number;
  staged: FileStatus[];
  unstaged: FileStatus[];
  untracked: FileStatus[];
  raw: string;
}

export interface DiffOutput {
  diff: string;
  files_changed: number;
  insertions: number;
  deletions: number;
  stat: string;
}

export interface CommitOutput {
  sha: string;
  message: string;
  files_changed: number;
}

export interface PushOutput {
  remote: string;
  branch: string;
  raw: string;
}

export interface BranchInfo {
  name: string;
  is_current: boolean;
  upstream: string | null;
}

export interface LogEntry {
  sha: string;
  message: string;
  author: string;
  date: string;
}

export interface PrOutput {
  url: string;
  number: number;
}
