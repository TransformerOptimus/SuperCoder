package services

import (
	"context"
	"errors"
	"strings"
)

// TerminalError marks a failure that must NOT be retried by Asynq. The
// streaming worker (WS5) uses errors.As to route terminal failures to
// markBatchTerminallyFailed via the durable batch manifest. Legacy callers
// (Pipeline.Index wrapper, /index/trigger consumer) treat it as an ordinary
// error and propagate it up — their existing retry policies apply.
//
// Reason is an optional stable code (e.g. "content_missing", "decode_error",
// "openai_embed") that the worker stamps into failed_files so the next /diff
// surfaces a useful reason to the client. Callers constructed via the legacy
// NewTerminalError helper leave Reason empty.
type TerminalError struct {
	Reason string
	Err    error
}

func (e *TerminalError) Error() string {
	if e == nil {
		return "terminal error"
	}
	switch {
	case e.Reason != "" && e.Err != nil:
		return "terminal[" + e.Reason + "]: " + e.Err.Error()
	case e.Reason != "":
		return "terminal[" + e.Reason + "]"
	case e.Err != nil:
		return "terminal: " + e.Err.Error()
	}
	return "terminal error"
}

func (e *TerminalError) Unwrap() error { return e.Err }

// NewTerminalError wraps err so that IsTerminal(err) returns true. The
// returned error has no Reason; WS5 code should prefer Terminal(reason, err)
// so the reason code is preserved through markBatchTerminallyFailed.
func NewTerminalError(err error) error {
	if err == nil {
		return nil
	}
	return &TerminalError{Err: err}
}

// Terminal wraps err with a stable Reason code. Returns nil when err is nil
// so callers can pass through pipeline errors without a nil check.
func Terminal(reason string, err error) error {
	if err == nil {
		return nil
	}
	return &TerminalError{Reason: reason, Err: err}
}

// IsTerminal reports whether err (or anything it wraps) is a TerminalError.
func IsTerminal(err error) bool {
	if err == nil {
		return false
	}
	var t *TerminalError
	return errors.As(err, &t)
}

// TerminalReason returns the Reason field when err is (or wraps) a
// TerminalError. Empty string otherwise. The WS5 worker uses this to stamp a
// stable code into failed_files instead of the raw error message.
func TerminalReason(err error) string {
	if err == nil {
		return ""
	}
	var t *TerminalError
	if errors.As(err, &t) {
		return t.Reason
	}
	return ""
}

// IsGloballyTerminalEmbedError reports whether err would fail uniformly
// across every sub-batch, making the bisect path in
// Pipeline.isolateTerminalEmbedFailure pointless wasted work. This is a
// strict subset of IsTerminalEmbedError: 401 (auth) and 403 (permission)
// errors are global — keys/IPs/orgs are per-request, not per-chunk, so every
// sub-batch returns the same error. 400 errors stay on the bisect path
// because they are typically content-specific (context-length, token-limit,
// invalid UTF-8) and bisect is the correct recovery.
//
// Called from isolateTerminalEmbedFailure to short-circuit the recursion
// when the failure is global, cutting cost from O(log2(N)*N) embed calls
// down to the single call that already fired. Critical during outages and
// credential misconfiguration where N can be several thousand chunks per
// batch: without this, a global 401 burns through OpenAI quota and
// saturates the worker rate limit on doomed retries, delaying recovery
// long after the upstream issue clears.
func IsGloballyTerminalEmbedError(err error) bool {
	if !IsTerminalEmbedError(err) {
		return false
	}
	code := extractAPIStatusCode(err.Error())
	return code == "401" || code == "403"
}

// IsTerminalEmbedError classifies an error returned by EmbedderService.Embed.
//
// Rules:
//
//	OpenAI 400 / 401 / 403 → terminal (bad request, auth, permission)
//	OpenAI 429 / 5xx, network errors → retryable
//	context.DeadlineExceeded → retryable (transient)
//	context.Canceled → retryable (caller-driven; WS5 retries)
//
// The supercoder embedder currently returns errors as formatted strings
// (see services/impl/embedder_service_impl.go:101), so classification is
// substring-based. To avoid false positives from incidental digits (e.g.
// "processed 400 chunks") the matcher first looks for the embedder's error
// prefix and only inspects the status-line portion that follows it.
func IsTerminalEmbedError(err error) bool {
	if err == nil {
		return false
	}
	// context errors are always retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	code := extractAPIStatusCode(err.Error())
	switch code {
	case "400", "401", "403":
		return true
	}
	return false
}

// extractAPIStatusCode scans msg for the embedder's error prefix
// ("API error: ") and returns the first whitespace-delimited token that
// follows (which in the embedder's format is the numeric HTTP status).
// Returns "" when the prefix is not present or the token is empty.
//
// Example:
//
//	"embedding batch 0-1000 failed: openai embedding API error: 400 Bad Request — ..."
//	                                                ^^^^^^^^^^^^^                prefix hit
//	                                                              ^^^            returned token
func extractAPIStatusCode(msg string) string {
	const prefix = "API error: "
	i := strings.Index(msg, prefix)
	if i < 0 {
		return ""
	}
	tail := msg[i+len(prefix):]
	// First whitespace-delimited token.
	if j := strings.IndexAny(tail, " \t\n"); j >= 0 {
		return tail[:j]
	}
	return tail
}
