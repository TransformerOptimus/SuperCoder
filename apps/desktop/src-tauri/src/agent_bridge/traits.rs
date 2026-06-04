use serde_json::Value;
use tauri::{AppHandle, Emitter};

// ── EventEmitter ──────────────────────────────────────────────────────────────

/// Abstracts Tauri event emission (`app.emit()`).
/// Returns `Result` so callers can log on failure instead of silent `let _ =`.
pub trait EventEmitter: Send + Sync {
    fn emit(&self, event: &str, payload: Value) -> Result<(), String>;
}

/// Emit an event, logging on failure. Use this everywhere instead of
/// `let _ = app.emit(...)`.
pub fn emit_or_log(emitter: &(impl EventEmitter + ?Sized), event: &str, payload: Value) {
    if let Err(e) = emitter.emit(event, payload) {
        log::warn!("[EventEmitter] Failed to emit '{}': {}", event, e);
    }
}

/// Production implementation wrapping `app.emit()`.
pub struct TauriEventEmitter {
    pub app: AppHandle,
}

impl TauriEventEmitter {
    pub fn new(app: AppHandle) -> Self {
        Self { app }
    }
}

impl EventEmitter for TauriEventEmitter {
    fn emit(&self, event: &str, payload: Value) -> Result<(), String> {
        self.app
            .emit(event, payload)
            .map_err(|e| format!("Failed to emit {event}: {e}"))
    }
}

// ── Mock ──────────────────────────────────────────────────────────────────────

#[cfg(test)]
pub mod mocks {
    use super::*;
    use std::sync::{Arc, Mutex};

    #[derive(Clone, Default)]
    pub struct MockEventEmitter {
        pub events: Arc<Mutex<Vec<(String, Value)>>>,
        should_fail: Arc<Mutex<bool>>,
    }

    impl MockEventEmitter {
        pub fn new() -> Self {
            Self::default()
        }

        pub fn set_should_fail(&self, fail: bool) {
            *self.should_fail.lock().unwrap() = fail;
        }

        pub fn event_count(&self) -> usize {
            self.events.lock().unwrap().len()
        }

        pub fn events_named(&self, name: &str) -> Vec<Value> {
            self.events
                .lock()
                .unwrap()
                .iter()
                .filter(|(n, _)| n == name)
                .map(|(_, v)| v.clone())
                .collect()
        }
    }

    impl EventEmitter for MockEventEmitter {
        fn emit(&self, event: &str, payload: Value) -> Result<(), String> {
            if *self.should_fail.lock().unwrap() {
                return Err(format!("Mock emit failure for {event}"));
            }
            self.events.lock().unwrap().push((event.to_string(), payload));
            Ok(())
        }
    }
}

#[cfg(test)]
mod tests {
    use super::mocks::*;
    use super::*;

    #[test]
    fn test_mock_event_emitter() {
        let emitter = MockEventEmitter::new();
        emitter.emit("agent:test", serde_json::json!({"foo": 1})).unwrap();
        assert_eq!(emitter.event_count(), 1);
        assert_eq!(emitter.events_named("agent:test").len(), 1);
    }

    #[test]
    fn test_emit_or_log_does_not_panic_on_failure() {
        let emitter = MockEventEmitter::new();
        emitter.set_should_fail(true);
        emit_or_log(&emitter, "test", serde_json::json!({}));
        assert_eq!(emitter.event_count(), 0);
    }
}
