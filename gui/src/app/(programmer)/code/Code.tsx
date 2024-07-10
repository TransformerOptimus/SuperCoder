'use client';
import React, { useEffect, useState, useRef, useMemo } from 'react';
import { usePathname } from 'next/navigation';
import Loader from '@/components/CustomLoaders/Loader';

export default function Code() {
  const [projectURL, setProjectURL] = useState('');
  const initialURL = useRef<string | null>(null);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const pathName = usePathname();
  const [isIframeLoaded, setIsIframeLoaded] = useState(false);

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

  const iframeElement = useMemo(() => {
    return (
        <iframe
            ref={iframeRef}
            src={projectURL}
            allow={'clipboard-read; clipboard-write;'}
            title={'Embedded Workspace'}
            style={{
              width: '100%',
              height: 'calc(100vh - 50px)',
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
      <>
        {!isIframeLoaded && pathName === '/code' && (
            <div
                className={
                  'absolute left-0 top-0 flex h-[720px] w-full items-center justify-center'
                }
            >
              <Loader size={120} text={'Please wait...'} />
            </div>
        )}
        {iframeElement}
      </>
  );
}
