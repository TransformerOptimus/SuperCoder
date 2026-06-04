import React, {
  useState,
  useRef,
  useEffect,
  ReactNode,
  ReactElement,
  FC,
  Children,
  isValidElement,
  cloneElement,
} from 'react';
import styles from './select.module.css';
import imagePath from '@/app/imagePath';

interface CustomSelectProps {
  children: ReactNode;
  multiple?: boolean;
  showTick?: boolean;
  icon?: ReactElement;
  selectedValues: string | string[];
  setSelectedValues: (
    values:
      | string
      | string[]
      | ((prev: string | string[]) => string | string[]),
  ) => void;
  disabled?: boolean;
}

interface ItemProps {
  children: ReactNode;
  value: string;
  selectedValues?: string | string[];
  handleSelect?: (value: string) => void;
  showTick?: boolean;
  disabled?: boolean;
}

interface SectionProps {
  children: ReactNode;
  title?: string;
  selectedValues?: string | string[];
  handleSelect?: (value: string) => void;
  showTick?: boolean;
  disabled?: boolean;
}

const CustomSelect: FC<CustomSelectProps> & {
  Item: FC<ItemProps>;
  Section: FC<SectionProps>;
} = ({
  children,
  multiple = false,
  showTick = false,
  icon,
  selectedValues,
  setSelectedValues,
  disabled = false,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const selectRef = useRef<HTMLDivElement>(null);

  const handleSelect = (value: string) => {
    setSelectedValues((prev: string | string[]) => {
      if (multiple) {
        if (Array.isArray(prev)) {
          return prev.includes(value)
            ? prev.filter((v) => v !== value)
            : [...prev, value];
        } else {
          return [value];
        }
      } else {
        return value;
      }
    });
    setIsOpen(false);
  };

  const handleClickOutside = (event: MouseEvent) => {
    if (
      selectRef.current &&
      !selectRef.current.contains(event.target as Node)
    ) {
      setIsOpen(false);
    }
  };

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  const displayValue = multiple
    ? Array.isArray(selectedValues)
      ? selectedValues.join(', ')
      : ''
    : selectedValues;

  return (
    <div
      className={`${styles.custom_select} ${disabled ? styles.disabled : ''}`}
      ref={selectRef}
    >
      <div
        className={styles.selected}
        onClick={() => !disabled && setIsOpen(!isOpen)}
      >
        <div className="flex items-center">
          {icon && <span className={styles.icon}>{icon}</span>}
          {displayValue}
        </div>
        <img
          src={
            isOpen ? imagePath.bottomArrowGrey : imagePath.rightArrowThinGrey
          }
          alt="arrow"
          className={styles.arrow_icon}
        />
      </div>
      {isOpen && (
        <div
          className={`${styles.dropdown} ${
            isOpen ? styles.expand : styles.shrink
          }`}
        >
          {Children.map(children, (child) => {
            if (isValidElement(child)) {
              return cloneElement(child, {
                selectedValues,
                handleSelect,
                showTick,
                disabled,
              } as Partial<ItemProps>);
            }
            return child;
          })}
        </div>
      )}
    </div>
  );
};

const Item: FC<ItemProps> = ({
  children,
  value,
  selectedValues = [],
  handleSelect,
  showTick,
  disabled = false,
}) => {
  const isSelected = Array.isArray(selectedValues)
    ? selectedValues.includes(value)
    : selectedValues === value;

  return (
    <div
      className={`${styles.item} ${isSelected ? styles.selected_item : ''} ${
        disabled ? styles.disabled_item : ''
      }`}
      onClick={() => !disabled && handleSelect && handleSelect(value)}
    >
      {showTick && isSelected && <span className={styles.tick}>✔</span>}
      {children}
    </div>
  );
};

const Section: FC<SectionProps> = ({
  children,
  title,
  selectedValues,
  handleSelect,
  showTick,
  disabled = false,
}) => {
  return (
    <div className={styles.section}>
      {title && <div className={styles.section_title}>{title}</div>}
      {Children.map(children, (child) => {
        if (isValidElement(child)) {
          return cloneElement(child, {
            selectedValues,
            handleSelect,
            showTick,
            disabled,
          } as Partial<ItemProps>);
        }
        return child;
      })}
    </div>
  );
};

CustomSelect.Item = Item;
CustomSelect.Section = Section;

export default CustomSelect;
