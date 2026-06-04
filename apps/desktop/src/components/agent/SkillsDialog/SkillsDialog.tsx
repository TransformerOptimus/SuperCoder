import { useState, useEffect, useCallback } from 'react';
import { X, BookOpen, Package, Globe, FolderOpen } from 'lucide-react';
import { agentTauriService } from '@/services/agentTauriService';
import { themedMessage } from '@/providers/AntDThemeProvider';
import type { SkillListEntry, SkillsPaths } from '@/types/agentContract';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  workingDir?: string | null;
}

const ORIGIN_INFO: Record<SkillListEntry['origin'], { label: string; icon: typeof Package; color: string }> = {
  default: {
    label: 'Built-in',
    icon: Package,
    color: 'text-gray-600 dark:text-gray-400 bg-gray-100 dark:bg-dark-surface',
  },
  global: {
    label: 'Global',
    icon: Globe,
    color: 'text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20',
  },
  project: {
    label: 'Project',
    icon: FolderOpen,
    color: 'text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-900/20',
  },
};

function OriginBadge({ origin }: { origin: SkillListEntry['origin'] }) {
  const info = ORIGIN_INFO[origin];
  const Icon = info.icon;
  return (
    <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${info.color}`}>
      <Icon className="w-3 h-3" />
      {info.label}
    </span>
  );
}

export default function SkillsDialog({ isOpen, onClose, workingDir }: Props) {
  const [skills, setSkills] = useState<SkillListEntry[]>([]);
  const [paths, setPaths] = useState<SkillsPaths | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    console.log('[skills] SkillsDialog opened, invoking listSkills workingDir=', workingDir);
    // Effect-scoped flag guards against (a) rapid workingDir change while a
    // fetch is in flight (last-to-resolve would otherwise win, possibly with
    // stale data) and (b) the dialog closing mid-fetch.
    let cancelled = false;
    setLoading(true);
    setError('');
    Promise.all([
      agentTauriService.listSkills(workingDir ?? null),
      agentTauriService.getSkillsPaths(workingDir ?? null),
    ])
      .then(([rows, resolvedPaths]) => {
        if (cancelled) return;
        console.log('[skills] listSkills returned', rows, 'paths', resolvedPaths);
        setSkills(rows);
        setPaths(resolvedPaths);
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('[skills] Failed to load skills:', err);
        // Clear stale list/paths so a failed refetch after a prior success
        // doesn't leave the dialog showing outdated data under a red error.
        setSkills([]);
        setPaths(null);
        setError('Failed to load skills');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, workingDir]);

  const handleToggle = useCallback(async (name: string, nextEnabled: boolean) => {
    console.log('[skills] toggle', name, '→', nextEnabled);
    setSkills((prev) =>
      prev.map((s) => (s.name === name ? { ...s, enabled: nextEnabled } : s))
    );
    try {
      await agentTauriService.setSkillEnabled(name, nextEnabled);
      console.log('[skills] toggle persisted', name, nextEnabled);
    } catch (err) {
      console.error('[skills] Failed to set skill enabled:', err);
      // Surface via global toast — it survives dialog unmount. setSkills rollback
      // still runs in case the dialog is still open.
      themedMessage.error(`Failed to toggle ${name}`);
      setSkills((prev) =>
        prev.map((s) => (s.name === name ? { ...s, enabled: !nextEnabled } : s))
      );
    }
  }, []);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="bg-white dark:bg-dark-bg rounded-xl shadow-xl w-full max-w-lg mx-4 overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-dark-border">
          <div className="flex items-center gap-2">
            <BookOpen className="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Skills</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-3 max-h-[70vh] overflow-y-auto">
          {loading ? (
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-8">Loading...</div>
          ) : skills.length === 0 ? (
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">
              <p>No skills installed.</p>
            </div>
          ) : (
            skills.map((skill) => (
              <div
                key={`${skill.origin}:${skill.name}`}
                className="flex items-start gap-3 p-3 rounded-lg border border-gray-200 dark:border-dark-border"
              >
                <button
                  type="button"
                  role="switch"
                  aria-checked={skill.enabled}
                  onClick={() => handleToggle(skill.name, !skill.enabled)}
                  className={`relative inline-flex h-5 w-9 shrink-0 mt-0.5 items-center rounded-full transition-colors ${
                    skill.enabled ? 'bg-blue-600' : 'bg-gray-300 dark:bg-dark-border'
                  }`}
                  aria-label={skill.enabled ? `Disable ${skill.name}` : `Enable ${skill.name}`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      skill.enabled ? 'translate-x-4' : 'translate-x-0.5'
                    }`}
                  />
                </button>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
                      {skill.name}
                    </span>
                    <OriginBadge origin={skill.origin} />
                  </div>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                    {skill.description}
                  </p>
                </div>
              </div>
            ))
          )}

          {error && (
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          )}

          {!loading && paths && (
            <div className="pt-4 mt-2 border-t border-gray-100 dark:border-dark-border/50 space-y-2">
              <p className="text-xs text-gray-400 dark:text-gray-500">
                Add your own by dropping a folder with{' '}
                <code className="px-1 py-0.5 bg-gray-100 dark:bg-dark-surface rounded">SKILL.md</code>{' '}
                into:
              </p>
              <div className="text-xs space-y-2">
                <div>
                  <div className="text-gray-500 dark:text-gray-400 mb-0.5 flex items-center gap-1.5">
                    <Globe className="w-3 h-3" />
                    Global (all projects)
                  </div>
                  <code className="block px-2 py-1 bg-gray-50 dark:bg-dark-surface rounded break-all">
                    {paths.global}
                  </code>
                </div>
                {paths.project && (
                  <div>
                    <div className="text-gray-500 dark:text-gray-400 mb-0.5 flex items-center gap-1.5">
                      <FolderOpen className="w-3 h-3" />
                      This project only
                    </div>
                    <code className="block px-2 py-1 bg-gray-50 dark:bg-dark-surface rounded break-all">
                      {paths.project}
                    </code>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="flex items-center justify-between gap-2 px-5 py-3 border-t border-gray-200 dark:border-dark-border">
          <p className="text-xs text-gray-400 dark:text-gray-500">
            Changes apply on the next message.
          </p>
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm rounded-lg bg-gray-900 dark:bg-white text-white dark:text-gray-900 hover:bg-gray-800 dark:hover:bg-gray-100 transition-colors"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
