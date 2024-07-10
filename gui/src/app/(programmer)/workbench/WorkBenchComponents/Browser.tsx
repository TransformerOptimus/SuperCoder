'use client';
import React, { useEffect, useState } from 'react';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import { BrowserProps } from '../../../../../types/workbenchTypes';

export default function Browser({ url, status }: BrowserProps) {
  const [iframeKey, setIframeKey] = useState(0);

  useEffect(() => {
    console.log('Status: ', status.toString());
    setIframeKey((prevKey) => prevKey + 1);
  }, [status]);

  return (
    <div
      id={'browser'}
      className={'flex w-full flex-col gap-[6px] border-none'}
    >
      <CustomInput
        format={'text'}
        value={url}
        type={'secondary'}
        icon={imagePath.browserIcon}
        size={'size-4'}
        alt={'search_icon'}
      />
      <div className={'mx-10'} style={{ height: 'calc(100vh - 590px)' }}>
        <iframe
          key={iframeKey}
          src={url}
          className={
            'h-[200%] w-[200%] origin-top-left scale-50 border-none bg-white'
          }
          loading={'lazy'}
        />
      </div>
    </div>
  );
}
