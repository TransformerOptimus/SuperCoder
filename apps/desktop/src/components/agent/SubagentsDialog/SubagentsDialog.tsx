import { useState, useEffect, useCallback } from 'react';
import { X, Users, Package, Globe, FolderOpen } from 'lucide-react';
import { agentTauriService } from '@/services/agentTauriService';
import { themedMessage } from '@/providers/AntDThemeProvider';
import type { SubagentListEntry, SubagentsPaths } from '@/types/agentContract';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  workingDir?: string | null;
}

const ORIGIN_INFO: Record<
  SubagentListEntry['origin'],
  { label: string; icon: typeof Package; color: string }
> = {
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

function OriginBadge({ origin }: { origin: SubagentListEntry['origin'] }) {
  const info = ORIGIN_INFO[origin];
  const Icon = info.icon;
  return (
    <span
      className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium ${info.color}`}
    >
      <Icon className="w-3 h-3" />
      {info.label}
    </span>
  );
}

export default function SubagentsDialog({ isOpen, onClose, workingDir }: Props) {
  const [subagents, setSubagents] = useState<SubagentListEntry[]>([]);
  const [paths, setPaths] = useState<SubagentsPaths | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    setLoading(true);
    setError('');
    Promise.all([
      agentTauriService.listSubagents(workingDir ?? null),
      agentTauriService.getSubagentsPaths(workingDir ?? null),
    ])
      .then(([rows, resolvedPaths]) => {
        if (cancelled) return;
        setSubagents(rows);
        setPaths(resolvedPaths);
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('[subagents] Failed to load subagents:', err);
        setSubagents([]);
        setPaths(null);
        setError('Failed to load subagents');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, workingDir]);

  const handleToggle = useCallback(async (name: string, nextEnabled: boolean) => {
    setSubagents((prev) =>
      prev.map((s) => (s.name === name ? { ...s, enabled: nextEnabled } : s))
    );
    try {
      await agentTauriService.setSubagentEnabled(name, nextEnabled);
    } catch (err) {
      console.error('[subagents] Failed to set enabled:', err);
      themedMessage.error(`Failed to toggle ${name}`);
      setSubagents((prev) =>
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
            <Users className="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Subagents</h2>
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
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-8">
              Loading...
            </div>
          ) : subagents.length === 0 ? (
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">
              <p>No subagents installed.</p>
            </div>
          ) : (
            subagents.map((a) => (
              <div
                key={`${a.origin}:${a.name}`}
                className="flex items-start gap-3 p-3 rounded-lg border border-gray-200 dark:border-dark-border"
              >
                <button
                  type="button"
                  role="switch"
                  aria-checked={a.enabled}
                  onClick={() => handleToggle(a.name, !a.enabled)}
                  className={`relative inline-flex h-5 w-9 shrink-0 mt-0.5 items-center rounded-full transition-colors ${
                    a.enabled ? 'bg-blue-600' : 'bg-gray-300 dark:bg-dark-border'
                  }`}
                  aria-label={a.enabled ? `Disable ${a.name}` : `Enable ${a.name}`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      a.enabled ? 'translate-x-4' : 'translate-x-0.5'
                    }`}
                  />
                </button>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
                      {a.name}
                    </span>
                    <OriginBadge origin={a.origin} />
                    {a.model && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-purple-50 dark:bg-purple-900/20 text-purple-700 dark:text-purple-300 font-medium">
                        {a.model}
                      </span>
                    )}
                    {a.allowedTools && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-300 font-medium">
                        {a.allowedTools.length} tool{a.allowedTools.length === 1 ? '' : 's'}
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                    {a.description}
                  </p>
                </div>
              </div>
            ))
          )}

          {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

          {!loading && paths && (
            <div className="pt-4 mt-2 border-t border-gray-100 dark:border-dark-border/50 space-y-2">
              <p className="text-xs text-gray-400 dark:text-gray-500">
                Add your own by dropping a folder with{' '}
                <code className="px-1 py-0.5 bg-gray-100 dark:bg-dark-surface rounded">
                  &lt;name&gt;.md
                </code>{' '}
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
