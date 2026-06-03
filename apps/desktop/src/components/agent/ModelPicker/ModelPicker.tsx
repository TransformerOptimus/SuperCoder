import { useEffect, useMemo, useCallback } from "react";
import { Bot, ChevronDown } from "lucide-react";
import { useAppStore } from "@/store";
import ActionChip from "@/components/common/ActionChip/ActionChip";
import CustomDropdown from "@/components/common/CustomDropdown/CustomDropdown";
import type { CustomDropdownItem } from "@/components/common/CustomDropdown/types";
import type { ProviderConfig } from "@/types/agent";

/** Display name for a provider group: kind label, or the host for OpenAI-compatible. */
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

/** Picks the coding model across all configured providers (grouped). When a
 * session is open the picker controls THAT session's model (re-pins it); with no
 * session open it sets the global default that new sessions snapshot. */
export default function ModelPicker() {
  const providers = useAppStore((s) => s.providers);
  const selection = useAppStore((s) => s.selection);
  const providersLoaded = useAppStore((s) => s.providersLoaded);
  const loadProviders = useAppStore((s) => s.loadProviders);
  const setActiveModel = useAppStore((s) => s.setActiveModel);
  const setSessionModel = useAppStore((s) => s.setSessionModel);
  const syncPickerToSession = useAppStore((s) => s.syncPickerToSession);
  const activeThreadId = useAppStore((s) => s.activeAgentThreadId);
  // The open session's pinned model (if any) — the picker reflects/controls it.
  const openSession = useAppStore((s) =>
    activeThreadId ? s.sessions.find((x) => x.id === activeThreadId) : undefined,
  );

  useEffect(() => {
    if (!providersLoaded) loadProviders();
  }, [providersLoaded, loadProviders]);

  // When a session opens, align the in-memory active model + vision gating to it.
  useEffect(() => {
    if (activeThreadId) syncPickerToSession(activeThreadId);
  }, [activeThreadId, openSession?.model, openSession?.providerId, syncPickerToSession]);

  const handleSelect = useCallback(
    (providerId: string, model: string) => {
      if (activeThreadId) setSessionModel(activeThreadId, providerId, model);
      else setActiveModel(providerId, model);
    },
    [activeThreadId, setSessionModel, setActiveModel],
  );

  // Flatten providers → models as grouped dropdown items (header + model rows).
  const dropdownItems: CustomDropdownItem[] = useMemo(() => {
    const items: CustomDropdownItem[] = [];
    for (const p of providers) {
      if (p.models.length === 0) continue;
      items.push({ key: `hdr-${p.id}`, label: providerName(p), disabled: true });
      for (const m of p.models) {
        items.push({
          key: `${p.id}::${m}`,
          label: m,
          icon: <Bot className="w-4 h-4 text-gray-500" />,
          onClick: () => handleSelect(p.id, m),
        });
      }
    }
    return items;
  }, [providers, handleSelect]);

  // Show the open session's model when one is open; otherwise the global default.
  const displayLabel = openSession?.model ?? selection.active?.model ?? "Select model";

  return (
    <CustomDropdown
      items={dropdownItems}
      placement="topLeft"
      searchable
      trigger={
        <ActionChip
          label={displayLabel}
          prefix={<Bot className="w-4 h-4 text-gray-500" />}
          suffix={<ChevronDown className="w-4 h-4 text-gray-500" />}
        />
      }
    />
  );
}
