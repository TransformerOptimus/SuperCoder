package impl

import (
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// ContractDiff captures a same-named function's signature change. Body-only edits never appear here.
type ContractDiff struct {
	FilePath     string
	Function     string
	OldSignature string
	NewSignature string
	Changes      []string // "params", "return_type", "modifiers"
}

// detectContractDiffs pairs added/removed signature lines per file. Single-line decls only.
func detectContractDiffs(files []services.PRFileInfo) []ContractDiff {
	var out []ContractDiff
	for _, f := range files {
		if f.Patch == "" {
			continue
		}
		added := map[string]string{}
		removed := map[string]string{}
		for _, line := range strings.Split(f.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
				continue
			case strings.HasPrefix(line, "+"):
				if name, sig := extractSignature(line[1:]); name != "" {
					added[name] = sig
				}
			case strings.HasPrefix(line, "-"):
				if name, sig := extractSignature(line[1:]); name != "" {
					removed[name] = sig
				}
			}
		}
		for name, newSig := range added {
			oldSig, ok := removed[name]
			if !ok || oldSig == newSig {
				continue
			}
			changes := classifySignatureChange(oldSig, newSig)
			if len(changes) == 0 {
				continue
			}
			out = append(out, ContractDiff{
				FilePath:     f.Filename,
				Function:     name,
				OldSignature: condenseSig(oldSig),
				NewSignature: condenseSig(newSig),
				Changes:      changes,
			})
		}
	}
	return out
}

// extractSignature returns (name, trimmedLine) when the line is a function decl.
func extractSignature(rawLine string) (name, sig string) {
	trimmed := strings.TrimSpace(rawLine)
	if trimmed == "" {
		return "", ""
	}

	// Go: func Name(... or func (r *T) Name(...
	if strings.HasPrefix(trimmed, "func ") {
		rest := trimmed[5:]
		if strings.HasPrefix(rest, "(") {
			if closeIdx := strings.Index(rest, ")"); closeIdx >= 0 {
				rest = strings.TrimSpace(rest[closeIdx+1:])
			}
		}
		if parenIdx := strings.IndexAny(rest, "([<"); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate, trimmed
			}
		}
	}

	// JS/TS: function Name(... or async function Name(...
	if strings.Contains(trimmed, "function ") {
		idx := strings.Index(trimmed, "function ")
		rest := trimmed[idx+len("function "):]
		if parenIdx := strings.IndexAny(rest, "(<"); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate, trimmed
			}
		}
	}

	// Python: def name(...
	if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") {
		rest := strings.TrimPrefix(strings.TrimPrefix(trimmed, "async "), "def ")
		if parenIdx := strings.Index(rest, "("); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate, trimmed
			}
		}
	}

	// Rust: fn name(... or pub fn name(...
	if strings.HasPrefix(trimmed, "fn ") || strings.Contains(trimmed, " fn ") {
		idx := strings.Index(trimmed, "fn ")
		rest := trimmed[idx+3:]
		if parenIdx := strings.IndexAny(rest, "(<"); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate, trimmed
			}
		}
	}

	// Java/C#/Kotlin: modifiers ... name(...
	if name := extractJavaLikeFunc(trimmed); name != "" {
		return name, trimmed
	}

	return "", ""
}

// classifySignatureChange returns labels (params / return_type / modifiers) that differ.
func classifySignatureChange(oldSig, newSig string) []string {
	oldParams, oldRet := splitSignature(oldSig)
	newParams, newRet := splitSignature(newSig)
	oldMods := extractModifiers(oldSig)
	newMods := extractModifiers(newSig)

	var changes []string
	if normalizeWS(oldParams) != normalizeWS(newParams) {
		changes = append(changes, "params")
	}
	if normalizeWS(oldRet) != normalizeWS(newRet) {
		changes = append(changes, "return_type")
	}
	if oldMods != newMods {
		changes = append(changes, "modifiers")
	}
	return changes
}

// splitSignature returns (paramsBlock, returnTypeOrTrailer) for a single-line decl.
func splitSignature(line string) (params, ret string) {
	open := strings.Index(line, "(")
	if open < 0 {
		return "", ""
	}
	depth := 0
	close := -1
	for i := open; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			close = i
			break
		}
	}
	if close < 0 {
		return line[open+1:], ""
	}
	params = line[open+1 : close]
	rest := strings.TrimSpace(line[close+1:])
	if idx := strings.IndexAny(rest, "{=;"); idx >= 0 {
		rest = strings.TrimSpace(rest[:idx])
	}
	return params, rest
}

var modifierWords = map[string]bool{
	"async": true, "public": true, "private": true, "protected": true,
	"static": true, "final": true, "abstract": true, "override": true,
	"synchronized": true, "suspend": true, "open": true, "internal": true,
	"export": true, "default": true, "pub": true, "extern": true, "unsafe": true,
}

func extractModifiers(line string) string {
	parts := strings.Fields(line)
	var mods []string
	for _, p := range parts {
		if modifierWords[p] {
			mods = append(mods, p)
		} else if p == "func" || p == "fn" || p == "function" || p == "def" {
			break
		}
	}
	return strings.Join(mods, " ")
}

func condenseSig(line string) string {
	if idx := strings.IndexAny(line, "{"); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(normalizeWS(line))
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	if !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isIdentStart(c) && !(c >= '0' && c <= '9') {
			return false
		}
	}
	// Reject reserved words that aren't valid function names.
	switch s {
	case "if", "for", "while", "switch", "return", "const", "let", "var", "type", "class", "interface", "enum":
		return false
	}
	return true
}
