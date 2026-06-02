import React, { useRef, useCallback } from "react";
import { Input, Dropdown } from "antd";
import type { TextAreaRef } from "antd/es/input/TextArea";
import type { MenuProps } from "antd";

const { TextArea } = Input;

interface RewindEditorProps {
  text: string;
  onChange: (text: string) => void;
  onCancel: () => void;
  onRewind: (restoreCode: boolean) => void;
  isCodingSession: boolean;
  isRewinding: boolean;
}

export default function RewindEditor({
  text,
  onChange,
  onCancel,
  onRewind,
  isCodingSession,
  isRewinding,
}: RewindEditorProps) {
  const focusedRef = useRef(false);
  const handleRef = useCallback((ref: TextAreaRef | null) => {
    if (ref && !focusedRef.current) {
      focusedRef.current = true;
      ref.focus({ cursor: "end" });
    }
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      onCancel();
    }
  };

  return (
    <div className="flex flex-col gap-2 p-3 rounded-lg" style={{ background: 'var(--white-opacity-4)' }}>
      <TextArea
        ref={handleRef}
        value={text}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={handleKeyDown}
        disabled={isRewinding}
        autoSize={{ minRows: 1, maxRows: 6 }}
        variant="borderless"
        placeholder="Edit your message..."
      />
      <div className="flex gap-2 justify-end">
        {isCodingSession ? (
          <Dropdown
            menu={{
              items: [
                { key: "context", label: "Resend (context only)" },
                { key: "code", label: "Resend (context + code)" },
              ],
              onClick: ({ key }: { key: string }) =>
                onRewind(key === "code"),
            } as MenuProps}
            trigger={["click"]}
            disabled={isRewinding || !text.trim()}
          >
            <button
              className="primary_small"
              disabled={isRewinding || !text.trim()}
            >
              {isRewinding ? "Resending..." : "Resend \u25BE"}
            </button>
          </Dropdown>
        ) : (
          <button
            className="primary_small"
            onClick={() => onRewind(false)}
            disabled={isRewinding || !text.trim()}
          >
            {isRewinding ? "Resending..." : "Resend"}
          </button>
        )}
        <button
          className="secondary_small"
          onClick={onCancel}
          disabled={isRewinding}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
