'use client';
import './style.css';
import { useEffect, useState } from 'react';
import Image from 'next/image';
import imagePath from '@/app/imagePath';
import { navbarOptions } from '@/app/constants/NavbarConstants';
import { useRouter } from 'next/navigation';
import useProjectDropdown from '@/hooks/useProjectDropdown';
import CustomImage from '@/components/ImageComponents/CustomImage';
import CustomDropdown from '@/components/CustomDropdown/CustomDropdown';
import { getUsernameInitials, logout } from '@/app/utils';
import CreateOrEditProjectBody from '@/components/HomeComponents/CreateOrEditProjectBody';
import { useUserContext } from '@/context/UserContext';

export default function NavBar() {
  const userContext = useUserContext();

  const [username, setUsername] = useState<string | undefined>(undefined);
  const [projectName, setProjectName] = useState(null);
  const [openEditProjectModal, setOpenEditProjectModal] = useState<
    boolean | null
  >(false);
  const router = useRouter();
  const showProjects = useProjectDropdown();

  const dropdownItems = [
    {
      key: 'settings',
      text: 'Settings',
      icon: (
        <CustomImage
          className={'size-4'}
          src={imagePath.settingsIcon}
          alt={'settings_icon'}
        />
      ),
      action: () => router.push('/settings'),
    },
    {
      key: 'logout',
      text: 'Logout',
      icon: (
        <CustomImage
          className={'size-4'}
          src={imagePath.logoutIcon}
          alt={'logout_icon'}
        />
      ),
      action: () => logout(),
    },
  ];

  const projectItems = [
    {
      key: 'edit',
      text: 'Edit',
      icon: (
        <CustomImage
          className={'size-4'}
          src={imagePath.editIcon}
          alt={'close_icon'}
        />
      ),
      action: () => setOpenEditProjectModal(true),
    },
    {
      key: 'move',
      text: 'Back to All Projects',
      icon: (
        <CustomImage
          className={'size-4 rotate-180'}
          src={imagePath.moveToIcon}
          alt={'close_icon'}
        />
      ),
      action: () => router.push('/projects'),
    },
  ];

  const usernameInitials = username ? getUsernameInitials(username) : '';

  useEffect(() => {
    if (typeof window !== 'undefined') {
      if (userContext && userContext.name) {
        setUsername(userContext.name);
      }
      setProjectName(localStorage.getItem('projectName'));
    }
  }, [userContext]);

  return (
    <nav id={'navbar_section'} className={'navbar proxima_nova'}>
      <CreateOrEditProjectBody
        id={'navbar'}
        openProjectModal={openEditProjectModal}
        setOpenProjectModal={setOpenEditProjectModal}
        edit={true}
      />
      <div className={'flex w-full flex-row justify-between'}>
        <div
          id={'project_selection_section'}
          className={'flex flex-row items-center justify-center gap-2'}
        >
          <Image
            width={0}
            height={0}
            sizes={'100vw'}
            src={imagePath.superagiIconLogo}
            alt={'superagi_logo'}
            className={'mx-4 size-10 cursor-pointer'}
            onClick={() => router.push('/projects')}
            priority
          />

          {showProjects && (
            <div
              className={'ml-3 flex flex-row items-center justify-center gap-1'}
            >
              <CustomImage
                className={'size-5'}
                src={imagePath.projectIcon}
                alt={'project_icon'}
                priority={true}
              />

              <span className={'text-[13px] font-normal'}>{projectName}</span>

              <CustomDropdown
                trigger={
                  <CustomImage
                    className={'size-4 cursor-pointer'}
                    src={imagePath.bottomArrowGrey}
                    alt={'bottom_arrow_grey'}
                  />
                }
                maxHeight={'200px'}
                gap={'10px'}
                position={'start'}
              >
                {projectItems &&
                  projectItems.map((item) => (
                    <CustomDropdown.Item key={item.key} onClick={item.action}>
                      <div
                        className={
                          'flex flex-row items-center justify-center gap-2'
                        }
                      >
                        {item.icon}
                        {item.text}
                      </div>
                    </CustomDropdown.Item>
                  ))}
              </CustomDropdown>
            </div>
          )}
        </div>

        <div
          id={'account_section'}
          className={'flex flex-row items-center justify-center gap-4'}
        >
          <div id={'help_section'} className={'flex flex-row'}>
            {navbarOptions.map((option, index) => (
              <div
                key={index}
                id={`${option.id}_${index}_option`}
                className={
                  'flex cursor-pointer flex-row items-center justify-center gap-2'
                }
                onClick={() => window.open(option.url, '_blank')}
              >
                <CustomImage
                  className={'size-4'}
                  src={option.image}
                  alt={option.id + '_icon'}
                />

                <span className={'text-[13px] font-normal'}>{option.text}</span>
              </div>
            ))}
          </div>

          <div className={'flex flex-row items-center justify-between gap-1'}>
            <div
              className={
                'initial_circle flex flex-row items-center justify-center gap-2'
              }
            >
              <span className={'text-sm font-normal opacity-60'}>
                {usernameInitials}
              </span>
            </div>
            <CustomDropdown
              trigger={
                <CustomImage
                  className={'size-4 cursor-pointer'}
                  src={imagePath.bottomArrowGrey}
                  alt={'bottom_arrow_grey'}
                />
              }
              maxHeight={'200px'}
              gap={'10px'}
              position={'end'}
            >
              <CustomDropdown.Section showDivider>
                <span className={'proxima_nova text-sm font-normal'}>
                  {username}
                </span>
              </CustomDropdown.Section>

              <CustomDropdown.Section>
                {dropdownItems &&
                  dropdownItems.map((item) => (
                    <CustomDropdown.Item key={item.key} onClick={item.action}>
                      <div
                        className={
                          'flex flex-row items-center justify-center gap-2'
                        }
                      >
                        {item.icon}
                        {item.text}
                      </div>
                    </CustomDropdown.Item>
                  ))}
              </CustomDropdown.Section>
            </CustomDropdown>
          </div>
        </div>
      </div>
    </nav>
  );
}
