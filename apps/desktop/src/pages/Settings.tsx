import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Input, Button, Select, Spin, Popconfirm, Switch } from "antd";
import { ArrowLeft, Plus, Pencil, Trash2 } from "lucide-react";
import { agentTauriService } from "@/services/agentTauriService";
import { useAppStore } from "@/store";
import { themedMessage } from "@/providers/AntDThemeProvider";
import type {
  ContextEngineSettings,
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

/** Known models for the built-in providers — the user picks the subset to use.
 * (OpenAI-compatible providers fetch their list live instead.) */
const CURATED_MODELS: Record<string, string[]> = {
  // GPT-5.x family + 4.1/4o (and their minis), verified against /v1/chat/completions.
  // Excludes codex + pro variants (those are /v1/responses-only) and the o-series.
  openai: [
    "gpt-5.5",
    "gpt-5.4",
    "gpt-5.4-mini",
    "gpt-5.4-nano",
    "gpt-5.3-chat-latest",
    "gpt-5.2",
    "gpt-5.1",
    "gpt-5",
    "gpt-5-mini",
    "gpt-5-nano",
    "gpt-4.1",
    "gpt-4.1-mini",
    "gpt-4o",
    "gpt-4o-mini",
  ],
  anthropic: [
    "claude-opus-4-8",
    "claude-opus-4-7",
    "claude-opus-4-6",
    "claude-sonnet-4-6",
    "claude-haiku-4-5",
  ],
};

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
  const loadProviders = useAppStore((s) => s.loadProviders);

  const [providers, setProviders] = useState<ProviderConfig[]>([]);
  const [selection, setSelection] = useState<ModelSelection>({ active: null, compaction: null, title: null });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  // Provider being added/edited (null = list view).
  const [draft, setDraft] = useState<ProviderConfig | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);
  const [contextEngine, setContextEngine] = useState<ContextEngineSettings>({ enabled: false, port: 8106 });

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
        setContextEngine(await agentTauriService.getContextEngine());
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
        const merged = Array.from(new Set([...draft.models, ...list]));
        setDraft({ ...draft, models: merged });
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
                  options={(CURATED_MODELS[draft.kind] ?? []).map((m) => ({ label: m, value: m }))}
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
            <h2 className="text-base font-semibold text-[var(--text-primary)] mt-8 mb-1">
              Context engine (semantic search)
            </h2>
            <p className="text-sm text-[var(--text-secondary)] mb-4">
              Optional. Runs a local Docker stack that indexes the open repo for semantic + graph code
              search (the agent's <code>codebase_search</code> / <code>codebase_graph</code> tools). The
              app stays zero-backend when off.
            </p>
            <div className="flex flex-col gap-4 border border-[var(--border)] rounded-lg p-5 mb-8">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">Enable context engine</div>
                  <div className="text-xs text-[var(--text-secondary)]">
                    Streams the open repo for indexing on session start.
                  </div>
                </div>
                <Switch
                  checked={contextEngine.enabled}
                  onChange={(v) => saveContextEngine({ ...contextEngine, enabled: v })}
                />
              </div>
              {contextEngine.enabled && (
                <>
                  <div>
                    <label className="block text-xs text-[var(--text-secondary)] mb-1">Port</label>
                    <Input
                      className="w-32"
                      type="number"
                      value={contextEngine.port}
                      onChange={(e) =>
                        setContextEngine({ ...contextEngine, port: Number(e.target.value) || 8106 })
                      }
                      onBlur={() => saveContextEngine(contextEngine)}
                    />
                  </div>
                  <div className="text-xs text-[var(--text-secondary)] leading-relaxed">
                    Setup — run the stack from <code>services/context-engine</code>:
                    <pre className="mt-1 p-2 rounded bg-[var(--border)] text-[var(--text-primary)] overflow-x-auto">
cp .env.example .env   # set SUPERAGI_OPENAI_API_KEY
docker compose up -d --build</pre>
                    The app connects to <code>http://127.0.0.1:{contextEngine.port}</code>.
                  </div>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
