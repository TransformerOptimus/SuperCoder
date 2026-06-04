import type { FileDiff, FileDiffStatus, DiffLine, DiffHunk, CodeChangesArtifact } from '@/types/agent';
import type { AgentDiffResult } from '@/types/agentContract';

const HUNK_HEADER_RE = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)/;

/** Parse unified diff output into FileDiff[]. */
export function parseUnifiedDiff(rawDiff: string): FileDiff[] {
  const files: FileDiff[] = [];
  const fileChunks = rawDiff.split(/^diff --git /m).filter(Boolean);
  for (const chunk of fileChunks) {
    const lines = chunk.split('\n');
    const headerMatch = lines[0]?.match(/a\/(.+?) b\/(.+)/);
    if (!headerMatch) continue;
    const filePath = headerMatch[2];
    let additions = 0;
    let deletions = 0;

    const hunks: DiffHunk[] = [];
    let currentHunk: DiffHunk | null = null;
    let oldLine = 0;
    let newLine = 0;
    // Collect lines without @@ headers for fallback
    const fallbackLines: DiffLine[] = [];
    let hasHunkHeaders = false;

    for (const line of lines) {
      const hunkMatch = line.match(HUNK_HEADER_RE);
      if (hunkMatch) {
        hasHunkHeaders = true;
        const old_start = parseInt(hunkMatch[1], 10);
        const old_lines = parseInt(hunkMatch[2] ?? '1', 10);
        const new_start = parseInt(hunkMatch[3], 10);
        const new_lines = parseInt(hunkMatch[4] ?? '1', 10);
        currentHunk = {
          old_start, old_lines, new_start, new_lines,
          header: hunkMatch[5]?.trim() ?? '',
          lines: [],
        };
        hunks.push(currentHunk);
        oldLine = old_start;
        newLine = new_start;
        continue;
      }
      if (line.startsWith('+') && !line.startsWith('+++')) {
        additions++;
        const diffLine: DiffLine = { type: 'add', content: line.slice(1) };
        if (currentHunk) {
          diffLine.new_line_number = newLine++;
          currentHunk.lines.push(diffLine);
        } else {
          fallbackLines.push(diffLine);
        }
      } else if (line.startsWith('-') && !line.startsWith('---')) {
        deletions++;
        const diffLine: DiffLine = { type: 'delete', content: line.slice(1) };
        if (currentHunk) {
          diffLine.old_line_number = oldLine++;
          currentHunk.lines.push(diffLine);
        } else {
          fallbackLines.push(diffLine);
        }
      } else if (line.startsWith(' ')) {
        const diffLine: DiffLine = { type: 'context', content: line.slice(1) };
        if (currentHunk) {
          diffLine.old_line_number = oldLine++;
          diffLine.new_line_number = newLine++;
          currentHunk.lines.push(diffLine);
        } else {
          fallbackLines.push(diffLine);
        }
      }
    }

    const isNewFile = lines.some((l) => l.startsWith('--- /dev/null'));
    const isDeletedFile = lines.some((l) => l.startsWith('+++ /dev/null'));
    let status: FileDiffStatus = 'modified';
    if (isNewFile) status = 'added';
    else if (isDeletedFile) status = 'deleted';

    // Use parsed hunks if @@ headers were found, otherwise fall back to single synthetic hunk
    const fileHunks = hasHunkHeaders
      ? hunks
      : fallbackLines.length > 0
        ? [{ old_start: 1, old_lines: deletions, new_start: 1, new_lines: additions, header: '', lines: fallbackLines }]
        : [];

    files.push({ file_path: filePath, status, additions, deletions, hunks: fileHunks });
  }
  return files;
}

// Paths that should never appear in diffs (internal agent artifacts)
const DIFF_BLOCKLIST = ['.agent/', '.agent\\'];

/** Filter FileDiff[] to only include files in the allowed list (agent-modified files).
 *  Always excludes internal agent paths regardless of the allowed list. */
export function filterDiffByFiles(files: FileDiff[], allowedPaths: string[]): FileDiff[] {
  // Always strip internal agent artifacts
  const cleaned = files.filter((f) =>
    !DIFF_BLOCKLIST.some((blocked) => f.file_path.includes(blocked))
  );
  if (allowedPaths.length === 0) return cleaned;
  return cleaned.filter((f) => {
    const fp = f.file_path.replace(/^\//, '');
    return allowedPaths.some((allowed) => {
      const ap = allowed.replace(/^\//, '');
      return fp === ap || fp.endsWith(`/${ap}`) || ap.endsWith(`/${fp}`) || fp.endsWith(ap) || ap.endsWith(fp);
    });
  });
}

/** Build a CodeChangesArtifact from parsed files. Stats are calculated from the files array. */
export function buildCodeChangesArtifact(
  id: string,
  _diffResult: AgentDiffResult,
  files: FileDiff[],
  turnCount?: number,
): CodeChangesArtifact {
  // Recalculate stats from the (possibly filtered) files array
  const total_additions = files.reduce((s, f) => s + f.additions, 0);
  const total_deletions = files.reduce((s, f) => s + f.deletions, 0);
  return {
    id,
    type: 'code_changes',
    name: 'Code Changes',
    created_at: new Date().toISOString(),
    files,
    total_additions,
    total_deletions,
    files_changed: files.length,
    turnCount,
  };
}

/** Generate DiffLine[] from edit tool args (old_string → new_string). */
export function generateEditPreviewLines(
  oldString: string,
  newString: string,
): DiffLine[] {
  const oldLines = oldString.split('\n');
  const newLines = newString.split('\n');
  const result: DiffLine[] = [];
  for (const line of oldLines) {
    result.push({ type: 'delete', content: line });
  }
  for (const line of newLines) {
    result.push({ type: 'add', content: line });
  }
  return result;
}

/** Generate DiffLine[] for write tool — diffs against existing content if available. */
export function generateWritePreviewLines(
  existingContent: string | null,
  newContent: string,
): DiffLine[] {
  const newLines = newContent.split('\n');
  if (existingContent == null) {
    // New file — all additions
    return newLines.map((line) => ({ type: 'add' as const, content: line }));
  }
  // Existing file — show removals then additions
  const oldLines = existingContent.split('\n');
  const result: DiffLine[] = [];
  for (const line of oldLines) {
    result.push({ type: 'delete', content: line });
  }
  for (const line of newLines) {
    result.push({ type: 'add', content: line });
  }
  return result;
}

