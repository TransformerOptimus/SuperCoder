use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use agent::subagents::registry::{read_tier_from_fs, SubagentInput};
use agent::subagents::{SubagentRegistry, DEFAULT_SUBAGENTS};

use super::db::AgentDb;

pub fn global_subagents_dir() -> PathBuf {
    crate::app_data_dir().join("subagents")
}

pub fn project_subagents_dir(working_dir: &Path) -> PathBuf {
    working_dir.join(".agent").join("subagents")
}

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

#[derive(Debug, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SubagentsPaths {
    pub global: String,
    pub project: Option<String>,
}

pub fn paths_for_display(working_dir: Option<&Path>) -> SubagentsPaths {
    SubagentsPaths {
        global: home_shorten(&global_subagents_dir()),
        project: working_dir.map(|wd| home_shorten(&project_subagents_dir(wd))),
    }
}

fn read_default_tier() -> Vec<SubagentInput> {
    DEFAULT_SUBAGENTS
        .dirs()
        .filter_map(|d| {
            let dir_name = d.path().file_name()?.to_str()?;
            let def_md = d.path().join(format!("{dir_name}.md"));
            DEFAULT_SUBAGENTS.get_file(&def_md).map(|f| SubagentInput {
                raw: String::from_utf8_lossy(f.contents()).into_owned(),
                path: PathBuf::from(format!("defaults://{}", d.path().display())),
            })
        })
        .collect()
}

/// Build a `SubagentRegistry` from default → global → project tiers, filtering
/// disabled entries. Returns None when empty so callers know to skip the
/// system-prompt block and the `spawn_subagent` tool registration.
pub fn build_registry_for_agent(
    agent_db: &AgentDb,
    working_dir: &Path,
) -> Option<Arc<SubagentRegistry>> {
    let disabled: HashSet<String> = agent_db.load_disabled_subagents().unwrap_or_else(|e| {
        log::warn!("[subagents] load_disabled failed, treating all enabled: {e}");
        HashSet::new()
    });

    let default = read_default_tier();
    let global_root = global_subagents_dir();
    let project_root = project_subagents_dir(working_dir);
    let global = read_tier_from_fs(&global_root);
    let project = read_tier_from_fs(&project_root);

    log::info!(
        "[subagents] build_registry_for_agent: default={} global={} (from {:?}) project={} (from {:?}) disabled={:?}",
        default.len(),
        global.len(),
        global_root,
        project.len(),
        project_root,
        disabled,
    );

    let registry = SubagentRegistry::new(default, global, project, &disabled);
    if registry.is_empty() {
        log::info!("[subagents] build_registry_for_agent: empty — subagents inactive this spawn");
        None
    } else {
        log::info!(
            "[subagents] build_registry_for_agent: {} subagent(s) loaded: {:?}",
            registry.len(),
            registry.names()
        );
        Some(Arc::new(registry))
    }
}

pub fn list_all_for_dialog(
    agent_db: &AgentDb,
    working_dir: Option<&Path>,
) -> Vec<DialogEntry> {
    let disabled = agent_db.load_disabled_subagents().unwrap_or_default();
    let default = read_default_tier();
    let global_root = global_subagents_dir();
    let global = read_tier_from_fs(&global_root);
    let project = working_dir
        .map(|wd| read_tier_from_fs(&project_subagents_dir(wd)))
        .unwrap_or_default();

    let resolved = SubagentRegistry::new(default, global, project, &HashSet::new());
    resolved
        .all()
        .into_iter()
        .map(|s| DialogEntry {
            name: s.name.clone(),
            description: s.description.clone(),
            origin: match s.origin {
                agent::subagents::Origin::Default => "default",
                agent::subagents::Origin::Global => "global",
                agent::subagents::Origin::Project => "project",
            }
            .to_string(),
            enabled: !disabled.contains(&s.name),
            allowed_tools: s.allowed_tools.clone(),
            model: s.model.clone(),
            path: s.path.display().to_string(),
        })
        .collect()
}

#[derive(Debug, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DialogEntry {
    pub name: String,
    pub description: String,
    pub origin: String,
    pub enabled: bool,
    pub allowed_tools: Option<Vec<String>>,
    pub model: Option<String>,
    pub path: String,
}
