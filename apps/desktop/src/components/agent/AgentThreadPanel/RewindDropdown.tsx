import { Pencil } from "lucide-react";

export interface RewindAgentProps {
  onEditAndResend?: () => void;
  isCodingSession: boolean;
  hasCheckpoints: boolean;
  isStreaming: boolean;
  isRewinding: boolean;
}

interface RewindDropdownProps extends RewindAgentProps {
  onPopupChange?: (open: boolean) => void;
}

export default function RewindDropdown({
  onEditAndResend,
  isStreaming,
  isRewinding,
}: RewindDropdownProps) {
  const disabled = isStreaming || isRewinding;

  if (!onEditAndResend) return null;

  return (
    <button
      onClick={onEditAndResend}
      className="p-1.5 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-dark-hover rounded"
      title={disabled ? "Stop the agent before editing" : "Edit & resend"}
      disabled={disabled}
    >
      <Pencil className="w-3.5 h-3.5" />
    </button>
  );
}
