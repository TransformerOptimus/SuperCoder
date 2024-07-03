'use client';
import React, { useEffect, useRef, useState } from 'react';
import imagePath from '@/app/imagePath';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { Button } from '@nextui-org/react';
import CreateEditStory from '@/components/StoryComponents/CreateEditStory';
import CustomDrawer from '@/components/CustomDrawer/CustomDrawer';
import StoryDetails from '@/components/StoryComponents/StoryDetails';
import { storyStatus } from '@/app/constants/BoardConstants';
import { useBoardContext } from '@/context/Boards';
import { toGetAllStoriesOfProjectUtils } from '@/app/utils';
import styles from './board.module.css';
import SetupModelModal from '@/components/StoryComponents/SetupModelModal';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import { SkeletonTypes } from '@/app/constants/SkeletonConstants';
import CustomInput from '@/components/CustomInput/CustomInput';

const TaskItem = ({ task, handleStoryClick }) => (
  <div
    id={`task_${task.id}`}
    className={'task_container flex cursor-pointer flex-col gap-1'}
    onClick={() => handleStoryClick(task)}
  >
    <span className={'text-xs font-normal opacity-60'}>#{task.story_id}</span>
    <span className={'text-sm font-normal'}>{task.story_name}</span>
  </div>
);

const TaskList = ({ title, tasks, dotImage, handleStoryClick }) => (
  <div className={'col-span-3'}>
    <div className={'card_container flex flex-col gap-2 p-2'}>
      <div className={'mb-1 mt-2 flex flex-row items-center justify-between'}>
        <div className={'flex flex-row items-center justify-center gap-1'}>
          <CustomImage
            className={'size-4'}
            src={dotImage}
            alt={`${title.toLowerCase()}_dot`}
          />

          <span className={'text-xs font-semibold opacity-60'}>{title}</span>
        </div>

        <div className={styles.number_tag}>{tasks && tasks.length}</div>
      </div>

      <div
        className={'flex flex-col gap-2 overflow-scroll'}
        style={{ maxHeight: 'calc(100vh - 180px)' }}
      >
        {tasks &&
          tasks.map((task, index) => (
            <TaskItem
              key={index}
              task={task}
              handleStoryClick={handleStoryClick}
            />
          ))}
      </div>
    </div>
  </div>
);

export default function Board() {
  const [openCreateStoryModal, setOpenCreateStoryModal] = useState<
    boolean | null
  >(false);

  const [openStoryDetailsModal, setOpenStoryDetailsModal] = useState<
    boolean | null
  >(false);

  const [openSetupModelModal, setOpenSetupModelModal] =
    useState<boolean>(false);
  const [boardData, setBoardData] = useState(null);
  const [selectedStory, setSelectedStory] = useState<number | null>(null);
  const { editTrue, setEditTrue } = useBoardContext();
  const [searchValue, setSearchValue] = useState<string | null>('');

  const handleStoryClick = (story) => {
    setSelectedStory(story.story_id);
    setOpenStoryDetailsModal(true);
    localStorage.setItem('storyId', story.story_id);
    setEditTrue(false);
  };

  const handleCreateStory = () => {
    setEditTrue(false);
    setOpenCreateStoryModal(true);
  };

  const handleSearchChange = (search: string) => {
    toGetAllStoriesOfProject();
  };

  useEffect(() => {
    toGetAllStoriesOfProject();
  }, [searchValue]);

  useEffect(() => {
    if (editTrue)
      setTimeout(() => {
        setOpenCreateStoryModal(true);
      }, 500);
  }, [editTrue]);

  function toGetAllStoriesOfProject() {
    toGetAllStoriesOfProjectUtils(setBoardData, searchValue).then().catch();
  }

  return boardData ? (
    <div id={'board'} className={'proxima_nova flex flex-col gap-2 p-4'}>
      <CustomDrawer
        open={openCreateStoryModal}
        onClose={() => setOpenCreateStoryModal(false)}
        direction={'right'}
        width={'40vw'}
        top={'50px'}
        contentCSS={'rounded-l-2xl'}
      >
        <CreateEditStory
          id={'board'}
          close={() => setOpenCreateStoryModal(false)}
          top={'50px'}
          story_id={selectedStory}
          toGetAllStoriesOfProject={toGetAllStoriesOfProject}
        />
      </CustomDrawer>

      <CustomDrawer
        open={openStoryDetailsModal}
        onClose={() => setOpenStoryDetailsModal(false)}
        direction={'right'}
        width={'50vw'}
        top={'50px'}
        contentCSS={'rounded-l-2xl'}
      >
        <StoryDetails
          id={'board'}
          story_id={selectedStory}
          tabCSS={'p-4'}
          toGetAllStoriesOfProject={toGetAllStoriesOfProject}
          close={() => setOpenStoryDetailsModal(false)}
          open_status={openStoryDetailsModal}
          number_of_stories_in_progress={
            boardData && boardData.IN_PROGRESS.length
          }
          setOpenSetupModelModal={setOpenSetupModelModal}
        />
      </CustomDrawer>
      <SetupModelModal
        openModal={openSetupModelModal}
        setOpenModel={setOpenSetupModelModal}
      />

      <div
        id={'board_filter_section'}
        className={'flex w-full flex-row items-center gap-2'}
      >
        <CustomInput
          format={'text'}
          value={searchValue}
          setter={setSearchValue}
          placeholder={'Enter search term...'}
          type={'primary'}
          icon={imagePath.searchIcon}
          size={'size-4'}
          alt={'search_icon'}
        />

        <Button
          className={'primary_medium'}
          onClick={() => handleCreateStory()}
        >
          Create Story
        </Button>
      </div>

      <div
        id={'board_task_section'}
        className={'grid size-full grid-cols-12 gap-2'}
      >
        <TaskList
          title={storyStatus.TODO}
          tasks={boardData.TODO}
          dotImage={imagePath.todoDot}
          handleStoryClick={handleStoryClick}
        />

        <TaskList
          title="IN PROGRESS"
          tasks={boardData.IN_PROGRESS}
          dotImage={imagePath.inprogressDot}
          handleStoryClick={handleStoryClick}
        />

        <TaskList
          title="IN REVIEW"
          tasks={boardData.IN_REVIEW}
          dotImage={imagePath.inReviewDot}
          handleStoryClick={handleStoryClick}
        />

        <TaskList
          title={storyStatus.DONE}
          tasks={boardData.DONE}
          dotImage={imagePath.doneDot}
          handleStoryClick={handleStoryClick}
        />
      </div>
    </div>
  ) : (
    <CustomLoaders type={'skeleton'} skeletonType={SkeletonTypes.BOARD} />
  );
}
