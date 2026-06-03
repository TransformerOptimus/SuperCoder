import React, { useRef, useCallback } from "react";
import { Input, Dropdown } from "antd";
import { X } from "lucide-react";
import type { TextAreaRef } from "antd/es/input/TextArea";
import type { MenuProps } from "antd";

const { TextArea } = Input;

interface RewindEditorProps {
  text: string;
  /** Image data-URLs attached to the message being edited. */
  images?: string[];
  /** Remove the image at the given index from the resend. */
  onRemoveImage?: (idx: number) => void;
  onChange: (text: string) => void;
  onCancel: () => void;
  onRewind: (restoreCode: boolean) => void;
  isCodingSession: boolean;
  isRewinding: boolean;
}

export default function RewindEditor({
  text,
  images = [],
  onRemoveImage,
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

  // Resend is allowed with text OR at least one kept image.
  const canResend = !!text.trim() || images.length > 0;

  return (
    <div className="flex flex-col gap-2 p-3 rounded-lg" style={{ background: 'var(--white-opacity-4)' }}>
      {images.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {images.map((src, i) => (
            <div key={i} className="relative group">
              <img
                src={src}
                alt={`attachment ${i + 1}`}
                className="h-16 w-16 rounded-lg border border-gray-200 dark:border-dark-border object-cover"
              />
              {onRemoveImage && !isRewinding && (
                <button
                  type="button"
                  aria-label="Remove image"
                  onClick={() => onRemoveImage(i)}
                  className="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-gray-800 text-white shadow hover:bg-black"
                >
                  <X size={12} />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
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
            disabled={isRewinding || !canResend}
          >
            <button
              className="primary_small"
              disabled={isRewinding || !canResend}
            >
              {isRewinding ? "Resending..." : "Resend \u25BE"}
            </button>
          </Dropdown>
        ) : (
          <button
            className="primary_small"
            onClick={() => onRewind(false)}
            disabled={isRewinding || !canResend}
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
