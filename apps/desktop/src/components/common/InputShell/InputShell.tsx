import { useLayoutEffect, useRef } from "react";
import { Button } from "antd";
import { SendHorizontal, CircleStop } from "lucide-react";
import type { InputShellProps } from "./types";

export default function InputShell({
  value,
  onChange,
  onKeyDown,
  onPaste,
  onBlur,
  placeholder = "Write a message",
  textareaRef,
  onSend,
  sendDisabled = false,
  isStop = false,
  maxRows = 10,
  toolbarLeft,
  toolbarRight,
  aboveContent,
  innerContent,
  belowToolbar,
}: InputShellProps) {
  const internalRef = useRef<HTMLTextAreaElement>(null);
  const resolvedRef = (textareaRef ?? internalRef) as React.RefObject<HTMLTextAreaElement>;

  useLayoutEffect(() => {
    const el = resolvedRef.current;
    if (!el) return;
    el.style.height = "auto";
    if (value) {
      const computed = getComputedStyle(el);
      const lineHeight = parseFloat(computed.lineHeight) || 20;
      const paddingTop = parseFloat(computed.paddingTop) || 12;
      const paddingBottom = parseFloat(computed.paddingBottom) || 12;
      const maxHeight = lineHeight * maxRows + paddingTop + paddingBottom;
      el.style.height = Math.min(el.scrollHeight, maxHeight) + "px";
    }
  }, [value, maxRows]);

  return (
    <div className="px-5 pt-0 pb-3">
      {aboveContent}

      <div className="relative bg-gray-100 dark:bg-dark-surface border border-gray-200 dark:border-dark-border rounded-xl">
        {innerContent}

        <textarea
          ref={resolvedRef}
          value={value}
          onChange={onChange}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          onBlur={onBlur}
          placeholder={placeholder}
          rows={1}
          className="w-full px-4 py-3 text-sm outline-none bg-transparent text-gray-900 dark:text-white placeholder:text-gray-400 resize-none min-h-[52px] overflow-y-auto"
        />

        <div className="flex items-center gap-2 px-3 pb-2">
          <div className="flex items-center gap-1 min-w-0 flex-1 overflow-x-auto no-scrollbar">
            {toolbarLeft}
          </div>
          {toolbarRight}
          <div className="shrink-0">
            <Button
              className="primary_small"
              type={isStop ? "default" : "primary"}
              size="small"
              onClick={onSend}
              disabled={!isStop && sendDisabled}
            >
              <span className="flex items-center gap-1.5">
                {isStop ? "Stop" : "Send"}
                {isStop ? <CircleStop size={14} /> : <SendHorizontal size={14} />}
              </span>
            </Button>
          </div>
        </div>

        {belowToolbar && (
          <div className="border-t border-gray-200 dark:border-dark-border mx-3 pt-2 pb-2">
            {belowToolbar}
          </div>
        )}
      </div>
    </div>
  );
}
