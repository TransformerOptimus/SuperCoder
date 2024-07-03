import styles from './container.module.css';
import { ReactNode, useRef, useState } from 'react';
import { CustomTabsNewProps } from '../../../types/customComponentTypes';
import CustomImage from '@/components/ImageComponents/CustomImage';

interface CustomContainersProps {
  id: string;
  height?: string;
  alignment: string;
  children?: ReactNode;
  header?: string;
  bgColor?: boolean;
  type?: 'normal' | 'tabs';
  tabsProps?: CustomTabsNewProps;
}

export default function CustomContainers({
  id,
  height,
  alignment,
  children,
  header,
  bgColor = true,
  type = 'normal',
  tabsProps,
}: CustomContainersProps) {
  const tabRef = useRef(null);
  const [selectedTab, setSelectedTab] = useState(tabsProps?.options[0].key);

  return (
    <div
      id={`${id}_custom_container`}
      className={`${styles.container_section} ${!header && alignment}`}
      style={{ height: height || 'auto', background: `${!bgColor && 'none'}` }}
    >
      {header ? (
        type === 'normal' ? (
          <div className={'flex flex-col'}>
            <span className={styles.container_header}>{header}</span>
            <div>{children}</div>
          </div>
        ) : (
          <div className={'flex flex-col'}>
            <div className={styles.container_tab_header}>
              {tabsProps?.options.map((tab) => (
                <div
                  key={tab.key}
                  className={`${styles.tab_item} ${
                    selectedTab === tab.key ? styles.tab_item_selected : ''
                  }`}
                  onClick={() => setSelectedTab(tab.key)}
                >
                  {tab.icon && (
                    <CustomImage
                      className={'size-4'}
                      src={tab.icon}
                      alt={'tab_icon'}
                    />
                  )}
                  {tab.text}
                </div>
              ))}
            </div>
            <div>
              {
                tabsProps?.options.find((tab) => tab.key === selectedTab)
                  ?.content
              }
            </div>
          </div>
        )
      ) : (
        children
      )}
    </div>
  );
}
