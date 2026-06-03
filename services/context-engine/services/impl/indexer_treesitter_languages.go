package impl

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	tsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// languageGrammar returns the tree-sitter grammar for the given language name.
// Returns nil for languages that use whole-file fallback.
func languageGrammar(lang string) *sitter.Language {
	switch lang {
	case LangGo:
		return golang.GetLanguage()
	case LangPython, LangStarlark:
		return python.GetLanguage()
	case LangJavaScript:
		return javascript.GetLanguage()
	case LangTypeScript:
		return typescript.GetLanguage()
	case LangTSX:
		return tsx.GetLanguage()
	case LangJava:
		return java.GetLanguage()
	case LangRust:
		return rust.GetLanguage()
	case LangC:
		return c.GetLanguage()
	case LangCPP:
		return cpp.GetLanguage()
	case LangRuby:
		return ruby.GetLanguage()
	default:
		return nil
	}
}

// languageFromExtension returns a language name for the given file extension,
// extending the base ExtensionToLanguage map with additional types.
func languageFromExtension(ext string) string {
	if lang, ok := ExtensionToLanguage[ext]; ok {
		return lang
	}
	return ""
}
