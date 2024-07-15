import React, { useState } from 'react';
import { useDesignContext } from '@/context/Design';
import { Button } from '@nextui-org/react';
import styles from '@/components/DesignStoryComponents/desingStory.module.css';
import { handleStoryStatus } from '@/app/utils';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import CustomTag from '@/components/CustomTag/CustomTag';
import Image from 'next/image';

const DesignStoryList: React.FC = () => {
  const {
    setOpenCreateStoryModal,
    setOpenStoryDetailsModal,
    storyList,
    setSelectedStory,
  } = useDesignContext();
  const [searchTerm, setSearchTerm] = useState('');
  const handleOpenStoryDetails = (storyDetails) => {
    setOpenStoryDetailsModal(true);
    setSelectedStory(storyDetails);
    localStorage.setItem('storyId', storyDetails.id);
  };

  const filteredStories = storyList.filter((story) =>
    story.title.toLowerCase().includes(searchTerm.toLowerCase()),
  );
  return (
    <div className={'flex flex-col gap-4'}>
      <div className={'flex flex-row gap-2'}>
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
        <Button
          className={'primary_medium'}
          onClick={() => setOpenCreateStoryModal(true)}
        >
          Create Story
        </Button>
      </div>
      <div className={'grid grid-cols-4 gap-8'}>
        {filteredStories &&
          filteredStories.map((story, index) => (
            <div
              id={'design_card_' + index}
              key={'design_card' + index}
              className={'col-span-1 flex w-full flex-col gap-3'}
              onClick={() => handleOpenStoryDetails(story)}
            >
              <div
                className={`${styles.story_image_container} relative flex h-48 w-full items-center justify-center overflow-hidden rounded-lg px-[10px] py-2`}
              >
                <div className={'relative h-full w-full'}>
                  <Image
                      src={story.input_file_url}
                      alt={'design_image'}
                      fill
                      className="object-contain"
                      loading="lazy"
                  />
                </div>
                <div className={'absolute left-[5px] top-1.5'}>
                  <CustomTag
                      icon={null}
                      iconClass={'size-4'}
                      className={'rounded-lg'}
                      text={handleStoryStatus(story.status).text}
                      color={handleStoryStatus(story.status).color}
                  />
                </div>
              </div>
              <div className={'flex flex-row gap-2'}>
                <span
                    className={`${styles.story_number} justify-center text-sm font-normal leading-normal`}
                >
                  #{story.id}
                </span>
                <span
                  className={`overflow-hidden truncate text-sm font-normal leading-normal text-white`}
                >
                  {story.title}
                </span>
              </div>
            </div>
          ))}
      </div>
    </div>
  );
};

export default DesignStoryList;
