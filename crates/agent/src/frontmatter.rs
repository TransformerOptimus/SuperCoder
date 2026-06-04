//! Shared YAML-frontmatter splitter for Markdown-with-frontmatter files
//! (skills, subagents). Handles BOM, leading whitespace, and the four
//! CRLF/LF permutations plus a Notepad-authored EOF-terminated close
//! delimiter. Typed deserialization and domain-specific validation live in
//! each consumer module — this file only splits raw YAML from raw body.

#[derive(Debug)]
pub struct FrontmatterRaw<'a> {
    pub yaml: &'a str,
    pub body: &'a str,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FrontmatterError {
    #[error("missing frontmatter delimiter")]
    MissingDelimiter,
}

pub fn split_frontmatter(raw: &str) -> Result<FrontmatterRaw<'_>, FrontmatterError> {
    let trimmed = raw
        .trim_start_matches('\u{feff}')
        .trim_start_matches(['\r', '\n']);
    let without_open = trimmed
        .strip_prefix("---\n")
        .or_else(|| trimmed.strip_prefix("---\r\n"))
        .ok_or(FrontmatterError::MissingDelimiter)?;

    let (yaml, body) =
        split_on_close_delim(without_open).ok_or(FrontmatterError::MissingDelimiter)?;
    Ok(FrontmatterRaw { yaml, body })
}

fn split_on_close_delim(s: &str) -> Option<(&str, &str)> {
    for pat in ["\n---\n", "\r\n---\r\n", "\n---\r\n", "\r\n---\n"] {
        if let Some(i) = s.find(pat) {
            return Some((&s[..i], &s[i + pat.len()..]));
        }
    }
    // Notepad-authored file ending exactly at the close delimiter (no trailing newline).
    if let Some(stripped) = s.strip_suffix("\r\n---") {
        return Some((stripped, ""));
    }
    if let Some(stripped) = s.strip_suffix("\n---") {
        return Some((stripped, ""));
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn splits_lf() {
        let raw = "---\nname: x\n---\nbody\n";
        let fm = split_frontmatter(raw).unwrap();
        assert_eq!(fm.yaml, "name: x");
        assert_eq!(fm.body, "body\n");
    }

    #[test]
    fn splits_crlf() {
        let raw = "---\r\nname: x\r\n---\r\nbody\r\n";
        let fm = split_frontmatter(raw).unwrap();
        assert_eq!(fm.yaml, "name: x");
        assert_eq!(fm.body, "body\r\n");
    }

    #[test]
    fn splits_notepad_no_trailing_newline() {
        let raw = "---\r\nname: x\r\n---";
        let fm = split_frontmatter(raw).unwrap();
        assert_eq!(fm.yaml, "name: x");
        assert_eq!(fm.body, "");
    }

    #[test]
    fn strips_bom_and_leading_newlines() {
        let raw = "\u{feff}\n\n---\nname: x\n---\nbody\n";
        let fm = split_frontmatter(raw).unwrap();
        assert_eq!(fm.yaml, "name: x");
    }

    #[test]
    fn rejects_missing_open() {
        assert_eq!(
            split_frontmatter("no delimiter here").unwrap_err(),
            FrontmatterError::MissingDelimiter
        );
    }

    #[test]
    fn rejects_missing_close() {
        assert_eq!(
            split_frontmatter("---\nname: x\nbody without close\n").unwrap_err(),
            FrontmatterError::MissingDelimiter
        );
    }
}
