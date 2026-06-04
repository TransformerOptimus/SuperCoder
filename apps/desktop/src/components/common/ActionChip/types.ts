export interface ActionChipProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'prefix'> {
  label: string;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  onClose?: () => void;
}
