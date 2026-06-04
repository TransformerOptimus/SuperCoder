package services

// CodeElement represents a parsed code construct (function, method, class, etc.).
type CodeElement struct {
	Kind        string
	Name        string
	Signature   string
	Docstring   string
	Body        string
	FilePath    string
	StartLine   uint32
	EndLine     uint32
	Language    string
	GithubOrgID string
	WorkspaceID string
	UserID      string
	MachineID   string
	RepoID      uint
}

type RelationshipKind string

const (
	RelCalls    RelationshipKind = "CALLS"
	RelInherits RelationshipKind = "INHERITS"
	RelImports  RelationshipKind = "IMPORTS"
)

// CodeRelationship represents a directed edge between two code elements.
type CodeRelationship struct {
	Kind     RelationshipKind
	FromName string
	FromFile string
	FromLine uint32
	ToName   string
	ToFile   string
	Language string
}
