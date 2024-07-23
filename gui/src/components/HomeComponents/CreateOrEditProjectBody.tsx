import CustomModal from '@/components/CustomModal/CustomModal';
import CustomImageSelector from '@/components/ImageComponents/CustomImageSelector';
import {
  backendFrameworkOptions,
  frontendFrameworkOptions,
} from '@/app/constants/ProjectConstants';
import { Button } from '@nextui-org/react';
import { useEffect, useRef, useState } from 'react';
import {
  CreateProjectPayload,
  ProjectTypes,
  UpdateProjectPayload,
} from '../../../types/projectsTypes';
import {
  createProject,
  getGithubRepos,
  getProjectById,
  isGithubConnected,
  updateProject,
} from '@/api/DashboardService';
import { useRouter } from 'next/navigation';
import { setProjectDetails } from '@/app/utils';
import CustomImage from '@/components/ImageComponents/CustomImage';
import CustomInput from '@/components/CustomInput/CustomInput';
import styles from './create-project.module.css';
import imagePath from '@/app/imagePath';
import { API_BASE_URL } from '@/api/apiConfig';
import Select from 'react-select';

interface CreateOrEditProjectBodyProps {
  id: string;
  openProjectModal: boolean;
  setOpenProjectModal: (open: boolean) => void;
  projectsList?: ProjectTypes[];
  edit?: boolean;
}

const customStyles = {
  control: (provided) => ({
    ...provided,
    borderRadius: '0.5rem',
    backgroundColor: '#1e1e1e',
    color: '#ffffff',
    borderColor: '#333333',
    boxShadow: 'none',
    '&:hover': {
      color: '#ffffff',
      borderColor: '#333333',
      boxShadow: 'none',
    },
  }),
  indicatorSeparator: (provided) => ({}),
  menu: (provided) => ({
    ...provided,
    backgroundColor: '#1e1e1e',
  }),
  input: (styles) => ({
    ...styles,
    color: '#ffffff',
  }),
  option: (provided, state) => ({
    ...provided,
    backgroundColor: state.isFocused ? '#333333' : '#1e1e1e',
    color: '#ffffff',
    '&:hover': {
      backgroundColor: '#2a2a2a',
      color: '#ffffff',
    },
  }),
  singleValue: (provided) => ({
    ...provided,
    color: '#ffffff',
  }),
};

export default function CreateOrEditProjectBody({
  id,
  openProjectModal,
  setOpenProjectModal,
  projectsList,
  edit = false,
}: CreateOrEditProjectBodyProps) {
  const [selectedBackendFramework, setSelectedBackendFramework] =
    useState<string>(backendFrameworkOptions[0].id);
  const [selectedFrontendFramework, setSelectedFrontendFramework] =
    useState<string>(frontendFrameworkOptions[0].id);
  const [projectName, setProjectName] = useState<string>('');
  const [projectDescription, setProjectDescription] = useState<string>('');
  const [isLoading, setIsLoading] = useState<boolean | null>(false);
  const [isError, setIsError] = useState<boolean | null>(false);
  const [errorMessage, setErrorMessage] = useState<string | null>('');
  const projectIdRef = useRef(null);
  const router = useRouter();

  const [integrationLoading, setIntegrationLoading] = useState<boolean>(false);
  const [isExternalGitIntegration, setIsExternalGitIntegration] =
    useState<boolean>(false);
  const [useExternalGit, setUseExternalGit] = useState<boolean>(false);
  const [repositories, setRepositories] = useState<any[]>([]);
  const [selectedRepository, setSelectedRepository] = useState<{
    label: string;
    value: string;
  } | null>(null);

  async function redirectToGithubIntegration() {
    setIntegrationLoading(true);
    try {
      const interval = setInterval(async () => {
        const gitIntegrated = await isGithubConnected();
        if (gitIntegrated) {
          setIsExternalGitIntegration(true);
          setIntegrationLoading(false);
          clearInterval(interval);
        }
      }, 1000);
      window.open(`${API_BASE_URL}/integrations/github/authorize`, '_blank');
    } catch (error) {
      console.error('Error: ', error);
    }
  }

  const handleProjectDuplicationCheck = () => {
    if (!projectsList) {
      return false; // or handle the case where projectsList is null
    }
    return projectsList.some((project) => project.project_name === projectName);
  };

  const selectedBackendOption = backendFrameworkOptions.find(
    (option) => option.id === selectedBackendFramework,
  );

  const selectedFrontendOption = frontendFrameworkOptions.find(
    (option) => option.id === selectedFrontendFramework,
  );

  const handleCreateNewProject = async () => {
    setIsLoading(true);
    const projectErrors = [
      {
        validation: selectedRepository === null && useExternalGit,
        message: 'Please select a github repository to import.',
      },
      {
        validation: handleProjectDuplicationCheck(),
        message: 'A project with the name entered already exists.',
      },
      {
        validation: !/^[a-zA-Z0-9-_]+$/.test(projectName),
        message:
          'Name can only contain alphanumeric characters, dashes (-), and underscores (_). No special characters are allowed.',
      },
    ];

    for (const error of projectErrors) {
      if (error.validation) {
        setIsError(true);
        setErrorMessage(error.message);
        setIsLoading(false);
        return;
      }
    }

    setIsError(false);

    if (edit) {
      const updateProjectPayload = {
        project_id: Number(projectIdRef.current),
        name: projectName,
        description: projectDescription,
      };
      await toUpdateProject(updateProjectPayload);
    } else {
      const newProjectPayload = {
        name: projectName,
        framework: selectedBackendFramework,
        frontend_framework: selectedFrontendFramework,
        description: projectDescription,
        repository: useExternalGit ? selectedRepository.label : undefined,
        repository_url: useExternalGit ? selectedRepository.value : undefined,
      };
      await toCreateNewProject(newProjectPayload);
    }
  };

  useEffect(() => {
    (async function () {
      const repositories = await getGithubRepos();
      const options = repositories.map(
        (repository: { name: string; url: string }) => {
          const { name, url } = repository;
          return {
            value: url,
            label: name,
          };
        },
      );
      setRepositories(options);
    })();
  }, [isExternalGitIntegration]);

  useEffect(() => {
    (async function () {
      const gitIntegrated = await isGithubConnected();
      setIsExternalGitIntegration(gitIntegrated);
    })();
  }, []);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      projectIdRef.current = localStorage.getItem('projectId');
      if (edit) toGetProjectById().then().catch();
    }
  }, []);

  useEffect(() => {
    if (openProjectModal && !edit) {
      setProjectName('');
      setProjectDescription('');
      setSelectedBackendFramework(backendFrameworkOptions[0].id);
      setSelectedFrontendFramework(frontendFrameworkOptions[0].id);
    }
  }, [openProjectModal]);

  async function toGetProjectById() {
    try {
      if (projectIdRef.current) {
        const response = await getProjectById(projectIdRef.current);
        if (response) {
          const data = response.data;
          setProjectName(data.Name);
          setProjectDescription(data.Description);
          setSelectedBackendFramework(data.BackendFramework);
          setSelectedFrontendFramework(data.FrontendFramework);
        }
      }
    } catch (error) {
      console.error('Error while fetching project by project id:: ', error);
    }
  }

  async function toCreateNewProject(payload: CreateProjectPayload) {
    try {
      const response = await createProject(payload);
      if (response) {
        const data = response.data;
        setOpenProjectModal(false);
        setProjectDetails(data);
        router.push(`/board`);
      }
    } catch (error) {
      console.error('Error while creating a new project:: ', error);
    } finally {
      setIsLoading(false);
    }
  }

  async function toUpdateProject(payload: UpdateProjectPayload) {
    try {
      const response = await updateProject(payload);
      if (response) {
        const data = response.data;
        setProjectDetails(data);
      }
    } catch (error) {
      console.error('Error while updating project data:: ', error);
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <CustomModal
      isOpen={openProjectModal}
      width={'30vw'}
      onClose={() => setOpenProjectModal(false)}
    >
      <CustomModal.Header title={edit ? 'Edit Project' : 'New Project'} />
      <CustomModal.Body>
        <div className={'flex flex-col gap-6'}>
          <div className={'flex flex-col gap-1'} id={'name_section'}>
            <span className={'secondary_color text-[13px] font-normal'}>
              {' '}
              Name{' '}
            </span>
            {edit ? (
              <span className={'text-sm font-normal'}>{projectName}</span>
            ) : (
              <CustomInput
                format={'text'}
                value={projectName}
                setter={setProjectName}
                placeholder={'Enter project name'}
                errorMessage={errorMessage}
                isError={isError}
                cssClass={'w-full'}
                disabled={isLoading}
              />
            )}
          </div>
          <div
            className={'flex flex-col gap-1'}
            id={'backend_framework_section'}
          >
            <span className={'secondary_color text-[13px] font-normal'}>
              {' '}
              Backend Framework{' '}
            </span>

            {edit ? (
              <div className={'flex flex-row items-center gap-2'}>
                <CustomImage
                  className={'size-6 rounded-[4px]'}
                  src={selectedBackendOption.src}
                  alt={'selected_framework_icon'}
                />
                <span className={'text-sm font-normal'}>
                  {selectedBackendOption.text}
                </span>
              </div>
            ) : (
              <CustomImageSelector
                size={'70px'}
                gap={'12px'}
                imageOptions={backendFrameworkOptions}
                selectedOption={selectedBackendFramework}
                onSelectOption={setSelectedBackendFramework}
              />
            )}
          </div>

          <div
            className={'flex flex-col gap-1'}
            id={'frontend_framework_section'}
          >
            <span className={'secondary_color text-[13px] font-normal'}>
              {' '}
              Frontend Framework{' '}
            </span>

            {edit ? (
              selectedFrontendFramework && (
                <div className={'flex flex-row items-center gap-2'}>
                  <CustomImage
                    className={'size-6 rounded-[4px]'}
                    src={selectedFrontendOption.src}
                    alt={'selected_framework_icon'}
                  />
                  <span className={'text-sm font-normal'}>
                    {selectedFrontendOption.text}
                  </span>
                </div>
              )
            ) : (
              <CustomImageSelector
                size={'70px'}
                gap={'12px'}
                imageOptions={frontendFrameworkOptions}
                selectedOption={selectedFrontendFramework}
                onSelectOption={setSelectedFrontendFramework}
              />
            )}
          </div>

          <div>
            <label className="secondary_color mb-1 block text-sm font-medium">
              Repository
            </label>
            <div className="flex items-center space-x-4">
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={!useExternalGit}
                  onClick={() => setUseExternalGit(false)}
                  name="import-repository"
                  className="mr-2 text-white"
                />
                <span className="text-white">Create new repository</span>
              </label>
              <label className="flex items-center">
                <input
                  type="radio"
                  checked={useExternalGit}
                  onClick={() => setUseExternalGit(true)}
                  name="import-repository"
                  className="mr-2 text-white"
                />
                <span className="text-white">Import from github</span>
              </label>
            </div>
            <p className="mt-1 text-xs opacity-40">
              *Repository name will be named after the project name itself.
            </p>
          </div>

          {useExternalGit && isExternalGitIntegration && (
            <>
              <Select
                onChange={(e) => setSelectedRepository(e)}
                className="text-white"
                styles={customStyles}
                options={repositories}
              />
            </>
          )}

          {useExternalGit && !isExternalGitIntegration && (
            <div
              className={`rounded-md p-3 ${styles.integrate_github_container}`}
            >
              <div className="flex">
                <span className="mr-1 inline-block font-light">ⓘ</span>
                <div className="ml-1">
                  <p className="mb-2 text-sm font-medium text-gray-300">
                    Please connect your github account to continue. This will
                    allow the supercoder to import your repositories.
                  </p>
                  <Button
                    onClick={() => redirectToGithubIntegration()}
                    className={`primary_medium`}
                    isLoading={integrationLoading}
                  >
                    <CustomImage
                      className={'size-5'}
                      src={imagePath.githubLogo}
                      alt={'github_logo'}
                    />
                    Connect to Github
                  </Button>
                </div>
              </div>
            </div>
          )}

          <div className={'flex flex-col gap-1'} id={'description_section'}>
            <span className={'secondary_color text-[13px] font-normal'}>
              {' '}
              Description{' '}
            </span>
            <textarea
              value={projectDescription}
              onChange={(event) => setProjectDescription(event.target.value)}
              className={'textarea_medium'}
              placeholder={'Enter project description'}
              disabled={isLoading}
            />
          </div>
        </div>
      </CustomModal.Body>
      <CustomModal.Footer>
        <Button
          onClick={handleCreateNewProject}
          className={'primary_medium w-fit'}
          disabled={projectName === ''}
          isLoading={isLoading}
        >
          {edit ? 'Update' : 'Create'}
        </Button>
      </CustomModal.Footer>
    </CustomModal>
  );
}
