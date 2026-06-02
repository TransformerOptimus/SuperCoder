import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Input, Button, Select, Spin } from "antd";
import { ArrowLeft } from "lucide-react";
import { agentTauriService } from "@/services/agentTauriService";
import { useAppStore } from "@/store";
import { themedMessage } from "@/providers/AntDThemeProvider";
import { GATEWAY_URL } from "@/config";
import type { ModelProfile } from "@/types/agent";

export default function Settings() {
  const navigate = useNavigate();
  const setSelectedModel = useAppStore((s) => s.setSelectedModel);

  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [model, setModel] = useState("");
  const [models, setModels] = useState<ModelProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fetchingModels, setFetchingModels] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const cfg = await agentTauriService.getLlmConfig();
        setBaseUrl(cfg.baseUrl);
        setApiKey(cfg.apiKey);
        setModel(cfg.model);
      } catch (err) {
        console.error("[Settings] Failed to load config:", err);
      } finally {
        setLoading(false);
      }
      try {
        const list = await agentTauriService.fetchModels();
        setModels(list);
      } catch {
        /* models optional */
      }
    })();
  }, []);

  const handleFetchModels = async () => {
    setFetchingModels(true);
    try {
      // Save first so the backend uses the latest base_url/key when listing models.
      await agentTauriService.saveLlmConfig({ baseUrl, apiKey, model });
      const list = await agentTauriService.fetchModels();
      setModels(list);
      if (list.length > 0) themedMessage.success(`Found ${list.length} models`);
      else themedMessage.info("No models returned by this endpoint");
    } catch (err) {
      console.error("[Settings] fetchModels failed:", err);
      themedMessage.error("Failed to fetch models — check the base URL and key");
    } finally {
      setFetchingModels(false);
    }
  };

  const handleSave = async () => {
    if (!baseUrl.trim()) {
      themedMessage.warning("Base URL is required");
      return;
    }
    setSaving(true);
    try {
      await agentTauriService.saveLlmConfig({ baseUrl: baseUrl.trim(), apiKey: apiKey.trim(), model });
      if (model) setSelectedModel(model);
      // Refresh the in-memory model list/selection used by the composer.
      useAppStore.getState().loadModels();
      themedMessage.success("Settings saved");
    } catch (err) {
      console.error("[Settings] save failed:", err);
      themedMessage.error("Failed to save settings");
    } finally {
      setSaving(false);
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

        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-1">LLM Settings</h1>
        <p className="text-sm text-[var(--text-secondary)] mb-6">
          Use any OpenAI-compatible endpoint. Leave it on the local gateway
          (<code className="text-[var(--text-primary)]">{GATEWAY_URL}</code>) or point it at your own.
        </p>

        <div className="flex flex-col gap-5">
          <div>
            <label className="block text-xs text-[var(--text-secondary)] mb-1">Base URL</label>
            <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder={GATEWAY_URL} />
            <button
              onClick={() => setBaseUrl(GATEWAY_URL)}
              className="text-[11px] text-blue-500 hover:underline mt-1"
            >
              Use local gateway
            </button>
          </div>

          <div>
            <label className="block text-xs text-[var(--text-secondary)] mb-1">API Key</label>
            <Input.Password value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-…" />
          </div>

          <div>
            <label className="block text-xs text-[var(--text-secondary)] mb-1">Model</label>
            <div className="flex gap-2">
              <Select
                className="flex-1"
                value={model || undefined}
                onChange={setModel}
                placeholder="Select a model"
                showSearch
                options={models.map((m) => ({ label: m.display_name || m.id, value: m.id }))}
                notFoundContent={models.length === 0 ? "Fetch models or type a model id" : undefined}
              />
              <Button onClick={handleFetchModels} loading={fetchingModels}>
                Fetch
              </Button>
            </div>
            <Input
              className="mt-2"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="…or enter a model id manually"
            />
          </div>

          <div>
            <Button type="primary" onClick={handleSave} loading={saving}>
              Save
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
