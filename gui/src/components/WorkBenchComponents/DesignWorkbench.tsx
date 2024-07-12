import React, { useEffect, useRef, useState } from 'react';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import Activity from '@/components/WorkBenchComponents/Activity';
import Browser from '@/components/WorkBenchComponents/Browser';
import { DesignStoryItem } from '../../../types/designStoryTypes';
import { getFrontendCode, getDesignStoryDetails } from '@/api/DashboardService';
import FrontendCodeSection from '@/components/DesignStoryComponents/FrontendCodeSection';
import styles from './workbenchComponents.module.css';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import { CodeFile } from '../../../types/customComponentTypes';
import { storyStatus } from '@/app/constants/BoardConstants';
import { DesignWorkbenchProps } from '../../../types/workbenchTypes';

const ActiveDesignWorkbench: React.FC<DesignWorkbenchProps> = ({
  activityLogs,
  selectedStoryId,
  executionInProcess,
}) => {
  const [selectedStory, setSelectedStory] = useState<DesignStoryItem | null>(
    null,
  );
  const [codeFiles, setCodeFiles] = useState<CodeFile[] | null>(null);
  const frontendURL = useRef('');

  useEffect(() => {
    getStoryDetails(selectedStoryId).then().catch();
  }, [selectedStoryId, executionInProcess]);

  async function getCode(story_id) {
    try {
      const response = await getFrontendCode(story_id);
      if (response) {
        const data = response.data;
        setCodeFiles(data.code_files);
      }
    } catch (error) {
      console.error(error);
    }
  }

  async function getStoryDetails(story_id) {
    try {
      const response = await getDesignStoryDetails(story_id);
      if (response) {
        const data = response.data;
        setSelectedStory(data.story);
        frontendURL.current = data.story ? data.story.frontend_url : '';
        if (
          data.story.status === storyStatus.DONE ||
          data.story.status === storyStatus.IN_REVIEW ||
          data.story.status === storyStatus.MAX_LOOP_ITERATIONS
        ) {
          getCode(story_id).then().catch();
        } else {
          setCodeFiles(null);
        }
      }
    } catch (error) {
      console.error(error);
    }
  }

  return (
    <div
      id={'active_workbench_content'}
      className={'grid w-full grid-cols-12 gap-2'}
    >
      <div
        className={'col-span-6 flex flex-col gap-2'}
        style={{ height: 'calc(100vh - 136px)' }}
      >
        <div
          className={`${styles.container_section} relative flex w-full flex-col overflow-hidden rounded-lg`}
          style={{ height: 'calc(50% - 4px)' }}
        >
          <div
            className={`${styles.container_header} secondary_color proxima_nova rounded-t-lg p-2 px-3 text-sm font-normal`}
          >
            Design Input
          </div>
          <div
            className={'flex h-full items-center justify-center'}
            style={{ height: 'calc(100% - 34px)', overflow: 'hidden' }}
          >
            {selectedStory && (
              <img
                src={selectedStory.input_file_url}
                alt="input_image"
                className={'h-full w-auto'}
              />
            )}
          </div>
        </div>
        <CustomContainers
          id={'activity'}
          alignment={'items-center justify-center'}
          header={'Activity'}
          height={'calc(50% - 4px)'}
        >
          <Activity activity={activityLogs} fullScreen={false} />
        </CustomContainers>
      </div>
      <div
        className={'col-span-6 flex flex-col gap-2'}
        style={{ height: 'calc(100vh - 136px)' }}
      >
        <div className="flex-1" style={{ height: 'calc(50% - 4px)' }}>
          <CustomContainers
            id={'UI Preview'}
            alignment={'items-center justify-center'}
            header={'Browser'}
            height={'100%'}
          >
            {codeFiles ? (
              <Browser url={frontendURL.current} showUrl={false} />
            ) : (
              <div
                className={'flex flex-col items-center justify-center'}
                style={{ height: 'calc(50vh - 108px)' }}
              >
                <CustomLoaders type={'default'} size={28} />
                <span className={'mt-2'}>Generating UI</span>
              </div>
            )}
          </CustomContainers>
        </div>
        <div className="flex-1" style={{ height: 'calc(50% - 4px)' }}>
          {codeFiles ? (
            <FrontendCodeSection
              height={''}
              allowCopy={false}
              backgroundColor={true}
              codeFiles={codeFiles}
            />
          ) : (
            <CustomContainers
              id={'Code Files'}
              alignment={'items-center justify-center'}
              header={'Code Files'}
              height={'100%'}
            >
              <div
                className={'flex flex-col items-center justify-center'}
                style={{ height: 'calc(50vh - 108px)' }}
              >
                <CustomLoaders type={'default'} size={28} />
                <span className={'mt-2'}>Generating Code</span>
              </div>
            </CustomContainers>
          )}
        </div>
      </div>
    </div>
  );
};

export default ActiveDesignWorkbench;
