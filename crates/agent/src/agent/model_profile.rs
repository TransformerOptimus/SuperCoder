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
}

/// Registry of known model profiles.
/// Initialized with hardcoded defaults, optionally refreshed from the gateway.
pub struct ModelRegistry {
    profiles: HashMap<String, ModelProfile>,
}

impl ModelRegistry {
    /// Create a registry with hardcoded fallback profiles.
    pub fn with_defaults() -> Self {
        let profiles = default_profiles()
            .into_iter()
            .map(|p| (p.id.clone(), p))
            .collect();
        Self { profiles }
    }

    /// Create an empty registry (for tests).
    pub fn empty() -> Self {
        Self { profiles: HashMap::new() }
    }

    /// Merge profiles from a gateway `GET /v1/models` response.
    /// Overwrites existing entries by ID, adds new ones.
    pub fn merge(&mut self, models: Vec<ModelProfile>) {
        for m in models {
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

    /// List all profiles (for frontend model picker).
    pub fn list(&self) -> Vec<&ModelProfile> {
        let mut profiles: Vec<_> = self.profiles.values().collect();
        profiles.sort_by(|a, b| a.id.cmp(&b.id));
        profiles
    }
}

/// Conservative fallback context window for unknown models.
const DEFAULT_CONTEXT_WINDOW: usize = 128_000;

/// Gateway response wrapper for deserialization.
#[derive(Debug, Deserialize)]
pub struct ModelsResponse {
    pub models: Vec<ModelProfile>,
}

fn default_profiles() -> Vec<ModelProfile> {
    vec![
        // Anthropic
        ModelProfile {
            id: "claude-sonnet-4-6".into(),
            display_name: "Claude Sonnet 4.6".into(),
            provider: "anthropic".into(),
            context_window: 200_000,
        },
        ModelProfile {
            id: "claude-opus-4-6".into(),
            display_name: "Claude Opus 4.6".into(),
            provider: "anthropic".into(),
            context_window: 200_000,
        },
        ModelProfile {
            id: "claude-haiku-4-5-20251001".into(),
            display_name: "Claude Haiku 4.5".into(),
            provider: "anthropic".into(),
            context_window: 200_000,
        },
        // OpenAI
        ModelProfile {
            id: "gpt-4o".into(),
            display_name: "GPT-4o".into(),
            provider: "openai".into(),
            context_window: 128_000,
        },
        ModelProfile {
            id: "gpt-4o-mini".into(),
            display_name: "GPT-4o Mini".into(),
            provider: "openai".into(),
            context_window: 128_000,
        },
        ModelProfile {
            id: "o3".into(),
            display_name: "O3".into(),
            provider: "openai".into(),
            context_window: 200_000,
        },
        ModelProfile {
            id: "o4-mini".into(),
            display_name: "O4 Mini".into(),
            provider: "openai".into(),
            context_window: 200_000,
        },
        ModelProfile {
            id: "gpt-5.4-2026-03-05".into(),
            display_name: "GPT-5.4".into(),
            provider: "openai".into(),
            context_window: 200_000,
        },
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_registry_has_all_models() {
        let reg = ModelRegistry::with_defaults();
        assert!(reg.get("claude-sonnet-4-6").is_some());
        assert!(reg.get("claude-opus-4-6").is_some());
        assert!(reg.get("claude-haiku-4-5-20251001").is_some());
        assert!(reg.get("gpt-4o").is_some());
        assert!(reg.get("gpt-4o-mini").is_some());
        assert!(reg.get("o3").is_some());
        assert!(reg.get("o4-mini").is_some());
        assert!(reg.get("gpt-5.4-2026-03-05").is_some());
    }

    #[test]
    fn test_get_known_model() {
        let reg = ModelRegistry::with_defaults();
        let profile = reg.get("claude-sonnet-4-6").unwrap();
        assert_eq!(profile.context_window, 200_000);
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
        assert_eq!(reg.context_window_for("claude-sonnet-4-6"), 200_000);
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
        }]);

        assert_eq!(reg.context_window_for("gpt-4o-mini"), 256_000);
        assert_eq!(reg.get("gpt-4o-mini").unwrap().display_name, "GPT-4o Mini (Updated)");
    }

    #[test]
    fn test_merge_adds_new() {
        let mut reg = ModelRegistry::with_defaults();
        assert!(reg.get("gpt-5").is_none());

        reg.merge(vec![ModelProfile {
            id: "gpt-5".into(),
            display_name: "GPT-5".into(),
            provider: "openai".into(),
            context_window: 1_000_000,
        }]);

        assert_eq!(reg.context_window_for("gpt-5"), 1_000_000);
    }

    #[test]
    fn test_list_sorted_by_id() {
        let reg = ModelRegistry::with_defaults();
        let list = reg.list();
        let ids: Vec<&str> = list.iter().map(|p| p.id.as_str()).collect();
        let mut sorted = ids.clone();
        sorted.sort();
        assert_eq!(ids, sorted);
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
