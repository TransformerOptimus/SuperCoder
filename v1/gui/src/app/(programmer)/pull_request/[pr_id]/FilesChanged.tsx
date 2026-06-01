import React from 'react';
import dynamic from 'next/dynamic';
import { FilesChangedProps } from '../../../../../types/pullRequestsTypes';

const MonacoDiffEditor = dynamic(
  () => import('@/components/CustomDiffEditor/MonacoDiffEditor'),
  { ssr: false },
);

const FilesChanged: React.FC<FilesChangedProps> = ({ diff }) => {
  const fileDiffs = diff && diff.split(/(?=diff --git a\/)/g);

  return (
    <div
      id={'files_changed'}
      className={'flex flex-col overflow-y-scroll px-[12vw] py-5'}
      style={{ maxHeight: 'calc(100vh - 200px)' }}
    >
      {diff &&
        fileDiffs.map((fileDiff, index) => {
          // Extract the file name from the diff string
          const match = fileDiff.match(/diff --git a\/(.+?) b\/(.+?)\n/);
          const fileName = match ? match[1] : `file${index + 1}`;

          return (
            <MonacoDiffEditor
              key={index}
              diffString={fileDiff}
              fileName={fileName}
            />
          );
        })}
    </div>
  );
};

export default FilesChanged;
