import { useState } from 'react';
import { Dropdown } from 'antd';
import { FileText, X } from 'lucide-react';
import { useAppStore } from '@/store';
import { agentTauriService } from '@/services/agentTauriService';
import type { PermissionLevel } from '@/types/agentContract';
import Markdown from '../../common/Markdown';
import styles from './PlanThreadPanel.module.css';

export default function PlanThreadPanel() {
  const activePlanProjectPath = useAppStore((s) => s.activePlanProjectPath);
  const completedPlan = useAppStore((s) =>
    activePlanProjectPath ? s.completedPlans[activePlanProjectPath] : undefined,
  );
  const setActivePlanProjectPath = useAppStore((s) => s.setActivePlanProjectPath);
  const agentBranch = useAppStore((s) => s.agentBranch);
  const [implementing, setImplementing] = useState(false);

  if (!completedPlan) return null;

  const { text: planText, projectPath, planPath } = completedPlan;

  const handleImplement = async (level: PermissionLevel) => {
    setImplementing(true);
    try {
      const store = useAppStore.getState();
      // Stash the plan for handoff to the coding thread (cleared on session-complete)
      store.setPendingPlanForCoding({ text: planText, projectPath, planPath });
      await agentTauriService.setPermission({
        project_path: projectPath,
        level,
        tool_overrides: null,
      });
      await agentTauriService.startCodingFromPlan(
        projectPath,
        planText,
        planPath,
        agentBranch ?? undefined,
      );
      setActivePlanProjectPath(null);
    } catch (err) {
      console.error('[PlanThreadPanel] Failed to start coding from plan:', err);
      // Revert the pending plan on failure so user can retry
      useAppStore.getState().setPendingPlanForCoding(null);
      setImplementing(false);
    }
  };

  const implementMenu = {
    items: [
      { key: 'auto', label: 'Auto-accept edits' },
      { key: 'approve', label: 'Approve each edit' },
    ],
    onClick: ({ key }: { key: string }) =>
      handleImplement(key === 'auto' ? 'AutoApproveAll' : 'ApproveEverything'),
  };

  const handleClose = () => setActivePlanProjectPath(null);

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <FileText className="w-4 h-4 text-purple-500 shrink-0" />
        <span className={styles.title}>Implementation Plan</span>
        <Dropdown menu={implementMenu} trigger={['click']} disabled={implementing}>
          <button className="primary_small" disabled={implementing}>
            {implementing ? 'Starting...' : 'Implement \u25BE'}
          </button>
        </Dropdown>
        <X
          className="w-4 h-4 cursor-pointer text-[var(--text-secondary)] hover:text-[var(--text-primary)] shrink-0"
          onClick={handleClose}
        />
      </div>

      <div className={styles.body}>
        <Markdown className={`${styles.planMarkdown} message-html`}>{planText}</Markdown>
      </div>
    </div>
  );
}
