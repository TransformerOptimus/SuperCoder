use std::sync::OnceLock;

use regex::Regex;
use serde::Deserialize;

use crate::frontmatter::{split_frontmatter, FrontmatterError};

const NAME_MAX: usize = 64;
const DESC_MAX: usize = 1024;

fn name_regex() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    // TODO(skills): pattern permits all-hyphen names like `-`, `---`. Pure UX
    // wart — a skill named `---` renders as "- ---: description" in the list
    // but still looks up correctly. Add an alphanumeric requirement if we care.
    RE.get_or_init(|| Regex::new(r"^[a-z0-9-]{1,64}$").expect("static regex"))
}

/// Parsed contents of a SKILL.md file.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParsedSkill {
    pub name: String,
    pub description: String,
    pub body: String,
}

#[derive(Debug, thiserror::Error)]
pub enum SkillParseError {
    #[error("missing frontmatter delimiter")]
    MissingFrontmatter,
    #[error("invalid YAML frontmatter: {0}")]
    InvalidYaml(String),
    #[error("invalid skill name `{0}` (must match ^[a-z0-9-]{{1,64}}$)")]
    InvalidName(String),
    #[error("description must be 1-{} characters", DESC_MAX)]
    InvalidDescription,
}

#[derive(Deserialize)]
struct Frontmatter {
    name: String,
    description: String,
}

/// Parse a SKILL.md file: extract YAML frontmatter between `---` delimiters,
/// validate `name` and `description`, return the body as the remaining prose.
pub fn parse_skill_md(raw: &str) -> Result<ParsedSkill, SkillParseError> {
    let fm_raw = split_frontmatter(raw).map_err(|e| match e {
        FrontmatterError::MissingDelimiter => SkillParseError::MissingFrontmatter,
    })?;

    let fm: Frontmatter = serde_yml::from_str(fm_raw.yaml)
        .map_err(|e| SkillParseError::InvalidYaml(e.to_string()))?;

    if fm.name.is_empty() || fm.name.len() > NAME_MAX || !name_regex().is_match(&fm.name) {
        return Err(SkillParseError::InvalidName(fm.name));
    }
    if fm.description.is_empty() || fm.description.len() > DESC_MAX {
        return Err(SkillParseError::InvalidDescription);
    }

    Ok(ParsedSkill {
        name: fm.name,
        description: fm.description,
        body: fm_raw.body.trim_start_matches(['\r', '\n']).to_string(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_valid_skill() {
        let raw = "---\nname: hello-skill\ndescription: A greeting.\n---\nBody here\n";
        let parsed = parse_skill_md(raw).unwrap();
        assert_eq!(parsed.name, "hello-skill");
        assert_eq!(parsed.description, "A greeting.");
        assert_eq!(parsed.body, "Body here\n");
    }

    #[test]
    fn parses_crlf() {
        let raw = "---\r\nname: x\r\ndescription: y\r\n---\r\nbody\r\n";
        let parsed = parse_skill_md(raw).unwrap();
        assert_eq!(parsed.name, "x");
        assert_eq!(parsed.body, "body\r\n");
    }

    #[test]
    fn parses_crlf_file_without_trailing_newline() {
        // Windows notepad-authored file ending exactly at the close delimiter.
        let raw = "---\r\nname: x\r\ndescription: y\r\n---";
        let parsed = parse_skill_md(raw).unwrap();
        assert_eq!(parsed.name, "x");
        assert_eq!(parsed.body, "");
    }

    #[test]
    fn rejects_missing_frontmatter() {
        assert!(matches!(
            parse_skill_md("no delimiter here"),
            Err(SkillParseError::MissingFrontmatter)
        ));
    }

    #[test]
    fn rejects_bad_name_with_uppercase() {
        let raw = "---\nname: Hello\ndescription: x\n---\nbody";
        assert!(matches!(
            parse_skill_md(raw),
            Err(SkillParseError::InvalidName(_))
        ));
    }

    #[test]
    fn rejects_bad_name_with_underscore() {
        let raw = "---\nname: bad_name\ndescription: x\n---\nbody";
        assert!(matches!(
            parse_skill_md(raw),
            Err(SkillParseError::InvalidName(_))
        ));
    }

    #[test]
    fn rejects_empty_description() {
        let raw = "---\nname: ok\ndescription: ''\n---\nbody";
        assert!(matches!(
            parse_skill_md(raw),
            Err(SkillParseError::InvalidDescription)
        ));
    }

    #[test]
    fn rejects_name_too_long() {
        let name: String = "a".repeat(65);
        let raw = format!("---\nname: {name}\ndescription: x\n---\nbody");
        assert!(matches!(
            parse_skill_md(&raw),
            Err(SkillParseError::InvalidName(_))
        ));
    }

    #[test]
    fn rejects_invalid_yaml() {
        let raw = "---\nname: [not-a-string\ndescription: x\n---\nbody";
        assert!(matches!(
            parse_skill_md(raw),
            Err(SkillParseError::InvalidYaml(_))
        ));
    }
}
