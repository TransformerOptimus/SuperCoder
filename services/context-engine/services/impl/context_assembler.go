package impl

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// GraphContext holds pre-assembled callers, callees, and importer paths for changed code.
type GraphContext struct {
	CallerMap         map[string][]string
	CalleeRefs        []CalleeRef
	Importers         []string
	Formatted         string
	FunctionsAnalyzed int
	CallersFound      int
	CalleesFound      int
}

// CalleeRef points the body-fetcher at a first-hop callee with line range.
type CalleeRef struct {
	CallerFile  string
	CallerName  string
	Name        string
	FilePath    string
	StartLine   int
	EndLine     int
}

// maxFunctionsInPrompt caps the dense graph-context block to keep token cost bounded.
const maxFunctionsInPrompt = 30

// AssembleGraphContext renders a dense, risk-sorted prompt block of callers and contract changes.
func AssembleGraphContext(
	ctx context.Context,
	graph repositories.GraphRepository,
	collectionName string,
	files []services.PRFileInfo,
	contractDiffs []ContractDiff,
	filter services.SearchFilter,
	logger *zap.Logger,
) *GraphContext {
	if graph == nil {
		logger.Warn("graph context skipped: GraphRepository not wired (indexer disabled?)")
		return nil
	}

	exists, err := graph.Exists(ctx, collectionName)
	if err != nil {
		logger.Warn("graph context skipped: Exists() check failed",
			zap.String("collection", collectionName),
			zap.Error(err))
		return nil
	}
	if !exists {
		logger.Warn("graph context skipped: graph not populated for this collection",
			zap.String("collection", collectionName))
		return nil
	}

	changedFuncs := extractChangedFunctions(files)
	if len(changedFuncs) == 0 {
		logger.Info("graph context skipped: no function definitions detected in diff",
			zap.Int("files", len(files)))
		return nil
	}

	contractByKey := indexContractDiffs(contractDiffs)

	type fnEntry struct {
		Name     string
		FilePath string
		Callers  []repositories.GraphResult
		Callees  []repositories.GraphResult
		Diff     *ContractDiff
	}

	var entries []fnEntry
	var calleeRefs []CalleeRef
	calleeSeen := make(map[string]bool)
	for _, cf := range changedFuncs {
		callers, callErr := graph.GetBlastRadius(ctx, collectionName, cf.Name, cf.FilePath, filter)
		if callErr != nil {
			logger.Debug("GetBlastRadius failed", zap.String("func", cf.Name), zap.Error(callErr))
		}
		callees, depErr := graph.GetDependencies(ctx, collectionName, cf.Name, cf.FilePath, filter)
		if depErr != nil {
			logger.Debug("GetDependencies failed", zap.String("func", cf.Name), zap.Error(depErr))
		}
		key := cf.FilePath + ":" + cf.Name
		diff := contractByKey[key]
		if len(callers) == 0 && len(callees) == 0 && diff == nil {
			continue
		}
		entries = append(entries, fnEntry{
			Name:     cf.Name,
			FilePath: cf.FilePath,
			Callers:  callers,
			Callees:  callees,
			Diff:     diff,
		})
		for _, cl := range callees {
			if cl.FilePath == "" || cl.Name == "" {
				continue
			}
			calleeKey := cl.FilePath + ":" + cl.Name
			if calleeSeen[calleeKey] {
				continue
			}
			calleeSeen[calleeKey] = true
			calleeRefs = append(calleeRefs, CalleeRef{
				CallerFile: cf.FilePath,
				CallerName: cf.Name,
				Name:       cl.Name,
				FilePath:   cl.FilePath,
				StartLine:  cl.StartLine,
				EndLine:    cl.EndLine,
			})
		}
	}

	if len(entries) == 0 {
		logger.Info("graph context skipped: no callers, callees, or contract diffs",
			zap.Int("functions_analyzed", len(changedFuncs)))
		return nil
	}

	importerSet := make(map[string]bool)
	for _, f := range files {
		if f.Patch == "" {
			continue
		}
		imps, err := graph.GetImporters(ctx, collectionName, f.Filename, filter)
		if err != nil {
			logger.Debug("GetImporters failed", zap.String("file", f.Filename), zap.Error(err))
			continue
		}
		for _, imp := range imps {
			if imp.FilePath != "" {
				importerSet[imp.FilePath] = true
			}
		}
	}
	importers := make([]string, 0, len(importerSet))
	for p := range importerSet {
		importers = append(importers, p)
	}
	sort.Strings(importers)
	const maxImportersShown = 20
	if len(importers) > maxImportersShown {
		importers = importers[:maxImportersShown]
	}

	// Sort by per-fn risk: contract diff > caller count > callee count.
	sort.SliceStable(entries, func(i, j int) bool {
		ri := contractWeight(entries[i].Diff)*10 + len(entries[i].Callers) + len(entries[i].Callees)/2
		rj := contractWeight(entries[j].Diff)*10 + len(entries[j].Callers) + len(entries[j].Callees)/2
		return ri > rj
	})

	totalEntries := len(entries)
	if len(entries) > maxFunctionsInPrompt {
		entries = entries[:maxFunctionsInPrompt]
	}

	result := &GraphContext{CallerMap: make(map[string][]string)}
	var sb strings.Builder

	sb.WriteString("<graph_context>\n")
	sb.WriteString("Markers: `←` caller, `→` callee, `⚠` contract change. Sorted highest risk first.\n\n")

	totalCallers := 0
	totalCallees := 0
	for _, e := range entries {
		header := fmt.Sprintf("%s [%s] %d callers, %d callees",
			e.Name, e.FilePath, len(e.Callers), len(e.Callees))
		sb.WriteString(header + "\n")

		if e.Diff != nil {
			for _, change := range e.Diff.Changes {
				sb.WriteString(fmt.Sprintf("    ⚠ %s: %s → %s\n",
					change, e.Diff.OldSignature, e.Diff.NewSignature))
			}
		}

		key := e.FilePath + ":" + e.Name
		const maxCallersShown = 5
		const maxCalleesShown = 5

		shownCallers := e.Callers
		if len(shownCallers) > maxCallersShown {
			shownCallers = shownCallers[:maxCallersShown]
		}
		var descs []string
		for _, c := range shownCallers {
			desc := fmt.Sprintf("    ← %s [%s]", c.Name, c.FilePath)
			descs = append(descs, desc)
			sb.WriteString(desc + "\n")
		}
		if len(e.Callers) > maxCallersShown {
			sb.WriteString(fmt.Sprintf("    ... +%d more callers\n", len(e.Callers)-maxCallersShown))
		}

		shownCallees := e.Callees
		if len(shownCallees) > maxCalleesShown {
			shownCallees = shownCallees[:maxCalleesShown]
		}
		for _, c := range shownCallees {
			sb.WriteString(fmt.Sprintf("    → %s [%s]\n", c.Name, c.FilePath))
		}
		if len(e.Callees) > maxCalleesShown {
			sb.WriteString(fmt.Sprintf("    ... +%d more callees\n", len(e.Callees)-maxCalleesShown))
		}
		sb.WriteByte('\n')

		result.CallerMap[key] = descs
		totalCallers += len(e.Callers)
		totalCallees += len(e.Callees)
	}

	if totalEntries > maxFunctionsInPrompt {
		sb.WriteString(fmt.Sprintf("Showing top %d of %d changed functions.\n",
			maxFunctionsInPrompt, totalEntries))
	}

	if len(importers) > 0 {
		sb.WriteString("\nImporters of changed files:\n")
		for _, p := range importers {
			sb.WriteString("  - " + p + "\n")
		}
	}

	sb.WriteString("</graph_context>")

	result.Formatted = sb.String()
	result.FunctionsAnalyzed = len(changedFuncs)
	result.CallersFound = totalCallers
	result.CalleesFound = totalCallees
	result.CalleeRefs = calleeRefs
	result.Importers = importers

	logger.Info("graph context assembled",
		zap.Int("functions_analyzed", result.FunctionsAnalyzed),
		zap.Int("callers_found", result.CallersFound),
		zap.Int("callees_found", result.CalleesFound),
		zap.Int("importers", len(result.Importers)),
		zap.Int("contract_diffs", len(contractDiffs)),
		zap.Int("formatted_bytes", len(result.Formatted)))

	return result
}

func indexContractDiffs(diffs []ContractDiff) map[string]*ContractDiff {
	out := make(map[string]*ContractDiff, len(diffs))
	for i := range diffs {
		key := diffs[i].FilePath + ":" + diffs[i].Function
		out[key] = &diffs[i]
	}
	return out
}

func contractWeight(d *ContractDiff) int {
	if d == nil {
		return 0
	}
	return len(d.Changes)
}

type changedFunc struct {
	Name     string
	FilePath string
}

// isIdentStart reports whether b can start an identifier (letter or underscore).
func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

// extractChangedFunctions returns every function whose body or signature was modified.
// Tracks the enclosing function via @@ hint, +/- decls, and context-line decls.
func extractChangedFunctions(files []services.PRFileInfo) []changedFunc {
	var funcs []changedFunc
	seen := make(map[string]bool)

	for _, f := range files {
		if f.Patch == "" {
			continue
		}

		var currentFn string
		for _, line := range strings.Split(f.Patch, "\n") {
			if strings.HasPrefix(line, "@@") {
				if hint := extractHunkFuncHint(line); hint != "" {
					currentFn = hint
				}
				continue
			}
			if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
				continue
			}

			body := line
			isAdd := strings.HasPrefix(line, "+")
			isDel := strings.HasPrefix(line, "-")
			if isAdd || isDel {
				body = body[1:]
			}

			trimmed := strings.TrimSpace(body)
			if name := extractFuncName(trimmed); name != "" {
				currentFn = name
			}

			if !isAdd && !isDel {
				continue
			}
			if currentFn == "" {
				continue
			}
			key := f.Filename + ":" + currentFn
			if !seen[key] {
				seen[key] = true
				funcs = append(funcs, changedFunc{Name: currentFn, FilePath: f.Filename})
			}
		}
	}

	if len(funcs) > 50 {
		funcs = funcs[:50]
	}
	return funcs
}

// extractHunkFuncHint pulls the function-name hint Git appends after `@@ ... @@`.
func extractHunkFuncHint(header string) string {
	idx := strings.Index(header, "@@")
	if idx < 0 {
		return ""
	}
	rest := header[idx+2:]
	idx2 := strings.Index(rest, "@@")
	if idx2 < 0 {
		return ""
	}
	hint := strings.TrimSpace(rest[idx2+2:])
	if hint == "" {
		return ""
	}
	return extractFuncName(hint)
}

// extractFuncName returns the function name if `trimmed` is a recognised decl line.
func extractFuncName(trimmed string) string {
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
				return candidate
			}
		}
	}

	if strings.Contains(trimmed, "function ") || strings.HasPrefix(trimmed, "export ") {
		rest := trimmed
		rest = strings.TrimPrefix(rest, "export ")
		rest = strings.TrimPrefix(rest, "default ")
		rest = strings.TrimPrefix(rest, "async ")
		rest = strings.TrimPrefix(rest, "function ")
		if parenIdx := strings.IndexAny(rest, "(={: "); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate
			}
		}
	}

	// Python / Ruby: def name(args). Ruby additionally allows paren-less defs,
	// `self.` / module prefixes, and `?` / `!` / `=` suffix chars on names.
	if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") {
		rest := strings.TrimPrefix(strings.TrimPrefix(trimmed, "async "), "def ")
		// Drop Ruby receiver prefixes: self.foo, ClassName.foo, Mod::Class.foo.
		if dot := strings.LastIndex(rest, "."); dot >= 0 {
			head := rest[:dot]
			if !strings.ContainsAny(head, "( ") {
				rest = rest[dot+1:]
			}
		}
		end := strings.IndexAny(rest, "( ")
		if end < 0 {
			end = len(rest)
		}
		candidate := strings.TrimRight(strings.TrimSpace(rest[:end]), "?!=")
		if isIdent(candidate) {
			return candidate
		}
	}

	if name := extractJavaLikeFunc(trimmed); name != "" {
		return name
	}

	if strings.HasPrefix(trimmed, "fn ") || strings.Contains(trimmed, " fn ") {
		idx := strings.Index(trimmed, "fn ")
		rest := trimmed[idx+3:]
		if parenIdx := strings.IndexAny(rest, "(<"); parenIdx > 0 {
			candidate := strings.TrimSpace(rest[:parenIdx])
			if isIdent(candidate) {
				return candidate
			}
		}
	}

	return ""
}

// javaModifiers are keywords that precede a method return type in Java/C#/Kotlin.
var javaModifiers = map[string]bool{
	"public": true, "private": true, "protected": true, "internal": true,
	"static": true, "final": true, "abstract": true, "override": true,
	"synchronized": true, "suspend": true, "open": true,
}

// extractJavaLikeFunc tries to extract a method name from Java/C#/Kotlin-style declarations.
// Pattern: [modifiers...] [return_type] methodName(
func extractJavaLikeFunc(line string) string {
	// Must contain a '(' to be a function declaration.
	parenIdx := strings.Index(line, "(")
	if parenIdx <= 0 {
		return ""
	}

	before := strings.TrimSpace(line[:parenIdx])
	parts := strings.Fields(before)
	if len(parts) < 2 {
		return ""
	}

	// The last token before '(' is the method name.
	candidate := parts[len(parts)-1]
	if !isIdentStart(candidate[0]) {
		return ""
	}

	// At least one preceding token must be a known modifier or "fun" (Kotlin).
	hasModifier := false
	for _, p := range parts[:len(parts)-1] {
		if javaModifiers[p] || p == "fun" {
			hasModifier = true
			break
		}
	}
	if !hasModifier {
		return ""
	}

	return candidate
}
