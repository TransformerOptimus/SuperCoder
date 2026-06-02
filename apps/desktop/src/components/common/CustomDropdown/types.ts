export interface CustomDropdownItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
  suffix?: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  dividerBefore?: boolean;
  children?: CustomDropdownItem[];
  childSearchable?: boolean;
}

export interface CustomDropdownProps {
  items: CustomDropdownItem[];
  trigger: React.ReactNode;
  placement?: "topLeft" | "topRight" | "bottomLeft" | "bottomRight" | "top" | "bottom" | "left" | "right";
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  minWidth?: number;
  searchable?: boolean;
  closeOnSelect?: boolean;
}

export interface DropdownPanelProps {
  items: CustomDropdownItem[];
  searchable: boolean;
  minWidth: number;
  onItemClick: (item: CustomDropdownItem) => void;
}

export interface DropdownItemRowProps {
  item: CustomDropdownItem;
  searchable: boolean;
  minWidth: number;
  onItemClick: (item: CustomDropdownItem) => void;
}
