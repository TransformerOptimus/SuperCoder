import { useState, useEffect, useCallback } from 'react';
import { X, Shield, ShieldAlert, ShieldOff, SlidersHorizontal, Globe, FolderOpen } from 'lucide-react';
import { agentTauriService } from '@/services/agentTauriService';
import type { PermissionConfig, PermissionLevel } from '@/types/agentContract';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  projectPath?: string | null;
}

type UIMode = PermissionLevel | 'Custom';

const LEVEL_INFO: Record<PermissionLevel, { label: string; shortLabel: string; desc: string; icon: typeof Shield }> = {
  ApproveDestructive: {
    label: 'Balanced',
    shortLabel: 'Balanced',
    desc: 'Reads and searches run automatically. Writes, shell, and git ask first.',
    icon: Shield,
  },
  ApproveEverything: {
    label: 'Ask for everything',
    shortLabel: 'Ask all',
    desc: 'Every tool asks for your approval before running.',
    icon: ShieldAlert,
  },
  AutoApproveAll: {
    label: 'Full auto',
    shortLabel: 'Full auto',
    desc: 'All tools run without asking. Best for trusted projects.',
    icon: ShieldOff,
  },
};

const MODES: { value: UIMode; label: string; desc: string; icon: typeof Shield }[] = [
  { value: 'ApproveDestructive', ...LEVEL_INFO.ApproveDestructive },
  { value: 'ApproveEverything', ...LEVEL_INFO.ApproveEverything },
  { value: 'AutoApproveAll', ...LEVEL_INFO.AutoApproveAll },
  {
    value: 'Custom',
    label: 'Custom',
    desc: 'Pick a base level and override individual tools.',
    icon: SlidersHorizontal,
  },
];

const DESTRUCTIVE_TOOLS = ['write', 'edit', 'bash', 'git', 'create_pr'];
const ALL_TOOLS = ['read', 'glob', 'grep', 'write', 'edit', 'bash', 'git', 'create_pr'];

const TOOL_LABELS: Record<string, string> = {
  read: 'Read files',
  glob: 'Search files',
  grep: 'Search content',
  write: 'Write files',
  edit: 'Edit files',
  bash: 'Run commands',
  git: 'Git operations',
  create_pr: 'Create PRs',
};

function LevelBadge({ level }: { level: PermissionLevel }) {
  const info = LEVEL_INFO[level];
  const Icon = info.icon;
  const colorMap: Record<PermissionLevel, string> = {
    AutoApproveAll: 'text-green-600 dark:text-green-400 bg-green-50 dark:bg-green-900/20',
    ApproveDestructive: 'text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20',
    ApproveEverything: 'text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20',
  };
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${colorMap[level]}`}>
      <Icon className="w-3 h-3" />
      {info.shortLabel}
    </span>
  );
}

export default function PermissionSettingsModal({ isOpen, onClose, projectPath }: Props) {
  const [level, setLevel] = useState<PermissionLevel>('ApproveDestructive');
  const [uiMode, setUiMode] = useState<UIMode>('ApproveDestructive');
  const [autoApprove, setAutoApprove] = useState<string[]>([]);
  const [alwaysAsk, setAlwaysAsk] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [scope, setScope] = useState<'global' | 'project'>(projectPath ? 'project' : 'global');

  // Load both configs for display
  const [globalLevel, setGlobalLevel] = useState<PermissionLevel | null>(null);
  const [projectLevel, setProjectLevel] = useState<PermissionLevel | null>(null);

  const isCustom = uiMode === 'Custom';

  // Load current config + both scope levels for the header summary
  useEffect(() => {
    if (!isOpen) return;
    setLoading(true);
    setError('');

    const loadBoth = async () => {
      try {
        // Always load global
        const globalConfig = await agentTauriService.getPermissions(undefined);
        setGlobalLevel(globalConfig.level as PermissionLevel);

        // Load project-specific if we have a project path
        let projConfig: PermissionConfig | null = null;
        if (projectPath) {
          projConfig = await agentTauriService.getPermissions(projectPath);
          setProjectLevel(projConfig.level as PermissionLevel);
        }

        // Set the active editor state based on current scope
        const activeConfig = scope === 'project' && projConfig ? projConfig : globalConfig;
        const lvl = activeConfig.level as PermissionLevel;
        setLevel(lvl);
        const overrides = activeConfig.tool_overrides;
        const hasOverrides = (overrides?.auto_approve?.length ?? 0) > 0 || (overrides?.always_ask?.length ?? 0) > 0;
        setAutoApprove(overrides?.auto_approve ?? []);
        setAlwaysAsk(overrides?.always_ask ?? []);
        setUiMode(hasOverrides ? 'Custom' : lvl);
      } catch (err) {
        console.error('Failed to load permissions:', err);
        setError('Failed to load permissions');
      } finally {
        setLoading(false);
      }
    };

    loadBoth();
  }, [isOpen, scope, projectPath]);

  const handleModeChange = useCallback((mode: UIMode) => {
    setUiMode(mode);
    if (mode !== 'Custom') {
      setLevel(mode);
      setAutoApprove([]);
      setAlwaysAsk([]);
    }
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    setError('');
    try {
      const hasOverrides = autoApprove.length > 0 || alwaysAsk.length > 0;
      const config: PermissionConfig = {
        project_path: scope === 'project' ? (projectPath ?? null) : null,
        level,
        tool_overrides: hasOverrides ? { auto_approve: autoApprove, always_ask: alwaysAsk } : null,
      };
      await agentTauriService.setPermission(config);

      // Update the summary badges
      if (scope === 'global') {
        setGlobalLevel(level);
      } else {
        setProjectLevel(level);
      }

      onClose();
    } catch (err: any) {
      setError(err?.message || 'Failed to save permissions');
    } finally {
      setSaving(false);
    }
  }, [level, autoApprove, alwaysAsk, scope, projectPath, onClose]);

  const toggleOverride = useCallback((tool: string, list: 'autoApprove' | 'alwaysAsk') => {
    if (list === 'autoApprove') {
      setAutoApprove((prev) =>
        prev.includes(tool) ? prev.filter((t) => t !== tool) : [...prev, tool]
      );
      setAlwaysAsk((prev) => prev.filter((t) => t !== tool));
    } else {
      setAlwaysAsk((prev) =>
        prev.includes(tool) ? prev.filter((t) => t !== tool) : [...prev, tool]
      );
      setAutoApprove((prev) => prev.filter((t) => t !== tool));
    }
  }, []);

  const wouldNeedApproval = useCallback((tool: string): boolean => {
    if (level === 'AutoApproveAll') return false;
    if (level === 'ApproveEverything') return true;
    return DESTRUCTIVE_TOOLS.includes(tool);
  }, [level]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="bg-white dark:bg-dark-bg rounded-xl shadow-xl w-full max-w-lg mx-4 overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-gray-200 dark:border-dark-border">
          <div className="flex items-center gap-2">
            <Shield className="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Tool Permissions</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-5 space-y-5 max-h-[70vh] overflow-y-auto">
          {loading ? (
            <div className="text-sm text-gray-500 dark:text-gray-400 text-center py-8">Loading...</div>
          ) : (
            <>
              {/* Current levels summary — always visible */}
              <div className="rounded-lg border border-gray-200 dark:border-dark-border p-3 space-y-2">
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
                  Current Levels
                </p>
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-1.5 flex-1">
                    <Globe className="w-3.5 h-3.5 text-gray-400" />
                    <span className="text-xs text-gray-500 dark:text-gray-400">Global:</span>
                    {globalLevel ? <LevelBadge level={globalLevel} /> : (
                      <span className="text-xs text-gray-400">Not set</span>
                    )}
                  </div>
                  {projectPath && (
                    <div className="flex items-center gap-1.5 flex-1">
                      <FolderOpen className="w-3.5 h-3.5 text-gray-400" />
                      <span className="text-xs text-gray-500 dark:text-gray-400">Project:</span>
                      {projectLevel ? <LevelBadge level={projectLevel} /> : (
                        <span className="text-xs text-gray-400 italic">Inherits global</span>
                      )}
                    </div>
                  )}
                </div>
                {projectPath && projectLevel && globalLevel && projectLevel !== globalLevel && (
                  <p className="text-[11px] text-amber-600 dark:text-amber-400 mt-1">
                    Project overrides global for this path
                  </p>
                )}
              </div>

              {/* Scope selector */}
              {projectPath && (
                <div>
                  <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide">
                    Editing
                  </label>
                  <div className="flex gap-2">
                    {(['global', 'project'] as const).map((s) => (
                      <button
                        key={s}
                        type="button"
                        onClick={() => setScope(s)}
                        className={`flex-1 px-3 py-2 text-sm rounded-lg border transition-colors ${
                          scope === s
                            ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300'
                            : 'border-gray-300 dark:border-dark-border text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-dark-hover'
                        }`}
                      >
                        <div className="flex items-center justify-center gap-1.5">
                          {s === 'global' ? <Globe className="w-3.5 h-3.5" /> : <FolderOpen className="w-3.5 h-3.5" />}
                          {s === 'global' ? 'Global (all projects)' : 'This project'}
                        </div>
                      </button>
                    ))}
                  </div>
                  {scope === 'project' && (
                    <p className="text-xs text-gray-400 dark:text-gray-500 mt-1 truncate">
                      {projectPath}
                    </p>
                  )}
                </div>
              )}

              {/* Approval mode */}
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide">
                  Approval Mode
                </label>
                <div className="space-y-2">
                  {MODES.map(({ value, label, desc, icon: Icon }) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => handleModeChange(value)}
                      className={`w-full flex items-start gap-3 p-3 rounded-lg border transition-colors text-left ${
                        uiMode === value
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                          : 'border-gray-200 dark:border-dark-border hover:bg-gray-50 dark:hover:bg-dark-hover'
                      }`}
                    >
                      <Icon className={`w-4 h-4 mt-0.5 shrink-0 ${
                        uiMode === value
                          ? 'text-blue-600 dark:text-blue-400'
                          : 'text-gray-400 dark:text-gray-500'
                      }`} />
                      <div>
                        <div className={`text-sm font-medium ${
                          uiMode === value
                            ? 'text-blue-700 dark:text-blue-300'
                            : 'text-gray-700 dark:text-gray-300'
                        }`}>
                          {label}
                        </div>
                        <div className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                          {desc}
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              </div>

              {/* Custom: base level + per-tool overrides */}
              {isCustom && (
                <>
                  <div>
                    <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide">
                      Base Level
                    </label>
                    <div className="flex gap-2">
                      {(['ApproveDestructive', 'ApproveEverything', 'AutoApproveAll'] as PermissionLevel[]).map((lvl) => (
                        <button
                          key={lvl}
                          type="button"
                          onClick={() => setLevel(lvl)}
                          className={`flex-1 px-3 py-2 text-sm rounded-lg border transition-colors ${
                            level === lvl
                              ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300'
                              : 'border-gray-300 dark:border-dark-border text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-dark-hover'
                          }`}
                        >
                          {LEVEL_INFO[lvl].shortLabel}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div>
                    <label className="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide">
                      Per-Tool Overrides
                    </label>
                    <p className="text-xs text-gray-400 dark:text-gray-500 mb-3">
                      Click a tool to cycle: default → always allow → always ask.
                    </p>
                    <div className="space-y-1">
                      {ALL_TOOLS.map((tool) => {
                        const needsApproval = wouldNeedApproval(tool);
                        const isAutoOverride = autoApprove.includes(tool);
                        const isAskOverride = alwaysAsk.includes(tool);

                        let stateLabel: string;
                        let stateColor: string;
                        let dotColor: string;
                        if (isAutoOverride) {
                          stateLabel = 'Always allow';
                          stateColor = 'text-green-600 dark:text-green-400';
                          dotColor = 'bg-green-500';
                        } else if (isAskOverride) {
                          stateLabel = 'Always ask';
                          stateColor = 'text-amber-600 dark:text-amber-400';
                          dotColor = 'bg-amber-500';
                        } else if (needsApproval) {
                          stateLabel = 'Asks (from base)';
                          stateColor = 'text-gray-400 dark:text-gray-500';
                          dotColor = 'bg-amber-300 dark:bg-amber-600';
                        } else {
                          stateLabel = 'Allowed (from base)';
                          stateColor = 'text-gray-400 dark:text-gray-500';
                          dotColor = 'bg-green-300 dark:bg-green-600';
                        }

                        const handleCycle = () => {
                          if (!isAutoOverride && !isAskOverride) {
                            toggleOverride(tool, 'autoApprove');
                          } else if (isAutoOverride) {
                            toggleOverride(tool, 'alwaysAsk');
                          } else {
                            setAutoApprove((p) => p.filter((t) => t !== tool));
                            setAlwaysAsk((p) => p.filter((t) => t !== tool));
                          }
                        };

                        return (
                          <button
                            key={tool}
                            type="button"
                            onClick={handleCycle}
                            className="w-full flex items-center justify-between px-3 py-2 rounded-lg hover:bg-gray-50 dark:hover:bg-dark-hover transition-colors"
                          >
                            <div className="flex items-center gap-2.5">
                              <span className={`w-2 h-2 rounded-full shrink-0 ${dotColor}`} />
                              <span className="text-sm text-gray-700 dark:text-gray-300">
                                {TOOL_LABELS[tool] || tool}
                              </span>
                              <code className="text-[10px] text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-dark-surface px-1 py-0.5 rounded">
                                {tool}
                              </code>
                            </div>
                            <span className={`text-xs ${stateColor}`}>
                              {stateLabel}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                </>
              )}
            </>
          )}

          {error && (
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-5 py-4 border-t border-gray-200 dark:border-dark-border">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm rounded-lg border border-gray-300 dark:border-dark-border text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-dark-hover transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || loading}
            className="px-4 py-2 text-sm rounded-lg bg-gray-900 dark:bg-white text-white dark:text-gray-900 hover:bg-gray-800 dark:hover:bg-gray-100 disabled:opacity-50 transition-colors"
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}
