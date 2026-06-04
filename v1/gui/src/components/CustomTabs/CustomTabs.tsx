import { useState, useEffect } from 'react';
import styles from './tabs.module.css';
import { CustomTabsNewProps } from '../../../types/customComponentTypes';
import CustomImage from '@/components/ImageComponents/CustomImage';

export default function CustomTabs({
  options,
  handle,
  children,
  position = 'start',
  containerCss = 'p-4 gap-4',
  tabCss = 'px-3 py-3',
  height,
}: CustomTabsNewProps) {
  const [selectedTab, setSelectedTab] = useState<string | null>(options[0].key);

  const handleTabClick = (key: string) => {
    if (handle) {
      handle(key);
    }
    setSelectedTab(key);
  };

  const selectedContent = options.find(
    (option) => option.key === selectedTab,
  )?.content;

  return (
      <div id={'custom_tabs'} className={`${styles.custom_tabs} ${containerCss}`}>
          <div
              className={`flex w-full ${
                  position === 'end' ? 'flex-row-reverse' : 'flex-row'
              } items-center justify-between`}
          >
              <div
                  className={`flex flex-row items-center gap-2 ${
                      position === 'end' && 'mx-2'
                  }`}
              >
          {options &&
            options.length > 0 &&
            options.map((option, index) => (
              <div
                key={index}
                className={`${styles.tab_item} ${
                  selectedTab === option.key ? styles.tab_item_selected : ''
                } ${tabCss}`}
                onClick={() => handleTabClick(option.key)}
              >
                {option.icon !== null && (
                  <CustomImage
                    src={
                      option.icon
                        ? option.icon
                        : selectedTab === option.key
                        ? option.selected
                        : option.unselected
                    }
                    alt={option.key + '_icon'}
                    className={styles.tab_icon}
                    priority={true}
                  />
                )}
                  {option.text && (
                      <span className={styles.tab_item_text}>{option.text}</span>
                  )}
                {option.count && (
                  <span className={styles.tab_item_count}>{option.count}</span>
                )}
              </div>
            ))}
        </div>

        {children}
      </div>

      <div
        className={`${styles.tab_content} ${
          selectedContent ? styles.tab_content_active : ''
        }`}
        style={{ height: `${height}` }}
      >
        {selectedContent}
      </div>
    </div>
  );
}
