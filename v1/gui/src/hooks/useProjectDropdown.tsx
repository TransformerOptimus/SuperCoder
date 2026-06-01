'use client';
import { usePathname } from 'next/navigation';

const ProjectHideDropdown = ['projects'];

const useProjectDropdown = () => {
  const pathname = usePathname();

  return !ProjectHideDropdown.some((item) => pathname.includes(item));
};

export default useProjectDropdown;
