package impl

import (
	"path/filepath"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// PRClassification determines how a PR should be reviewed.
type PRClassification int

const (
	// PRSkip means no AI review needed (docs, lockfiles, CI config).
	PRSkip PRClassification = iota
	// PRTrivial means a cheap fast review (config, test-only, small changes).
	PRTrivial
	// PRStandard means a normal review with graph context.
	PRStandard
)

func (c PRClassification) String() string {
	switch c {
	case PRSkip:
		return "skip"
	case PRTrivial:
		return "trivial"
	case PRStandard:
		return "standard"
	}
	return "unknown"
}

// skipExtensions are file extensions that never need AI review.
var skipExtensions = map[string]bool{
	".md": true, ".txt": true, ".rst": true, ".adoc": true,
	".lock": true, ".sum": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true,
}

// skipFiles are specific filenames that never need AI review.
var skipFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "Cargo.lock": true, "Gemfile.lock": true,
	"poetry.lock": true, "composer.lock": true,
	".gitignore": true, ".gitattributes": true, ".editorconfig": true,
	"LICENSE": true, "CHANGELOG.md": true, "README.md": true,
}

// trivialDirs are directories where changes are low-risk.
var trivialDirs = map[string]bool{
	"docs": true, "doc": true, "documentation": true,
	".github": true, ".circleci": true, ".gitlab": true,
	".vscode": true, ".idea": true,
}

// highRiskPathPatterns are substrings and suffixes that mark files as
// security-, auth-, payment-, or data-integrity-sensitive. Any PR touching
// one of these files is promoted to PRStandard regardless of size, because
// a 5-line diff in auth middleware deserves the same scrutiny as a 500-line
// feature PR elsewhere. Matched case-insensitively against the normalized
// forward-slash path.
var highRiskPathPatterns = []string{
	"/auth/", "/authn/", "/authz/",
	"/oauth", "/jwt", "/session",
	"/crypto/", "/encryption/", "/tls/",
	"/secret", "/credential", "/password",
	"/permission", "/rbac",
	"/payment", "/billing", "/invoice", "/refund", "/charge",
	"/migrations/", "/migration/",
	"/webhook",
	// fully-qualified filename hints
	"middleware.go", "middleware.py", "middleware.ts",
}

// hasHighRiskPath reports whether any file in the PR touches a high-risk path.
// The classification reason returned names the first matching path so operators
// can tell from the log why the gatekeeper promoted.
func hasHighRiskPath(files []services.PRFileInfo) (string, bool) {
	for _, f := range files {
		if f.Status == "removed" {
			continue
		}
		lower := strings.ToLower(filepath.ToSlash(f.Filename))
		for _, pat := range highRiskPathPatterns {
			if strings.Contains(lower, pat) {
				return f.Filename, true
			}
		}
	}
	return "", false
}

// ClassifyPR determines the review classification for a PR based on its files.
// This is the gatekeeper — it runs before any LLM call.
func ClassifyPR(files []services.PRFileInfo) (PRClassification, string) {
	if len(files) == 0 {
		return PRSkip, "no files"
	}

	codeFiles := 0
	testOnlyFiles := 0
	skippedCount := 0
	totalAdditions := 0

	for _, f := range files {
		if shouldSkipFile(f.Filename) {
			skippedCount++
			continue
		}
		if isTestFile(f.Filename) {
			testOnlyFiles++
		} else {
			codeFiles++
		}
		totalAdditions += int(f.Additions)
	}

	// All files are skippable (docs, lockfiles, images).
	if codeFiles == 0 && testOnlyFiles == 0 {
		return PRSkip, "only non-code files (docs, lockfiles, images)"
	}

	// High-risk paths (auth, payments, crypto, migrations, etc.) always get a
	// full review — a 5-line change to auth middleware is not "trivial."
	// This check runs before the test-only / small-change fast paths below.
	if riskFile, risky := hasHighRiskPath(files); risky {
		return PRStandard, "high-risk path touched: " + riskFile
	}

	// Test-only changes.
	if codeFiles == 0 && testOnlyFiles > 0 {
		return PRTrivial, "test-only changes"
	}

	// Very small change (< 20 added lines of code).
	if totalAdditions < 20 && codeFiles <= 2 {
		return PRTrivial, "small change"
	}

	return PRStandard, ""
}

func shouldSkipFile(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)

	if skipFiles[base] {
		return true
	}
	if skipExtensions[ext] {
		return true
	}

	// Check if in a trivial directory.
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if trivialDirs[p] {
			return true
		}
	}

	// Generated files.
	if strings.Contains(path, "/generated/") || strings.Contains(path, "/gen/") ||
		strings.HasSuffix(path, ".gen.go") || strings.HasSuffix(path, ".pb.go") {
		return true
	}

	return false
}

func isTestFile(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	testSuffixes := []string{
		"_test.go", ".test.ts", ".test.tsx", ".test.js", ".test.jsx",
		".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx",
	}
	for _, s := range testSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	if strings.HasPrefix(lower, "test_") && strings.HasSuffix(lower, ".py") {
		return true
	}
	return false
}

// FilterReviewableFiles returns only the files that need AI review,
// excluding docs, lockfiles, images, and generated code.
func FilterReviewableFiles(files []services.PRFileInfo) []services.PRFileInfo {
	var result []services.PRFileInfo
	for _, f := range files {
		if f.Status == "removed" {
			continue // deleted files have no + lines to review
		}
		if !shouldSkipFile(f.Filename) {
			result = append(result, f)
		}
	}
	return result
}
