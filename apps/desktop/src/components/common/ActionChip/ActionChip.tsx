import { forwardRef } from "react";
import { Trash2 } from "lucide-react";
import type { ActionChipProps } from "./types";

const ActionChip = forwardRef<HTMLButtonElement, ActionChipProps>(
  ({ label, prefix, suffix, onClose, className, ...rest }, ref) => {
    return (
      <button
        ref={ref}
        className={`action_chip${className ? ` ${className}` : ""}`}
        {...rest}
      >
        {prefix}
        <span className="text-xs text_color whitespace-nowrap">{label}</span>
        {suffix}
        {onClose && (
          <>
            <div className="border_8 h-3 w-0 mx-0.5" />
            <Trash2
              className="w-3.5 h-3.5 opacity-60 hover:opacity-100 cursor-pointer text-gray-500"
              onClick={(e) => {
                e.stopPropagation();
                onClose();
              }}
            />
          </>
        )}
      </button>
    );
  },
);

ActionChip.displayName = "ActionChip";

export default ActionChip;
