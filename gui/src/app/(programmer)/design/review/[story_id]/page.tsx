'use client';
import { Button } from '@nextui-org/react';
import Link from 'next/link';
import Image from 'next/image';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import styles from './review.module.css';
import FrontendCodeSection from '@/components/DesignStoryComponents/FrontendCodeSection';
import React, { useEffect, useRef, useState } from 'react';
import ReBuildModal from '@/components/RebuildModal/RebuildModal';
import Browser from '@/components/WorkBenchComponents/Browser';
import { CommentReBuildDesignStoryPayload } from '../../../../../../types/pullRequestsTypes';
import {
  getFrontendCode,
  getDesignStoryDetails,
  rebuildDesignStory,
  updateReviewViewedStatus,
} from '@/api/DashboardService';
import { DesignStoryItem } from '../../../../../../types/designStoryTypes';
import { CodeFile } from '../../../../../../types/customComponentTypes';
import CustomTabs from '@/components/CustomTabs/CustomTabs';
import { useRouter } from 'next/navigation';
import { useDesignContext } from '@/context/Design';

const ReviewPage: React.FC = (props) => {
  const [openRebuildModal, setOpenRebuildModal] = useState<boolean>(false);
  const [rebuildComment, setRebuildComment] = useState<string | null>('');
  const [story, setStory] = useState<DesignStoryItem | null>(null);
  const [codeFiles, setCodeFiles] = useState<CodeFile[]>([]);
  const story_id = props['params'].story_id;
  const frontendURL = useRef('');
  const router = useRouter();
  const { setSelectedStory } = useDesignContext();
  const tabOptions = [
    {
      key: 'preview',
      text: null,
      content: <Browser url={frontendURL.current} showUrl={false} />,
      selected: imagePath.visualDiffIconSelected,
      unselected: imagePath.visualDiffIconUnselected,
    },

    {
      key: 'code',
      text: null,
      content: (
        <FrontendCodeSection
          height={'calc(100vh - 242px)'}
          allowCopy={true}
          codeFiles={codeFiles}
        />
      ),
      selected: imagePath.codeIconSelected,
      unselected: imagePath.codeIconUnselected,
    },
  ];
  useEffect(() => {
    toGetStoryDetails(story_id).then().catch();
    getCode().then().catch();
    if (!openRebuildModal) {
      setRebuildComment('');
    }
  }, [openRebuildModal]);

  async function getCode() {
    try {
      const response = await getFrontendCode(story_id);
      if (response) {
        const data = response.data;
        setCodeFiles(data.code_files);
      }
    } catch (error) {
      console.error(error);
    }
  }

  async function toUpdateReviewViewedStatus(story_id: number) {
    try {
      await updateReviewViewedStatus(story_id);
    } catch (error) {
      console.error(error);
    }
  }

  async function toGetStoryDetails(story_id) {
    try {
      const response = await getDesignStoryDetails(story_id);
      if (response) {
        const data = response.data;
        setStory(data.story);
        if (data.story && data.story.review_viewed === false) {
          toUpdateReviewViewedStatus(data.story.id).then().catch();
        }
        frontendURL.current = data.story.frontend_url;
      }
    } catch (error) {
      console.error(error);
    }
  }
  const handleRebuildStory = () => {
    const data = {
      story_id: Number(story_id),
      comment: rebuildComment,
    };
    rebuildWithComment(data).then().catch();
    setOpenRebuildModal(false);
    setRebuildComment('');
  };

  async function rebuildWithComment(payload: CommentReBuildDesignStoryPayload) {
    try {
      const response = await rebuildDesignStory(payload);
      if (response) {
        const data = response.data;
        setSelectedStory(story);
        router.push('/workbench');
      }
    } catch (error) {
      console.error('Error while creating story:: ', error);
    }
  }
  return (
    <div>
      {story !== null && (
        <div className={'proxima_nova'}>
          <ReBuildModal
            openRebuildModal={openRebuildModal}
            setOpenRebuildModal={setOpenRebuildModal}
            rebuildComment={rebuildComment}
            setRebuildComment={setRebuildComment}
            handleRebuildStory={handleRebuildStory}
          />
          <div className={`flex flex-row ${styles.heading_container}`}>
            <Link href={`/design`} className={'flex justify-center py-6 pl-4'}>
              <Image
                src={imagePath.leftArrowGrey}
                alt={'black_arrow'}
                width={16}
                height={16}
              />
            </Link>
            <div className={'flex gap-2 px-4 py-6'}>
              <span className={'text-2xl font-semibold text-white'}>
                {story.title}
              </span>
              <span className={'secondary_color m-auto text-xl font-light'}>
                #{story.id}
              </span>
            </div>
          </div>
          <div className={'flex flex-col'}>
            <div className={'flex flex-row justify-between p-4 pb-2 pt-4'}>
              <div className={'flex flex-row gap-2'}>
                <Image
                  src={imagePath.visualDiffIconSelected}
                  alt={'eye_icon'}
                  width={20}
                  height={20}
                />
                <span className={'my-auto text-base font-semibold'}>
                  Visual Diff
                </span>
              </div>
              <Button
                className={'secondary_medium'}
                onClick={() => setOpenRebuildModal(true)}
              >
                <CustomImage
                  className={'size-4'}
                  src={imagePath.playIcon}
                  alt={'play_icon'}
                />{' '}
                Re-Build
              </Button>
            </div>
            <div className={`grid grid-cols-2 p-2`}>
              <div
                className={`${styles.container} flex h-fit flex-col rounded-tl-lg`}
              >
                <span
                  className={`${styles.heading_container} space_mono p-3 px-2 text-xs`}
                >
                  Input
                </span>
                <div
                  className={
                    'relative flex h-[40vh] w-full justify-center overflow-hidden'
                  }
                >
                  <Image
                    src={story.input_file_url}
                    alt={'design_image'}
                    fill
                    className="object-contain"
                    loading="lazy"
                  />
                </div>
              </div>
              <div
                className={`${styles.container} flex flex-col rounded-tr-lg`}
              >
                <CustomTabs
                  options={tabOptions}
                  position={'end'}
                  containerCss={'background'}
                  tabCss={'my-1 p-2'}
                >
                  <span className={'space_mono mx-2 text-xs'}>Output</span>
                </CustomTabs>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
export default ReviewPage;
