use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

use super::parse::{parse_subagent_md, ParsedSubagent};

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Origin {
    Default,
    Global,
    Project,
}

#[derive(Debug, Clone)]
pub struct Subagent {
    pub name: String,
    pub description: String,
    pub body: String,
    pub allowed_tools: Option<Vec<String>>,
    pub model: Option<String>,
    pub origin: Origin,
    pub path: PathBuf,
}

const MAX_BODY_TOKENS: usize = 20_000;

#[derive(Debug, Clone)]
pub struct SubagentInput {
    pub raw: String,
    pub path: PathBuf,
}

pub struct SubagentRegistry {
    agents: HashMap<String, Subagent>,
}

impl SubagentRegistry {
    pub fn new(
        default: Vec<SubagentInput>,
        global: Vec<SubagentInput>,
        project: Vec<SubagentInput>,
        disabled: &HashSet<String>,
    ) -> Self {
        let mut agents: HashMap<String, Subagent> = HashMap::new();

        for (origin, inputs) in [
            (Origin::Default, default),
            (Origin::Global, global),
            (Origin::Project, project),
        ] {
            for input in inputs {
                match load_one(input, origin) {
                    Ok(agent) => {
                        agents.insert(agent.name.clone(), agent);
                    }
                    Err(msg) => {
                        log::warn!("[subagents] skipped: {msg}");
                    }
                }
            }
        }

        agents.retain(|name, _| !disabled.contains(name));

        Self { agents }
    }

    pub fn is_empty(&self) -> bool {
        self.agents.is_empty()
    }

    pub fn len(&self) -> usize {
        self.agents.len()
    }

    pub fn get(&self, name: &str) -> Option<&Subagent> {
        self.agents.get(name)
    }

    /// `(name, description)` sorted — for the `# Available Subagents` prompt block
    /// and the `spawn_subagent` tool description.
    pub fn list_for_prompt(&self) -> Vec<(&str, &str)> {
        let mut out: Vec<_> = self
            .agents
            .values()
            .map(|a| (a.name.as_str(), a.description.as_str()))
            .collect();
        out.sort_by_key(|(n, _)| *n);
        out
    }

    pub fn all(&self) -> Vec<&Subagent> {
        let mut out: Vec<_> = self.agents.values().collect();
        out.sort_by(|a, b| a.name.cmp(&b.name));
        out
    }

    pub fn names(&self) -> Vec<String> {
        let mut out: Vec<String> = self.agents.keys().cloned().collect();
        out.sort();
        out
    }
}

fn load_one(input: SubagentInput, origin: Origin) -> Result<Subagent, String> {
    let parsed: ParsedSubagent = parse_subagent_md(&input.raw)
        .map_err(|e| format!("{}: parse error: {e}", input.path.display()))?;

    let approx_tokens = parsed.body.len() / 4;
    if approx_tokens > MAX_BODY_TOKENS {
        return Err(format!(
            "{}: body too large ({} > {} tokens)",
            input.path.display(),
            approx_tokens,
            MAX_BODY_TOKENS
        ));
    }

    Ok(Subagent {
        name: parsed.name,
        description: parsed.description,
        body: parsed.body,
        allowed_tools: parsed.allowed_tools,
        model: parsed.model,
        origin,
        path: input.path,
    })
}

/// Walk `<root>/<child>/<child>.md` discovering subagent definitions.
/// Each child dir contributes at most one subagent named after its own `<name>.md`
/// file inside it (e.g. `git-expert/git-expert.md`). Symlink guards mirror skills.
pub fn read_tier_from_fs(root: &Path) -> Vec<SubagentInput> {
    let mut out = Vec::new();
    let Ok(entries) = std::fs::read_dir(root) else {
        return out;
    };
    for entry in entries.flatten() {
        if entry
            .file_type()
            .map(|ft| ft.is_symlink())
            .unwrap_or(false)
        {
            log::warn!(
                "[subagents] skipping symlink dir entry: {}",
                entry.path().display()
            );
            continue;
        }
        let path = entry.path();
        let Some(dir_name) = path.file_name().and_then(|n| n.to_str()).map(String::from) else {
            continue;
        };
        let agent_md = path.join(format!("{dir_name}.md"));
        match std::fs::symlink_metadata(&agent_md) {
            Ok(meta) if meta.file_type().is_symlink() => {
                log::warn!(
                    "[subagents] skipping symlinked definition: {}",
                    agent_md.display()
                );
                continue;
            }
            Ok(_) => {}
            Err(_) => continue,
        }
        if let Ok(raw) = std::fs::read_to_string(&agent_md) {
            out.push(SubagentInput { raw, path: agent_md });
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn mk(raw: &str, path: &str) -> SubagentInput {
        SubagentInput {
            raw: raw.to_string(),
            path: PathBuf::from(path),
        }
    }

    #[test]
    fn empty_registry() {
        let reg = SubagentRegistry::new(vec![], vec![], vec![], &HashSet::new());
        assert!(reg.is_empty());
    }

    #[test]
    fn loads_and_sorts() {
        let a = mk("---\nname: bravo\ndescription: B.\n---\nb", "/b");
        let b = mk("---\nname: alpha\ndescription: A.\n---\na", "/a");
        let reg = SubagentRegistry::new(vec![], vec![a, b], vec![], &HashSet::new());
        assert_eq!(
            reg.list_for_prompt(),
            vec![("alpha", "A."), ("bravo", "B.")]
        );
    }

    #[test]
    fn project_overrides_global_overrides_default() {
        let d = mk("---\nname: s\ndescription: default.\n---\nd", "/d");
        let g = mk("---\nname: s\ndescription: global.\n---\ng", "/g");
        let p = mk("---\nname: s\ndescription: project.\n---\np", "/p");
        let reg = SubagentRegistry::new(vec![d], vec![g], vec![p], &HashSet::new());
        let a = reg.get("s").unwrap();
        assert_eq!(a.origin, Origin::Project);
        assert_eq!(a.description, "project.");
    }

    #[test]
    fn malformed_skipped_others_loaded() {
        let bad = mk("no frontmatter", "/bad");
        let good = mk("---\nname: good\ndescription: ok.\n---\nb", "/good");
        let reg = SubagentRegistry::new(vec![], vec![bad, good], vec![], &HashSet::new());
        assert_eq!(reg.len(), 1);
        assert!(reg.get("good").is_some());
    }

    #[test]
    fn oversized_rejected() {
        let huge_body = "x".repeat(MAX_BODY_TOKENS * 4 + 10);
        let raw = format!("---\nname: huge\ndescription: big.\n---\n{huge_body}");
        let reg = SubagentRegistry::new(vec![], vec![mk(&raw, "/h")], vec![], &HashSet::new());
        assert!(reg.is_empty());
    }

    #[test]
    fn disabled_filtered() {
        let a = mk("---\nname: on\ndescription: A.\n---\nb", "/a");
        let b = mk("---\nname: off\ndescription: B.\n---\nb", "/b");
        let mut disabled = HashSet::new();
        disabled.insert("off".to_string());
        let reg = SubagentRegistry::new(vec![], vec![a, b], vec![], &disabled);
        assert!(reg.get("on").is_some());
        assert!(reg.get("off").is_none());
    }

    #[test]
    fn carries_allowed_tools_and_model() {
        let raw = "---\nname: r\ndescription: d\nallowed-tools: [read, grep]\nmodel: claude-sonnet-4-6\n---\nb";
        let reg = SubagentRegistry::new(vec![], vec![mk(raw, "/r")], vec![], &HashSet::new());
        let a = reg.get("r").unwrap();
        assert_eq!(
            a.allowed_tools.as_deref(),
            Some(["read".to_string(), "grep".to_string()].as_slice())
        );
        assert_eq!(a.model.as_deref(), Some("claude-sonnet-4-6"));
    }

    // ── symlink guards (mirror skills) ──

    #[cfg(unix)]
    #[test]
    fn read_tier_skips_symlinked_definition_file() {
        let tmp = tempfile::tempdir().unwrap();
        let tier = tmp.path();

        let good_dir = tier.join("good");
        std::fs::create_dir(&good_dir).unwrap();
        std::fs::write(
            good_dir.join("good.md"),
            "---\nname: good\ndescription: Valid.\n---\nbody",
        )
        .unwrap();

        let secret = tmp.path().join("secret.txt");
        std::fs::write(&secret, "SENSITIVE").unwrap();
        let evil_dir = tier.join("evil");
        std::fs::create_dir(&evil_dir).unwrap();
        std::os::unix::fs::symlink(&secret, evil_dir.join("evil.md")).unwrap();

        let inputs = read_tier_from_fs(tier);
        assert_eq!(inputs.len(), 1);
        assert!(inputs[0].raw.contains("name: good"));
        assert!(!inputs.iter().any(|i| i.raw.contains("SENSITIVE")));
    }

    #[cfg(unix)]
    #[test]
    fn read_tier_skips_symlinked_agent_dir() {
        let tmp = tempfile::tempdir().unwrap();
        let tier = tmp.path();

        let good_dir = tier.join("good");
        std::fs::create_dir(&good_dir).unwrap();
        std::fs::write(
            good_dir.join("good.md"),
            "---\nname: good\ndescription: Valid.\n---\nb",
        )
        .unwrap();

        let outside_tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir(outside_tmp.path().join("evil")).unwrap();
        std::fs::write(
            outside_tmp.path().join("evil").join("evil.md"),
            "---\nname: evil\ndescription: Elsewhere.\n---\nb",
        )
        .unwrap();
        std::os::unix::fs::symlink(outside_tmp.path().join("evil"), tier.join("evil")).unwrap();

        let inputs = read_tier_from_fs(tier);
        assert_eq!(inputs.len(), 1);
        assert!(inputs[0].raw.contains("name: good"));
    }
}
