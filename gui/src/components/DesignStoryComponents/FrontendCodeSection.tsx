'use client';
import React, { useEffect, useState } from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { dracula } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { rgba } from 'color2k';
import { Button } from '@nextui-org/react';
import Image from 'next/image';
import imagePath from '@/app/imagePath';
import { FrontendCodeSectionProps } from '../../../types/customComponentTypes';
import styles from '@/components/DesignStoryComponents/desingStory.module.css';

const FrontendCodeSection: React.FC<FrontendCodeSectionProps> = ({
  height,
  allowCopy = true,
  backgroundColor = false,
  codeFiles = [],
}) => {
  const [selectedTab, setSelectedTab] = useState<string | null>(
    codeFiles.length > 0 ? codeFiles[0]?.file_name : null,
  );
  useEffect(() => {
    if (codeFiles.length > 0) {
      setSelectedTab(codeFiles[0].file_name);
    }
  }, [codeFiles]);
  const selectedCode = codeFiles.find(
    (option) => option.file_name === selectedTab,
  )?.code;
  const handleTabClick = (key: string) => {
    setSelectedTab(key);
  };
  const handleCopy = () => {
    navigator.clipboard.writeText(selectedCode);
  };
  return (
    <div
      className={`flex h-full flex-col bg-black ${
        backgroundColor && 'bg-code-section card_container'
      }`}
      style={{ height: height }}
    >
      <div
        className={`flex w-full flex-row items-center justify-between px-2 ${styles.tabs_container}`}
      >
        <div className={'flex flex-row items-center gap-2 overflow-auto'}>
          {codeFiles &&
            codeFiles.length > 0 &&
            codeFiles.map((option, index) => (
              <div
                key={index}
                className={`flex cursor-pointer flex-row items-center gap-2 rounded-lg px-2 py-1 transition-all duration-300 ease-in-out ${
                  selectedTab === option.file_name && styles.tab_item_selected
                }`}
                onClick={() => handleTabClick(option.file_name)}
              >
                {option.file_name && (
                  <span
                    className={`proxima_nova text-sm transition-colors duration-300 ease-in-out ${
                      selectedTab === option.file_name
                        ? 'font-medium'
                        : 'secondary_color'
                    }`}
                  >
                    {option.file_name}
                  </span>
                )}
              </div>
            ))}
        </div>
        {allowCopy && (
          <Button
            className={'secondary_color m-0 flex flex-row bg-transparent p-0'}
            size="sm"
            onClick={handleCopy}
          >
            <Image
              src={imagePath.copyIcon}
              alt={'copy_icon'}
              width={14}
              height={14}
            />
            Copy
          </Button>
        )}
      </div>
      {selectedTab && (
        <div
          className={`${styles.code_content} ${
            selectedCode ? styles.code_content_active : ''
          }`}
        >
          <SyntaxHighlighter
            language={selectedTab.split('.')[1]}
            style={dracula}
            customStyle={{
              fontSize: '12px',
              padding: '5px',
              margin: '0',
              borderRadius: '5px',
              width: '100%',
              background: rgba(0, 0, 0, 0.4),
            }}
            wrapLongLines={true}
          >
            {selectedCode}
          </SyntaxHighlighter>
        </div>
      )}
    </div>
  );
};

export default FrontendCodeSection;
