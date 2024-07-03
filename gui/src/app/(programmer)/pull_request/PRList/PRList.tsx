'use client';
import imagePath from '@/app/imagePath';
import styles from '../pr.module.css';
import GithubReviewButton from '@/app/(programmer)/pull_request/PRList/GithubReviewButton';
import {
  PRListItems,
  PRListProps,
} from '../../../../../types/pullRequestsTypes';
import { useRouter } from 'next/navigation';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { prStatuses } from '@/app/constants/PullRequestConstants';

export default function PRList({ type, list }: PRListProps) {
  const router = useRouter();
  const handlePRImage = (pr: PRListItems) => {
    const prStatus = {
      OPEN: imagePath.prReadyIcon,
      CLOSED: imagePath.prClosedIcon,
      MERGED: imagePath.prMergedIcon,
    };
    return prStatus[pr.status];
  };

  const handleReviewStatus = (pr: PRListItems) => {
    const reviewStatus = {
      OPEN: 'Review required',
      CLOSED: `Closed on ${pr.closed_on}`,
      MERGED: `Merged on ${pr.merged_on}`,
    };

    return reviewStatus[pr.status];
  };

  const handlePRClick = (pr: PRListItems) => {
    router.push(`/pull_request/${pr.pull_request_id}`);
  };

  return (
    <div
      id={`${type}_pr_list`}
      className={'flex flex-col overflow-scroll'}
      style={{ height: 'calc(100vh - 180px)' }}
    >
      {list &&
        list.length > 0 &&
        list.map((pr, index) => (
          <div
            key={index}
            className={styles.pr_list_container}
            onClick={() => handlePRClick(pr)}
          >
            <div className={'flex flex-row gap-2'}>
              <CustomImage
                className={'size-6'}
                src={handlePRImage(pr)}
                alt={'pr_icon'}
              />

              <div id={'pr_details'} className={'flex flex-col items-start'}>
                <span className={'text-base font-normal'}>
                  {pr.pull_request_name}
                </span>
                <div
                  className={
                    'flex flex-row items-center justify-center gap-1 text-[11px] font-normal opacity-60'
                  }
                >
                  #{pr.pull_request_id} created on {pr.created_on}
                  <span>· {handleReviewStatus(pr)}</span>
                </div>
              </div>
            </div>

            <div className={'flex flex-row items-center justify-center gap-4'}>
              {pr.status === prStatuses.OPEN && (
                <GithubReviewButton>Review</GithubReviewButton>
              )}
              <div
                className={
                  'flex flex-row items-center justify-center gap-1 text-[13px] font-normal opacity-60'
                }
              >
                <CustomImage
                  className={'size-4'}
                  src={imagePath.chatBubbleIcon}
                  alt={'chat_bubble_icon'}
                />
                {pr.total_comments}
              </div>
            </div>
          </div>
        ))}
    </div>
  );
}
