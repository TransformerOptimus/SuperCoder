'use client';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import { Button } from '@nextui-org/react';
import React, { Suspense, useEffect, useState } from 'react';
import CreateEditStory from '@/components/StoryComponents/CreateEditStory';
import ActiveWorkbench from '@/app/(programmer)/workbench/ActiveWorkbench';
import CustomDrawer from '@/components/CustomDrawer/CustomDrawer';
import { useRouter } from 'next/navigation';
import { useWorkbenchContext } from '@/context/Workbench';
import {
  getProjectTypeFromFramework,
  handleStoryStatus,
  toGetAllStoriesOfProjectUtils,
} from '@/app/utils';
import DesignActiveWorkbench from '@/app/(programmer)/workbench/DesignActiveWorkbench';
import { StoryListItems } from '../../../../types/workbenchTypes';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import CustomTag from '@/components/CustomTag/CustomTag';
import { storyStatus } from '@/app/constants/BoardConstants';
import { getActivityLogs } from '@/api/DashboardService';
import { projectTypes } from '@/app/constants/ProjectConstants';

export default function WorkBench() {
  const [isModalOpen, setIsModalOpen] = useState<boolean | null>(false);
  const [selectedStory, setSelectedStory] = useState<StoryListItems | null>(
    null,
  );
  const [status, setStatus] = useState<string | null>(null);
  const [projectFramework, setProjectFramework] = useState<string | null>(null);
  const {
    storiesList,
    setStoriesList,
    setActivityLogs,
    selectedStoryId,
    setSelectedStoryId,
    setExecutionInProcess,
    executionInProcess,
  } = useWorkbenchContext();
  const router = useRouter();

  const activeWorkbenchCondition = () => {
    return (
      storiesList &&
      (storiesList.IN_PROGRESS || storiesList.DONE || storiesList.IN_REVIEW) &&
      (storiesList.IN_PROGRESS.length > 0 || storiesList.DONE.length > 0 || storiesList.IN_REVIEW.length > 0)
    );
  };

  useEffect(() => {
    setExecutionInProcess(null);
    let id = null;
    if (typeof window !== 'undefined') {
      id = localStorage.getItem('storyId');
      const projectFramework = localStorage.getItem('projectFramework');
      setProjectFramework(projectFramework);
      setSelectedStoryId(id);
      toGetActivityLogs(id).then().catch();
    }
    toGetAllStoriesOfProjectUtils(setStoriesList).then().catch();
    setTimeout(() => {
      toGetAllStoriesOfProjectUtils(setStoriesList).then().catch();
    }, 10000);
  }, []);

  useEffect(() => {
    if (executionInProcess) {
      const id = localStorage.getItem('storyId');
      const intervalID = setInterval(() => {
        toGetActivityLogs(id).then().catch();
      }, 5000);

      return () => clearInterval(intervalID);
    }
  }, [executionInProcess]);

  useEffect(() => {
    if (
      storiesList &&
      (storiesList.IN_PROGRESS.length > 0 || storiesList.DONE.length > 0 || storiesList.IN_REVIEW.length > 0)
    )
      handleSelectedStory();
  }, [storiesList, selectedStoryId]);

  useEffect(() => {
    if (selectedStoryId) toGetActivityLogs(selectedStoryId).then().catch();
  }, [selectedStoryId, status]);

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
  const getStatus = (storyId: number) => {
    for (const [status, storyList] of Object.entries(storiesList)) {
      if (storyList.some(story => story.story_id === storyId)) {
        return status;
      }
    }
    return "";
  }

  const handleSelectedStory = () => {
    console.log(storiesList);
    const completeStoriesList = [
      ...storiesList.IN_PROGRESS,
      ...storiesList.DONE,
      ...storiesList.IN_REVIEW,
    ];
    console.log('handle selected story called');

    const story = completeStoriesList.find(
      (item) => item.story_id.toString() === selectedStoryId,
    );

    if (story) {
      setSelectedStory(story);
      const currentStatus = getStatus(story.story_id);
      if(selectedStoryId === story.story_id.toString() && currentStatus !== status){
        setStatus(currentStatus);
      }
    }
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

  const handleInReviewCheck = () => {
    return storiesList && storiesList.IN_REVIEW && storiesList.IN_REVIEW.length > 0;
  };

  return (
    <div id={'workbench'} className={'proxima_nova p-4'}>
      {activeWorkbenchCondition() ? (
        <Suspense fallback={<div>Loading....</div>}>
          {selectedStory && (
            <div id={'active_workbench'} className={'flex flex-col gap-5'}>
              <div
                id={'active_workbench_header'}
                className={'flex flex-row items-center justify-between'}
              >
                <div
                  className={'flex flex-row items-center justify-center gap-3'}
                >
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

                    {handleInReviewCheck() && (
                        <CustomDropdown.Section title={'IN REVIEW STORIES'}>
                          {storiesList.IN_REVIEW.map((story) => (
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

                  {executionInProcess && (
                    <CustomLoaders type={'default'} size={28} />
                  )}

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
                  {getProjectTypeFromFramework(projectFramework) ===
                    projectTypes.DESIGN && (
                    <CustomTag
                      icon={null}
                      iconClass={'size-4'}
                      text={'Design Story'}
                      className={'rounded-3xl'}
                    />
                  )}
                  {status && (
                    <CustomTag
                      icon={imagePath.whiteDot}
                      iconClass={'size-4'}
                      text={handleStoryStatus(status).text}
                      color={handleStoryStatus(status).color}
                      className={'rounded-3xl'}
                    />
                  )}
                </div>
              </div>
              {getProjectTypeFromFramework(projectFramework) ===
              projectTypes.DESIGN ? (
                <DesignActiveWorkbench />
              ) : (
                <ActiveWorkbench />
              )}
            </div>
          )}
        </Suspense>
      ) : (
        <CustomContainers
          id={'workbench_empty'}
          height={'calc(100vh - 80px)'}
          alignment={'items-center justify-center'}
          bgColor={false}
        >
          <div className={'flex flex-col items-center justify-center gap-3'}>
            <span className={'proxima_nova text-xl font-normal opacity-60'}>
              No Story in Progress!
            </span>
            <Button
              className={'primary_medium w-fit'}
              onClick={() => router.push('/board')}
            >
              Go to Board
            </Button>

            <CustomDrawer
              open={isModalOpen}
              onClose={() => setIsModalOpen(false)}
              direction={'right'}
              width={'40vw'}
              top={'50px'}
              contentCSS={'rounded-l-2xl'}
            >
              <CreateEditStory
                id={'workbench'}
                close={() => setIsModalOpen(false)}
                top={'50px'}
              />
            </CustomDrawer>
          </div>
        </CustomContainers>
      )}
    </div>
  );
}
