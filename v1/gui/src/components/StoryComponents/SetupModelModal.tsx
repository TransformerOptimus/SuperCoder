import React, { useState } from 'react';
import CustomModal from '@/components/CustomModal/CustomModal';
import { Button } from '@nextui-org/react';
import { useRouter } from 'next/navigation';
import { SetupModelModalProps } from '../../../types/storyTypes';

const SetupModelModal: React.FC<SetupModelModalProps> = ({
  openModal,
  setOpenModel,
}) => {
  const router = useRouter();
  return (
    <CustomModal
      isOpen={openModal}
      onClose={() => setOpenModel(false)}
      width={'30vw'}
    >
      <CustomModal.Header title={'Setup model'} />
      <CustomModal.Body padding={'24px 16px'}>
        <span className={'text-sm font-normal'}>
          Set up your model to move stories to 'In Progress'
        </span>
      </CustomModal.Body>
      <CustomModal.Footer>
        <Button
          className={'primary_medium'}
          onClick={() => router.push('/settings')}
        >
          Go To Settings
        </Button>
      </CustomModal.Footer>
    </CustomModal>
  );
};

export default SetupModelModal;
