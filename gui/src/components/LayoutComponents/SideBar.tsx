'use client';
import React, { useMemo, useCallback, useState, useEffect } from 'react';
import Image from 'next/image';
import './style.css';
import { sidebarOptions } from '@/app/constants/SidebarConstants';
import { SidebarOption } from '../../../types/sidebarTypes';
import { usePathname, useRouter } from 'next/navigation';
import { getProjectTypeFromFramework } from '@/app/utils';
import { projectTypes } from '@/app/constants/ProjectConstants';

const SideBar: React.FC = () => {
  const router = useRouter();
  const pathname = usePathname();
  const [projectFramework, setProjectFramework] = useState<string | null>(null);

  useEffect(() => {
    let storedProjectType = null;
    if (typeof window !== undefined) {
      storedProjectType = localStorage.getItem('projectFramework');
      setProjectFramework(storedProjectType);
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
    if (projectType === projectTypes.DESIGN) {
      return sidebarOptions.filter((option) =>
        ['design', 'workbench'].includes(option.id),
      );
    } else if (projectType === projectTypes.BACKEND) {
      return sidebarOptions.filter((option) =>
        ['board', 'workbench', 'code', 'pull_request'].includes(option.id),
      );
    }
    return null;
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
            <Image
              width={24}
              height={24}
              sizes={'100vw'}
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
  }, [handleOptionClick, handleSelectedOption, projectFramework]);

  return (
    <aside id="sidebar_section" className="sidebar">
      {renderedOptions}
    </aside>
  );
};

export default React.memo(SideBar);
