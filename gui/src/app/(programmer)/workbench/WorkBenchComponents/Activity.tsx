'use client';
import React, { useRef, useEffect } from 'react';
import styles from '../workbench.module.css';
import Image from 'next/image';
import imagePath from '@/app/imagePath';
import SyntaxDisplay from '@/components/SyntaxDisplay/SyntaxDisplay';
import { formatTimeAgo } from '@/app/utils';
import CustomImage from '@/components/ImageComponents/CustomImage';

interface ActivityProps {
  activity: {
    CreatedAt: string;
    ExecutionID: number;
    ExecutionStepID: number;
    ID: number;
    LogMessage: string;
    Type: string;
    UpdatedAt: string;
  }[];
}

const Activity: React.FC<ActivityProps> = ({ activity }) => {
  const activityLogsRef = useRef<HTMLDivElement>(null);

  const isErrorLog = (type: string) => {
    return type.includes('ERROR');
  };

  const scrollToBottom = () => {
    if (activityLogsRef.current) {
      activityLogsRef.current.scrollTop = activityLogsRef.current.scrollHeight;
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [activity]);

  return (
    <div
      id={'activity'}
      className={'proxima_nova flex flex-col gap-3 overflow-y-scroll p-2'}
      style={{ maxHeight: 'calc(100vh - 170px)' }}
      ref={activityLogsRef}
    >
      {activity &&
        activity.length > 0 &&
        activity.map((item, index) => (
          <div key={index} className={styles.activity_container}>
            {isErrorLog(item.Type) ? (
              <SyntaxDisplay error={item.LogMessage} />
            ) : (
              <div
                className={'text-sm font-normal'}
                dangerouslySetInnerHTML={{ __html: item.LogMessage }}
              />
            )}
            <div className={'flex flex-row items-center justify-center gap-1'}>
              <CustomImage
                className={'size-4'}
                src={imagePath.clockIcon}
                alt={'date_icon'}
              />

              <span className={'secondary_color text-xs'}>
                {formatTimeAgo(item.CreatedAt)}
              </span>
            </div>
          </div>
        ))}
    </div>
  );
};

export default Activity;
