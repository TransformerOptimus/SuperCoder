'use client';
import React, { useEffect, useState, useRef, useMemo } from 'react';
import { usePathname } from 'next/navigation';
import Loader from '@/components/CustomLoaders/Loader';
import { Button } from '@nextui-org/react';
import CustomImage from '@/components/ImageComponents/CustomImage';
import imagePath from '@/app/imagePath';
import CustomModal from '@/components/CustomModal/CustomModal';
import CustomInput from '@/components/CustomInput/CustomInput';
import { createPullRequest } from '@/api/DashboardService';
import toast from 'react-hot-toast';

export default function Code() {
  const [projectURL, setProjectURL] = useState('');
  const initialURL = useRef<string | null>(null);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const pathName = usePathname();
  const [isIframeLoaded, setIsIframeLoaded] = useState(false);
  const [prTitle, setPRTitle] = useState<string | null>('');
  const [prDescription, setPRDescription] = useState<string | null>('');
  const [isCreatePRModalOpen, setIsCreatePRModalOpen] = useState<
    boolean | null
  >(false);

  useEffect(() => {
    const storedURL = localStorage.getItem('projectURL');
    if (!initialURL.current && storedURL) {
      initialURL.current = storedURL;
      setProjectURL(storedURL);
    }
  }, []);

  useEffect(() => {
    const handleIframeLoad = () => {
      setIsIframeLoaded(true);
    };

    const handleIframeError = () => {
      setIsIframeLoaded(false);
    };

    const checkIframeLoaded = () => {
      if (!isIframeLoaded && iframeRef.current) {
        iframeRef.current.src = projectURL;
      }
    };

    const intervalId = setInterval(checkIframeLoaded, 10000);

    if (iframeRef.current) {
      iframeRef.current.addEventListener('load', handleIframeLoad);
      iframeRef.current.addEventListener('error', handleIframeError);
    }

    return () => {
      clearInterval(intervalId);
      if (iframeRef.current) {
        iframeRef.current.removeEventListener('load', handleIframeLoad);
        iframeRef.current.removeEventListener('error', handleIframeError);
      }
    };
  }, [projectURL, isIframeLoaded]);

  const handleCreatePRClick = () => {
    toCreatePullRequest().then().catch();
  };

  async function toCreatePullRequest() {
    try {
      const payload = {
        project_id: Number(localStorage.getItem('projectId')),
        title: prTitle,
        description: prDescription,
      };

      const response = await createPullRequest(payload);
      if (response) {
        const data = response.data;
        toast.success('Pull Request created Successfully!!');
        console.log(data);
      }
    } catch (error) {
      toast.error('Pull Request was not Created!!');
      console.error('Error while creating a Pull Request: ', error);
    } finally {
      setIsCreatePRModalOpen(false);
    }
  }

  const iframeElement = useMemo(() => {
    return (
      <iframe
        ref={iframeRef}
        src={projectURL}
        allow={'clipboard-read; clipboard-write;'}
        title={'Embedded Workspace'}
        style={{
          width: '100%',
          height: 'calc(100vh - 92px)',
          border: 'none',
          position: pathName === '/code' ? 'relative' : 'absolute',
          top: pathName === '/code' ? '0' : '-9999px',
          left: pathName === '/code' ? '0' : '-9999px',
          visibility: pathName === '/code' ? 'visible' : 'hidden',
        }}
      />
    );
  }, [projectURL, pathName]);

  return (
    <div
      className={`relative ${
        pathName !== '/code' && 'hidden'
      } proxima_nova h-screen w-full`}
    >
      <CustomModal
        isOpen={isCreatePRModalOpen}
        onClose={() => setIsCreatePRModalOpen(false)}
        width={'30vw'}
      >
        <CustomModal.Header title={'Create pull request'} />
        <CustomModal.Body>
          <div className={'flex flex-col gap-1 pb-6'} id={'title_section'}>
            <span className={'secondary_color text-[13px] font-normal'}>
              Title
            </span>
            <CustomInput
              format={'text'}
              value={prTitle}
              setter={setPRTitle}
              placeholder={'Enter pull request title'}
              type={'primary'}
            />
          </div>

          <div className={'flex flex-col gap-1'} id={'description_section'}>
            <span className={'secondary_color text-[13px] font-normal'}>
              Description
            </span>
            <textarea
              value={prDescription}
              onChange={(event) => setPRDescription(event.target.value)}
              className={'textarea_large'}
              placeholder={'Enter pull request description'}
            />
          </div>
        </CustomModal.Body>
        <CustomModal.Footer>
          <Button
            className={'primary_medium w-fit'}
            onClick={handleCreatePRClick}
          >
            Raise Request
          </Button>
        </CustomModal.Footer>
      </CustomModal>

      <div className={'code_header pl-2'}>
        <span className={'secondary_color text-[13px] font-semibold'}>
          Code Editor
        </span>
        <Button
          className={
            'rounded-none bg-transparent px-3 text-[13px] font-semibold text-white hover:bg-gray-600'
          }
          onClick={() => setIsCreatePRModalOpen(true)}
        >
          <CustomImage
            className={'size-4'}
            src={imagePath.prOpenGreyIcon}
            alt={'pull_request_icon'}
          />
          Create Pull Request
        </Button>
      </div>

      {!isIframeLoaded && pathName === '/code' && (
        <div className="absolute left-0 top-0 flex h-[720px] w-full items-center justify-center">
          <Loader size={120} text="Please wait..." />
        </div>
      )}
      {iframeElement}
    </div>
  );
}
