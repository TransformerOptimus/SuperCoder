'use client';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import { Button } from '@nextui-org/react';
import { Suspense, useEffect, useState } from 'react';
import CreateEditStory from '@/components/StoryComponents/CreateEditStory';
import ActiveWorkbench from '@/components/WorkBenchComponents/ActiveWorkbench';
import CustomDrawer from '@/components/CustomDrawer/CustomDrawer';
import { useRouter } from 'next/navigation';
import { useWorkbenchContext, WorkbenchProvider } from '@/context/Workbench';
import { toGetAllStoriesOfProjectUtils } from '@/app/utils';
import { storyTypes } from '@/app/constants/ProjectConstants';

export default function WorkBench() {
  const [isModalOpen, setIsModalOpen] = useState<boolean | null>(false);
  const { storiesList, setStoriesList } = useWorkbenchContext();
  const router = useRouter();

  const activeWorkbenchCondition = () => {
    return (
      storiesList &&
      (storiesList.IN_PROGRESS || storiesList.DONE) &&
      (storiesList.IN_PROGRESS.length > 0 || storiesList.DONE.length > 0)
    );
  };

  useEffect(() => {
    toGetAllStoriesOfProjectUtils(setStoriesList).then().catch();
    setTimeout(() => {
      toGetAllStoriesOfProjectUtils(setStoriesList).then().catch();
    }, 10000);
  }, []);

  return (
    <div id={'workbench'} className={'proxima_nova p-4'}>
      {activeWorkbenchCondition() ? (
        <Suspense fallback={<div>Loading....</div>}>
          <ActiveWorkbench
            storiesList={storiesList}
            storyType={storyTypes.BACKEND}
          />
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
