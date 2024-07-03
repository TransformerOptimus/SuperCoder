'use client';
import PRList from '@/app/(programmer)/pull_request/PRList/PRList';
import imagePath from '@/app/imagePath';
import CustomTabs from '@/components/CustomTabs/CustomTabs';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { useEffect, useRef } from 'react';
import { usePullRequestsContext } from '@/context/PullRequests';
import { toGetProjectPullRequests } from '@/app/utils';

export default function PullRequests() {
  const projectIdRef = useRef<string | null>(null);
  const { prList, setPRList } = usePullRequestsContext();

  const tabOptions = [
    {
      key: 'ALL',
      text: 'All',
      icon: null,
      content: <PRList type={'open'} list={prList} />,
    },
    {
      key: 'OPEN',
      text: 'Open',
      icon: imagePath.prReadyIcon,
      content: <PRList type={'open'} list={prList} />,
    },
    {
      key: 'MERGED',
      text: 'Merged',
      icon: imagePath.prMergedIcon,
      content: <PRList type={'open'} list={prList} />,
    },
    {
      key: 'CLOSED',
      text: 'Closed',
      icon: imagePath.prClosedIcon,
      content: <PRList type={'open'} list={prList} />,
    },
  ];

  const handleTabSelection = (key: string) => {
    toGetProjectPullRequests(setPRList, key).then().catch();
  };

  useEffect(() => {
    if (typeof window !== 'undefined') {
      projectIdRef.current = localStorage.getItem('projectId');
    }
    toGetProjectPullRequests(setPRList).then().catch();
  }, []);

  return (
    <div
      id={'pull_requests'}
      className={'proxima_nova flex size-full flex-col gap-4 px-[12vw] py-5'}
    >
      <div className={'flex w-full flex-row gap-2'}>
        <div className={'relative flex w-full items-center'}>
          <CustomImage
            className={'pointer-events-none absolute left-3 size-4'}
            src={imagePath.searchIcon}
            alt={'search_icon'}
          />
          <input
            type={'text'}
            className={'input_medium w-full rounded border py-2 pl-10 pr-3'}
          />
        </div>
      </div>

      <CustomTabs options={tabOptions} handle={handleTabSelection} />
    </div>
  );
}
