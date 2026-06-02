import { useEffect, useMemo, useCallback } from "react";
import { Bot, ChevronDown } from "lucide-react";
import { useAppStore } from "@/store";
import { agentTauriService } from "@/services/agentTauriService";
import ActionChip from "@/components/common/ActionChip/ActionChip";
import CustomDropdown from "@/components/common/CustomDropdown/CustomDropdown";
import type { CustomDropdownItem } from "@/components/common/CustomDropdown/types";

export default function ModelPicker() {
  const availableModels = useAppStore((s) => s.availableModels);
  const selectedModelId = useAppStore((s) => s.selectedModelId);
  const modelsLoaded = useAppStore((s) => s.modelsLoaded);
  const loadModels = useAppStore((s) => s.loadModels);
  const setSelectedModel = useAppStore((s) => s.setSelectedModel);

  useEffect(() => {
    if (!modelsLoaded) {
      loadModels();
    }
  }, [modelsLoaded, loadModels]);

  const handleSelect = useCallback(async (modelId: string) => {
    console.log("[ModelPicker] Selecting model:", modelId);
    setSelectedModel(modelId);
    try {
      const config = await agentTauriService.getLlmConfig();
      console.log("[ModelPicker] Current config:", config);
      await agentTauriService.saveLlmConfig({ ...config, model: modelId });
      console.log("[ModelPicker] Saved model:", modelId);
    } catch (e) {
      console.error("[ModelPicker] Failed to save model:", e);
    }
  }, [setSelectedModel]);

  const dropdownItems: CustomDropdownItem[] = useMemo(
    () =>
      availableModels.map((m) => ({
        key: m.id,
        label: m.display_name,
        icon: <Bot className="w-4 h-4 text-gray-500" />,
        onClick: () => handleSelect(m.id),
      })),
    [availableModels, handleSelect],
  );

  const selected = availableModels.find((m) => m.id === selectedModelId);
  const displayLabel = selected?.display_name ?? selectedModelId ?? "Select model";

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
