'use client';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import styles from './settingOptions.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import CustomModal from '@/components/CustomModal/CustomModal';
import CustomInput from '@/components/CustomInput/CustomInput';

export default function Users() {
  const [openInviteUserModal, setOpenInviteUserModal] =
    useState<boolean>(false);
  const [openRemoveUserModal, setOpenRemoveUserModal] =
    useState<boolean>(false);
  const [inviteUserEmail, setInviteUserEmail] = useState<string>('');
  const [removeUserEmail, setRemoveUserEmail] = useState<string>('');
  const email = ['admin@test.com', 'random@email.com', 'moin@mail.com'];
  useEffect(() => {
    setInviteUserEmail('');
  }, [openInviteUserModal]);

  const handleRemoveUser = () => {
    console.log('call remove user api');
    setOpenRemoveUserModal(false);
    setRemoveUserEmail('');
  };

  const handleOpenRemoveModal = (email) => {
    console.log(email);
    setRemoveUserEmail(email);
    setOpenRemoveUserModal(true);
  };

  const handleSendInvite = () => {
    console.log(inviteUserEmail);
    setOpenInviteUserModal(false);
  };

  const handleRevokeInvite = (email) => {
    console.log(email);
  };
  return (
    <div id={'users'} className={'proxima_nova flex flex-col gap-6'}>
      <CustomModal
        isOpen={openInviteUserModal}
        onClose={() => setOpenInviteUserModal(false)}
        width={'30vw'}
      >
        <CustomModal.Header title={'Add User'} />
        <CustomModal.Body padding={'24px 16px'}>
          <div className={'flex flex-col gap-2'}>
            <span className={'secondary_color text-sm font-normal'}>Email</span>
            <CustomInput
              format={'text'}
              value={inviteUserEmail}
              setter={setInviteUserEmail}
              placeholder={'Enter email Address'}
            />
          </div>
        </CustomModal.Body>
        <CustomModal.Footer>
          <Button className={'primary_medium'} onClick={handleSendInvite}>
            Invite User
          </Button>
        </CustomModal.Footer>
      </CustomModal>

      <CustomModal
        isOpen={openRemoveUserModal}
        onClose={() => setOpenRemoveUserModal(false)}
        width={'30vw'}
      >
        <CustomModal.Header title={'Remove User'} />
        <CustomModal.Body padding={'24px 16px'}>
          <span className={'secondary_color text-sm font-normal'}>
            Are you sure you want to remove {removeUserEmail}?
          </span>
        </CustomModal.Body>
        <CustomModal.Footer>
          <Button className={'primary_medium'} onClick={handleRemoveUser}>
            Remove
          </Button>
        </CustomModal.Footer>
      </CustomModal>
      <div className={'flex flex-row justify-between'}>
        <span className={'text-xl text-white'}>Users</span>
        <Button
          className={'primary_medium w-fit'}
          onClick={() => setOpenInviteUserModal(true)}
        >
          Add Users
        </Button>
      </div>
      <div className={`${styles.user_list_container} rounded-lg`}>
        <div className={`${styles.heading} rounded-t-lg px-3 py-2`}>
          <span className={'secondary_color text-sm'}>Email</span>
        </div>
        {email.map((item, index) => {
          return (
            <div className={`px-3 py-4 ${styles.item} flex justify-between`}>
              <span className={`text-sm text-white`}>{item}</span>
              {index !== 0 && index !== 2 ? (
                <CustomDropdown
                  trigger={
                    <CustomImage
                      className={'size-5 cursor-pointer'}
                      src={imagePath.verticalThreeDots}
                      alt={'three_dots_icon'}
                    />
                  }
                  maxHeight={'200px'}
                  gap={'10px'}
                  position={'start'}
                >
                  <CustomDropdown.Item
                    key={'1'}
                    onClick={() => handleOpenRemoveModal(item)}
                  >
                    <div
                      className={
                        'flex flex-row items-center justify-center gap-2'
                      }
                    >
                      <CustomImage
                        className={'size-4'}
                        src={imagePath.closeCircleIcon}
                        alt={'close_icon'}
                      />
                      Remove
                    </div>
                  </CustomDropdown.Item>
                </CustomDropdown>
              ) : (
                index === 2 && (
                  <Button
                    className={
                      'flex h-fit flex-row gap-1 border-none bg-transparent p-0'
                    }
                    onClick={() => handleRevokeInvite(item)}
                  >
                    <CustomImage
                      className={'size-4'}
                      src={imagePath.yellowBackIcon}
                      alt={'yellow back'}
                    />
                    <span className={`text-[13px] ${styles.revoke_color}`}>
                      Revoke Invite
                    </span>
                  </Button>
                )
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
