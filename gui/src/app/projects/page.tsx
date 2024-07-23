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

export default function Projects() {
  const [openNewProjectModal, setOpenNewProjectModal] = useState<
    boolean | null
  >(false);
  const [projectsList, setProjectsList] = useState<ProjectTypes[] | null>(null);
  const router = useRouter();

  const handleProjectClick = async (project: ProjectTypes) => {
    setProjectDetails(project);
    router.push(`/board`);
  };

  useEffect(() => {
    toGetAllProjects(setProjectsList).then().catch();
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
                      {project.project_repository && (
                        <div className="flex items-center">
                          <div className="-mt-1 mr-2 h-3.5 w-3.5">
                            <svg viewBox="0 0 24 24" className="fill-current">
                              <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
                            </svg>
                          </div>
                          <span className="secondary_color text-[11px] font-normal">
                            {project.project_repository.split('/').join(' / ')}
                          </span>
                        </div>
                      )}
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
