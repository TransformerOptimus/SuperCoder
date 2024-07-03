'use client';
import imagePath from '@/app/imagePath';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import StoryDetailsWorkbench from '@/app/(programmer)/workbench/WorkBenchComponents/StoryDetailsWorkbench';
import Browser from '@/app/(programmer)/workbench/WorkBenchComponents/Browser';
import Activity from '@/app/(programmer)/workbench/WorkBenchComponents/Activity';
import React, { useEffect, useRef, useState } from 'react';
import { handleStoryStatus } from '@/app/utils';
import CustomTag from '@/components/CustomTag/CustomTag';
import { getActivityLogs } from '@/api/DashboardService';
import { useSearchParams } from 'next/navigation';
import { StoryListItems } from '../../../../types/workbenchTypes';
import { BoardProvider } from '@/context/Boards';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { storyStatus } from '@/app/constants/BoardConstants';
import { CustomTabsNewProps } from '../../../../types/customComponentTypes';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import { useWorkbenchContext } from '@/context/Workbench';

const ActiveWorkbench: React.FC = () => {
  const [activityLogs, setActivityLogs] = useState(null);
  const [status, setStatus] = useState<string | null>(null);
  const [selectedStoryId, setSelectedStoryId] = useState(null);
  const [executionInProcess, setExecutionInProcess] = useState<boolean | null>(
    false,
  );
  const [selectedStory, setSelectedStory] = useState<StoryListItems | null>(
    null,
  );
  const backendURL = useRef('');
  const frontendURL = useRef('');
  const { storiesList } = useWorkbenchContext();

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

  const handleSelectedStory = () => {
    const completeStoriesList = [
      ...storiesList.IN_PROGRESS,
      ...storiesList.DONE,
    ];

    const story = completeStoriesList.find(
      (item) => item.story_id.toString() === selectedStoryId,
    );

    if (story) setSelectedStory(story);
    else {
      localStorage.setItem(
        'storyId',
        completeStoriesList[0].story_id.toString(),
      );
      setSelectedStoryId(completeStoriesList[0].story_id.toString());
    }
  };

  const handleItemSelect = (key: string) => {
    localStorage.setItem('storyId', key.toString());
    setSelectedStoryId(key.toString());
  };

  const handleInProgressCheck = () => {
    return (
      storiesList &&
      storiesList.IN_PROGRESS &&
      storiesList.IN_PROGRESS.length > 0
    );
  };

  const handleDoneCheck = () => {
    return storiesList && storiesList.DONE && storiesList.DONE.length > 0;
  };

  useEffect(() => {
    let id = null;
    if (typeof window !== 'undefined') {
      id = localStorage.getItem('storyId');
      backendURL.current = localStorage.getItem('projectURLBackend');
      frontendURL.current = localStorage.getItem('projectURLFrontend');
      setSelectedStoryId(id);
      toGetActivityLogs(id).then().catch();
    }
  }, []);

  useEffect(() => {
    if (executionInProcess) {
      const id = localStorage.getItem('storyId');
      const intervalID = setInterval(() => {
        toGetActivityLogs(id).then().catch();
      }, 10000);

      return () => clearInterval(intervalID);
    }
  }, [executionInProcess]);

  useEffect(() => {
    if (
      storiesList &&
      (storiesList.IN_PROGRESS.length > 0 || storiesList.DONE.length > 0)
    )
      handleSelectedStory();
  }, [storiesList, selectedStoryId]);

  useEffect(() => {
    if (selectedStoryId) toGetActivityLogs(selectedStoryId).then().catch();
  }, [selectedStoryId]);

  async function toGetActivityLogs(story_id: string) {
    try {
      const response = await getActivityLogs(story_id);
      if (response) {
        const data = response.data;
        setActivityLogs(data.Logs);
        setStatus(data.Status ? data.Status : 'IN_PROGRESS');

        if (data.Status === storyStatus.IN_PROGRESS || data.Status === '')
          setExecutionInProcess(true);
        else setExecutionInProcess(false);
      }
    } catch (error) {
      console.error('Error while fetching activity logs:: ', error);
    }
  }

  return (
    <div id={'active_workbench'} className={'flex flex-col gap-5'}>
      {selectedStory && (
        <div
          id={'active_workbench_header'}
          className={'flex flex-row items-center justify-between'}
        >
          <div className={'flex flex-row items-center justify-center gap-3'}>
            <CustomDropdown
              trigger={
                <div className={'secondary_small'}>
                  <CustomImage
                    className={'size-4'}
                    src={imagePath.bottomArrowGrey}
                    alt={'bottom_arrow'}
                  />
                </div>
              }
              maxHeight={'400px'}
              gap={'10px'}
              position={'start'}
            >
              {handleInProgressCheck() && (
                <CustomDropdown.Section
                  title={'IN PROGRESS STORIES'}
                  showDivider
                >
                  {storiesList.IN_PROGRESS.map((story) => (
                    <CustomDropdown.Item
                      key={story.story_id.toString()}
                      onClick={() =>
                        handleItemSelect(story.story_id.toString())
                      }
                    >
                      <span>{story.story_name}</span>
                    </CustomDropdown.Item>
                  ))}
                </CustomDropdown.Section>
              )}

              {handleDoneCheck() && (
                <CustomDropdown.Section title={'DONE STORIES'}>
                  {storiesList.DONE.map((story) => (
                    <CustomDropdown.Item
                      key={story.story_id.toString()}
                      onClick={() =>
                        handleItemSelect(story.story_id.toString())
                      }
                    >
                      <span>{story.story_name}</span>
                    </CustomDropdown.Item>
                  ))}
                </CustomDropdown.Section>
              )}
            </CustomDropdown>

            {executionInProcess && <CustomLoaders type={'default'} size={28} />}

            <div
              className={
                'flex flex-row items-center justify-center gap-2 text-xl font-semibold'
              }
            >
              {selectedStory.story_name}
              <span className={'secondary_color font-[300]'}>
                &nbsp;#{selectedStory.story_id}
              </span>
            </div>
          </div>

          <div className={'flex flex-row gap-3'}>
            {status && (
              <CustomTag
                icon={imagePath.whiteDot}
                iconClass={'size-4'}
                text={handleStoryStatus(status).text}
                color={handleStoryStatus(status).color}
              />
            )}
          </div>
        </div>
      )}

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
    </div>
  );
};

export default ActiveWorkbench;
