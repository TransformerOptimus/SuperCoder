package impl

// Language name constants.
const (
	LangGo         = "go"
	LangRuby       = "ruby"
	LangJavaScript = "javascript"
	LangTypeScript = "typescript"
	LangTSX        = "tsx"
	LangPython     = "python"
	LangJava       = "java"
	LangRust       = "rust"
	LangC          = "c"
	LangCPP        = "cpp"
	LangCSharp     = "csharp"
	LangStarlark   = "starlark"
	LangProtobuf   = "protobuf"
	LangSQL        = "sql"
	LangYAML       = "yaml"
	LangMarkdown   = "markdown"
)

// ExtensionToLanguage maps file extensions to language names.
var ExtensionToLanguage = map[string]string{
	".go":    LangGo,
	".rb":    LangRuby,
	".js":    LangJavaScript,
	".jsx":   LangJavaScript,
	".ts":    LangTypeScript,
	".tsx":   LangTSX,
	".py":    LangPython,
	".java":  LangJava,
	".rs":    LangRust,
	".c":     LangC,
	".h":     LangC,
	".cpp":   LangCPP,
	".cc":    LangCPP,
	".cxx":   LangCPP,
	".hpp":   LangCPP,
	".cs":    LangCSharp,
	".bzl":   LangStarlark,
	".bazel": LangStarlark,
	".proto": LangProtobuf,
	".sql":   LangSQL,
	".yaml":  LangYAML,
	".yml":   LangYAML,
	".md":    LangMarkdown,
}

// DefaultSkipDirs lists directories that should be skipped during traversal.
var DefaultSkipDirs = map[string]bool{
	".git":         true,
	".venv":        true,
	".env":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	"target":       true,
	"build":        true,
	"dist":         true,
}
