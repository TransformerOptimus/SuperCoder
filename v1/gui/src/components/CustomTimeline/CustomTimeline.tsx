import React from 'react';
import styles from './timeline.module.css';

interface TimelineItem {
  id: number;
  content: string;
  timestamp: string;
}

interface CustomTimelineProps {
  items: TimelineItem[];
}

export default function CustomTimeline({ items }: CustomTimelineProps) {
  return (
    <div
      className={styles.timeline_container}
      style={{ maxHeight: 'calc(100vh - 160px)' }}
    >
      <div className={styles.line} />
      {items.map((item) => (
        <div className={styles.timeline_item_container} key={item.id}>
          <div className={styles.bullet} />
          <div className={styles.content}>
            {item.content}
            <div className={styles.timestamp}>{item.timestamp}</div>
          </div>
        </div>
      ))}
    </div>
  );
}
