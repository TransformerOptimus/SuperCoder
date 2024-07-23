'use client';
import React, { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import CustomTextImage from '@/components/ImageComponents/CustomTextImage';
import imagePath from '@/app/imagePath';
import styles from './projects.module.css';
import { useRouter } from 'next/navigation';
import { ProjectTypes } from '../../../types/projectsTypes';
import { setProjectDetails, toGetAllProjects } from '@/app/utils';
import CreateOrEditProjectBody from '@/components/HomeComponents/CreateOrEditProjectBody';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import { SkeletonTypes } from '@/app/constants/SkeletonConstants';
import CustomImage from '@/components/ImageComponents/CustomImage';
import CustomModal from '@/components/CustomModal/CustomModal';

export default function Projects() {
  const [openNewProjectModal, setOpenNewProjectModal] = useState<
    boolean | null
  >(false);
  const [projectsList, setProjectsList] = useState<ProjectTypes[] | null>(null);
  const [showGithubStar, setShowGithubStar] = useState<boolean | null>(true);
  const router = useRouter();

  const handleProjectClick = async (project: ProjectTypes) => {
    setProjectDetails(project);
    router.push(`/board`);
  };

  const handleGithubStarClick = () => {
    window.open('https://github.com/TransformerOptimus/SuperCoder', '_blank');
    setModalInteraction();
  };

  const handleGithubStarClose = () => {
    setModalInteraction();
  };

  const setModalInteraction = () => {
    const fourWeeksFromNow = new Date();
    fourWeeksFromNow.setDate(fourWeeksFromNow.getDate() + 28);
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
    toGetAllProjects(setProjectsList).then().catch();
  }, []);

  useEffect(() => {
    if (checkModalInteraction()) {
      setShowGithubStar(false);
    }
  }, []);

  return (
    <div
      id={'projects_page'}
      className={'proxima_nova h-screen w-screen px-[10vw] py-10 text-white'}
    >
      <CreateOrEditProjectBody
        id={'projects'}
        openProjectModal={openNewProjectModal}
        setOpenProjectModal={setOpenNewProjectModal}
        projectsList={projectsList}
      />

      <CustomModal
        isOpen={showGithubStar}
        onClose={handleGithubStarClose}
        width={'32vw'}
      >
        <CustomModal.Body padding={'24px'}>
          <div
            className={
              'proxima_nova flex flex-col items-center gap-3 text-white'
            }
          >
            <div className={'text-base font-medium'}>
              Support the project by leaving a star on GitHub repository
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

      {projectsList ? (
        <div className={'flex flex-col gap-6'}>
          <div
            id={'project_page_header'}
            className={'flex flex-row items-center justify-between'}
          >
            <span className={'text-2xl font-semibold'}>Projects</span>
            <Button
              className={'primary_medium'}
              onClick={() => setOpenNewProjectModal(true)}
            >
              New Project
            </Button>
          </div>

          <div
            id={'projects_list'}
            className={'grid w-full grid-cols-12 gap-3 overflow-y-scroll'}
            style={{ maxHeight: 'calc(100vh - 160px)' }}
          >
            {projectsList &&
              (projectsList.length > 0 ? (
                projectsList.map((project, index) => (
                  <div
                    id={`project_${project.project_id}`}
                    key={index}
                    className={`${styles.project_container} card_container col-span-6`}
                    onClick={() => handleProjectClick(project)}
                  >
                    <div className={'flex flex-col gap-2'}>
                      <span className={'text-xl font-semibold'}>
                        {project.project_name}
                      </span>
                      <span className={'secondary_color text-sm font-normal'}>
                        {project.project_description}
                      </span>
                    </div>

                    <div className={'flex flex-row items-center gap-3'}>
                      <CustomTextImage
                        gap={'gap-1'}
                        textCSS={'text-[11px] secondary_color font-normal'}
                        text={
                          project.pull_request_count &&
                          project.pull_request_count.toString()
                        }
                        imageCSS={'size-[14px]'}
                        src={imagePath.prOpenGreyIcon}
                        alt={'number_of_commits'}
                        priority={true}
                      />
                    </div>
                  </div>
                ))
              ) : (
                <div className={'col-span-12 grid'}>
                  <div
                    className={
                      'flex flex-col items-center justify-center gap-2 py-44'
                    }
                  >
                    <CustomImage
                      className={'size-24'}
                      src={imagePath.emptyFilesIcons}
                      alt={'empty_icon'}
                    />
                    <span className={'proxima_nova secondary_color text-xl'}>
                      No projects created yet!
                    </span>
                  </div>
                </div>
              ))}
          </div>
        </div>
      ) : (
        <CustomLoaders type={'skeleton'} skeletonType={SkeletonTypes.PROJECT} />
      )}
    </div>
  );
}
