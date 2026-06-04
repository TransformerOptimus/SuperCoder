import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Input, Button, Select, Spin, Popconfirm, Switch, Segmented } from "antd";
import { ArrowLeft, Plus, Pencil, Trash2, Sun, Moon, Monitor } from "lucide-react";
import { agentTauriService } from "@/services/agentTauriService";
import { useAppStore } from "@/store";
import { themedMessage } from "@/providers/AntDThemeProvider";
import { useTheme, type ThemeMode } from "@/context/ThemeContext";
import type {
  ContextEngineSettings,
  ContextEngineStatus,
  EngineMode,
  IndexedRepo,
  CuratedModel,
  ModelRef,
  ModelSelection,
  ProviderConfig,
  SelectionRole,
} from "@/types/agent";

const DEFAULT_BASE_URL: Record<string, string> = {
  openai: "https://api.openai.com/v1",
  anthropic: "https://api.anthropic.com",
  openai_compatible: "",
};

/** Format a context window as a compact label, e.g. 1_000_000 → "1M", 128_000 → "128k". */
function formatContext(tokens: number): string {
  if (tokens >= 1_000_000) return `${tokens / 1_000_000}M`;
  if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}k`;
  return String(tokens);
}

function providerName(p: ProviderConfig): string {
  if (p.kind === "openai") return "OpenAI";
  if (p.kind === "anthropic") return "Anthropic";
  if (p.label?.trim()) return p.label.trim();
  try {
    return new URL(p.baseUrl).host || "OpenAI-compatible";
  } catch {
    return "OpenAI-compatible";
  }
}

const isBuiltin = (p: ProviderConfig) => p.kind === "openai" || p.kind === "anthropic";

const refToValue = (r: ModelRef | null) => (r ? `${r.providerId}::${r.model}` : undefined);
const parseValue = (v: string): ModelRef => {
  const idx = v.indexOf("::");
  return { providerId: v.slice(0, idx), model: v.slice(idx + 2) };
};

export default function Settings() {
  const navigate = useNavigate();
  const { mode: themeMode, setMode: setThemeMode, dark } = useTheme();
  const loadProviders = useAppStore((s) => s.loadProviders);
  // Live file-watcher status per repo path, refreshed by context-watcher-status events.
  const contextWatcherStatus = useAppStore((s) => s.contextWatcherStatus);

  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [selection, setSelection] = useState<ModelSelection>({ active: null, compaction: null, title: null });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  // Provider being added/edited (null = list view).
  const [draft, setDraft] = useState<ProviderConfig | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [contextEngine, setContextEngine] = useState<ContextEngineSettings>({
    enabled: false,
    base_url: "http://127.0.0.1:8106",
  });
  const [ceStatus, setCeStatus] = useState<ContextEngineStatus | null>(null);
  const [ceConnecting, setCeConnecting] = useState(false);
  const [ceRepos, setCeRepos] = useState<IndexedRepo[]>([]);
  const [ceReposLoading, setCeReposLoading] = useState(false);
  // App-managed engine lifecycle (only meaningful when engineMode === "app").
  const [engineMode, setEngineMode] = useState<EngineMode>("user");
  const [hasKey, setHasKey] = useState(false);
  const [keyInput, setKeyInput] = useState("");
  const [savingKey, setSavingKey] = useState(false);
  const [engineBusy, setEngineBusy] = useState(false);
  const engineStatus = useAppStore((s) => s.engineStatus);
  const engineProgress = useAppStore((s) => s.engineProgress);
  // Built-in model registry (context window + vision), fetched from the backend.
  const [curatedModels, setCuratedModels] = useState<CuratedModel[]>([]);

  const refresh = async () => {
    const res = await agentTauriService.listProviders();
    setProviders(res.providers);
    setSelection(res.selection);
    loadProviders(); // keep composer picker in sync
  };

  useEffect(() => {
    (async () => {
      try {
        await refresh();
        const ce = await agentTauriService.getContextEngine();
        setContextEngine(ce);
        setCuratedModels(await agentTauriService.listModels());
        const mode = await agentTauriService.engineMode();
        setEngineMode(mode);
        if (mode === "app") {
          // App-managed: seed the lifecycle status + key presence. Live updates
          // arrive via engine:status events; repos load when the stack is running.
          setHasKey(await agentTauriService.engineHasKey());
          const st = await agentTauriService.engineStatus();
          useAppStore.getState().setEngineStatus(st);
          if (st.state === "running") await loadCeRepos();
        } else {
          // User-managed: probe the saved backend so the panel opens with live
          // status + the indexed-repo list already populated.
          agentTauriService
            .contextEngineStatus(ce.base_url)
            .then(async (status) => {
              setCeStatus(status);
              if (status.connected) await loadCeRepos();
            })
            .catch(() => setCeStatus({ connected: false, error: "unreachable" }));
        }
      } catch (err) {
        console.error("[Settings] Failed to load providers:", err);
      } finally {
        setLoading(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveContextEngine = async (next: ContextEngineSettings) => {
    setContextEngine(next);
    try {
      await agentTauriService.setContextEngine(next);
    } catch (err) {
      console.error("[Settings] Failed to save context engine settings:", err);
      themedMessage.error("Failed to save context engine settings");
    }
  };

  const loadCeRepos = async () => {
    setCeReposLoading(true);
    try {
      setCeRepos(await agentTauriService.contextEngineRepos());
    } catch (err) {
      console.error("[Settings] Failed to list indexed repos:", err);
    } finally {
      setCeReposLoading(false);
    }
  };

  const deleteIndexedRepo = async (path: string) => {
    try {
      await agentTauriService.deleteContextEngineRepo(path);
      themedMessage.success("Index deleted");
      await loadCeRepos();
    } catch (err) {
      console.error("[Settings] Failed to delete index:", err);
      themedMessage.error("Failed to delete index");
    }
  };

  // Probe a backend URL + load its repo list. Does NOT persist settings.
  const probeBackend = async (baseUrl: string) => {
    setCeConnecting(true);
    try {
      const status = await agentTauriService.contextEngineStatus(baseUrl);
      setCeStatus(status);
      if (status.connected) await loadCeRepos();
      else setCeRepos([]);
    } catch (err) {
      setCeStatus({ connected: false, error: String(err) });
    } finally {
      setCeConnecting(false);
    }
  };

  // Master toggle. Persist the new `enabled`. In user mode we probe the backend;
  // in app mode `agent_set_context_engine` starts/stops the docker stack and the
  // status arrives via engine:status events (no probe here).
  const toggleContextEngine = async (enabled: boolean) => {
    const next = { ...contextEngine, enabled };
    // App mode: turning off stops the docker stack (a few seconds) — show a
    // loading toast since the call blocks until the containers are down.
    if (engineMode === "app" && !enabled) {
      const toastKey = "ce-stop";
      themedMessage.loading("Stopping engine…", toastKey);
      try {
        await saveContextEngine(next);
        themedMessage.success("Engine stopped", toastKey);
      } catch (err) {
        themedMessage.error(`Failed to stop: ${err}`, toastKey);
      }
      return;
    }
    await saveContextEngine(next);
    if (engineMode === "app") return;
    if (enabled) await probeBackend(next.base_url);
    else {
      setCeStatus(null);
      setCeRepos([]);
    }
  };

  // Connect button (user mode): persist current settings (incl. edited URL), then probe.
  const connectContextEngine = async () => {
    await saveContextEngine(contextEngine);
    await probeBackend(contextEngine.base_url);
  };

  // ── App-managed lifecycle handlers ──────────────────────────────────────
  // Persist the key (if a new one was entered), then (re)start the stack so the
  // key is injected into the engine's process env. `engineStart` recreates the
  // server/worker containers when the env changed and waits for /api/health.
  const saveAndRestart = async () => {
    const running = engineStatus?.state === "running";
    const toastKey = "ce-restart";
    setSavingKey(true);
    themedMessage.loading(running ? "Restarting engine…" : "Starting engine…", toastKey);
    try {
      if (keyInput.trim()) {
        await agentTauriService.engineSetKey(keyInput.trim());
        setHasKey(true);
        setKeyInput("");
      }
      await agentTauriService.engineStart();
      await loadCeRepos();
      themedMessage.success(running ? "Engine restarted" : "Engine started", toastKey);
    } catch (err) {
      themedMessage.error(`${running ? "Restart" : "Start"} failed: ${err}`, toastKey);
    } finally {
      setSavingKey(false);
    }
  };

  const startEngine = async () => {
    setEngineBusy(true);
    try {
      await agentTauriService.engineStart();
      await loadCeRepos();
    } catch (err) {
      themedMessage.error(`Engine failed to start: ${err}`);
    } finally {
      setEngineBusy(false);
    }
  };

  const stopEngine = async () => {
    setEngineBusy(true);
    try {
      await agentTauriService.engineStop();
    } catch (err) {
      themedMessage.error(`Failed to stop: ${err}`);
    } finally {
      setEngineBusy(false);
    }
  };

  const removeEngineData = async () => {
    setEngineBusy(true);
    try {
      await agentTauriService.engineRemove(true);
      setCeRepos([]);
      themedMessage.success("Indexed data removed");
    } catch (err) {
      themedMessage.error(`Failed to remove data: ${err}`);
    } finally {
      setEngineBusy(false);
    }
  };

  // Load the repo list once the app-managed stack reports healthy.
  useEffect(() => {
    if (engineMode === "app" && engineStatus?.state === "running") {
      loadCeRepos();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [engineMode, engineStatus?.state]);

  // Whether the indexed-repos list has a live backend to query.
  const ceConnected =
    engineMode === "app" ? engineStatus?.state === "running" : !!ceStatus?.connected;

  // Indexed-repositories list — shared by both modes.
  const reposSection = (
    <div className="border-t border-[var(--border)] pt-4">
      <div className="flex items-center justify-between mb-2">
        <div className="text-sm font-medium text-[var(--text-primary)]">Indexed repositories</div>
        <Button size="small" onClick={loadCeRepos} loading={ceReposLoading} disabled={!ceConnected}>
          Refresh
        </Button>
      </div>
      {!ceConnected ? (
        <div className="text-xs text-[var(--text-secondary)]">
          {engineMode === "app"
            ? "Start the engine to see indexed repos."
            : "Connect to a backend to see indexed repos."}
        </div>
      ) : ceRepos.length === 0 ? (
        <div className="text-xs text-[var(--text-secondary)]">
          {ceReposLoading ? "Loading…" : "No repositories indexed yet — open a session in a repo to index it."}
        </div>
      ) : (
        <div className="flex flex-col gap-1.5 max-h-56 overflow-y-auto">
          {ceRepos.map((r) => {
            // Prefer the live watcher status; fall back to the fetched /index/status.
            const live = contextWatcherStatus[r.path];
            const tag = live?.status ?? r.status ?? null;
            const fileCount = live?.fileCount ?? r.file_count ?? null;
            const indexed = tag === "indexed" || (r.exists && !r.empty);
            const empty = r.exists && r.empty;
            return (
              <div
                key={r.path}
                className="flex items-center gap-2 border border-[var(--border)] rounded-md px-3 py-2"
              >
                <span className="flex-1 text-xs text-[var(--text-primary)] truncate" title={r.path}>
                  {r.path}
                </span>
                <span className="text-[11px] whitespace-nowrap">
                  {tag === "indexing" ? (
                    <span className="text-[var(--text-secondary)]">
                      <Spin size="small" className="mr-1" />Indexing…
                    </span>
                  ) : tag && tag.startsWith("error") ? (
                    <span className="text-red-500" title={live?.reason ?? undefined}>⚠ Error</span>
                  ) : indexed ? (
                    <span className="text-green-500">
                      ✓ Indexed
                      {fileCount != null ? ` · ${fileCount} files` : ""}
                    </span>
                  ) : empty ? (
                    <span className="text-amber-500">○ Empty</span>
                  ) : (
                    <span className="text-[var(--text-secondary)]">○ Not indexed</span>
                  )}
                </span>
                {r.exists && (
                  <Popconfirm
                    title="Delete this index?"
                    description="Removes the indexed vectors + graph for this repo."
                    okText="Delete"
                    okButtonProps={{ danger: true }}
                    onConfirm={() => deleteIndexedRepo(r.path)}
                  >
                    <Button type="text" size="small" danger icon={<Trash2 className="w-3.5 h-3.5" />} />
                  </Popconfirm>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );

  // Grouped (provider, model) options for the compaction/title pickers.
  const modelOptions = useMemo(
    () =>
      providers
        .filter((p) => p.models.length > 0)
        .map((p) => ({
          label: providerName(p),
          options: p.models.map((m) => ({ label: m, value: `${p.id}::${m}` })),
        })),
    [providers],
  );

  const startAddCompatible = () => {
    setDraft({ id: "", kind: "openai_compatible", label: "", baseUrl: "", apiKey: "", models: [] });
    setIsNew(true);
  };
  const startEdit = (p: ProviderConfig) => {
    setDraft({ ...p });
    setIsNew(false);
  };
  const cancelEdit = () => {
    setDraft(null);
    setIsNew(false);
  };

  const handleFetchModels = async () => {
    if (!draft) return;
    if (!draft.baseUrl.trim()) {
      themedMessage.warning("Base URL is required to fetch models");
      return;
    }
    setFetchingModels(true);
    try {
      const list = await agentTauriService.fetchProviderModels({ ...draft, baseUrl: draft.baseUrl.trim() });
      if (list.length > 0) {
        // Merge fetched ids into the model list (keep any manually-added ones).
        const merged = Array.from(new Set([...draft.models, ...list.map((m) => m.id)]));
        // Record any discovered context length + the provider's vision flag per model.
        const modelMeta = { ...(draft.modelMeta ?? {}) };
        for (const m of list) {
          modelMeta[m.id] = {
            contextLength: m.contextLength,
            supportsImages: draft.modelMeta?.[m.id]?.supportsImages ?? draft.supportsImages ?? false,
          };
        }
        setDraft({ ...draft, models: merged, modelMeta });
        themedMessage.success(`Found ${list.length} models`);
      } else {
        themedMessage.info("No models returned by this endpoint");
      }
    } catch (err) {
      console.error("[Settings] fetchModels failed:", err);
      themedMessage.error("Failed to fetch models — check the base URL and key");
    } finally {
      setFetchingModels(false);
    }
  };

  const handleSaveProvider = async () => {
    if (!draft) return;
    if (!draft.baseUrl.trim()) {
      themedMessage.warning("Base URL is required");
      return;
    }
    const payload: ProviderConfig = {
      ...draft,
      baseUrl: draft.baseUrl.trim(),
      apiKey: draft.apiKey.trim(),
    };
    setSaving(true);
    try {
      // Validate the key via a tiny probe — blocks the save only on a clear 401/403.
      if (payload.apiKey) {
        try {
          await agentTauriService.verifyProvider(payload);
        } catch (e) {
          themedMessage.error(typeof e === "string" ? e : "API key rejected");
          setSaving(false);
          return;
        }
      }
      if (isNew) await agentTauriService.addProvider(payload);
      else await agentTauriService.updateProvider(payload);
      await refresh();
      cancelEdit();
      themedMessage.success("Provider saved");
    } catch (err) {
      console.error("[Settings] save failed:", err);
      themedMessage.error("Failed to save provider");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await agentTauriService.deleteProvider(id);
      await refresh();
      themedMessage.success("Provider removed");
    } catch (err) {
      console.error("[Settings] delete failed:", err);
      themedMessage.error("Failed to remove provider");
    }
  };

  const handleSelect = async (role: SelectionRole, value: string | undefined) => {
    if (!value) return;
    const { providerId, model } = parseValue(value);
    setSelection((s) => ({ ...s, [role]: { providerId, model } }));
    try {
      await agentTauriService.setModelSelection(role, providerId, model);
      loadProviders();
    } catch (err) {
      console.error("[Settings] set selection failed:", err);
      themedMessage.error("Failed to set model");
    }
  };

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <Spin />
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-2xl mx-auto px-6 py-8">
        <button
          onClick={() => navigate("/")}
          className="flex items-center gap-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] mb-6"
        >
          <ArrowLeft className="w-4 h-4" /> Back
        </button>

        {/* Appearance */}
        <h2 className="text-base font-semibold text-[var(--text-primary)] mb-1">Appearance</h2>
        <p className="text-sm text-[var(--text-secondary)] mb-3">
          Choose the theme. System follows your OS appearance.
        </p>
        <Segmented
          value={themeMode}
          onChange={(val) => setThemeMode(val as ThemeMode)}
          className="mb-8"
          options={[
            { label: <span className="flex items-center gap-1.5"><Sun size={14} /> Light</span>, value: "light" },
            { label: <span className="flex items-center gap-1.5"><Moon size={14} /> Dark</span>, value: "dark" },
            { label: <span className="flex items-center gap-1.5"><Monitor size={14} /> System</span>, value: "system" },
          ]}
        />

        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-1">LLM Providers</h1>
        <p className="text-sm text-[var(--text-secondary)] mb-6">
          Set the API key + base URL for each provider and fetch its models. Pick the coding model from
          the composer; pick the background models for compaction and titles below.
        </p>

        {/* Provider editor */}
        {draft ? (
          <div className="flex flex-col gap-5 border border-[var(--border)] rounded-lg p-5 mb-6">
            <div className="text-sm font-medium text-[var(--text-primary)]">
              {isNew ? "Add OpenAI-compatible provider" : `Edit ${providerName(draft)}`}
            </div>

            {!isBuiltin(draft) && (
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">Name</label>
                <Input
                  value={draft.label}
                  onChange={(e) => setDraft({ ...draft, label: e.target.value })}
                  placeholder="e.g. Baseten, Together, Fireworks"
                />
              </div>
            )}

            <div>
              <label className="block text-xs text-[var(--text-secondary)] mb-1">Base URL</label>
              <Input
                value={draft.baseUrl}
                onChange={(e) => setDraft({ ...draft, baseUrl: e.target.value })}
                placeholder={DEFAULT_BASE_URL[draft.kind] || "https://your-endpoint/v1"}
              />
            </div>

            <div>
              <label className="block text-xs text-[var(--text-secondary)] mb-1">API Key</label>
              <Input.Password
                value={draft.apiKey}
                onChange={(e) => setDraft({ ...draft, apiKey: e.target.value })}
                placeholder="sk-…"
              />
            </div>

            <div>
              <label className="block text-xs text-[var(--text-secondary)] mb-1">Models</label>
              {isBuiltin(draft) ? (
                // Built-in providers: pick the subset to use from the known set.
                <Select
                  mode="multiple"
                  className="w-full"
                  value={draft.models}
                  onChange={(models) => setDraft({ ...draft, models })}
                  options={curatedModels
                    .filter((m) => m.provider === draft.kind)
                    .map((m) => ({
                      label: `${m.id} · ${formatContext(m.contextWindow)}`,
                      value: m.id,
                    }))}
                  placeholder="Pick the models to use"
                />
              ) : (
                // OpenAI-compatible: fetch the list live, or type ids.
                <div className="flex gap-2">
                  <Select
                    mode="tags"
                    className="flex-1"
                    value={draft.models}
                    onChange={(models) => setDraft({ ...draft, models })}
                    options={draft.models.map((m) => ({ label: m, value: m }))}
                    placeholder="Fetch, or type model ids and press Enter"
                    tokenSeparators={[",", " "]}
                  />
                  <Button onClick={handleFetchModels} loading={fetchingModels}>
                    Fetch models
                  </Button>
                </div>
              )}
            </div>

            {!isBuiltin(draft) && (
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-sm text-[var(--text-primary)]">Supports images (vision)</div>
                  <div className="text-xs text-[var(--text-secondary)]">
                    Enables image attachments for this provider's models.
                  </div>
                </div>
                <Switch
                  checked={draft.supportsImages ?? false}
                  onChange={(checked) => {
                    // Apply the flag to the provider and propagate to known per-model meta.
                    const modelMeta = { ...(draft.modelMeta ?? {}) };
                    for (const id of Object.keys(modelMeta)) {
                      modelMeta[id] = { ...modelMeta[id], supportsImages: checked };
                    }
                    setDraft({ ...draft, supportsImages: checked, modelMeta });
                  }}
                />
              </div>
            )}

            <div className="flex gap-2">
              <Button type="primary" onClick={handleSaveProvider} loading={saving}>
                {isNew ? "Add provider" : "Save"}
              </Button>
              <Button onClick={cancelEdit}>Cancel</Button>
            </div>
          </div>
        ) : (
          <>
            <div className="flex flex-col gap-3 mb-4">
              {providers.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center gap-3 border border-[var(--border)] rounded-lg px-4 py-3"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text-primary)] truncate">
                        {providerName(p)}
                      </span>
                      <span className="text-[10px] text-[var(--text-secondary)]">
                        {p.models.length} model{p.models.length === 1 ? "" : "s"}
                        {p.apiKey ? "" : " · no key"}
                      </span>
                    </div>
                    <div className="text-xs text-[var(--text-secondary)] truncate">{p.baseUrl}</div>
                  </div>
                  <Button type="text" icon={<Pencil className="w-4 h-4" />} onClick={() => startEdit(p)} />
                  {!isBuiltin(p) && (
                    <Popconfirm
                      title="Remove this provider?"
                      onConfirm={() => handleDelete(p.id)}
                      okText="Remove"
                      cancelText="Cancel"
                    >
                      <Button type="text" danger icon={<Trash2 className="w-4 h-4" />} />
                    </Popconfirm>
                  )}
                </div>
              ))}
            </div>
            <Button icon={<Plus className="w-4 h-4" />} onClick={startAddCompatible} className="mb-8">
              Add OpenAI-compatible provider
            </Button>

            {/* Background models (global) */}
            <h2 className="text-base font-semibold text-[var(--text-primary)] mb-1">Background models</h2>
            <p className="text-sm text-[var(--text-secondary)] mb-4">
              Cheap models used automatically — picked from any configured provider.
            </p>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">Compaction model</label>
                <Select
                  className="w-full"
                  value={refToValue(selection.compaction)}
                  onChange={(v) => handleSelect("compaction", v)}
                  options={modelOptions}
                  placeholder="Select a model"
                  showSearch
                  notFoundContent="Fetch models on a provider first"
                />
              </div>
              <div>
                <label className="block text-xs text-[var(--text-secondary)] mb-1">Title model</label>
                <Select
                  className="w-full"
                  value={refToValue(selection.title)}
                  onChange={(v) => handleSelect("title", v)}
                  options={modelOptions}
                  placeholder="Select a model"
                  showSearch
                  notFoundContent="Fetch models on a provider first"
                />
              </div>
            </div>

            {/* Context engine (opt-in semantic + graph search) */}
            <div className="flex items-center justify-between mt-8 mb-1">
              <h2 className="text-base font-semibold text-[var(--text-primary)]">
                Semantic search
              </h2>
              <Switch checked={contextEngine.enabled} onChange={toggleContextEngine} />
            </div>
            <p className="text-sm text-[var(--text-secondary)] mb-4 leading-relaxed">
            Lets the agent search your code by meaning and relationships, not just keywords.
            </p>
            {engineMode === "user" && (
              <p className="text-sm text-[var(--text-secondary)] mb-4 leading-relaxed">
              Requires a local
              backend — run <code>docker compose up -d</code> in <code>services/context-engine</code>.
              </p>
            )}

            {/* User-managed: connect to a self-run backend by URL. */}
            {contextEngine.enabled && engineMode === "user" && (
              <div className="flex flex-col gap-5 border border-[var(--border)] rounded-lg p-5 mb-8">
                {/* Connection */}
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">Backend URL</label>
                  <div className="flex gap-2">
                    <Input
                      className="flex-1"
                      placeholder="http://127.0.0.1:8106"
                      value={contextEngine.base_url}
                      onChange={(e) => {
                        setContextEngine({ ...contextEngine, base_url: e.target.value });
                        setCeStatus(null);
                      }}
                      onPressEnter={connectContextEngine}
                    />
                    <Button
                      type="primary"
                      onClick={connectContextEngine}
                      loading={ceConnecting}
                      style={dark ? { background: "#fff", color: "#131315", borderColor: "#fff" } : undefined}
                    >
                      Connect
                    </Button>
                  </div>
                  <div className="mt-2 text-xs flex items-center gap-1.5">
                    {ceStatus === null ? (
                      <span className="text-[var(--text-secondary)]">○ Not checked</span>
                    ) : ceStatus.connected ? (
                      <span className="text-green-500">● Connected</span>
                    ) : (
                      <span className="text-red-500">
                        ● Not connected{ceStatus.error ? ` — ${ceStatus.error}` : ""}
                      </span>
                    )}
                  </div>
                </div>

                {reposSection}
              </div>
            )}

            {/* App-managed: the app runs the docker stack itself. */}
            {contextEngine.enabled && engineMode === "app" && (
              <div className="flex flex-col gap-5 border border-[var(--border)] rounded-lg p-5 mb-8">
                {/* Embedding key (injected into the stack's process env). */}
                <div>
                  <label className="block text-xs text-[var(--text-secondary)] mb-1">
                    OpenAI embedding API key {hasKey && <span className="text-green-500">· saved</span>}
                  </label>
                  <div className="flex gap-2">
                    <Input.Password
                      className="flex-1"
                      placeholder={hasKey ? "•••••••• (stored) — enter to replace" : "sk-…"}
                      value={keyInput}
                      onChange={(e) => setKeyInput(e.target.value)}
                      onPressEnter={saveAndRestart}
                    />
                    <Button
                      type="primary"
                      onClick={saveAndRestart}
                      loading={savingKey}
                      disabled={!keyInput.trim() && !hasKey}
                      style={dark ? { background: "#fff", color: "#131315", borderColor: "#fff" } : undefined}
                    >
                      {engineStatus?.state === "running" ? "Save & Restart" : "Save & Start"}
                    </Button>
                  </div>
                  <div className="mt-1.5 text-[11px] text-[var(--text-secondary)]">
                    Used only for embeddings. Stored locally and injected into the engine — never written to disk as a file.
                  </div>
                </div>

                {/* Lifecycle controls + status */}
                <div className="border-t border-[var(--border)] pt-4">
                  <div className="flex items-center justify-between gap-3">
                    <div className="text-xs flex items-center gap-1.5 min-w-0">
                      {(() => {
                        const st = engineStatus?.state;
                        if (st === "running")
                          return <span className="text-green-500">● Running</span>;
                        if (st === "pulling" || st === "starting")
                          return (
                            <span className="text-[var(--text-secondary)] flex items-center">
                              <Spin size="small" className="mr-1.5" />
                              {st === "pulling" ? "Pulling & starting…" : "Waiting for engine…"}
                            </span>
                          );
                        if (st === "docker_missing")
                          return (
                            <span className="text-red-500 truncate" title={engineStatus?.state === "docker_missing" ? engineStatus.reason : undefined}>
                              ● Docker unavailable
                            </span>
                          );
                        if (st === "error")
                          return (
                            <span className="text-red-500 truncate" title={engineStatus?.state === "error" ? engineStatus.reason : undefined}>
                              ● Error{engineStatus?.state === "error" ? ` — ${engineStatus.reason}` : ""}
                            </span>
                          );
                        return <span className="text-[var(--text-secondary)]">○ Stopped</span>;
                      })()}
                    </div>
                    <div className="flex gap-2 shrink-0">
                      {engineStatus?.state === "running" ? (
                        <Button size="small" onClick={stopEngine} loading={engineBusy}>
                          Stop
                        </Button>
                      ) : (
                        <Button
                          size="small"
                          type="primary"
                          onClick={startEngine}
                          loading={engineBusy || engineStatus?.state === "pulling" || engineStatus?.state === "starting"}
                          disabled={!hasKey}
                          style={dark ? { background: "#fff", color: "#131315", borderColor: "#fff" } : undefined}
                        >
                          {engineStatus?.state === "error" || engineStatus?.state === "docker_missing" ? "Retry" : "Start"}
                        </Button>
                      )}
                    </div>
                  </div>

                  {!hasKey && (
                    <div className="mt-2 text-[11px] text-amber-500">Add an embedding key above to start the engine.</div>
                  )}
                  {engineStatus?.state === "docker_missing" && (
                    <div className="mt-2 text-[11px] text-[var(--text-secondary)]">
                      {engineStatus.reason}{" "}
                      <a
                        className="text-blue-500 underline"
                        href="https://docs.docker.com/get-docker/"
                        target="_blank"
                        rel="noreferrer"
                      >
                        Install Docker
                      </a>
                    </div>
                  )}
                  {(engineStatus?.state === "pulling" || engineStatus?.state === "starting") && engineProgress && (
                    <div className="mt-2 text-[11px] text-[var(--text-secondary)] font-mono truncate" title={engineProgress}>
                      {engineProgress}
                    </div>
                  )}
                  {engineStatus?.state === "error" && engineStatus.logs_tail && (
                    <pre className="mt-2 text-[10px] leading-4 text-[var(--text-secondary)] bg-[var(--bg-secondary)] rounded p-2 max-h-32 overflow-auto whitespace-pre-wrap">
                      {engineStatus.logs_tail.split("\n").slice(-12).join("\n")}
                    </pre>
                  )}
                </div>

                {reposSection}

                {/* Destructive cleanup */}
                <div className="border-t border-[var(--border)] pt-4 flex items-center justify-between">
                  <div className="text-xs text-[var(--text-secondary)]">
                    Stop & delete all indexed data (drops the engine's volumes).
                  </div>
                  <Popconfirm
                    title="Remove all indexed data?"
                    description="Tears the stack down and deletes every indexed repo. Cannot be undone."
                    okText="Remove"
                    okButtonProps={{ danger: true }}
                    onConfirm={removeEngineData}
                  >
                    <Button size="small" danger loading={engineBusy}>
                      Remove indexed data
                    </Button>
                  </Popconfirm>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
