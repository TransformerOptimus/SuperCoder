'use client';
import imagePath from '@/app/imagePath';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import StoryDetailsWorkbench from '@/app/(programmer)/workbench/WorkBenchComponents/StoryDetailsWorkbench';
import Browser from '@/app/(programmer)/workbench/WorkBenchComponents/Browser';
import Activity from '@/app/(programmer)/workbench/WorkBenchComponents/Activity';
import React, { useEffect, useRef, useState } from 'react';
import { BoardProvider } from '@/context/Boards';
import { CustomTabsNewProps } from '../../../../types/customComponentTypes';
import { useWorkbenchContext } from '@/context/Workbench';

const ActiveWorkbench: React.FC = () => {
  const backendURL = useRef('');
  const frontendURL = useRef('');
  const { activityLogs, selectedStoryId } = useWorkbenchContext();
  const tabsProps: CustomTabsNewProps = {
    options: [
      {
        key: 'backend',
        text: 'Backend',
        icon: imagePath.browserIconDark,
        content: <Browser url={backendURL.current} />,
      },
      {
        key: 'frontend',
        text: 'Frontend',
        icon: imagePath.browserIconDark,
        content: <Browser url={frontendURL.current} />,
      },
    ],
  };

  useEffect(() => {
    if (typeof window !== 'undefined') {
      backendURL.current = localStorage.getItem('projectURLBackend');
      frontendURL.current = localStorage.getItem('projectURLFrontend');
    }
  }, []);

  return (
    <div
      id={'active_workbench_content'}
      className={'grid w-full grid-cols-12 gap-2'}
    >
      <div className={'col-span-6'}>
        <CustomContainers
          id={'activity'}
          alignment={'items-center justify-center'}
          header={'Activity'}
          height={'calc(100vh - 126px)'}
        >
          <Activity activity={activityLogs} fullScreen={true} />
        </CustomContainers>
      </div>
      <div
        className={'col-span-6 flex flex-col gap-2'}
        style={{ height: 'calc(100vh - 126px)' }}
      >
        <div className={'flex-1'} style={{ height: 'calc(50% - 4px)' }}>
          <CustomContainers
            id={'browser'}
            alignment={'items-center justify-center'}
            header={'Browser'}
            height={'100%'}
            tabsProps={tabsProps}
            type={'tabs'}
          />
        </div>
        <div className={'flex-1'} style={{ height: 'calc(50% - 4px)' }}>
          <CustomContainers
            id={'story_details'}
            alignment={'items-center justify-center'}
            header={'Story Details'}
            height={'100%'}
          >
            <BoardProvider>
              <StoryDetailsWorkbench id={selectedStoryId} />
            </BoardProvider>
          </CustomContainers>
        </div>
      </div>
    </div>
  );
};

export default ActiveWorkbench;
