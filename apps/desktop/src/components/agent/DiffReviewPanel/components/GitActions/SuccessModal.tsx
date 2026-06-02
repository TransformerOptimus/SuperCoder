import { Modal, Button } from "antd";
import { open as shellOpen } from "@tauri-apps/plugin-shell";
import { Check, GitBranch, ExternalLink } from "lucide-react";
import styles from "./GitActions.module.css";

interface SuccessModalProps {
  open: boolean;
  title: string;
  branch: string;
  filesChanged?: number;
  additions?: number;
  deletions?: number;
  prUrl?: string;
  onClose: () => void;
}

export default function SuccessModal({
  open,
  title,
  branch,
  filesChanged,
  additions,
  deletions,
  prUrl,
  onClose,
}: SuccessModalProps) {
  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      closable
      centered
      width={420}
    >
      <div className={styles.modal_header}>
        <div className={styles.success_icon}>
          <Check size={20} />
        </div>
        <span className={styles.modal_title}>{title}</span>
      </div>

      <div className={styles.info_row}>
        <span className={styles.info_label}>Branch</span>
        <span className={styles.info_value}>
          <GitBranch size={14} />
          {branch}
        </span>
      </div>

      {filesChanged !== undefined && (
        <div className={styles.info_row}>
          <span className={styles.info_label}>Changes</span>
          <span className={styles.info_value}>
            {filesChanged} files
            {additions !== undefined && <span className={styles.stat_add}>+{additions}</span>}
            {deletions !== undefined && <span className={styles.stat_del}>-{deletions}</span>}
          </span>
        </div>
      )}

      <div className={styles.success_footer}>
        <Button className={`secondary_medium ${styles.flex_1}`} onClick={onClose}>
          Close
        </Button>
        {prUrl && (
          <Button
            className={`primary_medium ${styles.flex_1}`}
            onClick={() => shellOpen(prUrl)}
          >
            <ExternalLink size={14} />
            Open PR
          </Button>
        )}
      </div>
    </Modal>
  );
}
