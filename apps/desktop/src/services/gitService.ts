import { invoke } from '@tauri-apps/api/core';
import type {
  BranchInfo,
  CommitOutput,
  DiffOutput,
  LogEntry,
  PrOutput,
  PushOutput,
  StatusOutput,
} from '../types/git';

export const gitService = {
  async branches(repoPath: string): Promise<BranchInfo[]> {
    return invoke('git_branches', { repoPath });
  },

  async diff(repoPath: string): Promise<DiffOutput> {
    return invoke('git_diff', { repoPath });
  },

  async status(repoPath: string): Promise<StatusOutput> {
    return invoke('git_status', { repoPath });
  },

  async commit(repoPath: string, message: string): Promise<CommitOutput> {
    return invoke('git_commit', { repoPath, message });
  },

  async push(repoPath: string, branch: string): Promise<PushOutput> {
    return invoke('git_push', { repoPath, branch });
  },

  async log(repoPath: string, count?: number): Promise<LogEntry[]> {
    return invoke('git_log', { repoPath, count: count ?? 20 });
  },

  async createBranch(repoPath: string, name: string, from?: string): Promise<void> {
    return invoke('git_create_branch', { repoPath, name, from: from ?? null });
  },

  async switchBranch(repoPath: string, name: string): Promise<void> {
    return invoke('git_switch_branch', { repoPath, name });
  },

  async createPr(
    repoPath: string,
    title: string,
    body: string,
    branch: string,
    base: string,
  ): Promise<PrOutput> {
    return invoke('git_create_pr', { repoPath, title, body, branch, base });
  },
};
