use std::path::Path;

use crate::llm::types::CacheControl;
use crate::skills::SkillRegistry;
use crate::subagents::SubagentRegistry;
use crate::tool::ToolMode;

/// Raw prompt templates embedded at compile time from .txt files.
/// Templates contain the STATIC body only (no env section) plus a few inline
/// placeholders like `{{working_dir}}` that are session-stable.
const ASK_PROMPT_TEMPLATE: &str = include_str!("ask_prompt.txt");
const CODING_PROMPT_TEMPLATE: &str = include_str!("coding_prompt.txt");
const PLAN_PROMPT_TEMPLATE: &str = include_str!("plan_prompt.txt");

/// One block of the system prompt. Anthropic sends these in order; each block
/// can independently carry a `cache_control` marker.
#[derive(Debug, Clone)]
pub struct SystemBlock {
    pub text: String,
    pub cache_control: Option<CacheControl>,
}

/// Build a system prompt for the given mode as an ordered list of blocks.
///
/// Returns 2 or 3 blocks depending on whether a skill registry is provided:
///   [0] static body — everything except the Environment section, with
///       `{{working_dir}}` interpolated inline. Marked `cache_control: ephemeral`
///       (1h TTL) so Anthropic caches this prefix (stable per project).
///   [1] (optional) Available Skills list — injected when `skills` is Some
///       and non-empty. Marked `cache_control: ephemeral` (1h TTL) on its own
///       breakpoint so toggling a skill invalidates only this block's cache,
///       not block 0's larger prefix. Independence is best-effort: both blocks
///       expire after 1h.
///   [last] environment section — Working directory, branch, project note,
///       date, OS/arch. NOT cached: `{{date}}` rotates daily, and we don't
///       want a fresh cache write every midnight.
///
/// Across sessions on the same project: block 0 is byte-identical → cache hit.
/// Across the midnight boundary: block 0 still cache-hits; env block is sent
/// uncached (~50 tokens, negligible).
pub fn build_system_prompt(
    mode: ToolMode,
    working_dir: &Path,
    branch: Option<&str>,
    project_note: Option<&str>,
    skills: Option<&SkillRegistry>,
    subagents: Option<&SubagentRegistry>,
) -> Vec<SystemBlock> {
    let template = match mode {
        ToolMode::Ask => ASK_PROMPT_TEMPLATE,
        ToolMode::Coding => CODING_PROMPT_TEMPLATE,
        ToolMode::Plan => PLAN_PROMPT_TEMPLATE,
    };

    let static_body = template.replace("{{working_dir}}", &working_dir.display().to_string());

    let date = chrono::Local::now().format("%Y-%m-%d").to_string();
    let os = std::env::consts::OS;
    let arch = std::env::consts::ARCH;
    let branch_line = branch.map(|b| format!("- Git branch: {b}\n")).unwrap_or_default();
    let note_line = project_note.map(|n| format!("- {n}\n")).unwrap_or_default();
    let env_section = format!(
        "# Environment\n- Working directory: {}\n{}{}- Date: {}\n- OS: {}/{}\n",
        working_dir.display(),
        branch_line,
        note_line,
        date,
        os,
        arch,
    );

    let mut blocks = vec![SystemBlock {
        text: static_body,
        cache_control: Some(CacheControl::ephemeral()),
    }];

    // Skills + Subagents share ONE ephemeral breakpoint. Toggling either
    // invalidates the combined block once (not twice).
    let skills_entries = skills.map(|r| r.list_for_prompt()).unwrap_or_default();
    let subagents_entries = subagents.map(|r| r.list_for_prompt()).unwrap_or_default();

    if !skills_entries.is_empty() || !subagents_entries.is_empty() {
        let mut combined = String::new();
        if !skills_entries.is_empty() {
            combined.push_str(
                "# Available Skills\n\
                 Skills are instruction packs loaded on demand via the `skill` tool. \
                 The descriptions below are a MENU — they do not contain the rules \
                 themselves. Whenever a task matches a skill by name, topic, or \
                 intent — or the user explicitly mentions one — CALL THE `skill` \
                 TOOL FIRST and follow the body verbatim. Do not guess at the rules \
                 from the description. Skip the tool only when no skill below is \
                 clearly relevant.\n\n\
                 Available:\n",
            );
            for (name, description) in &skills_entries {
                combined.push_str(&format!("- {name}: {description}\n"));
            }
        }
        if !subagents_entries.is_empty() {
            if !combined.is_empty() {
                combined.push('\n');
            }
            combined.push_str(
                "# Available Subagents\n\
                 Subagents currently enabled for this turn (see \"Default Subagents\" \
                 in the main prompt for routing guidance):\n",
            );
            for (name, description) in &subagents_entries {
                combined.push_str(&format!("- {name}: {description}\n"));
            }
        }
        log::info!(
            "[prompt] skills+subagents block: {} skill(s), {} subagent(s), {} chars",
            skills_entries.len(),
            subagents_entries.len(),
            combined.len()
        );
        blocks.push(SystemBlock {
            text: combined,
            cache_control: Some(CacheControl::ephemeral()),
        });
    }

    blocks.push(SystemBlock {
        text: env_section,
        cache_control: None,
    });

    blocks
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn joined(blocks: &[SystemBlock]) -> String {
        blocks.iter().map(|b| b.text.as_str()).collect::<Vec<_>>().join("\n")
    }

    use crate::skills::{registry::SkillInput, SkillRegistry};
    use std::collections::HashSet;

    fn mk_skill_registry() -> SkillRegistry {
        let input = SkillInput {
            raw: "---\nname: hello\ndescription: A greeting skill.\n---\nbody\n".to_string(),
            path: PathBuf::from("/hello"),
        };
        SkillRegistry::new(vec![], vec![input], vec![], &HashSet::new())
    }

    // ── Structural: two-block layout with correct cache markers ──

    #[test]
    fn test_build_system_prompt_splits_into_two_blocks() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let blocks = build_system_prompt(mode, &PathBuf::from("/p"), Some("main"), None, None, None);
            assert_eq!(blocks.len(), 2, "{:?}: expected exactly 2 blocks", mode);
            assert!(blocks[0].cache_control.is_some(), "{:?}: block 0 must be cached", mode);
            assert!(blocks[1].cache_control.is_none(), "{:?}: block 1 must NOT be cached", mode);
        }
    }

    #[test]
    fn test_skills_block_inserted_when_registry_non_empty() {
        let registry = mk_skill_registry();
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let blocks = build_system_prompt(
                mode,
                &PathBuf::from("/p"),
                Some("main"),
                None,
                Some(&registry),
                None,
            );
            assert_eq!(blocks.len(), 3, "{:?}: expected 3 blocks with skills", mode);
            assert!(blocks[0].cache_control.is_some(), "{:?}: static body cached", mode);
            assert!(blocks[1].cache_control.is_some(), "{:?}: skills cached", mode);
            assert!(blocks[2].cache_control.is_none(), "{:?}: env uncached", mode);
            assert!(blocks[1].text.contains("# Available Skills"));
            assert!(blocks[1].text.contains("hello: A greeting skill."));
        }
    }

    #[test]
    fn test_skills_block_omitted_when_registry_empty() {
        let empty = SkillRegistry::new(vec![], vec![], vec![], &HashSet::new());
        let blocks = build_system_prompt(
            ToolMode::Ask,
            &PathBuf::from("/p"),
            Some("main"),
            None,
            Some(&empty),
            None,
        );
        assert_eq!(blocks.len(), 2, "empty registry must not add a block");
    }

    #[test]
    fn test_static_body_excludes_date() {
        let today = chrono::Local::now().format("%Y-%m-%d").to_string();
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let blocks = build_system_prompt(mode, &PathBuf::from("/p"), Some("main"), None, None, None);
            assert!(
                !blocks[0].text.contains(&today),
                "{:?}: static body must not contain today's date (would invalidate cache daily)",
                mode
            );
        }
    }

    #[test]
    fn test_env_block_contains_date_and_working_dir() {
        let today = chrono::Local::now().format("%Y-%m-%d").to_string();
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let blocks = build_system_prompt(mode, &PathBuf::from("/home/user/project"), Some("main"), None, None, None);
            assert!(blocks[1].text.contains(&today), "{:?}: env must contain date", mode);
            assert!(blocks[1].text.contains("/home/user/project"), "{:?}: env must contain working_dir", mode);
            assert!(blocks[1].text.contains("main"), "{:?}: env must contain branch when provided", mode);
        }
    }

    // ── Legacy coverage: working_dir injection, placeholder resolution ──

    #[test]
    fn test_working_dir_injected_all_modes() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let prompt = joined(&build_system_prompt(mode, &PathBuf::from("/home/user/project"), None, None, None, None));
            assert!(prompt.contains("/home/user/project"), "Mode {:?} missing working_dir", mode);
            assert!(!prompt.contains("{{working_dir}}"), "Mode {:?} has unresolved placeholder", mode);
        }
    }

    #[test]
    fn test_branch_injected() {
        let prompt = joined(&build_system_prompt(ToolMode::Coding, &PathBuf::from("/tmp"), Some("feature/login"), None, None, None));
        assert!(prompt.contains("feature/login"));
    }

    #[test]
    fn test_branch_omitted_when_none() {
        let prompt = joined(&build_system_prompt(ToolMode::Ask, &PathBuf::from("/tmp"), None, None, None, None));
        assert!(!prompt.contains("Git branch"));
    }

    #[test]
    fn test_date_injected() {
        let prompt = joined(&build_system_prompt(ToolMode::Ask, &PathBuf::from("/tmp"), None, None, None, None));
        let today = chrono::Local::now().format("%Y-%m-%d").to_string();
        assert!(prompt.contains(&today));
    }

    #[test]
    fn test_os_arch_injected() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let prompt = joined(&build_system_prompt(mode, &PathBuf::from("/tmp"), None, None, None, None));
            assert!(prompt.contains(std::env::consts::OS), "Mode {:?} missing OS", mode);
            assert!(prompt.contains(std::env::consts::ARCH), "Mode {:?} missing ARCH", mode);
        }
    }

    #[test]
    fn test_no_unresolved_placeholders() {
        for mode in [ToolMode::Ask, ToolMode::Coding, ToolMode::Plan] {
            let prompt = joined(&build_system_prompt(mode, &PathBuf::from("/tmp"), Some("main"), None, None, None));
            assert!(!prompt.contains("{{"), "Mode {:?} has unresolved placeholder: {}", mode,
                prompt.find("{{").map(|i| &prompt[i..(i+30).min(prompt.len())]).unwrap_or(""));
        }
    }
}
