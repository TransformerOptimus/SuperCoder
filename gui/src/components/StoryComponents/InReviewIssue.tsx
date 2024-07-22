import React from 'react';
import { Button } from '@nextui-org/react';
import CustomImage from '@/components/ImageComponents/CustomImage';
import styles from './story.module.css';
import { useRouter } from 'next/navigation';
import { storyActions } from '@/app/constants/BoardConstants';
import imagePath from '@/app/imagePath';
import { IssueContainerProps } from '../../../types/storyTypes';

const IssueContainer: React.FC<IssueContainerProps> = ({
  title,
  description,
  actions,
  image = imagePath.overviewWarningYellow,
  handleMoveToInProgressClick,
}) => {
  const router = useRouter();

  return (
    <div
      className={`${styles.issue_container} m-4 flex flex-row gap-2 rounded-lg p-3`}
    >
      <CustomImage
        className={'mt-[2px] size-5'}
        src={image}
        alt={'error_icon'}
      />

      <div
        id={'in_review_description'}
        className={'proxima_nova flex flex-col gap-2 text-white'}
      >
        <span id={'issue_title'} className={'text-base font-medium'}>
          {title}
        </span>

        <span
          id={'issue_description'}
          className={'text-sm font-normal opacity-80'}
        >
          {description}
        </span>

        <div id={'issue_action_buttons'} className={'my-2 flex flex-row gap-2'}>
          {actions.length > 0 &&
            actions.map((action, index) => (
              <Button
                key={index}
                onClick={() => {
                  if (action.link === storyActions.REBUILD) {
                    handleMoveToInProgressClick().then().catch();
                  } else {
                    router.push(action.link);
                  }
                }}
                className={
                  action.label === storyActions.REBUILD
                    ? 'primary_medium'
                    : 'light_medium'
                }
              >
                {action.label}
              </Button>
            ))}
        </div>
      </div>
    </div>
  );
};

export default IssueContainer;
