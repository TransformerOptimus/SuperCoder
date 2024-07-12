'use client';
import React, { useMemo, useCallback } from 'react';
import Image from 'next/image';
import './style.css';
import { sidebarOptions } from '@/app/constants/SidebarConstants';
import { SidebarOption } from '../../../types/sidebarTypes';
import { usePathname, useRouter } from 'next/navigation';

const SideBar: React.FC = () => {
  const router = useRouter();
  const pathname = usePathname();

  const handleOptionClick = useCallback(
    (option: SidebarOption) => {
      router.push(option.route);
    },
    [router],
  );

  const handleSelectedOption = useCallback(
    (option: SidebarOption) => {
      return (
        pathname === option.route || pathname.startsWith(option.route + '/')
      );
    },
    [pathname],
  );

  const renderedOptions = useMemo(() => {
    return sidebarOptions.map((option, index) => (
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
            handleSelectedOption(option) ? option.selected : option.unselected
          }
          alt={`${option.id}_icon`}
          priority
        />
        <span className="proxima_nova text-[9px] font-normal">
          {option.text}
        </span>
      </div>
    ));
  }, [handleOptionClick, handleSelectedOption]);

  return (
    <aside id="sidebar_section" className="sidebar">
      {renderedOptions}
    </aside>
  );
};

export default React.memo(SideBar);
