import styles from '@/components/DesignStoryComponents/story.module.css';
import { useRouter } from 'next/navigation';
import GithubReviewButton from '@/app/(programmer)/pull_request/PRList/GithubReviewButton';
import { useDesignContext } from '@/context/Design';
import { storyStatus } from '@/app/constants/BoardConstants';
import React, { useMemo, useState } from 'react';
import imagePath from '@/app/imagePath';
import CustomInput from '@/components/CustomInput/CustomInput';
const ReviewList: React.FC = () => {
  const router = useRouter();
  const openReview = (id: number) => {
    router.push(`design/review/${id}`);
  };
  const { storyList } = useDesignContext();
  const [searchTerm, setSearchTerm] = useState('');
  const doneStories = useMemo(() => {
    return storyList.filter(
      (story) =>
        story.status === storyStatus.DONE &&
        story.title.toLowerCase().includes(searchTerm.toLowerCase()),
    );
  }, [storyList, searchTerm]);
  return (
    <div className={'mx-60'}>
      <CustomInput
        format={'text'}
        value={searchTerm}
        setter={setSearchTerm}
        placeholder={'Enter search term...'}
        type={'primary'}
        icon={imagePath.searchIcon}
        size={'size-4'}
        alt={'search_icon'}
      />
      <div>
        {doneStories && doneStories.length > 0 ? (
          doneStories.map((story, index) => {
            if (story.status === storyStatus.DONE) {
              return (
                <div
                  key={'review_item_' + index}
                  className={`${styles.review_item} flex flex-row gap-4 px-2 py-3`}
                  onClick={() => openReview(story.id)}
                >
                  <div
                    className={`${styles.review_image_container} relative flex h-[60px] w-[60px] items-center justify-center overflow-hidden rounded-lg`}
                  >
                    <img
                      src={story.input_file_url}
                      alt="image"
                      className="max-h-full max-w-full object-contain"
                    />
                  </div>
                  <div
                    className={
                      'proxima_nova m-auto flex h-fit w-full flex-row justify-between'
                    }
                  >
                    <div className={'flex flex-col justify-center gap-2'}>
                      <span
                        className={`line-clamp-2 overflow-hidden text-sm font-normal leading-normal text-white`}
                      >
                        {story.title}
                      </span>
                      <div
                        className={
                          'secondary_color flex flex-row gap-1 text-xs font-normal leading-normal'
                        }
                      >
                        <span>#{story.id}</span>
                        <span>created on {story.created_on}</span>
                      </div>
                    </div>
                    {!story.review_viewed ? (
                      <GithubReviewButton>Review</GithubReviewButton>
                    ) : null}
                  </div>
                </div>
              );
            }
          })
        ) : (
          <div className={'flex items-center justify-center py-44'}>
            <span className={'proxima_nova secondary_color text-xl'}>
              No reviews found!
            </span>
          </div>
        )}
      </div>
    </div>
  );
};
export default ReviewList;
