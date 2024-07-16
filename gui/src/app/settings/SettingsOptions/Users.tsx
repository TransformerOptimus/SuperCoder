'use client';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import styles from './settingOptions.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import CustomModal from '@/components/CustomModal/CustomModal';
import CustomInput from '@/components/CustomInput/CustomInput';
import {
  addUserToOrganisation,
  getOrganisationMembers,
  removeUserFromOrganisation,
} from '@/api/DashboardService';
import { validateEmail } from '@/app/utils';

export default function Users() {
  const [openInviteUserModal, setOpenInviteUserModal] =
    useState<boolean>(false);
  const [openRemoveUserModal, setOpenRemoveUserModal] =
    useState<boolean>(false);

  const email = [
    { email: 'admin@test.com'},
    { email: 'random@email.com'},
    { email: 'moin@mail.com'},
  ];

  const [userList, setUserList] = useState<{email: string}[]>(email);
  const [inviteUserEmail, setInviteUserEmail] = useState<string>('');
  const [removeUserEmail, setRemoveUserEmail] = useState<string>('');
  const [emailErrorMsg, setEmailErrorMsg] = useState<string>('');

  useEffect(() => {
    fetchUsersFromOrganisation().then().catch();
  }, []);

  useEffect(() => {
    setInviteUserEmail('');
  }, [openInviteUserModal]);

  async function fetchUsersFromOrganisation() {
    try {
      const response = await getOrganisationMembers();
      if (response) {
        const data = response.data;
        setUserList(data.user_list);
      }
    } catch (error) {
      console.error(error);
    }
  }

  async function sendInvite() {
    try {
      if (!validateEmail(inviteUserEmail)) {
        setEmailErrorMsg('Enter a Valid Email.');
        return;
      }
      const response = await addUserToOrganisation(inviteUserEmail);
      if (response) {
        fetchUsersFromOrganisation().catch();
      }
    } catch (error) {
      console.error(error);
    } finally {
      setOpenInviteUserModal(false);
    }
  }

  async function toRemoveUserFromOrganisation() {
    try {
      const response = await removeUserFromOrganisation(removeUserEmail);
      if (response) {
        fetchUsersFromOrganisation().then().catch();
      }
    } catch (error) {
      console.error(error);
    } finally {
      setOpenRemoveUserModal(false);
      setRemoveUserEmail('');
    }
  }

  const handleOpenRemoveModal = (email: string) => {
    setRemoveUserEmail(email);
    setOpenRemoveUserModal(true);
  };

  const onSetEmail = (value) => {
    setInviteUserEmail(value);
    setEmailErrorMsg('');
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
              setter={onSetEmail}
              placeholder={'Enter Email Address'}
              isError={emailErrorMsg !== ''}
              errorMessage={emailErrorMsg}
            />
          </div>
        </CustomModal.Body>
        <CustomModal.Footer>
          <Button className={'primary_medium'} onClick={sendInvite}>
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
          <Button
            className={'primary_medium'}
            onClick={toRemoveUserFromOrganisation}
          >
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
        {userList &&
          userList.map((item, index) => {
            return (
              <div className={`px-3 py-4 ${styles.item} flex justify-between`}>
                <span className={`text-sm text-white`}>{item.email}</span>
                {index !== 0 && (
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
                      onClick={() => handleOpenRemoveModal(item.email)}
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
                )}
              </div>
            );
          })}
      </div>
    </div>
  );
}
