'use client';
import React from 'react';
import CustomInput from '@/components/CustomInput/CustomInput';
import imagePath from '@/app/imagePath';
import { BrowserProps } from '../../../../../types/workbenchTypes';

export default function Browser({ url, showUrl = true }: BrowserProps) {
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
        className="relative w-full overflow-hidden"
        style={{
          height: showUrl ? 'calc(30vh)' : 'calc(35vh)',
        }}
      >
        <iframe
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
