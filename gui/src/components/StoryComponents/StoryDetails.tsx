import styles from './story.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import CustomTabs from '@/components/CustomTabs/CustomTabs';
import Overview from '@/components/StoryComponents/Overview';
import Instructions from '@/components/StoryComponents/Instructions';
import {
  deleteStory,
  getStoryById,
  updateStoryStatus,
} from '@/api/DashboardService';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import CustomTag from '@/components/CustomTag/CustomTag';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import TestCases from '@/components/StoryComponents/TestCases';
import IssueContainer from '@/components/StoryComponents/InReviewIssue';
import {
  handleInProgressStoryStatus,
  handleStoryInReviewIssue,
  handleStoryStatus,
} from '@/app/utils';
import { useRouter } from 'next/navigation';
import {
  StoryDetailsProps,
  StoryInReviewIssue,
} from '../../../types/storyTypes';
import {
  showStoryDetailsDropdown,
  storyStatus,
} from '@/app/constants/BoardConstants';
import { useBoardContext } from '@/context/Boards';

export default function StoryDetails({
  id,
  story_id,
  open_status = true,
  numberOfStoriesInProgress,
  tabCSS,
  toGetAllStoriesOfProject,
  close,
  setOpenSetupModelModal,
}: StoryDetailsProps) {
  const {
    storyDetails,
    setStoryDetails,
    storyOverview,
    setStoryOverview,
    storyTestCases,
    setStoryTestCases,
    storyInstructions,
    setStoryInstructions,
    setEditTrue,
  } = useBoardContext();
  const router = useRouter();

  const [issue, setIssue] = useState<StoryInReviewIssue | null>({
    title: null,
    description: null,
    actions: [],
  });

  const resetIssue = () => {
    setIssue({
      title: null,
      description: null,
      actions: [],
    });
  };

  const tabOptions = [
    {
      key: 'overview',
      text: 'Overview',
      content: <Overview overview={storyOverview} />,
      selected: imagePath.overviewIconSelected,
      unselected: imagePath.overviewIconUnselected,
    },

    {
      key: 'test_cases',
      text: 'Test Cases',
      content: <TestCases cases={storyTestCases} />,
      selected: imagePath.testCasesIconSelected,
      unselected: imagePath.testCasesIconUnselected,
    },

    {
      key: 'instructions',
      text: 'Instructions',
      content: <Instructions instructions={storyInstructions} />,
      selected: imagePath.instructionsIconSelected,
      unselected: imagePath.instructionsIconUnselected,
    },
  ];

  const dropdownItems = [
    {
      key: '1',
      text: 'Edit',
      icon: (
        <CustomImage
          className={'size-4'}
          src={imagePath.editIcon}
          alt={'close_icon'}
        />
      ),
      action: () => handleEditAction(),
    },
    {
      key: '2',
      text: 'Delete',
      icon: (
        <CustomImage
          className={'size-4'}
          src={imagePath.deleteIcon}
          alt={'close_icon'}
        />
      ),

      action: () => toDeleteStory(),
    },
  ];

  const handleMoveToInProgressClick = async () => {
    const openWorkbench = await handleInProgressStoryStatus(
      setOpenSetupModelModal,
      numberOfStoriesInProgress,
      toUpdateStoryStatus,
    );
    if (openWorkbench) {
      router.push(`/workbench`);
    }
  };

  const handleEditAction = () => {
    setEditTrue(true);
    close();
  };

  useEffect(() => {
    if (story_id && open_status)
      toGetStoryById(story_id.toString()).then().catch();
  }, [story_id, open_status]);

  async function toGetStoryById(story_id: string) {
    try {
      const response = await getStoryById(story_id);

      if (response) {
        const data = response.data;
        setStoryDetails(data.story);
        setStoryOverview(data.story.overview);
        setStoryTestCases(data.story.test_cases);
        setStoryInstructions(data.story.instructions);

        if (data.story.status === storyStatus.IN_REVIEW) {
          const issue = handleStoryInReviewIssue(data);
          setIssue(issue);
        } else {
          resetIssue();
        }
      }
    } catch (error) {
      console.error('Error while fetching story by id:: ', error);
      resetIssue();
    }
  }

  async function toUpdateStoryStatus(status: string) {
    try {
      const response = await updateStoryStatus(status, story_id);
      if (response && response.data && response.data.status.includes('OK')) {
        toGetAllStoriesOfProject();
      }
    } catch (error) {
      console.error('Error while updating story status:: ', error);
    } finally {
      close();
    }
  }

  async function toDeleteStory() {
    try {
      const response = await deleteStory(Number(story_id));
      if (response && response.data && response.data.status.includes('OK')) {
        toGetAllStoriesOfProject();
      }
    } catch (error) {
      console.error('Error while deleting story: ', error);
    } finally {
      close();
    }
  }

  return (
    storyDetails && (
      <div id={`${id}_story_details`}>
        {id !== 'workbench' && (
          <div
            id={'story_details_header'}
            className={styles.story_details_header}
          >
            <div
              className={
                'flex flex-row flex-nowrap items-center gap-1 text-xl font-medium'
              }
            >
              <span className={`inline-block max-w-[22vw] truncate`}>
                {storyDetails.overview.name}{' '}
              </span>
              <span className={'secondary_color font-[300]'}>#{story_id}</span>
            </div>

            <div className={'flex flex-row items-center gap-3'}>
              {storyDetails.status === storyStatus.TODO && (
                <Button
                  className={'primary_medium'}
                  onClick={() => handleMoveToInProgressClick()}
                >
                  Move to In Progress
                </Button>
              )}

              <CustomTag
                icon={imagePath.whiteDot}
                iconClass={'size-4'}
                text={handleStoryStatus(storyDetails.status).text}
                color={handleStoryStatus(storyDetails.status).color}
                className={'rounded-3xl'}
              />

              <CustomDropdown
                trigger={
                  <CustomImage
                    className={'size-5 cursor-pointer'}
                    src={imagePath.horizontalThreeDots}
                    alt={'three_dots_icon'}
                  />
                }
                maxHeight={'200px'}
                gap={'10px'}
                position={'end'}
                show={showStoryDetailsDropdown.includes(storyDetails.status)}
              >
                {dropdownItems &&
                  dropdownItems.map((item) => (
                    <CustomDropdown.Item key={item.key} onClick={item.action}>
                      <div
                        className={
                          'flex flex-row items-center justify-center gap-2'
                        }
                      >
                        {item.icon}
                        {item.text}
                      </div>
                    </CustomDropdown.Item>
                  ))}
              </CustomDropdown>

              <CustomImage
                className={'size-5 cursor-pointer'}
                src={imagePath.closeIcon}
                alt={'close_icon'}
                onClick={close}
              />
            </div>
          </div>
        )}
        {id !== 'workbench' &&
          storyDetails.status === storyStatus.IN_REVIEW && (
            <IssueContainer
              title={issue?.title}
              description={issue?.description}
              actions={issue?.actions || []}
              handleMoveToInProgressClick={handleMoveToInProgressClick}
            />
          )}

        <div id={'story_details_body'} className={tabCSS}>
          <CustomTabs
            options={tabOptions}
            height={id === 'workbench' ? '28vh' : ''}
          />
        </div>
      </div>
    )
  );
}
