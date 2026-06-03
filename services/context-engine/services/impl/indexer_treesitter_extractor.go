package impl

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// extractElements parses source content with tree-sitter and returns CodeElements
// and CodeRelationships. For languages without a grammar, the entire file is
// returned as a single "file" element.
func extractElements(ctx context.Context, content []byte, lang, relPath string) ([]services.CodeElement, []services.CodeRelationship, error) {
	grammar := languageGrammar(lang)
	if grammar == nil {
		// Whole-file fallback for unsupported grammars (proto, SQL, YAML, markdown, etc.)
		return []services.CodeElement{{
			Kind:      "file",
			Name:      relPath,
			Body:      string(content),
			FilePath:  relPath,
			StartLine: 1,
			EndLine:   uint32(countLines(content)),
			Language:  lang,
		}}, nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(grammar)

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	nodeTypes := nodeTypesForLanguage(lang)

	var elements []services.CodeElement
	var relationships []services.CodeRelationship

	walkNode(root, content, lang, relPath, nodeTypes, &elements, &relationships)

	// File-level fallback: if no elements were extracted, treat whole file as one element.
	if len(elements) == 0 {
		elements = append(elements, services.CodeElement{
			Kind:      "file",
			Name:      relPath,
			Body:      string(content),
			FilePath:  relPath,
			StartLine: 1,
			EndLine:   uint32(countLines(content)),
			Language:  lang,
		})
	}

	return elements, relationships, nil
}

func walkNode(node *sitter.Node, content []byte, lang, relPath string, nodeTypes map[string]string, elements *[]services.CodeElement, relationships *[]services.CodeRelationship) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	if kind, ok := nodeTypes[nodeType]; ok {
		name := extractNodeName(node, content, lang, nodeType)
		body := string(content[node.StartByte():node.EndByte()])
		startLine := node.StartPoint().Row + 1
		endLine := node.EndPoint().Row + 1

		*elements = append(*elements, services.CodeElement{
			Kind:      kind,
			Name:      name,
			Signature: name,
			Body:      body,
			FilePath:  relPath,
			StartLine: uint32(startLine),
			EndLine:   uint32(endLine),
			Language:  lang,
		})

		// Extract import relationships.
		if kind == "import" {
			importName := extractImportName(node, content, lang)
			if importName != "" {
				*relationships = append(*relationships, services.CodeRelationship{
					Kind:     services.RelImports,
					FromName: relPath,
					FromFile: relPath,
					FromLine: uint32(startLine),
					ToName:   importName,
					Language: lang,
				})
			}
		}

		// Extract CALLS relationships from function/method bodies.
		if kind == "function" || kind == "method" {
			if callTypes := callNodeTypesForLanguage(lang); callTypes != nil {
				extractCallsFromBody(node, content, lang, relPath, name, callTypes, relationships)
			}
		}

		// Don't recurse into matched nodes, except containers that hold nested definitions.
		if !isContainerNode(nodeType) {
			return
		}
	}

	// Recurse into children.
	for i := 0; i < int(node.ChildCount()); i++ {
		walkNode(node.Child(i), content, lang, relPath, nodeTypes, elements, relationships)
	}
}

// isContainerNode returns true for AST nodes that contain nested definitions
// and should be recursed into even after matching (e.g. Java/C++ classes contain methods).
func isContainerNode(nodeType string) bool {
	switch nodeType {
	case "type_declaration",
		"class_declaration", "interface_declaration",
		"class_specifier", "struct_specifier",
		"impl_item",
		"class", "module",
		"class_definition":
		return true
	}
	return false
}

// nodeTypesForLanguage returns a map of tree-sitter node type → CodeElement kind.
func nodeTypesForLanguage(lang string) map[string]string {
	switch lang {
	case LangGo:
		return map[string]string{
			"function_declaration": "function",
			"method_declaration":   "method",
			"type_declaration":     "class",
			"import_declaration":   "import",
		}
	case LangPython, LangStarlark:
		return map[string]string{
			"function_definition":   "function",
			"class_definition":      "class",
			"decorated_definition":  "function",
			"import_statement":      "import",
			"import_from_statement": "import",
		}
	case LangTypeScript, LangTSX, LangJavaScript:
		return map[string]string{
			"function_declaration":  "function",
			"class_declaration":     "class",
			"method_definition":     "method",
			"interface_declaration": "class",
			"import_statement":      "import",
		}
	case LangJava:
		return map[string]string{
			"class_declaration":       "class",
			"interface_declaration":   "class",
			"method_declaration":      "method",
			"constructor_declaration": "method",
			"import_declaration":      "import",
		}
	case LangRust:
		return map[string]string{
			"function_item":   "function",
			"impl_item":       "class",
			"struct_item":     "class",
			"trait_item":      "class",
			"use_declaration": "import",
		}
	case LangC:
		return map[string]string{
			"function_definition": "function",
			"struct_specifier":    "class",
			"preproc_include":     "import",
		}
	case LangCPP:
		return map[string]string{
			"function_definition": "function",
			"struct_specifier":    "class",
			"class_specifier":     "class",
			"preproc_include":     "import",
		}
	case LangRuby:
		return map[string]string{
			"method":           "function",
			"singleton_method": "function",
			"class":            "class",
			"module":           "class",
		}
	default:
		return nil
	}
}

// extractNodeName extracts a human-readable name from an AST node.
func extractNodeName(node *sitter.Node, content []byte, lang, nodeType string) string {
	// Try to find a "name" or "identifier" child.
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return string(content[nameNode.StartByte():nameNode.EndByte()])
	}

	// Language-specific name extraction.
	switch lang {
	case LangGo:
		return extractGoName(node, content, nodeType)
	case LangRust:
		return extractRustName(node, content, nodeType)
	case LangC, LangCPP:
		return extractCName(node, content, nodeType)
	}

	// Fallback: try first named child that is an identifier.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" || child.Type() == "type_identifier" {
			return string(content[child.StartByte():child.EndByte()])
		}
	}

	return nodeType
}

func extractGoName(node *sitter.Node, content []byte, nodeType string) string {
	switch nodeType {
	case "type_declaration":
		// type_declaration → type_spec → name
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "type_spec" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					return string(content[nameNode.StartByte():nameNode.EndByte()])
				}
			}
		}
	case "method_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	}
	return nodeType
}

func extractRustName(node *sitter.Node, content []byte, nodeType string) string {
	switch nodeType {
	case "impl_item":
		// impl Type { ... } — look for type_identifier child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "type_identifier" || child.Type() == "generic_type" {
				return string(content[child.StartByte():child.EndByte()])
			}
		}
	}
	return nodeType
}

func extractCName(node *sitter.Node, content []byte, nodeType string) string {
	switch nodeType {
	case "function_definition":
		decl := node.ChildByFieldName("declarator")
		if decl != nil {
			nameNode := decl.ChildByFieldName("declarator")
			if nameNode != nil {
				return string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}
	case "struct_specifier", "class_specifier":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	}
	return nodeType
}

// extractImportName extracts the imported module/package name from an import node.
func extractImportName(node *sitter.Node, content []byte, lang string) string {
	switch lang {
	case LangGo:
		// import_declaration has import_spec children with path.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "import_spec_list" {
				// Multiple imports — return full text.
				return strings.TrimSpace(string(content[child.StartByte():child.EndByte()]))
			}
			if child.Type() == "import_spec" {
				pathNode := child.ChildByFieldName("path")
				if pathNode != nil {
					return strings.Trim(string(content[pathNode.StartByte():pathNode.EndByte()]), "\"")
				}
			}
		}
	case LangPython, LangStarlark:
		// import X or from X import Y.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "dotted_name" || child.Type() == "aliased_import" {
				return string(content[child.StartByte():child.EndByte()])
			}
		}
	case LangTypeScript, LangTSX, LangJavaScript:
		// import ... from "module".
		srcNode := node.ChildByFieldName("source")
		if srcNode != nil {
			return strings.Trim(string(content[srcNode.StartByte():srcNode.EndByte()]), "\"'")
		}
	case LangJava:
		// import com.foo.Bar;
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "scoped_identifier" {
				return string(content[child.StartByte():child.EndByte()])
			}
		}
	case LangRust:
		// use std::io;
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "use_list" || child.Type() == "scoped_identifier" || child.Type() == "use_wildcard" || child.Type() == "identifier" {
				return string(content[child.StartByte():child.EndByte()])
			}
		}
	}
	return ""
}

// callNodeTypesForLanguage returns the AST node types that represent function/method
// calls for each supported language.
func callNodeTypesForLanguage(lang string) map[string]bool {
	switch lang {
	case LangGo:
		return map[string]bool{"call_expression": true}
	case LangPython, LangStarlark:
		return map[string]bool{"call": true}
	case LangTypeScript, LangTSX, LangJavaScript:
		return map[string]bool{"call_expression": true}
	case LangJava:
		return map[string]bool{"method_invocation": true}
	case LangRuby:
		return map[string]bool{"method_call": true, "call": true}
	case LangRust:
		return map[string]bool{"call_expression": true}
	case LangC, LangCPP:
		return map[string]bool{"call_expression": true}
	default:
		return nil
	}
}

// extractCallsFromBody recursively walks the children of a function/method node
// to find call expressions and emit RelCalls relationships.
func extractCallsFromBody(node *sitter.Node, content []byte, lang, relPath, fromName string, callTypes map[string]bool, relationships *[]services.CodeRelationship) {
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}
		if callTypes[n.Type()] {
			calledName := extractCallName(n, content, lang)
			if calledName != "" && calledName != fromName {
				key := calledName + "|" + fromName
				if !seen[key] {
					seen[key] = true
					*relationships = append(*relationships, services.CodeRelationship{
						Kind:     services.RelCalls,
						FromName: fromName,
						FromFile: relPath,
						FromLine: uint32(n.StartPoint().Row + 1),
						ToName:   calledName,
						Language: lang,
					})
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)
}

// extractCallName extracts the name of the function/method being called from a
// call expression AST node.
func extractCallName(node *sitter.Node, content []byte, lang string) string {
	var funcNode *sitter.Node

	switch lang {
	case LangJava:
		// method_invocation → name field
		funcNode = node.ChildByFieldName("name")
	case LangRuby:
		// method_call → method field; call → method field
		funcNode = node.ChildByFieldName("method")
		if funcNode == nil {
			// Fallback: try first child for simple calls
			if node.ChildCount() > 0 {
				funcNode = node.Child(0)
			}
		}
	default:
		// Go, Python, JS/TS, Rust, C/C++: function field or first child
		funcNode = node.ChildByFieldName("function")
		if funcNode == nil && node.ChildCount() > 0 {
			funcNode = node.Child(0)
		}
	}

	if funcNode == nil {
		return ""
	}

	// For member/attribute access (obj.method, pkg.Func), extract the rightmost identifier.
	return extractTerminalName(funcNode, content)
}

// extractTerminalName extracts the rightmost identifier from a node.
// For selector expressions like a.b.c, returns "c".
// For simple identifiers, returns the identifier text.
func extractTerminalName(node *sitter.Node, content []byte) string {
	switch node.Type() {
	case "identifier", "property_identifier", "field_identifier", "type_identifier":
		return string(content[node.StartByte():node.EndByte()])
	case "selector_expression":
		// Go: a.B → field is "field"
		field := node.ChildByFieldName("field")
		if field != nil {
			return string(content[field.StartByte():field.EndByte()])
		}
	case "member_expression":
		// JS/TS: a.b → property
		prop := node.ChildByFieldName("property")
		if prop != nil {
			return string(content[prop.StartByte():prop.EndByte()])
		}
	case "attribute":
		// Python: a.b → attribute
		attr := node.ChildByFieldName("attribute")
		if attr != nil {
			return string(content[attr.StartByte():attr.EndByte()])
		}
	case "scoped_identifier", "field_expression":
		// Rust/C++: a::b or a.b → last child identifier
		for i := int(node.NamedChildCount()) - 1; i >= 0; i-- {
			child := node.NamedChild(i)
			if child.Type() == "identifier" || child.Type() == "type_identifier" {
				return string(content[child.StartByte():child.EndByte()])
			}
		}
	}

	// Fallback: return full text for simple expressions.
	text := string(content[node.StartByte():node.EndByte()])
	if len(text) < 80 && !strings.Contains(text, "\n") {
		return text
	}
	return ""
}

func countLines(data []byte) int {
	n := 1
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}
