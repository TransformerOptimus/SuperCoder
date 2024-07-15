import React, { ReactNode } from 'react';

export interface TabItem {
  id: string;
  label: string;
  content: React.ReactNode;
  selected: string;
  unselected: string;
}

export interface CustomTabsProps {
  tabs: TabItem[];
  max_height: string;
}

export interface CustomDrawerProps {
  open: boolean;
  onClose: () => void;
  direction?: 'bottom' | 'top' | 'left' | 'right';
  top?: string;
  width?: string;
  children: ReactNode;
  contentCSS?: string;
}

export interface OptionItems {
  key: string;
  text: string;
  selected?: string;
  unselected?: string;
  count?: number;
  content: ReactNode;
  icon?: string | null;
}

export interface CustomTabsNewProps {
  children?: ReactNode;
  handle?: (key: string) => void;
  options: OptionItems[];
  position?: 'start' | 'end';
  height?: string | number;
  containerCss?: string;
  tabCss?: string;
}

export interface ReBuildModalProps {
  openRebuildModal: boolean;
  setOpenRebuildModal: React.Dispatch<React.SetStateAction<boolean>>;
  rebuildComment: string;
  setRebuildComment: React.Dispatch<React.SetStateAction<string>>;
  handleRebuildStory: () => void;
}

export interface CustomTagProps {
  text: string;
  icon: string;
  iconClass: string;
  iconBack?: string;
  iconBackClass?: string;
  color?: string;
  children?: ReactNode;
  className?: string;
}

export interface CustomDropdownItem {
  key: string;
  text: string;
  icon: React.ReactNode;
  action: () => void;
}

export type DropdownFormat = 'detailed' | 'simple' | 'object';

export interface CustomDropdownProps {
  items: CustomDropdownItem[] | string[] | Record<string, any>[];
  trigger: React.ReactNode;
  onHover?: boolean;
  maxHeight?: string;
  gap?: string;
  position?: 'start' | 'end' | 'center';
  format: DropdownFormat;
  onItemSelect?: (item: any, key?: string) => void;
  keyField?: string;
  valueField?: string;
}

export interface CustomInputProps {
  value: string | number | null;
  change?: (key: string | number) => void;
  placeholder?: string;
  type?: string;
  icon: string;
  iconCSS?: string;
  size: string;
  alt: string;
}

// CUSTOM SIDEBAR COMPONENT TYPES
export interface SidebarOptions {
  key: string;
  text: string;
  selected: string;
  unselected: string;
  icon_css: string;
  component: React.ReactNode;
}

export interface CustomSidebarProps {
  id: string;
  title?: string;
  options?: SidebarOptions[];
  onOptionSelect?: (key: string) => void;
}

export interface FrontendCodeSectionProps {
  height: string;
  allowCopy?: boolean;
  backgroundColor?: boolean;
  codeFiles: CodeFile[];
}

export interface CodeFile {
  file_name: string;
  code: string;
}
