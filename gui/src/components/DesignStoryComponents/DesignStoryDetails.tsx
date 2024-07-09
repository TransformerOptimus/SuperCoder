import styles from '@/components/DesignStoryComponents/desingStory.module.css';
import { DesignStoryDetailsProps } from '../../../types/designStoryTypes';
import imagePath from '@/app/imagePath';
import CustomImage from '@/components/ImageComponents/CustomImage';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import React from 'react';
import CustomTag from '@/components/CustomTag/CustomTag';
import { handleInProgressStoryStatus, handleStoryStatus } from '@/app/utils';
import { useDesignContext } from '@/context/Design';
import { deleteStory, updateStoryStatus } from '@/api/DashboardService';
import { storyStatus } from '@/app/constants/BoardConstants';
import { useRouter } from 'next/navigation';
import { Button } from '@nextui-org/react';

const DesignStoryDetails: React.FC<DesignStoryDetailsProps> = ({
  id,
  close,
  top,
  toGetAllDesignStoriesOfProject,
  setOpenSetupModelModal,
  number_of_stories_in_progress,
}) => {
  const { selectedStory, setEditTrue } = useDesignContext();
  const router = useRouter();
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
      number_of_stories_in_progress,
      toUpdateStoryStatus,
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
