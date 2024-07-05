'use client';
import React, { useEffect, useState } from 'react';
import CreateEditDesignStory from '@/components/DesignStoryComponents/CreateEditDesignStory';
import CustomDrawer from '@/components/CustomDrawer/CustomDrawer';
import { Button } from '@nextui-org/react';
import DesignStoryDetails from '@/components/DesignStoryComponents/DesignStoryDetails';
import { useDesignContext } from '@/context/Design';
import StoryList from '@/components/DesignStoryComponents/DesignStoryList';
import ReviewList from '@/components/DesignStoryComponents/ReviewList';
import { getAllDesignStoriesOfProject } from '@/api/DashboardService';
import SetupModelModal from '@/components/StoryComponents/SetupModelModal';
import CustomTabs from '@/components/CustomTabs/CustomTabs';

const DesignPage: React.FC = () => {
  const {
    openStoryDetailsModal,
    setOpenStoryDetailsModal,
    openCreateStoryModal,
    setOpenCreateStoryModal,
    storyList,
    editTrue,
    setStoryList,
  } = useDesignContext();
  const [openSetupModelModal, setOpenSetupModelModal] =
    useState<boolean>(false);
  const [numberOfStoriesInProgress, setNumberOfStoriesInProgress] =
    useState<number>(0);

  useEffect(() => {
    toGetAllDesignStoriesOfProject().then().catch();
  }, []);

  useEffect(() => {
    if (editTrue)
      setTimeout(() => {
        setOpenCreateStoryModal(true);
      }, 500);
  }, [editTrue]);

  const tabOptions = [
    {
      key: 'stories',
      text: 'Stories',
      content: <StoryList />,
      icon: null,
    },
    {
      key: 'reviews',
      text: 'Reviews',
      content: <ReviewList />,
      icon: null,
    },
  ];

  async function toGetAllDesignStoriesOfProject() {
    try {
      const project_id = localStorage.getItem('projectId');
      const response = await getAllDesignStoriesOfProject(project_id);
      if (response) {
        const data = response.data;
        if (data.stories) {
          setStoryList(data.stories);
        } else {
          setStoryList([]);
        }
      }
    } catch (error) {
      console.error('Error while fetching all stories of project:: ', error);
    }
  }

  const handleCreateStory = () => {
    setOpenCreateStoryModal(true);
  };

  return (
    <div className={'h-screen w-full'}>
      <CustomDrawer
        open={openStoryDetailsModal}
        onClose={() => setOpenStoryDetailsModal(false)}
        direction={'right'}
        width={'40vw'}
        top={'50px'}
        contentCSS={'rounded-l-2xl'}
      >
        <DesignStoryDetails
          id={'design'}
          close={() => setOpenStoryDetailsModal(false)}
          top={'50px'}
          toGetAllDesignStoriesOfProject={toGetAllDesignStoriesOfProject}
          setOpenSetupModelModal={setOpenSetupModelModal}
          number_of_stories_in_progress={numberOfStoriesInProgress}
        />
      </CustomDrawer>
      <CustomDrawer
        open={openCreateStoryModal}
        onClose={() => setOpenCreateStoryModal(false)}
        direction={'right'}
        width={'30vw'}
        top={'50px'}
        contentCSS={'rounded-l-2xl'}
      >
        <CreateEditDesignStory
          id={'design'}
          close={() => setOpenCreateStoryModal(false)}
          top={'50px'}
          toGetAllDesignStoriesOfProject={toGetAllDesignStoriesOfProject}
        />
      </CustomDrawer>
      <SetupModelModal
        openModal={openSetupModelModal}
        setOpenModel={setOpenSetupModelModal}
      />
      {storyList &&
        (storyList.length > 0 ? (
          <div>
            <CustomTabs options={tabOptions} />
          </div>
        ) : (
          <div
            className={
              'flex h-screen flex-col items-center justify-center gap-4'
            }
          >
            <span>Get started by creating your first design story!</span>
            <Button className={'primary_medium'} onClick={handleCreateStory}>
              Create Story
            </Button>
          </div>
        ))}
    </div>
  );
};

export default DesignPage;
