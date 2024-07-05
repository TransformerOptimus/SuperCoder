'use client';
import React, { useRef, useEffect } from 'react';
import styles from '../workbench.module.css';
import imagePath from '@/app/imagePath';
import SyntaxDisplay from '@/components/SyntaxDisplay/SyntaxDisplay';
import { formatTimeAgo } from '@/app/utils';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { ActivityItem } from '../../../../../types/workbenchTypes';

interface ActivityProps {
  activity: ActivityItem[];
  fullScreen?: boolean;
}

const Activity: React.FC<ActivityProps> = ({ activity, fullScreen = true }) => {
  const activityLogsRef = useRef<HTMLDivElement>(null);

  const isErrorLog = (type: string) => {
    return type.includes('ERROR');
  };

  const isCodeLog = (type: string) => {
    return type.includes('CODE');
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
      style={{
        maxHeight: fullScreen ? 'calc(100vh - 170px)' : 'calc(50vh - 120px)',
      }}
      ref={activityLogsRef}
    >
      {activity &&
        activity.length > 0 &&
        activity.map((item, index) => (
          <div key={index} className={styles.activity_container}>
            {isErrorLog(item.Type) ? (
              <SyntaxDisplay msg={item.LogMessage} type={'ERROR'} />
            ) : isCodeLog(item.Type) ? (
              <SyntaxDisplay msg={item.LogMessage} type={'CODE'} />
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
