use std::collections::HashMap;
use serde::{Deserialize, Serialize};

/// Minimal model metadata — only what the agent crate needs for correct behavior.
/// Display info (display_name, provider) is for the frontend model picker.
/// The only field that affects agent logic is `context_window` (drives compaction).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModelProfile {
    pub id: String,
    pub display_name: String,
    pub provider: String,
    pub context_window: usize,
    /// Whether the model accepts image inputs (vision). Defaulted so older
    /// `merge()` / `ModelsResponse` JSON without the field still deserializes.
    #[serde(default)]
    pub supports_images: bool,
}

/// Registry of known model profiles.
/// Initialized with hardcoded defaults, optionally refreshed from the gateway.
/// Insertion order is preserved (`order`) so the model picker lists models
/// newest-first as declared in `default_profiles()`, not alphabetically.
pub struct ModelRegistry {
    profiles: HashMap<String, ModelProfile>,
    order: Vec<String>,
}

impl ModelRegistry {
    /// Create a registry with hardcoded fallback profiles.
    pub fn with_defaults() -> Self {
        let mut reg = Self::empty();
        reg.merge(default_profiles());
        reg
    }

    /// Create an empty registry (for tests).
    pub fn empty() -> Self {
        Self { profiles: HashMap::new(), order: Vec::new() }
    }

    /// Merge profiles from a gateway `GET /v1/models` response.
    /// Overwrites existing entries by ID, appends new ones (preserving order).
    pub fn merge(&mut self, models: Vec<ModelProfile>) {
        for m in models {
            if !self.profiles.contains_key(&m.id) {
                self.order.push(m.id.clone());
            }
            self.profiles.insert(m.id.clone(), m);
        }
    }

    /// Look up a profile by exact model ID.
    pub fn get(&self, model_id: &str) -> Option<&ModelProfile> {
        self.profiles.get(model_id)
    }

    /// Get the context window for a model, with a conservative fallback for unknown models.
    pub fn context_window_for(&self, model_id: &str) -> usize {
        self.get(model_id)
            .map(|p| p.context_window)
            .unwrap_or(DEFAULT_CONTEXT_WINDOW)
    }

    /// List all profiles in declaration/insertion order (newest-first for the picker).
    pub fn list(&self) -> Vec<&ModelProfile> {
        self.order.iter().filter_map(|id| self.profiles.get(id)).collect()
    }
}

/// Conservative fallback context window for unknown models.
const DEFAULT_CONTEXT_WINDOW: usize = 128_000;

/// Gateway response wrapper for deserialization.
#[derive(Debug, Deserialize)]
pub struct ModelsResponse {
    pub models: Vec<ModelProfile>,
}

/// Helper to keep the (long) default list terse and readable.
fn profile(id: &str, name: &str, provider: &str, ctx: usize, vision: bool) -> ModelProfile {
    ModelProfile {
        id: id.into(),
        display_name: name.into(),
        provider: provider.into(),
        context_window: ctx,
        supports_images: vision,
    }
}

/// The single authoritative list of built-in models. Drives both the
/// context-window sizing (compaction / context bar) AND the Settings model
/// picker (exposed to the frontend via `agent_list_models`). All entries are
/// vision-capable. Context windows verified against provider docs (June 2026).
fn default_profiles() -> Vec<ModelProfile> {
    vec![
        // ── Anthropic ──────────────────────────────────────────────────────
        // Opus/Sonnet 4.6+ ship a 1M context window; Haiku 4.5 caps at 200k.
        profile("claude-opus-4-8", "Claude Opus 4.8", "anthropic", 1_000_000, true),
        profile("claude-opus-4-7", "Claude Opus 4.7", "anthropic", 1_000_000, true),
        profile("claude-opus-4-6", "Claude Opus 4.6", "anthropic", 1_000_000, true),
        profile("claude-sonnet-4-6", "Claude Sonnet 4.6", "anthropic", 1_000_000, true),
        profile("claude-haiku-4-5", "Claude Haiku 4.5", "anthropic", 200_000, true),
        // ── OpenAI ─────────────────────────────────────────────────────────
        // GPT-5.5 + GPT-5.4 (standard) and GPT-4.1 are 1M. The GPT-5.4 mini/nano
        // variants and all of GPT-5.0–5.2 are 400k. GPT-4o is 128k.
        profile("gpt-5.5", "GPT-5.5", "openai", 1_000_000, true),
        profile("gpt-5.4", "GPT-5.4", "openai", 1_000_000, true),
        profile("gpt-5.4-mini", "GPT-5.4 Mini", "openai", 400_000, true),
        profile("gpt-5.4-nano", "GPT-5.4 Nano", "openai", 400_000, true),
        profile("gpt-5.2", "GPT-5.2", "openai", 400_000, true),
        profile("gpt-5.1", "GPT-5.1", "openai", 400_000, true),
        profile("gpt-5", "GPT-5", "openai", 400_000, true),
        profile("gpt-5-mini", "GPT-5 Mini", "openai", 400_000, true),
        profile("gpt-5-nano", "GPT-5 Nano", "openai", 400_000, true),
        profile("gpt-4.1", "GPT-4.1", "openai", 1_000_000, true),
        profile("gpt-4.1-mini", "GPT-4.1 Mini", "openai", 1_000_000, true),
        profile("gpt-4o", "GPT-4o", "openai", 128_000, true),
        profile("gpt-4o-mini", "GPT-4o Mini", "openai", 128_000, true),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_registry_has_all_models() {
        let reg = ModelRegistry::with_defaults();
        assert!(reg.get("claude-sonnet-4-6").is_some());
        assert!(reg.get("claude-opus-4-8").is_some());
        assert!(reg.get("claude-opus-4-6").is_some());
        assert!(reg.get("claude-haiku-4-5").is_some());
        assert!(reg.get("gpt-4o").is_some());
        assert!(reg.get("gpt-4o-mini").is_some());
        assert!(reg.get("gpt-5.5").is_some());
        assert!(reg.get("gpt-5.4").is_some());
        assert!(reg.get("gpt-4.1").is_some());
        assert!(reg.get("claude-opus-4-7").is_some());
    }

    #[test]
    fn test_get_known_model() {
        let reg = ModelRegistry::with_defaults();
        let profile = reg.get("claude-sonnet-4-6").unwrap();
        assert_eq!(profile.context_window, 1_000_000);
        assert_eq!(profile.provider, "anthropic");
        assert_eq!(profile.display_name, "Claude Sonnet 4.6");
    }

    #[test]
    fn test_get_unknown_returns_none() {
        let reg = ModelRegistry::with_defaults();
        assert!(reg.get("some-unknown-model").is_none());
    }

    #[test]
    fn test_context_window_for_known() {
        let reg = ModelRegistry::with_defaults();
        assert_eq!(reg.context_window_for("claude-sonnet-4-6"), 1_000_000);
        assert_eq!(reg.context_window_for("gpt-4o-mini"), 128_000);
    }

    #[test]
    fn test_context_window_for_unknown_returns_fallback() {
        let reg = ModelRegistry::with_defaults();
        assert_eq!(reg.context_window_for("mystery-model"), DEFAULT_CONTEXT_WINDOW);
    }

    #[test]
    fn test_merge_overwrites_existing() {
        let mut reg = ModelRegistry::with_defaults();
        assert_eq!(reg.context_window_for("gpt-4o-mini"), 128_000);

        reg.merge(vec![ModelProfile {
            id: "gpt-4o-mini".into(),
            display_name: "GPT-4o Mini (Updated)".into(),
            provider: "openai".into(),
            context_window: 256_000, // gateway says it grew
            supports_images: true,
        }]);

        assert_eq!(reg.context_window_for("gpt-4o-mini"), 256_000);
        assert_eq!(reg.get("gpt-4o-mini").unwrap().display_name, "GPT-4o Mini (Updated)");
    }

    #[test]
    fn test_merge_adds_new() {
        let mut reg = ModelRegistry::with_defaults();
        assert!(reg.get("future-model-x").is_none());

        reg.merge(vec![ModelProfile {
            id: "future-model-x".into(),
            display_name: "Future Model X".into(),
            provider: "openai".into(),
            context_window: 2_000_000,
            supports_images: false,
        }]);

        assert_eq!(reg.context_window_for("future-model-x"), 2_000_000);
    }

    #[test]
    fn test_list_preserves_declaration_order() {
        let reg = ModelRegistry::with_defaults();
        let ids: Vec<&str> = reg.list().iter().map(|p| p.id.as_str()).collect();
        // Newest-first as declared in default_profiles(): Opus 4.8 leads, the
        // legacy gpt-4o-mini trails. Not alphabetical.
        assert_eq!(ids.first(), Some(&"claude-opus-4-8"));
        assert_eq!(ids.last(), Some(&"gpt-4o-mini"));
        // gpt-5.5 must precede gpt-5.4 (newer first), which alphabetical sort would invert.
        let pos = |needle: &str| ids.iter().position(|id| *id == needle).unwrap();
        assert!(pos("gpt-5.5") < pos("gpt-5.4"));
        assert!(pos("gpt-5.4") < pos("gpt-4o"));
    }

    #[test]
    fn test_merge_appends_new_in_order() {
        let mut reg = ModelRegistry::empty();
        reg.merge(vec![
            profile("a", "A", "openai", 100, false),
            profile("b", "B", "openai", 100, false),
        ]);
        // Re-merging "a" (overwrite) must not move it or duplicate it.
        reg.merge(vec![profile("a", "A2", "openai", 200, false)]);
        reg.merge(vec![profile("c", "C", "openai", 100, false)]);
        let ids: Vec<&str> = reg.list().iter().map(|p| p.id.as_str()).collect();
        assert_eq!(ids, vec!["a", "b", "c"]);
        assert_eq!(reg.get("a").unwrap().display_name, "A2");
    }

    #[test]
    fn test_empty_registry() {
        let reg = ModelRegistry::empty();
        assert!(reg.get("claude-sonnet-4-6").is_none());
        assert_eq!(reg.context_window_for("anything"), DEFAULT_CONTEXT_WINDOW);
        assert!(reg.list().is_empty());
    }

    #[test]
    fn test_models_response_deserialization() {
        let json = r#"{
            "models": [
                {
                    "id": "test-model",
                    "display_name": "Test Model",
                    "provider": "test",
                    "context_window": 32000
                }
            ]
        }"#;
        let resp: ModelsResponse = serde_json::from_str(json).unwrap();
        assert_eq!(resp.models.len(), 1);
        assert_eq!(resp.models[0].id, "test-model");
        assert_eq!(resp.models[0].context_window, 32000);
    }
}
