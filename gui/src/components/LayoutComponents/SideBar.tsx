'use client';
import React, { useMemo, useCallback, useState, useEffect } from 'react';
import './style.css';
import {
  designSidebarOptions,
  backendSidebarOptions,
} from '@/app/constants/SidebarConstants';
import { SidebarOption } from '../../../types/sidebarTypes';
import { usePathname, useRouter } from 'next/navigation';
import { getProjectTypeFromFramework } from '@/app/utils';
import { projectTypes } from '@/app/constants/ProjectConstants';
import CustomImage from '@/components/ImageComponents/CustomImage';

const SideBar: React.FC = () => {
  const router = useRouter();
  const pathname = usePathname();
  const [projectFramework, setProjectFramework] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window !== undefined) {
      setProjectFramework(localStorage.getItem('projectFramework'));
    }
  }, []);

  const handleOptionClick = useCallback(
      (option: SidebarOption) => {
        router.push(option.route);
      },
      [router],
  );

  const handleSelectedOption = useCallback(
      (option: SidebarOption) => {
        return pathname.includes(option.id);
      },
      [pathname],
  );

  const filteredOptions = useMemo(() => {
    const projectType = getProjectTypeFromFramework(projectFramework);
    switch (projectType) {
      case projectTypes.DESIGN:
        return designSidebarOptions;
      case projectTypes.BACKEND:
        return backendSidebarOptions;
      default:
        return null;
    }
  }, [projectFramework]);

  const renderedOptions = useMemo(() => {
    return (
        filteredOptions &&
        filteredOptions.map((option, index) => {
          return (
              <div
                  key={index}
                  id={`${option.id}_${index}_option`}
                  className={`${
                      handleSelectedOption(option)
                          ? 'sidebar_item_selected'
                          : 'sidebar_item'
                  } flex flex-col items-center justify-center gap-1 text-white`}
                  onClick={() => handleOptionClick(option)}
              >
                <CustomImage
                    className={'size-6'}
                    src={
                      handleSelectedOption(option)
                          ? option.selected
                          : option.unselected
                    }
                    alt={`${option.id}_icon`}
                    priority
                />
                <span className="proxima_nova text-[9px] font-normal">
              {option.text}
            </span>
              </div>
          );
        })
    );
  }, [handleOptionClick, handleSelectedOption, filteredOptions]);

  return (
      <aside id="sidebar_section" className="sidebar">
        {renderedOptions}
      </aside>
  );
};

export default React.memo(SideBar);
