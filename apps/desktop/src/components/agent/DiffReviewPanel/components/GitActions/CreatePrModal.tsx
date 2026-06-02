import { useState } from "react";
import { Modal, Button, Input } from "antd";
import { Github, GitBranch } from "lucide-react";
import styles from "./GitActions.module.css";

interface CreatePrModalProps {
  open: boolean;
  branch: string;
  baseBranch?: string;
  defaultTitle?: string;
  loading?: boolean;
  onSubmit: (title: string, body: string) => void;
  onCancel: () => void;
}

export default function CreatePrModal({
  open,
  branch,
  baseBranch = 'main',
  defaultTitle = '',
  loading,
  onSubmit,
  onCancel,
}: CreatePrModalProps) {
  const [title, setTitle] = useState(defaultTitle);
  const [body, setBody] = useState("");

  const handleSubmit = () => {
    const trimmed = title.trim();
    if (!trimmed) return;
    onSubmit(trimmed, body.trim());
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
          <Github size={20} />
        </div>
        <span className={styles.modal_title}>Create pull request</span>
      </div>

      <div className={styles.info_row}>
        <span className={styles.info_label}>Base</span>
        <span className={styles.info_value}>
          <GitBranch size={14} />
          {baseBranch}
        </span>
      </div>

      <div className={styles.info_row}>
        <span className={styles.info_label}>Head</span>
        <span className={styles.info_value}>
          <GitBranch size={14} />
          {branch}
        </span>
      </div>

      <div className={styles.field}>
        <div className={styles.field_label}>Title</div>
        <Input
          placeholder="PR title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
      </div>

      <div className={styles.field}>
        <div className={styles.field_label}>Description</div>
        <Input.TextArea
          placeholder="Optional description"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={3}
        />
      </div>

      <div className={styles.footer_actions}>
        <Button
          className="primary_medium"
          onClick={handleSubmit}
          loading={loading}
          disabled={!title.trim()}
          block
        >
          Create PR
        </Button>
      </div>
    </Modal>
  );
}
