import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Dropdown } from 'antd';
import { Maximize2 } from 'lucide-react';
import { useAppStore } from '@/store';
import { agentTauriService } from '@/services/agentTauriService';
import type { PermissionLevel } from '@/types/agentContract';
import styles from './PlanCard.module.css';

interface PlanCardProps {
  planText: string;
  projectPath: string;
  planPath: string;
  branch?: string;
}

export default function PlanCard({ planText, projectPath, planPath, branch }: PlanCardProps) {
  const [expanded, setExpanded] = useState(true);
  const [implementing, setImplementing] = useState(false);

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
      await agentTauriService.startCodingFromPlan(projectPath, planText, planPath, branch);
    } catch (err) {
      console.error('[PlanCard] Failed to start coding from plan:', err);
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

  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <button className={styles.toggle} onClick={() => setExpanded(!expanded)}>
          <span className={styles.chevron}>{expanded ? '\u25BE' : '\u25B8'}</span>
          <span className={styles.title}>Implementation Plan</span>
        </button>
        <div className={styles.actions}>
          <Dropdown menu={implementMenu} trigger={['click']} disabled={implementing}>
            <button className="primary_small" disabled={implementing}>
              {implementing ? 'Starting...' : 'Implement \u25BE'}
            </button>
          </Dropdown>
          <Maximize2
            className="w-4 h-4 text-[var(--text-secondary)] opacity-60 hover:opacity-100 cursor-pointer transition-opacity shrink-0"
            onClick={() => useAppStore.getState().setActivePlanProjectPath(projectPath)}
          />
        </div>
      </div>

      {expanded && (
        <div className={styles.body}>
          <div className={`${styles.planText} ${styles.planMarkdown} message-html`}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{planText}</ReactMarkdown>
          </div>
        </div>
      )}
    </div>
  );
}
