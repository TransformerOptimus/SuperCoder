'use client';
import Image from 'next/image';
import imagePath from '@/app/imagePath';
import styles from '@/app/(programmer)/pull_request/pr.module.css';
import { useRouter } from 'next/navigation';
import {
  CommitItems,
  PRListItems,
} from '../../../../../types/pullRequestsTypes';
import React, { useEffect, useRef, useState } from 'react';
import CommitLog from '@/app/(programmer)/pull_request/[pr_id]/CommitLogs';
import { Button } from '@nextui-org/react';
import CustomTabs from '@/components/CustomTabs/CustomTabs';
import FilesChanged from '@/app/(programmer)/pull_request/[pr_id]/FilesChanged';
import { usePullRequestsContext } from '@/context/PullRequests';
import {
  commentRebuildStory,
  getCommitsPullRequest,
  getPullRequestDiff,
  mergePullRequest,
} from '@/api/DashboardService';
import { toGetProjectPullRequests } from '@/app/utils';
import CustomTag from '@/components/CustomTag/CustomTag';
import { prStatuses } from '@/app/constants/PullRequestConstants';
import toast from 'react-hot-toast';
import ReBuildModal from '@/components/RebuildModal/RebuildModal';

export default function PRDetails(props) {
  const [openRebuildModal, setOpenRebuildModal] = useState<boolean | null>(
    false,
  );
  const [selectedPRId, setSelectedPRId] = useState<string | null>(
    props['params'].pr_id,
  );
  const [selectedPR, setSelectedPR] = useState<PRListItems | null>(null);
  const [rebuildComment, setRebuildComment] = useState<string | null>('');
  const [commits, setCommits] = useState<CommitItems[] | null>(null);
  const [diff, setDiff] = useState<string | null>(null);
  const { prList, setPRList } = usePullRequestsContext();
  const router = useRouter();
  const projectNameRef = useRef(null);
  const tabOptions = [
    {
      key: 'files_changed',
      text: 'Files Changed',
      selected: imagePath.filesChangedIconSelected,
      unselected: imagePath.filesChangedIconUnselected,
      count: diff && diff.split(/(?=diff --git a\/)/g).length,
      content: <FilesChanged diff={diff} />,
    },
    {
      key: 'commits',
      text: 'Commits',
      selected: imagePath.commitsIconSelected,
      unselected: imagePath.commitsIconUnselected,
      count: commits && commits.length,
      content: <CommitLog commits={commits} />,
    },
  ];

  const handleSelectedPR = () => {
    const pr = prList.find(
      (item) => item.pull_request_id.toString() === selectedPRId,
    );
    setSelectedPR(pr);
  };

  const handleRebuildCommentClick = () => {
    setRebuildComment('');
    setOpenRebuildModal(true);
  };

  const handleRebuildStory = () => {
    if (rebuildComment === '') return;
    toCommentRebuildStory().then().catch();
  };

  const handlePRStatus = (status: string) => {
    const prStatus = {
      OPEN: {
        image: imagePath.prOpenWhiteIcon,
        text: 'Open',
        color: 'green',
      },
      MERGED: {
        image: imagePath.prMergedIcon,
        text: 'Merged',
        color: 'purple',
      },
      CLOSED: {
        image: imagePath.prClosedIcon,
        text: 'Closed',
        color: 'grey',
      },
    };

    return prStatus[status]
      ? prStatus[status]
      : {
          image: imagePath.prClosedIcon,
          text: 'Default',
          color: 'grey',
        };
  };

  useEffect(() => {
    if (typeof window !== 'undefined')
      projectNameRef.current = localStorage.getItem('projectName');
    toGetProjectPullRequests(setPRList).then().catch();
    toGetCommitsPullRequest().then().catch();
    toGetPullRequestDiff().then().catch();
  }, []);

  useEffect(() => {
    setSelectedPRId(props['params'].pr_id);
    if (selectedPRId && prList) handleSelectedPR();
  }, [selectedPRId, prList]);

  async function toCommentRebuildStory() {
    try {
      const payload = {
        pull_request_id: Number(selectedPRId),
        comment: rebuildComment,
      };

      const response = await commentRebuildStory(payload);
      if (response) {
        const data = response.data;
        console.log(data);
        router.push(`/workbench`);
      }
    } catch (error) {
      console.error('Error while commenting and rebuilding: ', error);
    }
  }

  async function toMergePullRequest() {
    try {
      const response = await mergePullRequest(Number(selectedPRId));
      if (response) {
        const data = response.data;
        console.log(data);
        toast.success('PR has been Successfully Merged');
        router.push(`/pull_request`);
      }
    } catch (error) {
      console.error('Error while merging pull request: ', error);
      toast.error('Error occurred while Merging the PR');
    }
  }

  async function toGetCommitsPullRequest() {
    try {
      const response = await getCommitsPullRequest(Number(selectedPRId));
      if (response) {
        const data = response.data;
        setCommits(data.all_commits);
        console.log(data.all_commits.length);
      }
    } catch (error) {
      console.error('Error while fetching commits: ', error);
    }
  }

  async function toGetPullRequestDiff() {
    try {
      const response = await getPullRequestDiff(Number(selectedPRId));
      if (response) {
        const data = response.data;
        setDiff(data.diff);
      }
    } catch (error) {
      console.error('Error while fetching pull request diff: ', error);
    }
  }

  return (
    <div id={`${selectedPRId}_pr_details`} className={'flex flex-col'}>
      <ReBuildModal
        openRebuildModal={openRebuildModal}
        setOpenRebuildModal={setOpenRebuildModal}
        rebuildComment={rebuildComment}
        setRebuildComment={setRebuildComment}
        handleRebuildStory={handleRebuildStory}
      />

      {selectedPR && (
        <div className={styles.pr_details_header_container}>
          <div className={'flex flex-row items-center justify-center gap-2'}>
            <Image
              width={0}
              height={0}
              sizes={'100vw'}
              className={'size-5 cursor-pointer'}
              src={imagePath.leftArrowGrey}
              alt={'left_arrow_grey'}
              onClick={() => router.push('/pull_request')}
            />

            <span className={'text-2xl font-semibold'}>
              {selectedPR.pull_request_name}
            </span>
            <span className={'secondary_color text-xl font-[300]'}>
              #{selectedPRId}
            </span>
          </div>

          <CustomTag
            icon={imagePath.prOpenWhiteIcon}
            iconClass={'size-4'}
            text={handlePRStatus(selectedPR.status).text}
            color={handlePRStatus(selectedPR.status).color}
            className={'rounded-3xl'}
          />
        </div>
      )}

      <div className={'flex flex-col p-4'}>
        <div className={'flex flex-row items-center justify-between'}>
          {commits && diff && (
            <CustomTabs options={tabOptions}>
              {selectedPR && selectedPR.status === prStatuses.OPEN && (
                <div id={'pr_actions'} className={'flex flex-row gap-2'}>
                  <Button
                    className={'secondary_medium'}
                    startContent={
                      <Image
                        width={0}
                        height={0}
                        sizes={'100vw'}
                        className={'size-5'}
                        src={imagePath.playIcon}
                        alt={'play_icon'}
                      />
                    }
                    onClick={() => handleRebuildCommentClick()}
                  >
                    Re-Build
                  </Button>
                  <Button
                    className={'primary_medium'}
                    onClick={() => toMergePullRequest()}
                  >
                    Merge Request
                  </Button>
                </div>
              )}
            </CustomTabs>
          )}
        </div>
      </div>
    </div>
  );
}
