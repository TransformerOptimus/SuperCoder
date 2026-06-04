import { useState, useCallback } from "react";
import { Dropdown, Button } from "antd";
import { GitCommitHorizontal, ArrowUpFromLine, Github, ChevronDown } from "lucide-react";
import { gitService } from "@/services/gitService";
import { agentTauriService } from "@/services/agentTauriService";
import { useAppStore } from "@/store";
import CommitModal from "./CommitModal";
import CreatePrModal from "./CreatePrModal";
import SuccessModal from "./SuccessModal";

type GitState = 'dirty' | 'committed' | 'pushed' | 'pr_created';

interface GitActionsDropdownProps {
  repoPath: string;
  branch: string;
  filesChanged: number;
  totalAdditions: number;
  totalDeletions: number;
  taskSummary?: string;
}

export default function GitActionsDropdown({
  repoPath,
  branch,
  filesChanged,
  totalAdditions,
  totalDeletions,
  taskSummary,
}: GitActionsDropdownProps) {
  const [gitState, setGitState] = useState<GitState>('dirty');
  const [loading, setLoading] = useState(false);
  const [showCommitModal, setShowCommitModal] = useState(false);
  const [showPrModal, setShowPrModal] = useState(false);
  const [successModal, setSuccessModal] = useState<{
    title: string;
    prUrl?: string;
  } | null>(null);
  const [commitResult, setCommitResult] = useState<{ sha: string; filesChanged: number } | null>(null);

  // Check actual git status to determine if there are uncommitted changes
  const refreshGitState = useCallback(async () => {
    try {
      const status = await gitService.status(repoPath);
      const hasDirtyFiles = status.staged.length > 0 || status.unstaged.length > 0 || status.untracked.length > 0;
      if (hasDirtyFiles) {
        setGitState('dirty');
      }
    } catch {
      // Ignore — status check is best-effort
    }
  }, [repoPath]);

  // Re-check git status when dropdown opens
  const handleDropdownOpenChange = (open: boolean) => {
    if (open) {
      refreshGitState();
    }
  };

  // ── Actions ──

  // After committing, the working tree is clean — refresh the active session's
  // diff stats so the header / "Code Changes" card update to +0 -0 immediately.
  const refreshActiveDiff = async () => {
    const st = useAppStore.getState();
    const sid = st.activeAgentThreadId;
    if (!sid) return;
    try {
      const d = await agentTauriService.getFullDiff(sid);
      st.setThreadDiffStats(sid, d.insertions, d.deletions, d.files_changed);
    } catch {
      /* ignore */
    }
  };

  const doCommit = async (message: string) => {
    const commitMsg = message || `${taskSummary || 'Agent changes'}`;
    const result = await gitService.commit(repoPath, commitMsg);
    setCommitResult({ sha: result.sha, filesChanged: result.files_changed });
    setGitState('committed');
    await refreshActiveDiff();
    return result;
  };

  const doPush = async () => {
    await gitService.push(repoPath, branch);
    setGitState('pushed');
  };

  const doCreatePr = async (title: string, body: string) => {
    const result = await gitService.createPr(repoPath, title, body, branch, 'main');
    setGitState('pr_created');
    return result;
  };

  // ── Handlers ──

  const handleCommitSubmit = async (message: string, nextStep: string) => {
    setLoading(true);
    try {
      await doCommit(message);
      setShowCommitModal(false);

      if (nextStep === 'commit_push') {
        await doPush();
        setSuccessModal({ title: 'Changes committed & pushed' });
      } else if (nextStep === 'commit_push_pr') {
        await doPush();
        setShowPrModal(true);
      } else {
        setSuccessModal({ title: 'Changes committed' });
      }
    } catch (err: any) {
      const errMsg = err?.message || String(err);
      console.error('[GitActions] Commit flow failed:', errMsg);
      // Handle "nothing to commit" — treat as already committed
      if (errMsg.includes('nothing to commit') || errMsg.includes('exit 1')) {
        setShowCommitModal(false);
        setGitState('committed');
        setSuccessModal({ title: 'No changes to commit' });
      }
    } finally {
      setLoading(false);
    }
  };

  const handlePush = async () => {
    setLoading(true);
    try {
      await doPush();
      setSuccessModal({ title: 'Changes pushed' });
    } catch (err: any) {
      console.error('[GitActions] Push failed:', err?.message || err);
    } finally {
      setLoading(false);
    }
  };

  const handleCreatePrSubmit = async (title: string, body: string) => {
    setLoading(true);
    try {
      const result = await doCreatePr(title, body);
      setShowPrModal(false);
      setSuccessModal({ title: 'Pull request created', prUrl: result.url });
    } catch (err: any) {
      console.error('[GitActions] Create PR failed:', err?.message || err);
    } finally {
      setLoading(false);
    }
  };

  // ── Dropdown label ──

  const buttonLabel = gitState === 'dirty' ? 'Commit'
    : gitState === 'committed' ? 'Push'
    : gitState === 'pushed' ? 'Create PR'
    : 'Done';

  const ButtonIcon = gitState === 'dirty' ? GitCommitHorizontal
    : gitState === 'committed' ? ArrowUpFromLine
    : Github;

  // ── Menu items ──

  const menuItems = [
    {
      key: 'commit',
      icon: <GitCommitHorizontal size={14} />,
      label: 'Commit',
      disabled: gitState !== 'dirty',
      onClick: () => setShowCommitModal(true),
    },
    {
      key: 'push',
      icon: <ArrowUpFromLine size={14} />,
      label: 'Push',
      disabled: gitState !== 'committed',
      onClick: handlePush,
    },
    {
      key: 'create_pr',
      icon: <Github size={14} />,
      label: 'Create PR',
      disabled: gitState !== 'pushed',
      onClick: () => setShowPrModal(true),
    },
  ];

  return (
    <>
      <Dropdown
        menu={{ items: menuItems }}
        trigger={['click']}
        placement="bottomRight"
        onOpenChange={handleDropdownOpenChange}
      >
        <Button className="secondary_small" loading={loading}>
          <ButtonIcon size={14} />
          {buttonLabel}
          <ChevronDown size={12} />
        </Button>
      </Dropdown>

      <CommitModal
        open={showCommitModal}
        branch={branch}
        filesChanged={filesChanged}
        additions={totalAdditions}
        deletions={totalDeletions}
        loading={loading}
        onSubmit={handleCommitSubmit}
        onCancel={() => setShowCommitModal(false)}
      />

      <CreatePrModal
        open={showPrModal}
        branch={branch}
        defaultTitle={taskSummary || ''}
        loading={loading}
        onSubmit={handleCreatePrSubmit}
        onCancel={() => setShowPrModal(false)}
      />

      {successModal && (
        <SuccessModal
          open
          title={successModal.title}
          branch={branch}
          filesChanged={commitResult?.filesChanged}
          additions={totalAdditions}
          deletions={totalDeletions}
          prUrl={successModal.prUrl}
          onClose={() => setSuccessModal(null)}
        />
      )}
    </>
  );
}
