import { useState } from "react";
import { Modal, Button, Input, Radio } from "antd";
import { GitCommitHorizontal, ArrowUpFromLine, GitBranch, Github } from "lucide-react";
import styles from "./GitActions.module.css";

type NextStep = 'commit' | 'commit_push' | 'commit_push_pr';

interface CommitModalProps {
  open: boolean;
  branch: string;
  filesChanged: number;
  additions: number;
  deletions: number;
  loading?: boolean;
  onSubmit: (message: string, nextStep: NextStep) => void;
  onCancel: () => void;
}

export default function CommitModal({
  open,
  branch,
  filesChanged,
  additions,
  deletions,
  loading,
  onSubmit,
  onCancel,
}: CommitModalProps) {
  const [message, setMessage] = useState("");
  const [nextStep, setNextStep] = useState<NextStep>('commit');

  const handleSubmit = () => {
    onSubmit(message.trim(), nextStep);
    setMessage("");
  };

  return (
    <Modal
      open={open}
      onCancel={onCancel}
      footer={null}
      closable
      centered
      width={480}
    >
      <div className={styles.modal_header}>
        <div className={styles.modal_icon}>
          <GitCommitHorizontal size={20} />
        </div>
        <span className={styles.modal_title}>Commit your changes</span>
      </div>

      <div className={styles.info_row}>
        <span className={styles.info_label}>Branch</span>
        <span className={styles.info_value}>
          <GitBranch size={14} />
          {branch}
        </span>
      </div>

      <div className={styles.info_row}>
        <span className={styles.info_label}>Changes</span>
        <span className={styles.info_value}>
          {filesChanged} files
          <span className={styles.stat_add}>+{additions}</span>
          <span className={styles.stat_del}>-{deletions}</span>
        </span>
      </div>

      <div className={styles.field}>
        <div className={styles.field_label}>Commit message</div>
        <Input.TextArea
          placeholder="Leave blank to autogenerate a commit message"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          rows={3}
          autoFocus
        />
      </div>

      <div className={styles.next_steps}>
        <div className={styles.field_label}>Next steps</div>
        <Radio.Group
          value={nextStep}
          onChange={(e) => setNextStep(e.target.value)}
          className={styles.radio_group_full}
        >
          <div
            className={`${styles.next_step_option} ${nextStep === 'commit' ? styles.next_step_option_selected : ''}`}
            onClick={() => setNextStep('commit')}
          >
            <GitCommitHorizontal size={16} />
            <span className={styles.next_step_label}>Commit</span>
            <Radio value="commit" />
          </div>
          <div
            className={`${styles.next_step_option} ${nextStep === 'commit_push' ? styles.next_step_option_selected : ''}`}
            onClick={() => setNextStep('commit_push')}
          >
            <ArrowUpFromLine size={16} />
            <span className={styles.next_step_label}>Commit and push</span>
            <Radio value="commit_push" />
          </div>
          <div
            className={`${styles.next_step_option} ${nextStep === 'commit_push_pr' ? styles.next_step_option_selected : ''}`}
            onClick={() => setNextStep('commit_push_pr')}
          >
            <Github size={16} />
            <span className={styles.next_step_label}>Commit and create PR</span>
            <Radio value="commit_push_pr" />
          </div>
        </Radio.Group>
      </div>

      <div className={styles.footer_actions}>
        <Button
          className="primary_medium"
          onClick={handleSubmit}
          loading={loading}
          block
        >
          Continue
        </Button>
      </div>
    </Modal>
  );
}
