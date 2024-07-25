import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import CustomModal from '@/components/CustomModal/CustomModal';

const GithubStarModal = () => {
  const [showGithubStar, setShowGithubStar] = useState<boolean | null>(null);

  const handleGithubStarClick = () => {
    window.open('https://github.com/TransformerOptimus/SuperCoder', '_blank');
    setModalInteraction();
  };

  const handleGithubStarClose = () => {
    setModalInteraction();
  };

  const setModalInteraction = () => {
    const fourWeeksFromNow = new Date();
    console.log('Todays Date: ', fourWeeksFromNow);
    fourWeeksFromNow.setDate(fourWeeksFromNow.getDate() + 28);
    console.log('Enabling Date: ', fourWeeksFromNow);
    localStorage.setItem(
      'githubStarHiddenTill',
      fourWeeksFromNow.toISOString(),
    );
    setShowGithubStar(false);
  };

  const checkModalInteraction = () => {
    const closedUntil = localStorage.getItem('githubStarHiddenTill');
    if (closedUntil) {
      const closedUntilDate = new Date(closedUntil);
      return closedUntilDate > new Date();
    }
    return false;
  };

  useEffect(() => {
    if (!checkModalInteraction()) {
      setShowGithubStar(true);
    } else {
      setShowGithubStar(false);
    }
  }, []);

  return (
    <CustomModal
      isOpen={showGithubStar}
      onClose={handleGithubStarClose}
      width={'32vw'}
    >
      <CustomModal.Body padding={'22px'}>
        <div
          className={'proxima_nova flex flex-col items-center gap-3 text-white'}
        >
          <div className={'text-base font-medium'}>
            Support the project by leaving a star on GitHub
          </div>

          <Button
            className={'secondary_medium mt-3 w-fit'}
            onClick={handleGithubStarClick}
          >
            Leave a ⭐ star on GitHub
          </Button>

          <div
            className={
              'secondary_color cursor-pointer text-xs font-normal opacity-80'
            }
            onClick={handleGithubStarClose}
          >
            I’ll do it later
          </div>
        </div>
      </CustomModal.Body>
    </CustomModal>
  );
};

export default GithubStarModal;
