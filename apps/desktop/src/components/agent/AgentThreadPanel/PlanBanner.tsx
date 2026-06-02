import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import styles from './PlanBanner.module.css';

interface PlanBannerProps {
  planText: string;
}

export default function PlanBanner({ planText }: PlanBannerProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={styles.banner}>
      <button className={styles.header} onClick={() => setExpanded(!expanded)}>
        <span className={styles.chevron}>{expanded ? '\u25BE' : '\u25B8'}</span>
        <span className={styles.title}>Implementation Plan</span>
      </button>
      {expanded && (
        <div className={styles.body}>
          <div className={`${styles.markdown} message-html`}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{planText}</ReactMarkdown>
          </div>
        </div>
      )}
    </div>
  );
}
