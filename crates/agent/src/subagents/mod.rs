pub mod parse;
pub mod registry;
pub mod tool;
pub mod write_lock;

pub use parse::{parse_subagent_md, ParsedSubagent, SubagentParseError};
pub use registry::{read_tier_from_fs, Origin, Subagent, SubagentInput, SubagentRegistry};
pub use tool::{ApprovalHandlerFactory, SpawnSubagentTool, SubagentInheritance};
pub use write_lock::WriteLockRegistry;

/// Subagents embedded in the binary at compile time. Sourced from
/// `src-tauri/agent/default-subagents/`. Empty in v1 (just a `.gitkeep`).
pub static DEFAULT_SUBAGENTS: include_dir::Dir<'_> =
    include_dir::include_dir!("$CARGO_MANIFEST_DIR/default-subagents");
