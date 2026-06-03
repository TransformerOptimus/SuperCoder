/** Width tier of the composer, driven by a ResizeObserver on the shell. */
export type WidthTier = 'narrow' | 'medium' | 'wide';

export interface InputShellProps {
  value: string;
  onChange: (e: React.ChangeEvent<HTMLTextAreaElement>) => void;
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  onPaste?: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void;
  onBlur?: () => void;
  placeholder?: string;
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>;
  onSend: () => void;
  sendDisabled?: boolean;
  isStop?: boolean;
  /** Maximum number of visible rows before scrolling (default: 10) */
  maxRows?: number;
  toolbarLeft?: React.ReactNode;
  /** Content rendered next to the send button (right side of toolbar) */
  toolbarRight?: React.ReactNode;
  aboveContent?: React.ReactNode;
  innerContent?: React.ReactNode;
  /** Content rendered below the toolbar row, inside the bordered box, with a top divider */
  belowToolbar?: React.ReactNode;
  /** Notified when the composer's width tier changes (for responsive collapse). */
  onWidthChange?: (tier: WidthTier) => void;
}
