import styles from '@/components/DesignStoryComponents/desingStory.module.css';
import { DesignStoryDetailsProps } from '../../../types/designStoryTypes';
import imagePath from '@/app/imagePath';
import CustomImage from '@/components/ImageComponents/CustomImage';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import React, { useEffect, useState } from 'react';
import CustomTag from '@/components/CustomTag/CustomTag';
import { handleInProgressStoryStatus, handleStoryStatus } from '@/app/utils';
import { useDesignContext } from '@/context/Design';
import { deleteStory, updateStoryStatus } from '@/api/DashboardService';
import { storyStatus, storyActions } from '@/app/constants/BoardConstants';
import { useRouter } from 'next/navigation';
import { Button } from '@nextui-org/react';

interface Issue {
  title: string | null;
  description: string | null;
  actions: { label: string; link: string }[];
}

const DesignStoryDetails: React.FC<DesignStoryDetailsProps> = ({
  id,
  close,
  top,
  toGetAllDesignStoriesOfProject,
  setOpenSetupModelModal,
  numberOfStoriesInProgress,
}) => {
  const { selectedStory, setEditTrue } = useDesignContext();
  const router = useRouter();

  const [issue, setIssue] = useState<Issue | null>({
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

  useEffect(() => {
    if (selectedStory && selectedStory.status === storyStatus.IN_REVIEW) {
      let issueTitle = '';
      let issueDescription = '';
      const actions = [];

      switch (selectedStory.reason) {
        case storyStatus.MAX_LOOP_ITERATIONS:
          issueTitle = 'Action Needed: Maximum number of iterations reached';
          issueDescription =
            'The story execution in the workbench has exceeded the maximum allowed iterations. You can update the story details and re-build it.';
          actions.push(
            { label: 'Re-Build', link: storyActions.REBUILD },
            {
              label: 'Get Help',
              link: 'https://discord.com/invite/dXbRe5BHJC',
            },
          );
          break;
        case storyStatus.LLM_KEY_NOT_FOUND:
          issueTitle = 'Action Needed: LLM API Key Configuration Error';
          issueDescription =
            'There is an issue with the LLM API Key configuration, which may involve an invalid or expired API key. Please verify the API Key settings and update them to continue.';
          actions.push(
            { label: 'Re-Build', link: storyActions.REBUILD },
            { label: 'Go to Settings', link: '/settings' },
          );
          break;
      }

      setIssue({
        title: issueTitle,
        description: issueDescription,
        actions,
      });
    } else {
      resetIssue();
    }
  }, [selectedStory]);

  const handleEditAction = () => {
    setEditTrue(true);
    close();
  };
  const handleDeleteAction = () => {
    toDeleteStory().then().catch();
  };

  async function toDeleteStory() {
    try {
      const response = await deleteStory(Number(selectedStory.id));
      if (response && response.data && response.data.status.includes('OK')) {
        toGetAllDesignStoriesOfProject();
      }
    } catch (error) {
      console.error('Error while deleting story: ', error);
    } finally {
      close();
    }
  }
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
      action: () => handleDeleteAction(),
    },
  ];

  const handleMoveToInProgressClick = async () => {
    const openWorkbench = await handleInProgressStoryStatus(
      setOpenSetupModelModal,
      numberOfStoriesInProgress,
      toUpdateStoryStatus,
      'frontend',
    );
    if (openWorkbench) {
      router.push(`/design_workbench`);
    }
  };

  async function toUpdateStoryStatus(status: string) {
    try {
      const response = await updateStoryStatus(status, selectedStory.id);
      if (response && response.data && response.data.status.includes('OK')) {
        toGetAllDesignStoriesOfProject();
      }
    } catch (error) {
      console.error('Error while updating story status:: ', error);
    } finally {
      close();
    }
  }

  return (
    selectedStory && (
      <div
        id={`${id}_design_story_details`}
        className={`${styles.new_story_container} proxima_nova text-white`}
        style={{ height: `calc(100% - ${top})` }}
      >
        <div
          id={'story_details_header'}
          className={`${styles.story_details_header} flex flex-shrink-0 flex-row items-center justify-between px-4 py-6`}
        >
          <div
            className={
              'flex flex-row flex-nowrap items-center gap-1 text-xl font-medium'
            }
          >
            <span className={`inline-block max-w-[22vw] truncate`}>
              {selectedStory.title}{' '}
            </span>
            <span className={'secondary_color font-[300]'}>
              #{selectedStory.id}
            </span>
          </div>

          <div className={'flex flex-row items-center gap-3'}>
            {selectedStory.status === storyStatus.TODO && (
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
              text={handleStoryStatus(selectedStory.status).text}
              color={handleStoryStatus(selectedStory.status).color}
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
              className={'size-5'}
              src={imagePath.closeIcon}
              alt={'close_icon'}
              onClick={close}
            />
          </div>
        </div>
        {selectedStory.status === storyStatus.IN_REVIEW && (
          <div
            className={`${styles.issue_container} m-4 flex flex-row gap-2 rounded-lg p-3`}
          >
            <CustomImage
              className={'mt-[2px] size-5'}
              src={imagePath.overviewWarningYellow}
              alt={'error_icon'}
            />

            <div
              id={'in_review_description'}
              className={'proxima_nova flex flex-col gap-2 text-white'}
            >
              <span id={'issue_title'} className={'text-base font-medium'}>
                {issue?.title}
              </span>

              <span
                id={'issue_description'}
                className={'text-sm font-normal opacity-80'}
              >
                {issue?.description}
              </span>

              <div
                id={'issue_action_buttons'}
                className={'my-2 flex flex-row gap-2'}
              >
                {issue?.actions?.length > 0 &&
                  issue.actions.map((action, index) => (
                    <Button
                      key={index}
                      onClick={() => {
                        if (action.link === storyActions.REBUILD) {
                          handleMoveToInProgressClick().then().catch();
                        } else {
                          router.push(action.link);
                        }
                      }}
                      className={
                        action.label === storyActions.REBUILD
                          ? 'primary_medium'
                          : 'light_medium'
                      }
                    >
                      {action.label}
                    </Button>
                  ))}
              </div>
            </div>
          </div>
        )}
        <div className={'flex flex-col gap-8 p-4'}>
          <div className={'flex flex-col gap-2'}>
            <span className={`secondary_color text-sm font-normal`}>
              DESIGN (Image)
            </span>
            <div
              className={`relative flex max-h-[50vh] justify-center overflow-hidden rounded-lg ${styles.story_image_container}`}
            >
              <img
                className={'max-h-full max-w-full object-contain'}
                src={selectedStory.input_file_url}
                alt={'input_image '}
              />
            </div>
          </div>
        </div>
      </div>
    )
  );
};

export default DesignStoryDetails;
