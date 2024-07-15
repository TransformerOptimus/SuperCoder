'use client';
import React from 'react';
import CustomContainers from '@/components/CustomContainers/CustomContainers';
import { Button } from '@nextui-org/react';
import { Suspense, useEffect, useState } from 'react';
import ActiveWorkbench from '@/components/WorkBenchComponents/ActiveWorkbench';
import { useRouter } from 'next/navigation';
import { toGetAllStoriesOfProjectUtils } from '@/app/utils';
import { StoryList } from '../../../../types/workbenchTypes';
import { storyTypes } from '@/app/constants/ProjectConstants';

const DesignWorkBenchPage: React.FC = () => {
  const [storiesList, setStoriesList] = useState<StoryList | null>(null);
  const router = useRouter();
  const activeDesignWorkbenchCondition = () => {
    return (
      storiesList &&
      (storiesList.IN_PROGRESS || storiesList.DONE) &&
      (storiesList.IN_PROGRESS.length > 0 || storiesList.DONE.length > 0)
    );
  };

  useEffect(() => {
    toGetAllStoriesOfProjectUtils(setStoriesList, '', 'frontend')
      .then()
      .catch();
    setTimeout(() => {
      toGetAllStoriesOfProjectUtils(setStoriesList, '', 'frontend')
        .then()
        .catch();
    }, 10000);
  }, []);

  return (
    <div id={'workbench'} className={'proxima_nova p-4'}>
      {activeDesignWorkbenchCondition() ? (
        <Suspense fallback={<div>Loading....</div>}>
          <ActiveWorkbench
            storiesList={storiesList}
            storyType={storyTypes.DESIGN}
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
              onClick={() => router.push('/design')}
            >
              Go to Design Board
            </Button>
          </div>
        </CustomContainers>
      )}
    </div>
  );
};
export default DesignWorkBenchPage;
