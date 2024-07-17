import CustomContainers from '@/components/CustomContainers/CustomContainers';
import Activity from '@/components/WorkBenchComponents/Activity';
import { BoardProvider } from '@/context/Boards';
import StoryDetailsWorkbench from '@/components/WorkBenchComponents/StoryDetailsWorkbench';
import React, { useEffect, useRef } from 'react';
import { CustomTabsNewProps } from '../../../types/customComponentTypes';
import imagePath from '@/app/imagePath';
import Browser from '@/components/WorkBenchComponents/Browser';
import { BackendWorkbenchProps } from '../../../types/workbenchTypes';
import TerminalComponent from '@/components/Terminal/Terminal';

const BackendWorkbench: React.FC<BackendWorkbenchProps> = ({
  activityLogs,
  selectedStoryId,
  status,
}) => {
  const backendURL = useRef('');
  const frontendURL = useRef('');
  useEffect(() => {
    if (typeof window !== 'undefined') {
      backendURL.current = localStorage.getItem('projectURLBackend');
      frontendURL.current = localStorage.getItem('projectURLFrontend');
    }
  }, []);

  const browserTabsProps: CustomTabsNewProps = {
    options: [
      {
        key: 'backend',
        text: 'Backend',
        icon: imagePath.browserIconDark,
        content: <Browser url={backendURL.current} status={status} />,
      },
      {
        key: 'frontend',
        text: 'Frontend',
        icon: imagePath.browserIconDark,
        content: <Browser url={frontendURL.current} status={status} />,
      },
    ],
  };

  const actionTabsProps: CustomTabsNewProps = {
    options: [
      {
        key: 'story_details',
        text: 'Story Details',
        icon: imagePath.storyDetailsIconSelected,
        content: (
          <BoardProvider>
            <StoryDetailsWorkbench id={selectedStoryId} />
          </BoardProvider>
        ),
      },
      {
        key: 'terminal',
        text: 'Terminal',
        icon: imagePath.terminalIconSelected,
        content: <TerminalComponent />,
      },
    ],
  };

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
          <Activity activity={activityLogs} />
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
            tabsProps={browserTabsProps}
            type={'tabs'}
          />
        </div>
        <div className={'flex-1'} style={{ height: 'calc(50% - 4px)' }}>
          <CustomContainers
            id={'actions'}
            alignment={'items-center justify-center'}
            header={'Action'}
            height={'100%'}
            tabsProps={actionTabsProps}
            type={'tabs'}
          />
        </div>
      </div>
    </div>
  );
};

export default BackendWorkbench;
