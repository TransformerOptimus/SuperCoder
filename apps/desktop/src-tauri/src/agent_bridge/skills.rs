use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use agent::skills::registry::{read_tier_from_fs, SkillInput};
use agent::skills::{SkillRegistry, DEFAULT_SKILLS};

use super::db::AgentDb;

/// Resolve the user-global skills directory. Matches the `dirs::data_local_dir()`
/// pattern used for `agent_data.db` so user-global skills live next to the DB at
/// e.g. macOS: `~/Library/Application Support/.supercoder/skills/`.
pub fn global_skills_dir() -> PathBuf {
    crate::app_data_dir().join("skills")
}

/// Project-scoped skills directory: `<cwd>/.agent/skills/`.
pub fn project_skills_dir(working_dir: &Path) -> PathBuf {
    working_dir.join(".agent").join("skills")
}

/// Replace the user's home-dir prefix with `~` for display. Falls back to the
/// full path if HOME isn't set or the path doesn't live under home.
///
/// Uses `Path::strip_prefix` rather than string-level `str::strip_prefix` so a
/// home of `/Users/alice` doesn't match a path like `/Users/alice2/foo`
/// (which the byte-level match would collapse to `~2/foo`).
fn home_shorten(path: &Path) -> String {
    if let Some(home) = dirs::home_dir() {
        if let Ok(rest) = path.strip_prefix(&home) {
            let rest_str = rest.display().to_string();
            if rest_str.is_empty() {
                return "~".to_string();
            }
            return format!("~/{rest_str}");
        }
    }
    path.display().to_string()
}

/// Paths returned to the /skills dialog so the empty-state can show the user
/// exactly where to drop a SKILL.md folder. Both paths are home-shortened for
/// display.
#[derive(Debug, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SkillsPaths {
    pub global: String,
    pub project: Option<String>,
}

/// Compute the display paths. Project is populated only when a working dir is
/// supplied.
pub fn paths_for_display(working_dir: Option<&Path>) -> SkillsPaths {
    SkillsPaths {
        global: home_shorten(&global_skills_dir()),
        project: working_dir.map(|wd| home_shorten(&project_skills_dir(wd))),
    }
}

/// Pull embedded default skills (compile-time `include_dir!`).
fn read_default_tier() -> Vec<SkillInput> {
    DEFAULT_SKILLS
        .dirs()
        .filter_map(|d| {
            let skill_md = d.path().join("SKILL.md");
            DEFAULT_SKILLS
                .get_file(&skill_md)
                .map(|f| SkillInput {
                    raw: String::from_utf8_lossy(f.contents()).into_owned(),
                    path: PathBuf::from(format!("defaults://{}", d.path().display())),
                })
        })
        .collect()
}

/// Build a `SkillRegistry` from the three tiers (default → global → project),
/// filtering out skills the user has toggled off. Returns `None` if the
/// resulting registry is empty — callers treat that as "no skills present,
/// don't touch the prompt or register the tool."
pub fn build_registry_for_agent(
    agent_db: &AgentDb,
    working_dir: &Path,
) -> Option<Arc<SkillRegistry>> {
    let disabled: HashSet<String> = agent_db.load_disabled_skills().unwrap_or_else(|e| {
        log::warn!("[skills] failed to load disabled skills, treating all as enabled: {e}");
        HashSet::new()
    });

    let default = read_default_tier();
    let global_root = global_skills_dir();
    let project_root = project_skills_dir(working_dir);
    let global = read_tier_from_fs(&global_root);
    let project = read_tier_from_fs(&project_root);

    log::info!(
        "[skills] build_registry_for_agent: default={} global={} (from {:?}) project={} (from {:?}) disabled={:?}",
        default.len(),
        global.len(),
        global_root,
        project.len(),
        project_root,
        disabled,
    );

    let registry = SkillRegistry::new(default, global, project, &disabled);
    if registry.is_empty() {
        log::info!("[skills] build_registry_for_agent: empty registry — skills feature inactive this spawn");
        None
    } else {
        let names: Vec<String> = registry.names();
        log::info!(
            "[skills] build_registry_for_agent: registry active, {} skill(s) loaded: {:?}",
            registry.len(),
            names,
        );
        Some(Arc::new(registry))
    }
}

/// Enumerate all discovered skills across all tiers for the `/skills` dialog,
/// including those toggled off. The `enabled` flag reflects current user
/// preference.
pub fn list_all_for_dialog(
    agent_db: &AgentDb,
    working_dir: Option<&Path>,
) -> Vec<DialogEntry> {
    let disabled = agent_db.load_disabled_skills().unwrap_or_default();
    let default = read_default_tier();
    let global_root = global_skills_dir();
    let global = read_tier_from_fs(&global_root);
    let project = working_dir
        .map(|wd| read_tier_from_fs(&project_skills_dir(wd)))
        .unwrap_or_default();

    log::info!(
        "[skills] list_all_for_dialog: default={} global={} (from {:?}) project={} working_dir={:?} disabled={:?}",
        default.len(),
        global.len(),
        global_root,
        project.len(),
        working_dir,
        disabled,
    );

    // Build an empty-disabled registry purely for its precedence/parse logic —
    // the dialog shows whatever precedence would win at agent spawn time.
    let resolved = SkillRegistry::new(default, global, project, &HashSet::new());
    let entries: Vec<DialogEntry> = resolved
        .all()
        .into_iter()
        .map(|s| DialogEntry {
            name: s.name.clone(),
            description: s.description.clone(),
            origin: match s.origin {
                agent::skills::Origin::Default => "default",
                agent::skills::Origin::Global => "global",
                agent::skills::Origin::Project => "project",
            }
            .to_string(),
            enabled: !disabled.contains(&s.name),
            path: s.path.display().to_string(),
        })
        .collect();
    log::info!(
        "[skills] list_all_for_dialog: returning {} entries to frontend",
        entries.len()
    );
    entries
}

/// Flat struct returned to the frontend for the `/skills` dialog.
#[derive(Debug, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DialogEntry {
    pub name: String,
    pub description: String,
    pub origin: String,
    pub enabled: bool,
    pub path: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    // `home_shorten` uses `dirs::home_dir()` at runtime which we can't override
    // per-test; the test below exercises the path-component boundary logic
    // directly by inlining a clone of the function with an explicit home arg.
    fn home_shorten_with(path: &Path, home: &Path) -> String {
        if let Ok(rest) = path.strip_prefix(home) {
            let rest_str = rest.display().to_string();
            if rest_str.is_empty() {
                return "~".to_string();
            }
            return format!("~/{rest_str}");
        }
        path.display().to_string()
    }

    #[test]
    fn home_shorten_boundary_respects_path_components() {
        let home = Path::new("/Users/alice");
        // Prefix-collision case that the old `str::strip_prefix` would garble.
        assert_eq!(
            home_shorten_with(Path::new("/Users/alice2/foo"), home),
            "/Users/alice2/foo"
        );
        // Real home match.
        assert_eq!(
            home_shorten_with(Path::new("/Users/alice/Library/foo"), home),
            "~/Library/foo"
        );
        // Path IS home itself.
        assert_eq!(home_shorten_with(home, home), "~");
        // Path outside home, no collision.
        assert_eq!(
            home_shorten_with(Path::new("/tmp/skills"), home),
            "/tmp/skills"
        );
    }
}
