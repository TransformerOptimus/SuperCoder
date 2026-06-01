pub mod parse;
pub mod registry;
pub mod tool;

pub use parse::{parse_skill_md, ParsedSkill, SkillParseError};
pub use registry::{Origin, Skill, SkillRegistry};
pub use tool::SkillTool;

/// Skills embedded in the binary at compile time. Sourced from
/// `src-tauri/agent/default-skills/`. In v1 the directory is empty
/// (just a `.gitkeep`). Iterating child dirs yields zero entries.
pub static DEFAULT_SKILLS: include_dir::Dir<'_> =
    include_dir::include_dir!("$CARGO_MANIFEST_DIR/default-skills");
