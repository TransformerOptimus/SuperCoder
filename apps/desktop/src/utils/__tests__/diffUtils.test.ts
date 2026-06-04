import { describe, it, expect } from 'vitest';
import { parseUnifiedDiff, buildCodeChangesArtifact } from '../diffUtils';
import type { AgentDiffResult } from '@/types/agentContract';

const SINGLE_FILE_DIFF = `diff --git a/src/main.ts b/src/main.ts
--- a/src/main.ts
+++ b/src/main.ts
@@ -1,3 +1,4 @@
 import { app } from './app';
+import { logger } from './logger';

 app.start();`;

const MULTI_FILE_DIFF = `diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -1,2 +1,3 @@
 const a = 1;
+const b = 2;
 export { a };
diff --git a/src/b.ts b/src/b.ts
--- a/src/b.ts
+++ b/src/b.ts
@@ -1,3 +1,2 @@
 const x = 'hello';
-const y = 'world';
 export { x };`;

const ADDED_FILE_DIFF = `diff --git a/src/new.ts b/src/new.ts
--- /dev/null
+++ b/src/new.ts
@@ -0,0 +1,2 @@
+export const foo = 1;
+export const bar = 2;`;

const DELETED_FILE_DIFF = `diff --git a/src/old.ts b/src/old.ts
--- a/src/old.ts
+++ /dev/null
@@ -1,2 +0,0 @@
-export const foo = 1;
-export const bar = 2;`;

describe('parseUnifiedDiff', () => {
  it('parses a single-file diff', () => {
    const files = parseUnifiedDiff(SINGLE_FILE_DIFF);
    expect(files).toHaveLength(1);
    expect(files[0].file_path).toBe('src/main.ts');
    expect(files[0].additions).toBe(1);
    expect(files[0].deletions).toBe(0);
    expect(files[0].status).toBe('modified'); // existing file with additions only is still 'modified'
    expect(files[0].hunks).toHaveLength(1);
    expect(files[0].hunks[0].lines.some((l) => l.type === 'add' && l.content.includes('logger'))).toBe(true);
  });

  it('parses multi-file diff', () => {
    const files = parseUnifiedDiff(MULTI_FILE_DIFF);
    expect(files).toHaveLength(2);
    expect(files[0].file_path).toBe('src/a.ts');
    expect(files[0].additions).toBe(1);
    expect(files[0].deletions).toBe(0);
    expect(files[1].file_path).toBe('src/b.ts');
    expect(files[1].additions).toBe(0);
    expect(files[1].deletions).toBe(1);
  });

  it('detects added file status', () => {
    const files = parseUnifiedDiff(ADDED_FILE_DIFF);
    expect(files).toHaveLength(1);
    expect(files[0].status).toBe('added');
    expect(files[0].additions).toBe(2);
    expect(files[0].deletions).toBe(0);
  });

  it('detects deleted file status', () => {
    const files = parseUnifiedDiff(DELETED_FILE_DIFF);
    expect(files).toHaveLength(1);
    expect(files[0].status).toBe('deleted');
    expect(files[0].additions).toBe(0);
    expect(files[0].deletions).toBe(2);
  });

  it('returns empty array for empty diff', () => {
    expect(parseUnifiedDiff('')).toEqual([]);
  });
});

describe('buildCodeChangesArtifact', () => {
  it('builds artifact from diff result and files', () => {
    const diffResult: AgentDiffResult = {
      files_changed: 1, insertions: 5, deletions: 2, stat: '', diff: '',
    };
    const files = parseUnifiedDiff(SINGLE_FILE_DIFF);
    const artifact = buildCodeChangesArtifact('diff-123', diffResult, files);
    expect(artifact.id).toBe('diff-123');
    expect(artifact.type).toBe('code_changes');
    expect(artifact.name).toBe('Code Changes');
    expect(artifact.total_additions).toBe(5);
    expect(artifact.total_deletions).toBe(2);
    expect(artifact.files).toHaveLength(1);
    expect(artifact.created_at).toBeTruthy();
  });
});
