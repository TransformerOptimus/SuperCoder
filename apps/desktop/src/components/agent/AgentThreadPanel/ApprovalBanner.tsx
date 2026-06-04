import { useState, useEffect } from 'react';
import { Code } from 'lucide-react';
import type { PendingApproval } from '@/types/agentContract';
import type { DiffLine } from '@/types/agent';
import { generateEditPreviewLines, generateWritePreviewLines } from '@/utils/diffUtils';
import { agentTauriService } from '@/services/agentTauriService';

const TOOL_LABELS: Record<string, string> = {
  bash: 'Run Command',
  git: 'Git Operation',
  write: 'Write File',
  edit: 'Edit File',
  grep: 'Search Files',
  glob: 'Find Files',
  read: 'Read File',
  create_pr: 'Create Pull Request',
  todo_write: 'Update Todos',
  todo_read: 'Read Todos',
};

const COLLAPSED_LINES = 20;

/** Clean up raw JSON strings into human-readable text */
function cleanDescription(toolName: string, text: string): string {
  const trimmed = text.trim();
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    if (toolName === 'todo_write' || trimmed.includes('"todos"')) {
      const match = trimmed.match(/"content"\s*:\s*"([^"]+)"/);
      if (match) return `Updating todos: ${match[1]}`;
      return 'Updating agent task list';
    }
    if (toolName === 'todo_read') return 'Reading agent task list';
    // Generic JSON — don't show raw
    return '';
  }
  return text;
}

/** Extract diff preview lines from tool args */
function useDiffPreview(approval: PendingApproval): { filePath: string | null; allLines: DiffLine[] } {
  const [writeExisting, setWriteExisting] = useState<string | null | undefined>(undefined);
  const { toolName, args } = approval;

  const isEdit = toolName === 'edit' && args?.oldString != null && args?.newString != null;
  const isWrite = toolName === 'write' && args?.content != null;
  const filePath = (args?.filePath as string) ?? null;

  // For write tool, async-load current file content
  useEffect(() => {
    if (!isWrite || !filePath) return;
    let cancelled = false;
    agentTauriService.readFileText(filePath).then((content) => {
      if (!cancelled) setWriteExisting(content);
    }).catch(() => {
      if (!cancelled) setWriteExisting(null);
    });
    return () => { cancelled = true; };
  }, [isWrite, filePath]);

  if (isEdit) {
    return { filePath, allLines: generateEditPreviewLines(args.oldString as string, args.newString as string) };
  }

  if (isWrite && writeExisting !== undefined) {
    return { filePath, allLines: generateWritePreviewLines(writeExisting, args.content as string) };
  }

  return { filePath: null, allLines: [] };
}

interface ApprovalBannerProps {
  approval: PendingApproval;
  onApprove: () => void;
  onDeny: () => void;
  disabled?: boolean;
}

export default function ApprovalBanner({ approval, onApprove, onDeny, disabled }: ApprovalBannerProps) {
  const [expanded, setExpanded] = useState(false);
  const toolLabel = TOOL_LABELS[approval.toolName] ?? approval.toolName;
  const description = approval.description ? cleanDescription(approval.toolName, approval.description) : '';
  const rawCommand = approval.rawCommand ? cleanDescription(approval.toolName, approval.rawCommand) : '';
  const { filePath, allLines } = useDiffPreview(approval);
  const hasDiffPreview = allLines.length > 0;
  const isCollapsible = allLines.length > COLLAPSED_LINES;
  const visibleLines = expanded ? allLines : allLines.slice(0, COLLAPSED_LINES);
  const hiddenCount = allLines.length - visibleLines.length;

  const summaryText = description || rawCommand || '';

  return (
    <div className="mx-1 my-1.5">
      <div className="rounded-lg border border-[var(--border-color-8)] bg-[var(--bg-secondary)] overflow-hidden">
        {/* Header row: tool label + description + actions */}
        <div className="flex items-start gap-3 px-3 py-2.5">
          <div className="flex-1 min-w-0 text-xs text-[var(--text-primary)] leading-relaxed pt-0.5">
            <span className="font-medium">{toolLabel}</span>
            {summaryText && (
              <span className="text-[var(--text-secondary)]"> · {summaryText}</span>
            )}
          </div>
          <div className="flex gap-1.5 shrink-0">
            <button className="secondary_small" onClick={onDeny} disabled={disabled}>
              Deny
            </button>
            <button className="primary_small" onClick={onApprove} disabled={disabled}>
              Allow
            </button>
          </div>
        </div>

        {/* Diff preview (only for edit/write tools) */}
        {hasDiffPreview && (
          <div className="border-t border-[var(--border-color-8)]">
            {filePath && (
              <div className="flex items-center gap-1.5 px-3 py-1 bg-[var(--bg-secondary)] border-b border-[var(--border-color-8)] min-w-0">
                <Code className="w-3 h-3 text-[var(--text-secondary)] shrink-0" />
                <span className="text-[11px] font-mono text-[var(--text-secondary)] truncate min-w-0" title={filePath} dir="rtl">{filePath}</span>
              </div>
            )}
            <div className={expanded ? "max-h-[60vh] overflow-auto font-mono text-[11px] leading-[1.6]" : "max-h-[200px] overflow-auto font-mono text-[11px] leading-[1.6]"}>
              {visibleLines.map((line, i) => (
                <div
                  key={i}
                  className={
                    line.type === 'add'
                      ? 'bg-[var(--diff-add-bg)] text-[var(--diff-add-color)] px-3 whitespace-pre flex w-fit min-w-full'
                      : line.type === 'delete'
                        ? 'bg-[var(--diff-del-bg)] text-[var(--diff-del-color)] px-3 whitespace-pre flex w-fit min-w-full'
                        : 'text-[var(--text-secondary)] px-3 whitespace-pre flex w-fit min-w-full'
                  }
                >
                  <span className="inline-block w-4 shrink-0 select-none">
                    {line.type === 'add' ? '+' : line.type === 'delete' ? '-' : ' '}
                  </span>
                  <span>{line.content}</span>
                </div>
              ))}
              {isCollapsible && (
                <button
                  className="w-full px-3 py-1.5 text-[11px] text-[var(--text-secondary)] hover:text-[var(--text-primary)] bg-[var(--bg-secondary)] border-t border-[var(--border-color-8)] cursor-pointer text-left"
                  onClick={() => setExpanded(!expanded)}
                >
                  {expanded ? 'Show less' : `Show ${hiddenCount} more line${hiddenCount !== 1 ? 's' : ''}`}
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
