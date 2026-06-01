use std::sync::OnceLock;

use regex::Regex;
use serde::Deserialize;

use crate::frontmatter::{split_frontmatter, FrontmatterError};

const NAME_MAX: usize = 64;
// Larger than skills' 1024 — the description is the primary LLM-facing signal
// for which subagent to pick, so authors need room to explain capabilities.
const DESC_MAX: usize = 2048;

fn name_regex() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"^[a-z0-9-]{1,64}$").expect("static regex"))
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParsedSubagent {
    pub name: String,
    pub description: String,
    pub body: String,
    pub allowed_tools: Option<Vec<String>>,
    pub model: Option<String>,
}

#[derive(Debug, thiserror::Error)]
pub enum SubagentParseError {
    #[error("missing frontmatter delimiter")]
    MissingFrontmatter,
    #[error("invalid YAML frontmatter: {0}")]
    InvalidYaml(String),
    #[error("invalid subagent name `{0}` (must match ^[a-z0-9-]{{1,64}}$)")]
    InvalidName(String),
    #[error("description must be 1-{} characters", DESC_MAX)]
    InvalidDescription,
    #[error("allowed-tools must be a list of strings")]
    InvalidAllowedTools,
    #[error("model must be a non-empty string")]
    InvalidModel,
}

#[derive(Deserialize)]
struct Frontmatter {
    name: String,
    description: String,
    #[serde(rename = "allowed-tools", default)]
    allowed_tools: Option<Vec<String>>,
    #[serde(default)]
    model: Option<String>,
}

pub fn parse_subagent_md(raw: &str) -> Result<ParsedSubagent, SubagentParseError> {
    let fm_raw = split_frontmatter(raw).map_err(|e| match e {
        FrontmatterError::MissingDelimiter => SubagentParseError::MissingFrontmatter,
    })?;

    let fm: Frontmatter = serde_yml::from_str(fm_raw.yaml)
        .map_err(|e| SubagentParseError::InvalidYaml(e.to_string()))?;

    if fm.name.is_empty() || fm.name.len() > NAME_MAX || !name_regex().is_match(&fm.name) {
        return Err(SubagentParseError::InvalidName(fm.name));
    }
    if fm.description.is_empty() || fm.description.len() > DESC_MAX {
        return Err(SubagentParseError::InvalidDescription);
    }
    if let Some(tools) = &fm.allowed_tools {
        if tools.iter().any(|t| t.is_empty()) {
            return Err(SubagentParseError::InvalidAllowedTools);
        }
    }
    if let Some(m) = &fm.model {
        if m.is_empty() {
            return Err(SubagentParseError::InvalidModel);
        }
    }

    Ok(ParsedSubagent {
        name: fm.name,
        description: fm.description,
        body: fm_raw.body.trim_start_matches(['\r', '\n']).to_string(),
        allowed_tools: fm.allowed_tools,
        model: fm.model,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_minimal() {
        let raw = "---\nname: reviewer\ndescription: Reviews code.\n---\nBody.";
        let p = parse_subagent_md(raw).unwrap();
        assert_eq!(p.name, "reviewer");
        assert_eq!(p.description, "Reviews code.");
        assert_eq!(p.body, "Body.");
        assert!(p.allowed_tools.is_none());
        assert!(p.model.is_none());
    }

    #[test]
    fn parses_full_frontmatter() {
        let raw = "---\nname: git-expert\ndescription: Git ops.\nallowed-tools:\n  - read\n  - bash\nmodel: claude-sonnet-4-6\n---\nBody.";
        let p = parse_subagent_md(raw).unwrap();
        assert_eq!(
            p.allowed_tools.as_deref(),
            Some(["read".to_string(), "bash".to_string()].as_slice())
        );
        assert_eq!(p.model.as_deref(), Some("claude-sonnet-4-6"));
    }

    #[test]
    fn rejects_missing_frontmatter() {
        assert!(matches!(
            parse_subagent_md("no delimiter"),
            Err(SubagentParseError::MissingFrontmatter)
        ));
    }

    #[test]
    fn rejects_bad_name() {
        let raw = "---\nname: Bad_Name\ndescription: x\n---\n";
        assert!(matches!(
            parse_subagent_md(raw),
            Err(SubagentParseError::InvalidName(_))
        ));
    }

    #[test]
    fn rejects_oversized_description() {
        let desc = "x".repeat(DESC_MAX + 1);
        let raw = format!("---\nname: ok\ndescription: {desc}\n---\nbody");
        assert!(matches!(
            parse_subagent_md(&raw),
            Err(SubagentParseError::InvalidDescription)
        ));
    }

    #[test]
    fn rejects_empty_allowed_tools_entry() {
        let raw = "---\nname: ok\ndescription: d\nallowed-tools:\n  - ''\n---\nb";
        assert!(matches!(
            parse_subagent_md(raw),
            Err(SubagentParseError::InvalidAllowedTools)
        ));
    }

    #[test]
    fn rejects_empty_model() {
        let raw = "---\nname: ok\ndescription: d\nmodel: ''\n---\nb";
        assert!(matches!(
            parse_subagent_md(raw),
            Err(SubagentParseError::InvalidModel)
        ));
    }

    #[test]
    fn rejects_invalid_yaml() {
        let raw = "---\nname: [not-a-string\ndescription: x\n---\nbody";
        assert!(matches!(
            parse_subagent_md(raw),
            Err(SubagentParseError::InvalidYaml(_))
        ));
    }

    #[test]
    fn description_2048_boundary() {
        let desc = "x".repeat(DESC_MAX);
        let raw = format!("---\nname: ok\ndescription: {desc}\n---\n");
        assert!(parse_subagent_md(&raw).is_ok());
    }
}
