fn main() {
    // Load environment-specific .env file (shared with Vite frontend).
    // APP_ENV is set by npm scripts or defaults to "development".
    let mode = std::env::var("APP_ENV").unwrap_or_else(|_| "development".to_string());
    let env_file = format!("../.env.{}", mode);

    // Load mode-specific file first, then base .env as fallback
    let _ = dotenvy::from_filename(&env_file);
    let _ = dotenvy::from_filename("../.env");

    // Forward VITE_* config vars to the Rust compiler so they're
    // available via env!() macro at compile time
    let config_keys = ["VITE_APP_ENV", "LLM_BASE_URL"];

    for key in &config_keys {
        if let Ok(val) = std::env::var(key) {
            println!("cargo:rustc-env={}={}", key, val);
        }
    }

    // Re-run build.rs if env files change
    println!("cargo:rerun-if-changed=../.env");
    println!("cargo:rerun-if-changed=../.env.{}", mode);
    println!("cargo:rerun-if-changed=../.env.local");
    println!("cargo:rerun-if-env-changed=APP_ENV");

    tauri_build::build()
}
