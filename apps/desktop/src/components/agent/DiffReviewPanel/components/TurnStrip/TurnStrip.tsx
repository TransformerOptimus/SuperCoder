import type { CheckpointSummary } from '@/types/agent';
import styles from './TurnStrip.module.css';

interface TurnStripProps {
  checkpoints: CheckpointSummary[];
  selectedTurn: number | null;
  onSelectTurn: (turn: number) => void;
  onSelectAll: () => void;
  onRestore?: (turn: number) => void;
}

export default function TurnStrip({
  checkpoints,
  selectedTurn,
  onSelectAll,
  onSelectTurn,
  onRestore,
}: TurnStripProps) {
  const displayCheckpoints = checkpoints.filter((cp) => cp.turn_count > 0);

  if (displayCheckpoints.length === 0) return null;

  return (
    <div className={styles.strip_wrapper}>
    <div className={styles.strip}>
      <button
        onClick={onSelectAll}
        className={`${styles.all_btn} ${selectedTurn === null ? styles.all_btn_active : ''}`}
      >
        All changes
      </button>

      <div className={styles.divider} />

      {displayCheckpoints.map((cp) => (
        <div key={cp.turn_count} className={styles.turn_wrapper}>
          <button
            onClick={() => onSelectTurn(cp.turn_count)}
            className={`${styles.turn_btn} ${selectedTurn === cp.turn_count ? styles.turn_btn_active : ''}`}
          >
            <span>Turn {cp.turn_count}</span>
            {(cp.additions > 0 || cp.deletions > 0) && (
              <span className={styles.turn_stats}>
                <span className={styles.stat_add}>+{cp.additions}</span>
                <span>/</span>
                <span className={styles.stat_del}>-{cp.deletions}</span>
              </span>
            )}
          </button>

          {onRestore && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onRestore(cp.turn_count);
              }}
              className={styles.restore_btn}
              title={`Restore workspace to state after turn ${cp.turn_count}`}
            >
              Restore here
            </button>
          )}
        </div>
      ))}
    </div>
    </div>
  );
}
