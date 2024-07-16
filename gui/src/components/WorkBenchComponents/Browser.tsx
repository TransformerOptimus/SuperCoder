'use client';
import React, { useEffect, useState } from 'react';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import { BrowserProps } from '../../../types/workbenchTypes';

export default function Browser({
  url,
  status = false,
  showUrl = true,
}: BrowserProps) {
  const [iframeKey, setIframeKey] = useState(0);

  useEffect(() => {
    setIframeKey((prevKey) => prevKey + 1);
  }, [status]);

  return (
    <div
      id={'browser'}
      className={'flex w-full flex-col gap-[6px] border-none'}
    >
      {showUrl && (
        <CustomInput
          format={'text'}
          value={url}
          type={'secondary'}
          icon={imagePath.browserIcon}
          size={'size-4'}
          alt={'search_icon'}
        />
      )}
      <div
        className={'relative w-full overflow-hidden'}
        style={{
          height: showUrl ? 'calc(30vh)' : 'calc(35vh)',
        }}
      >
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
