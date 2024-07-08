'use client';
import { useEffect, useState } from 'react';
import { Button } from '@nextui-org/react';
import CustomTextImage from '@/components/ImageComponents/CustomTextImage';
import imagePath from '@/app/imagePath';
import styles from './projects.module.css';
import { useRouter } from 'next/navigation';
import { ProjectTypes } from '../../../types/projectsTypes';
import {
  getProjectTypeFromFramework,
  setProjectDetails,
  toGetAllProjects,
} from '@/app/utils';
import CreateOrEditProjectBody from '@/components/HomeComponents/CreateOrEditProjectBody';
import CustomLoaders from '@/components/CustomLoaders/CustomLoaders';
import { SkeletonTypes } from '@/app/constants/SkeletonConstants';
import { projectTypes } from '@/app/constants/ProjectConstants';

export default function Projects() {
  const [openNewProjectModal, setOpenNewProjectModal] = useState<
    boolean | null
  >(false);
  const [projectsList, setProjectsList] = useState<ProjectTypes[] | null>(null);
  const router = useRouter();

  const handleProjectClick = async (project: ProjectTypes) => {
    setProjectDetails(project);
    if (
      getProjectTypeFromFramework(project.project_framework) ===
      projectTypes.DESIGN
    ) {
      router.push('/design');
    } else {
      router.push(`/board`);
    }
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
              projectsList.length > 0 &&
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
              ))}
          </div>
        </div>
      ) : (
        <CustomLoaders type={'skeleton'} skeletonType={SkeletonTypes.PROJECT} />
      )}
    </div>
  );
}
