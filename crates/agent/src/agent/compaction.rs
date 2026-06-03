use crate::llm::types::ChatMessage;
use super::config::CompactionConfig;

/// Check if compaction is needed based on actual token count OR message array length.
/// Triggers when:
/// - total_tokens >= threshold_pct * context_limit (token-based), OR
/// - message_count >= max_messages (array length guard)
/// If total_tokens is 0 (first call, or after compaction reset), only the message count check applies.
pub fn needs_compaction_by_tokens(total_tokens: usize, message_count: usize, config: &CompactionConfig) -> bool {
    // Message array length guard — prevents OpenAI 400 "array too long" errors.
    // Always active, even when token-based auto-compaction is disabled.
    if message_count >= config.max_messages {
        return true;
    }
    // Token-based check — skipped when the context limit is unknown.
    if !config.auto_compact || total_tokens == 0 {
        return false;
    }
    let threshold = (config.context_limit as f64 * config.threshold_pct) as usize;
    total_tokens >= threshold
}

/// Estimate total token count from message content using chars/4 heuristic.
/// Used as a pre-LLM safety check when total_tokens_used is stale.
pub fn estimate_token_count(messages: &[ChatMessage]) -> usize {
    messages.iter().map(|m| {
        let content_len = m.content.as_ref().map_or(0, |c| c.text().len());
        let tool_calls_len = m.tool_calls.as_ref().map_or(0, |tcs| {
            tcs.iter().map(|tc| tc.function.name.len() + tc.function.arguments.len() + 20).sum()
        });
        let thinking_len = m.thinking.as_ref().map_or(0, |t| t.len());
        (content_len + tool_calls_len + thinking_len) / 4 + 4 // +4 per message overhead (role, delimiters)
    }).sum()
}

/// Calculate the boundaries for compaction.
///
/// Strategy: keep system prompt + last `keep_recent` messages, summarize everything in between.
/// The `end` boundary is snapped to a turn boundary so tool_call/tool_result pairs are never split.
///
/// Returns `Some((start, end))` — the range of messages to compact.
/// Returns `None` if there's nothing meaningful to compact.
pub fn compaction_boundaries(
    messages: &[ChatMessage],
    keep_recent: usize,
) -> Option<(usize, usize)> {
    if messages.is_empty() {
        return None;
    }

    // Start after system prompt
    let start = if messages[0].role == "system" { 1 } else { 0 };

    // Raw end = total - keep_recent
    let total = messages.len();
    let raw_end = total.saturating_sub(keep_recent);

    if raw_end <= start {
        return None;
    }

    // Snap end to a turn boundary — don't split tool_call/tool_result pairs.
    // Walk backward from raw_end until we land on a message that isn't a "tool" result
    // and isn't an assistant with tool_calls whose results would be cut off.
    let mut end = snap_to_turn_boundary(messages, raw_end);

    // Bug 3 fix (Layer 3): forward-walk safety check. After the backward snap,
    // verify no retained tool_result in messages[end..] references a tool_call_id
    // that exists ONLY in the about-to-be-compacted range [start..end). If found,
    // advance `end` to drop those orphans (compacting them away is safer than
    // sending an orphaned tool_result to the LLM).
    end = advance_past_orphan_tool_results(messages, start, end);

    if end <= start {
        return None;
    }

    // Need at least 2 messages to be worth compacting
    if end - start < 2 {
        return None;
    }

    Some((start, end))
}

/// Advance `end` forward past any tool_result messages in messages[end..] whose
/// tool_call_id has no matching tool_call in the kept regions (messages[..start]
/// or messages[end..]). This is a defense-in-depth check on top of
/// `snap_to_turn_boundary` for unusual message sequences.
fn advance_past_orphan_tool_results(
    messages: &[ChatMessage],
    start: usize,
    end: usize,
) -> usize {
    use std::collections::HashSet;

    // Build the set of tool_call_ids that will survive compaction.
    let mut valid_ids: HashSet<&str> = HashSet::new();
    for msg in messages[..start].iter().chain(messages[end..].iter()) {
        if let Some(ref tcs) = msg.tool_calls {
            for tc in tcs {
                valid_ids.insert(tc.id.as_str());
            }
        }
    }

    let mut new_end = end;
    while new_end < messages.len() {
        let msg = &messages[new_end];
        if msg.role != "tool" {
            break;
        }
        let id = match msg.tool_call_id.as_deref() {
            Some(id) => id,
            None => break,
        };
        if valid_ids.contains(id) {
            break;
        }
        // Orphan — push it into the compacted range.
        new_end += 1;
    }
    new_end
}

/// Find the nearest safe compaction boundary at or before `target`.
/// A safe boundary is a position where the message at `target` is NOT:
/// - A tool result (role="tool") — would orphan it from its assistant+tool_calls
/// - Immediately after an assistant with tool_calls — would split the pair
fn snap_to_turn_boundary(messages: &[ChatMessage], target: usize) -> usize {
    let mut end = target;
    while end > 0 {
        // If the message just before `end` is a tool result, we're mid-turn
        if messages[end - 1].role == "tool" {
            end -= 1;
            continue;
        }
        // If it's an assistant with tool_calls, the tool results follow after it
        // and would be split — step back past this assistant too
        if messages[end - 1].role == "assistant" && messages[end - 1].tool_calls.is_some() {
            end -= 1;
            continue;
        }
        break;
    }
    end
}

/// Build the compaction prompt — formats the messages to be summarized.
/// The summarization instruction is provided separately as a system message in the LLM call.
pub fn build_compaction_prompt(messages: &[ChatMessage]) -> String {
    let mut prompt = String::new();

    for msg in messages {
        let role = &msg.role;
        let content = msg.content.as_ref().map(|c| c.text()).unwrap_or("[no content]");
        prompt.push_str(&format!("{role}: {content}\n\n"));
    }

    prompt
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::llm::types::{FunctionCall, ToolCall};

    fn default_config() -> CompactionConfig {
        CompactionConfig::default()
    }

    #[test]
    fn test_needs_compaction_by_tokens_zero() {
        let config = default_config();
        assert!(!needs_compaction_by_tokens(0, 5, &config));
    }

    #[test]
    fn test_needs_compaction_by_tokens_under() {
        let config = default_config();
        assert!(!needs_compaction_by_tokens(50_000, 5, &config));
    }

    #[test]
    fn test_needs_compaction_by_tokens_over() {
        let config = default_config();
        assert!(needs_compaction_by_tokens(110_000, 5, &config));
    }

    #[test]
    fn test_needs_compaction_by_tokens_exact_threshold() {
        let mut config = default_config();
        config.context_limit = 100;
        assert!(needs_compaction_by_tokens(80, 5, &config));
    }

    #[test]
    fn test_needs_compaction_by_message_count() {
        let config = default_config();
        // Under max_messages → no compaction (even with 0 tokens)
        assert!(!needs_compaction_by_tokens(0, 100, &config));
        // At max_messages → triggers compaction regardless of tokens
        assert!(needs_compaction_by_tokens(0, config.max_messages, &config));
        assert!(needs_compaction_by_tokens(0, config.max_messages + 1, &config));
    }

    #[test]
    fn test_auto_compact_disabled_suppresses_token_trigger() {
        let mut config = default_config();
        config.auto_compact = false;
        // Token threshold would normally fire — but auto-compaction is off.
        assert!(!needs_compaction_by_tokens(110_000, 5, &config));
        // The message-count guard still applies regardless.
        assert!(needs_compaction_by_tokens(0, config.max_messages, &config));
    }

    #[test]
    fn test_compaction_boundaries_preserves_system_prompt() {
        let messages = vec![
            ChatMessage::system("You are helpful"),
            ChatMessage::user("msg 1"),
            ChatMessage::assistant(Some("reply 1".into()), None, None),
            ChatMessage::user("msg 2"),
            ChatMessage::assistant(Some("reply 2".into()), None, None),
            ChatMessage::user("msg 3"),
            ChatMessage::assistant(Some("reply 3".into()), None, None),
        ];
        let result = compaction_boundaries(&messages, 2);
        // Should compact [1, 5) — everything except system prompt and last 2
        assert_eq!(result, Some((1, 5)));
    }

    #[test]
    fn test_compaction_boundaries_keeps_recent_turns() {
        let messages = vec![
            ChatMessage::user("old 1"),
            ChatMessage::assistant(Some("old reply".into()), None, None),
            ChatMessage::user("recent 1"),
            ChatMessage::assistant(Some("recent reply".into()), None, None),
        ];
        let result = compaction_boundaries(&messages, 2);
        // Compact [0, 2) — first 2 messages, keep last 2
        assert_eq!(result, Some((0, 2)));
    }

    #[test]
    fn test_compaction_boundaries_nothing_to_compact() {
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("hello"),
        ];
        let result = compaction_boundaries(&messages, 10);
        assert_eq!(result, None);
    }

    #[test]
    fn test_compaction_boundaries_too_few_messages() {
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("only one"),
        ];
        let result = compaction_boundaries(&messages, 0);
        assert_eq!(result, None);
    }

    #[test]
    fn test_snap_avoids_splitting_tool_pair() {
        let tc = vec![ToolCall {
            id: "tc1".into(),
            type_: "function".into(),
            function: FunctionCall { name: "read".into(), arguments: "{}".into() },
        }];
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("first question"),                  // idx 1
            ChatMessage::assistant(Some("answer 1".into()), None, None),// idx 2
            ChatMessage::user("do something"),                    // idx 3
            ChatMessage::assistant(None, Some(tc), None),               // idx 4: assistant with tools
            ChatMessage::tool_result("tc1", "file contents"),     // idx 5: tool result
            ChatMessage::user("thanks"),                          // idx 6
            ChatMessage::assistant(Some("done".into()), None, None),    // idx 7
        ];

        // keep_recent=2 → raw_end = 8-2 = 6
        // messages[5] is tool → snap to 5, messages[4] is assistant+tools → snap to 4
        // messages[3] is user → safe. end=4.
        // Compacts [1, 4) = user("first question"), assistant("answer 1"), user("do something")
        let result = compaction_boundaries(&messages, 2);
        assert_eq!(result, Some((1, 4)));
    }

    #[test]
    fn test_snap_leaves_clean_boundary() {
        // user + assistant(text only) + user + assistant(text only)
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("msg 1"),
            ChatMessage::assistant(Some("reply 1".into()), None, None),
            ChatMessage::user("msg 2"),
            ChatMessage::assistant(Some("reply 2".into()), None, None),
        ];
        // keep_recent=2 → raw_end = 5-2 = 3
        // messages[2] is assistant(no tool_calls) → safe
        let result = compaction_boundaries(&messages, 2);
        assert_eq!(result, Some((1, 3)));
    }

    #[test]
    fn test_snap_with_multiple_tool_results() {
        let tc = vec![
            ToolCall { id: "tc1".into(), type_: "function".into(), function: FunctionCall { name: "read".into(), arguments: "{}".into() } },
            ToolCall { id: "tc2".into(), type_: "function".into(), function: FunctionCall { name: "grep".into(), arguments: "{}".into() } },
        ];
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("first"),                            // idx 1
            ChatMessage::assistant(Some("ok".into()), None, None),       // idx 2
            ChatMessage::user("search"),                           // idx 3
            ChatMessage::assistant(None, Some(tc), None),                // idx 4
            ChatMessage::tool_result("tc1", "result 1"),           // idx 5
            ChatMessage::tool_result("tc2", "result 2"),           // idx 6
            ChatMessage::assistant(Some("found it".into()), None, None), // idx 7
            ChatMessage::user("thanks"),                           // idx 8
        ];
        // keep_recent=2 → raw_end=7
        // messages[6] is tool → 6, messages[5] is tool → 5,
        // messages[4] is assistant+tools → 4, messages[3] is user → safe. end=4
        let result = compaction_boundaries(&messages, 2);
        assert_eq!(result, Some((1, 4)));
    }

    #[test]
    fn test_snap_returns_none_when_all_are_tool_pairs() {
        let tc = vec![ToolCall {
            id: "tc1".into(), type_: "function".into(),
            function: FunctionCall { name: "read".into(), arguments: "{}".into() },
        }];
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::assistant(None, Some(tc), None),
            ChatMessage::tool_result("tc1", "result"),
        ];
        // keep_recent=0 → raw_end=3, but everything is a tool pair
        // snap: messages[2] is tool → 2, messages[1] is assistant+tools → 1, messages[0] is system
        // end=1, start=1 → end <= start → None
        let result = compaction_boundaries(&messages, 0);
        assert_eq!(result, None);
    }

    #[test]
    fn test_build_compaction_prompt_formats_messages() {
        let messages = vec![
            ChatMessage::user("What does main.rs do?"),
            ChatMessage::assistant(Some("It starts the server.".into()), None, None),
            ChatMessage::user("Can you refactor it?"),
        ];
        let prompt = build_compaction_prompt(&messages);
        assert!(prompt.contains("user: What does main.rs do?"));
        assert!(prompt.contains("assistant: It starts the server."));
        assert!(prompt.contains("user: Can you refactor it?"));
    }

    #[test]
    fn test_build_compaction_prompt_handles_none_content() {
        let messages = vec![
            ChatMessage::assistant(None, Some(vec![ToolCall {
                id: "call_1".to_string(),
                type_: "function".to_string(),
                function: FunctionCall {
                    name: "read".to_string(),
                    arguments: "{}".to_string(),
                },
            }]), None),
        ];
        let prompt = build_compaction_prompt(&messages);
        assert!(prompt.contains("assistant: [no content]"));
    }

    #[test]
    fn test_build_compaction_prompt_no_instruction_text() {
        let messages = vec![ChatMessage::user("hello")];
        let prompt = build_compaction_prompt(&messages);
        assert!(!prompt.contains("Summarize"), "Prompt should not contain instruction text");
    }

    // ─── Bug 3 fix (Layer 3): forward-walk orphan check ───

    #[test]
    fn test_forward_walk_advances_past_unmatched_tool_result() {
        // Construct a scenario where the backward snap lands on `end` but the
        // first kept message is a `tool` whose tool_call_id only existed in
        // the about-to-be-compacted range. The forward walk must advance past
        // this orphan so it doesn't get shipped to the LLM.
        let tc1 = vec![ToolCall {
            id: "tc1".into(),
            type_: "function".into(),
            function: FunctionCall { name: "read".into(), arguments: "{}".into() },
        }];
        let messages = vec![
            ChatMessage::system("prompt"),                          // 0
            ChatMessage::assistant(None, Some(tc1), None),          // 1: would be compacted
            ChatMessage::tool_result("tc1", "data"),                // 2: orphan if assistant compacted
            ChatMessage::user("next"),                              // 3
            ChatMessage::assistant(Some("ok".into()), None, None),  // 4
        ];
        // Force a scenario where snap backward lands at end=2 (mid-pair).
        // We exercise the helper directly:
        let advanced = advance_past_orphan_tool_results(&messages, 1, 2);
        assert_eq!(
            advanced, 3,
            "tool_result at idx 2 references tc1 which lives in [1..2) (about to be compacted) — must advance"
        );
    }

    #[test]
    fn test_forward_walk_keeps_valid_tool_result() {
        // tool_result references a tool_call that survives in messages[end..].
        let tc1 = vec![ToolCall {
            id: "tc1".into(),
            type_: "function".into(),
            function: FunctionCall { name: "read".into(), arguments: "{}".into() },
        }];
        let messages = vec![
            ChatMessage::system("prompt"),                          // 0
            ChatMessage::user("old"),                               // 1: compacted
            ChatMessage::assistant(None, Some(tc1), None),          // 2: KEPT (assistant)
            ChatMessage::tool_result("tc1", "data"),                // 3: KEPT (paired)
        ];
        // start=1, end=2 → kept range starts with assistant(tc1), tool_result(tc1).
        let advanced = advance_past_orphan_tool_results(&messages, 1, 2);
        assert_eq!(advanced, 2, "tc1 is in the kept range — should NOT advance");
    }

    #[test]
    fn test_forward_walk_noop_when_first_kept_is_not_tool() {
        let messages = vec![
            ChatMessage::system("prompt"),
            ChatMessage::user("old"),
            ChatMessage::user("new"),
        ];
        let advanced = advance_past_orphan_tool_results(&messages, 1, 2);
        assert_eq!(advanced, 2, "non-tool first kept message — no advancement");
    }
}
