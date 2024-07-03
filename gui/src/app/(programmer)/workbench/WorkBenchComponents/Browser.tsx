'use client';
import React from 'react';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import { BrowserProps } from '../../../../../types/workbenchTypes';

export default function Browser({ url }: BrowserProps) {
  return (
    <div
      id={'browser'}
      className={'flex w-full flex-col gap-[6px] border-none p-2'}
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
          src={url}
          className={'h-full w-full border-none bg-white'}
          loading={'lazy'}
        />
      </div>
    </div>
  );
}
