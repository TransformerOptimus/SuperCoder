import React, { useState, useRef, useEffect, ReactNode } from 'react';
import styles from './dropdown.module.css';

interface CustomDropdownProps {
  trigger: ReactNode;
  onHover?: boolean;
  maxHeight?: string;
  gap?: string;
  position?: 'start' | 'end' | 'center';
  children: ReactNode;
  key?: string;
}

interface CustomDropdownItemProps {
  key: string;
  onClick: () => void;
  children: ReactNode;
}

interface CustomDropdownSectionProps {
  title?: string;
  showDivider?: boolean;
  children: ReactNode;
}

const CustomDropdown: React.FC<CustomDropdownProps> & {
  Item: React.FC<CustomDropdownItemProps>;
  Section: React.FC<CustomDropdownSectionProps>;
} = ({ trigger, onHover = false, maxHeight, gap, position, children }) => {
  const [isOpen, setIsOpen] = useState(false);
  const [isClosing, setIsClosing] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const handleClickOutside = (event: MouseEvent) => {
    if (
      dropdownRef.current &&
      !dropdownRef.current.contains(event.target as Node)
    ) {
      closeDropdown();
    }
  };

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  const handleTrigger = () => {
    if (isOpen) {
      closeDropdown();
    } else {
      setIsOpen(true);
    }
  };

  const handleMouseEnter = () => {
    if (onHover) {
      setIsOpen(true);
    }
  };

  const handleMouseLeave = () => {
    if (onHover) {
      closeDropdown();
    }
  };

  const closeDropdown = () => {
    setIsClosing(true);
    setTimeout(() => {
      setIsOpen(false);
      setIsClosing(false);
    }, 100);
  };

  const dropdownPosition = {
    start: { left: 0 },
    end: { right: 0 },
    center: { left: '50%', transform: 'translateX(-50%)' },
  };

  return (
    <div
      className={styles.dropdown_container}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      <div onClick={handleTrigger}>{trigger}</div>
      {isOpen && (
        <div
          ref={dropdownRef}
          className={`${styles.dropdown} ${isClosing ? styles.fade_out : ''}`}
          style={{ maxHeight, marginTop: gap, ...dropdownPosition[position] }}
        >
          {children}
        </div>
      )}
    </div>
  );
};

const CustomDropdownItem: React.FC<CustomDropdownItemProps> = ({
  key,
  onClick,
  children,
}) => (
  <div key={key} className={styles.dropdown_item} onClick={onClick}>
    {children}
  </div>
);

const CustomDropdownSection: React.FC<CustomDropdownSectionProps> = ({
  title,
  showDivider,
  children,
}) => (
  <div className={styles.dropdown_section}>
    {title && <div className={styles.dropdown_section_title}>{title}</div>}
    {children}
    {showDivider && <div className={styles.dropdown_section_divider} />}
  </div>
);

CustomDropdown.Item = CustomDropdownItem;
CustomDropdown.Section = CustomDropdownSection;

export default CustomDropdown;
