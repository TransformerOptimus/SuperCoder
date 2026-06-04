use std::collections::{HashMap, HashSet};
use std::path::{Path, PathBuf};

use super::parse::{parse_skill_md, ParsedSkill};

/// Where a skill was discovered.
#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Origin {
    /// Embedded in the binary at compile time.
    Default,
    /// User-global app data directory.
    Global,
    /// Project-scoped (`<cwd>/.agent/skills/`).
    Project,
}

/// A fully loaded skill ready to be injected into the prompt list.
#[derive(Debug, Clone)]
pub struct Skill {
    pub name: String,
    pub description: String,
    pub body: String,
    pub origin: Origin,
    /// Filesystem path for display. For embedded defaults this is a virtual
    /// `defaults://<name>` URI.
    pub path: PathBuf,
}

/// Approximation of Anthropic's token count: chars/4. Matches the agent's
/// existing `estimate_token_count` heuristic.
const MAX_BODY_TOKENS: usize = 20_000;

/// A raw input to the registry: the SKILL.md contents plus the display path.
#[derive(Debug, Clone)]
pub struct SkillInput {
    pub raw: String,
    pub path: PathBuf,
}

/// The in-memory skill registry built once per agent spawn.
pub struct SkillRegistry {
    skills: HashMap<String, Skill>,
}

impl SkillRegistry {
    /// Build a registry from the three tiers. Later tiers override earlier
    /// ones on name collision (project > global > default). Malformed skills
    /// are skipped with a warning log and never block startup. Skills whose
    /// name appears in `disabled` are filtered out.
    pub fn new(
        default: Vec<SkillInput>,
        global: Vec<SkillInput>,
        project: Vec<SkillInput>,
        disabled: &HashSet<String>,
    ) -> Self {
        let mut skills: HashMap<String, Skill> = HashMap::new();

        for (origin, inputs) in [
            (Origin::Default, default),
            (Origin::Global, global),
            (Origin::Project, project),
        ] {
            for input in inputs {
                match load_one(input, origin) {
                    Ok(skill) => {
                        skills.insert(skill.name.clone(), skill);
                    }
                    Err(msg) => {
                        log::warn!("[skills] skipped skill: {msg}");
                    }
                }
            }
        }

        skills.retain(|name, _| !disabled.contains(name));

        Self { skills }
    }

    pub fn is_empty(&self) -> bool {
        self.skills.is_empty()
    }

    pub fn len(&self) -> usize {
        self.skills.len()
    }

    pub fn get(&self, name: &str) -> Option<&Skill> {
        self.skills.get(name)
    }

    /// Iterator over `(name, description)` pairs, sorted by name for
    /// deterministic prompt output.
    pub fn list_for_prompt(&self) -> Vec<(&str, &str)> {
        let mut out: Vec<_> = self
            .skills
            .values()
            .map(|s| (s.name.as_str(), s.description.as_str()))
            .collect();
        out.sort_by_key(|(n, _)| *n);
        out
    }

    /// All loaded skills, sorted by name.
    pub fn all(&self) -> Vec<&Skill> {
        let mut out: Vec<_> = self.skills.values().collect();
        out.sort_by(|a, b| a.name.cmp(&b.name));
        out
    }

    /// Available skill names, sorted — used in error messages when the LLM
    /// calls the `skill` tool with an unknown name.
    pub fn names(&self) -> Vec<String> {
        let mut out: Vec<String> = self.skills.keys().cloned().collect();
        out.sort();
        out
    }
}

fn load_one(input: SkillInput, origin: Origin) -> Result<Skill, String> {
    let parsed: ParsedSkill = parse_skill_md(&input.raw)
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

    Ok(Skill {
        name: parsed.name,
        description: parsed.description,
        body: parsed.body,
        origin,
        path: input.path,
    })
}

/// Helper for Tauri-side discovery: walk an immediate-children directory and
/// load each `<child>/SKILL.md` into a `SkillInput`. Missing root is fine.
///
/// Symlinks are skipped at BOTH levels:
///   - the child directory entry itself (prevents `skills/evil -> /etc`)
///   - the SKILL.md file inside it (prevents `skills/evil/SKILL.md -> ~/.ssh/id_rsa`)
///
/// Both matter: a malicious repo cloned locally could plant either form in
/// `<repo>/.agent/skills/`, and without the file-level check the contents would
/// be read and injected into the system prompt (leaking to the LLM provider).
pub fn read_tier_from_fs(root: &Path) -> Vec<SkillInput> {
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
            log::warn!("[skills] skipping symlink dir entry: {}", entry.path().display());
            continue;
        }
        let path = entry.path();
        let skill_md = path.join("SKILL.md");
        // `symlink_metadata` does NOT follow symlinks, so we can reject the file
        // before `read_to_string` (which DOES follow) slurps anything outside.
        match std::fs::symlink_metadata(&skill_md) {
            Ok(meta) if meta.file_type().is_symlink() => {
                log::warn!(
                    "[skills] skipping symlinked SKILL.md: {}",
                    skill_md.display()
                );
                continue;
            }
            Ok(_) => {}
            Err(_) => continue, // missing / unreadable — nothing to load
        }
        if let Ok(raw) = std::fs::read_to_string(&skill_md) {
            out.push(SkillInput { raw, path });
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn mk(raw: &str, path: &str) -> SkillInput {
        SkillInput {
            raw: raw.to_string(),
            path: PathBuf::from(path),
        }
    }

    #[test]
    fn empty_registry() {
        let reg = SkillRegistry::new(vec![], vec![], vec![], &HashSet::new());
        assert!(reg.is_empty());
    }

    #[test]
    fn loads_and_sorts() {
        let a = mk("---\nname: bravo\ndescription: B.\n---\nbody b", "/b");
        let b = mk("---\nname: alpha\ndescription: A.\n---\nbody a", "/a");
        let reg = SkillRegistry::new(vec![], vec![a, b], vec![], &HashSet::new());
        let list = reg.list_for_prompt();
        assert_eq!(list, vec![("alpha", "A."), ("bravo", "B.")]);
    }

    #[test]
    fn project_overrides_global_overrides_default() {
        let d = mk("---\nname: s\ndescription: default.\n---\nfrom default", "/d");
        let g = mk("---\nname: s\ndescription: global.\n---\nfrom global", "/g");
        let p = mk("---\nname: s\ndescription: project.\n---\nfrom project", "/p");
        let reg = SkillRegistry::new(vec![d], vec![g], vec![p], &HashSet::new());
        let skill = reg.get("s").unwrap();
        assert_eq!(skill.origin, Origin::Project);
        assert_eq!(skill.description, "project.");
        assert!(skill.body.contains("from project"));
    }

    #[test]
    fn global_overrides_default() {
        let d = mk("---\nname: s\ndescription: default.\n---\nfrom default", "/d");
        let g = mk("---\nname: s\ndescription: global.\n---\nfrom global", "/g");
        let reg = SkillRegistry::new(vec![d], vec![g], vec![], &HashSet::new());
        assert_eq!(reg.get("s").unwrap().origin, Origin::Global);
    }

    #[test]
    fn malformed_skipped_others_loaded() {
        let bad = mk("no frontmatter here", "/bad");
        let good = mk("---\nname: good\ndescription: ok.\n---\nbody", "/good");
        let reg = SkillRegistry::new(vec![], vec![bad, good], vec![], &HashSet::new());
        assert_eq!(reg.len(), 1);
        assert!(reg.get("good").is_some());
    }

    #[test]
    fn oversized_rejected() {
        let huge_body = "x".repeat(20_001 * 4 + 10);
        let raw = format!("---\nname: huge\ndescription: big.\n---\n{huge_body}");
        let reg = SkillRegistry::new(vec![], vec![mk(&raw, "/h")], vec![], &HashSet::new());
        assert!(reg.is_empty());
    }

    #[test]
    fn disabled_filtered() {
        let a = mk("---\nname: on-skill\ndescription: A.\n---\nbody", "/a");
        let b = mk("---\nname: off-skill\ndescription: B.\n---\nbody", "/b");
        let mut disabled = HashSet::new();
        disabled.insert("off-skill".to_string());
        let reg = SkillRegistry::new(vec![], vec![a, b], vec![], &disabled);
        assert!(reg.get("on-skill").is_some());
        assert!(reg.get("off-skill").is_none());
    }

    // ── read_tier_from_fs symlink guards ──

    #[cfg(unix)]
    #[test]
    fn read_tier_skips_symlinked_skill_md_file() {
        // Sets up: tmp/
        //           good/SKILL.md  (real file, valid frontmatter)
        //           evil/SKILL.md  (symlink to secret.txt outside the tier root)
        //           secret.txt
        let tmp = tempfile::tempdir().unwrap();
        let tier = tmp.path();

        // Good skill — should be loaded.
        let good_dir = tier.join("good");
        std::fs::create_dir(&good_dir).unwrap();
        std::fs::write(
            good_dir.join("SKILL.md"),
            "---\nname: good\ndescription: Valid.\n---\nbody",
        )
        .unwrap();

        // Secret lives outside the tier root; point a symlink at it.
        let secret = tmp.path().join("secret.txt");
        std::fs::write(&secret, "SENSITIVE").unwrap();
        let evil_dir = tier.join("evil");
        std::fs::create_dir(&evil_dir).unwrap();
        std::os::unix::fs::symlink(&secret, evil_dir.join("SKILL.md")).unwrap();

        let inputs = read_tier_from_fs(tier);
        // Only `good` should have been loaded; `evil` rejected by the symlink
        // guard on the SKILL.md file.
        assert_eq!(inputs.len(), 1);
        assert!(inputs[0].raw.contains("name: good"));
        assert!(!inputs.iter().any(|i| i.raw.contains("SENSITIVE")));
    }

    #[cfg(unix)]
    #[test]
    fn read_tier_skips_symlinked_skill_dir() {
        let tmp = tempfile::tempdir().unwrap();
        let tier = tmp.path();

        // A real skill dir to confirm the happy path still works.
        let good_dir = tier.join("good");
        std::fs::create_dir(&good_dir).unwrap();
        std::fs::write(
            good_dir.join("SKILL.md"),
            "---\nname: good\ndescription: Valid.\n---\nbody",
        )
        .unwrap();

        // A dir symlink pointing to a SEPARATE tempdir's skill — the
        // directory-level guard should reject the symlink.
        let outside_tmp = tempfile::tempdir().unwrap();
        std::fs::write(
            outside_tmp.path().join("SKILL.md"),
            "---\nname: evil\ndescription: Elsewhere.\n---\nbody",
        )
        .unwrap();
        std::os::unix::fs::symlink(outside_tmp.path(), tier.join("evil")).unwrap();

        let inputs = read_tier_from_fs(tier);
        assert_eq!(inputs.len(), 1);
        assert!(inputs[0].raw.contains("name: good"));
    }
}
