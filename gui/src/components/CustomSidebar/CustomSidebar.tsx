'use client';
import React, { useState, useCallback } from 'react';
import styles from './sidebar.module.css';
import CustomImage from '@/components/ImageComponents/CustomImage';
import { CustomSidebarProps } from '../../../types/customComponentTypes';

const CustomSidebar: React.FC<CustomSidebarProps> = ({
  id,
  title,
  options,
  onOptionSelect,
}) => {
  const [selectedKey, setSelectedKey] = useState<string | null>(options[0].key);

  const handleOptionClick = useCallback(
    (key: string) => {
      setSelectedKey(key);
      if (onOptionSelect) {
        onOptionSelect(key);
      }
    },
    [onOptionSelect],
  );

  const SelectedComponent = selectedKey
    ? options?.find((option) => option.key === selectedKey)?.component
    : null;

  return (
    <div className={styles.container}>
      <div id={`${id}_sidebar`} className={styles.sidebar}>
        {title && <span className={styles.sidebar_title}>{title}</span>}
        <div className={'flex flex-col gap-1'}>
          {options &&
            options.map((option) => (
              <div
                key={option.key}
                className={`flex flex-row ${styles.option} ${
                  selectedKey === option.key ? styles.active : ''
                }`}
                onClick={() => handleOptionClick(option.key)}
              >
                <CustomImage
                  className={option.icon_css}
                  src={
                    selectedKey === option.key
                      ? option.selected
                      : option.unselected
                  }
                  alt={`${option.key}_icon`}
                />
                <span className={'pt-[1px]'}>{option.text}</span>
              </div>
            ))}
        </div>
      </div>
      <div className={styles.content}>{SelectedComponent}</div>
    </div>
  );
};

export default CustomSidebar;
