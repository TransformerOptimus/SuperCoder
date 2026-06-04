package impl

import (
	"context"
	"fmt"
	"strings"

	falkordb "github.com/FalkorDB/falkordb-go/v2"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type graphRepositoryImpl struct {
	db     *falkordb.FalkorDB
	logger *zap.Logger
}

func NewGraphRepository(cfg config.FalkorDBConfig, logger *zap.Logger) (repositories.GraphRepository, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host(), cfg.Port())
	db, err := falkordb.FalkorDBNew(&falkordb.ConnectionOption{
		Addr:     addr,
		Password: cfg.Password(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to FalkorDB at %s: %w", addr, err)
	}

	return &graphRepositoryImpl{
		db:     db,
		logger: logger.Named("graph-repo"),
	}, nil
}

func (g *graphRepositoryImpl) EnsureIndices(ctx context.Context, graphName string) error {
	graph := g.db.SelectGraph(graphName)
	indices := []string{
		"CREATE INDEX ON :Function(name)",
		"CREATE INDEX ON :Function(chunk_id)",
		"CREATE INDEX ON :Function(repo_id)",
		"CREATE INDEX ON :Method(name)",
		"CREATE INDEX ON :Method(chunk_id)",
		"CREATE INDEX ON :Method(repo_id)",
		"CREATE INDEX ON :Class(name)",
		"CREATE INDEX ON :Class(chunk_id)",
		"CREATE INDEX ON :Class(repo_id)",
		"CREATE INDEX ON :File(file_path)",
		"CREATE INDEX ON :File(repo_id)",
	}

	for _, q := range indices {
		if _, err := graph.Query(q, nil, nil); err != nil {
			if strings.Contains(err.Error(), "already indexed") || strings.Contains(err.Error(), "Index already exists") {
				continue
			}
			g.logger.Warn("Failed to create index", zap.String("query", q), zap.Error(err))
		}
	}
	return nil
}

func (g *graphRepositoryImpl) UpsertElements(ctx context.Context, graphName string, elements []services.CodeElement) error {
	graph := g.db.SelectGraph(graphName)
	var errCount int
	for _, el := range elements {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		chunkID := fmt.Sprintf("%s:%d-%d", el.FilePath, el.StartLine, el.EndLine)
		label := elementLabel(el.Kind)

		query := fmt.Sprintf(
			`MERGE (n:%s {chunk_id: $chunk_id})
			 SET n.name = $name,
			     n.file_path = $file_path,
			     n.language = $language,
			     n.start_line = $start_line,
			     n.end_line = $end_line,
			     n.github_org_id = $github_org_id,
			     n.workspace_id = $workspace_id,
			     n.user_id = $user_id,
			     n.machine_id = $machine_id,
			     n.repo_id = $repo_id`,
			label,
		)

		params := map[string]interface{}{
			"chunk_id":      chunkID,
			"name":          el.Name,
			"file_path":     el.FilePath,
			"language":      el.Language,
			"start_line":    int(el.StartLine),
			"end_line":      int(el.EndLine),
			"github_org_id": el.GithubOrgID,
			"workspace_id":  el.WorkspaceID,
			"user_id":       el.UserID,
			"machine_id":    el.MachineID,
			"repo_id":       int(el.RepoID),
		}

		if _, err := graph.Query(query, params, nil); err != nil {
			errCount++
			g.logger.Warn("Failed to upsert element",
				zap.String("name", el.Name),
				zap.String("chunk_id", chunkID),
				zap.Error(err))
		}
	}
	if errCount > 0 {
		return fmt.Errorf("failed to upsert %d of %d elements", errCount, len(elements))
	}
	return nil
}

func (g *graphRepositoryImpl) UpsertFileNodes(ctx context.Context, graphName string, elements []services.CodeElement) error {
	graph := g.db.SelectGraph(graphName)
	seen := make(map[string]bool)
	var errCount int
	for _, el := range elements {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if seen[el.FilePath] {
			continue
		}
		seen[el.FilePath] = true

		query := `MERGE (n:File {file_path: $file_path})
		          SET n.name = $name, n.language = $language, n.github_org_id = $github_org_id, n.workspace_id = $workspace_id, n.user_id = $user_id, n.machine_id = $machine_id, n.repo_id = $repo_id`
		params := map[string]interface{}{
			"file_path":     el.FilePath,
			"name":          fileBaseName(el.FilePath),
			"language":      el.Language,
			"github_org_id": el.GithubOrgID,
			"workspace_id":  el.WorkspaceID,
			"user_id":       el.UserID,
			"machine_id":    el.MachineID,
			"repo_id":       int(el.RepoID),
		}

		if _, err := graph.Query(query, params, nil); err != nil {
			errCount++
			g.logger.Warn("Failed to upsert file node", zap.String("file", el.FilePath), zap.Error(err))
		}
	}
	if errCount > 0 {
		return fmt.Errorf("failed to upsert %d of %d file nodes", errCount, len(elements))
	}
	return nil
}

func (g *graphRepositoryImpl) UpsertRelationships(ctx context.Context, graphName string, rels []services.CodeRelationship, elements []services.CodeElement) error {
	graph := g.db.SelectGraph(graphName)
	elementsByName := make(map[string]services.CodeElement)
	for _, el := range elements {
		elementsByName[el.Name] = el
	}

	var errCount int
	for _, rel := range rels {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		switch rel.Kind {
		case services.RelCalls:
			err = g.upsertCallEdge(graph, rel, elementsByName)
		case services.RelImports:
			err = g.upsertImportEdge(graph, rel)
		case services.RelInherits:
			err = g.upsertInheritEdge(graph, rel, elementsByName)
		}
		if err != nil {
			errCount++
		}
	}
	if errCount > 0 {
		return fmt.Errorf("failed to upsert %d of %d relationships", errCount, len(rels))
	}
	return nil
}

func (g *graphRepositoryImpl) upsertCallEdge(graph *falkordb.Graph, rel services.CodeRelationship, elementsByName map[string]services.CodeElement) error {
	fromChunkID := g.resolveChunkID(rel.FromName, rel.FromFile, elementsByName)
	fromLabel := "Function"

	toEl, found := elementsByName[rel.ToName]
	var query string
	var params map[string]interface{}

	if found {
		toChunkID := fmt.Sprintf("%s:%d-%d", toEl.FilePath, toEl.StartLine, toEl.EndLine)
		toLabel := elementLabel(toEl.Kind)
		query = fmt.Sprintf(
			`MATCH (a:%s {chunk_id: $from_id})
			 MATCH (b:%s {chunk_id: $to_id})
			 MERGE (a)-[:CALLS]->(b)`,
			fromLabel, toLabel,
		)
		params = map[string]interface{}{
			"from_id": fromChunkID,
			"to_id":   toChunkID,
		}
	} else {
		query = fmt.Sprintf(
			`MATCH (a:%s {chunk_id: $from_id})
			 MERGE (b:External {name: $to_name})
			 MERGE (a)-[:CALLS]->(b)`,
			fromLabel,
		)
		params = map[string]interface{}{
			"from_id": fromChunkID,
			"to_name": rel.ToName,
		}
	}

	if _, err := graph.Query(query, params, nil); err != nil {
		g.logger.Debug("Failed to create CALLS edge",
			zap.String("from", rel.FromName),
			zap.String("to", rel.ToName),
			zap.Error(err))
		return err
	}
	return nil
}

func (g *graphRepositoryImpl) upsertImportEdge(graph *falkordb.Graph, rel services.CodeRelationship) error {
	query := `MATCH (a:File {file_path: $from_file})
	          MERGE (b:Package {name: $to_name})
	          MERGE (a)-[:IMPORTS]->(b)`
	params := map[string]interface{}{
		"from_file": rel.FromFile,
		"to_name":   rel.ToName,
	}

	if _, err := graph.Query(query, params, nil); err != nil {
		g.logger.Debug("Failed to create IMPORTS edge",
			zap.String("from", rel.FromFile),
			zap.String("to", rel.ToName),
			zap.Error(err))
		return err
	}
	return nil
}

func (g *graphRepositoryImpl) upsertInheritEdge(graph *falkordb.Graph, rel services.CodeRelationship, elementsByName map[string]services.CodeElement) error {
	fromEl, fromFound := elementsByName[rel.FromName]
	toEl, toFound := elementsByName[rel.ToName]

	var query string
	var params map[string]interface{}

	if fromFound && toFound {
		fromChunkID := fmt.Sprintf("%s:%d-%d", fromEl.FilePath, fromEl.StartLine, fromEl.EndLine)
		toChunkID := fmt.Sprintf("%s:%d-%d", toEl.FilePath, toEl.StartLine, toEl.EndLine)
		query = fmt.Sprintf(
			`MATCH (a:%s {chunk_id: $from_id})
			 MATCH (b:%s {chunk_id: $to_id})
			 MERGE (a)-[:INHERITS]->(b)`,
			elementLabel(fromEl.Kind), elementLabel(toEl.Kind),
		)
		params = map[string]interface{}{
			"from_id": fromChunkID,
			"to_id":   toChunkID,
		}
	} else if fromFound {
		fromChunkID := fmt.Sprintf("%s:%d-%d", fromEl.FilePath, fromEl.StartLine, fromEl.EndLine)
		query = fmt.Sprintf(
			`MATCH (a:%s {chunk_id: $from_id})
			 MERGE (b:External {name: $to_name})
			 MERGE (a)-[:INHERITS]->(b)`,
			elementLabel(fromEl.Kind),
		)
		params = map[string]interface{}{
			"from_id": fromChunkID,
			"to_name": rel.ToName,
		}
	} else {
		return nil
	}

	if _, err := graph.Query(query, params, nil); err != nil {
		g.logger.Debug("Failed to create INHERITS edge",
			zap.String("from", rel.FromName),
			zap.String("to", rel.ToName),
			zap.Error(err))
		return err
	}
	return nil
}

func (g *graphRepositoryImpl) GetBlastRadius(ctx context.Context, graphName string, functionName, filePath string, filter services.SearchFilter) ([]repositories.GraphResult, error) {
	exists, err := g.Exists(ctx, graphName)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !exists {
		return nil, nil
	}

	graph := g.db.SelectGraph(graphName)
	var query string
	params := map[string]interface{}{
		"name": functionName,
	}

	isolationFilterTarget := ""
	isolationFilterCaller := ""
	if filter.OrgID != "" {
		isolationFilterTarget += " AND target.github_org_id = $org_id"
		isolationFilterCaller += " AND caller.github_org_id = $org_id"
		params["org_id"] = filter.OrgID
	}
	if filter.WorkspaceID != "" {
		isolationFilterTarget += " AND target.workspace_id = $workspace_id"
		isolationFilterCaller += " AND caller.workspace_id = $workspace_id"
		params["workspace_id"] = filter.WorkspaceID
	}
	if filter.UserID != "" {
		isolationFilterTarget += " AND target.user_id = $user_id"
		isolationFilterCaller += " AND caller.user_id = $user_id"
		params["user_id"] = filter.UserID
	}
	if filter.MachineID != "" {
		isolationFilterTarget += " AND target.machine_id = $machine_id"
		isolationFilterCaller += " AND caller.machine_id = $machine_id"
		params["machine_id"] = filter.MachineID
	}
	if filter.RepoID > 0 {
		isolationFilterTarget += " AND target.repo_id = $repo_id"
		isolationFilterCaller += " AND caller.repo_id = $repo_id"
		params["repo_id"] = int(filter.RepoID)
	}

	if filePath != "" {
		query = fmt.Sprintf(`MATCH (target {name: $name, file_path: $file_path})
		         WHERE (target:Function OR target:Method)%s
		         MATCH path = (caller)-[:CALLS*1..1]->(target)
		         WHERE (caller:Function OR caller:Method)
		         AND caller.file_path IS NOT NULL%s
		         RETURN DISTINCT caller.name AS name,
		                caller.chunk_id AS chunk_id,
		                caller.file_path AS file_path,
		                length(path) AS depth
		         ORDER BY depth ASC
		         LIMIT 50`, isolationFilterTarget, isolationFilterCaller)
		params["file_path"] = filePath
	} else {
		query = fmt.Sprintf(`MATCH (target {name: $name})
		         WHERE (target:Function OR target:Method)%s
		         MATCH path = (caller)-[:CALLS*1..1]->(target)
		         WHERE (caller:Function OR caller:Method)
		         AND caller.file_path IS NOT NULL%s
		         RETURN DISTINCT caller.name AS name,
		                caller.chunk_id AS chunk_id,
		                caller.file_path AS file_path,
		                length(path) AS depth
		         ORDER BY depth ASC
		         LIMIT 50`, isolationFilterTarget, isolationFilterCaller)
	}

	result, err := graph.ROQuery(query, params, nil)
	if err != nil {
		return nil, fmt.Errorf("blast radius query failed: %w", err)
	}

	return collectResults(result), nil
}

func (g *graphRepositoryImpl) GetDependencies(ctx context.Context, graphName string, functionName, filePath string, filter services.SearchFilter) ([]repositories.GraphResult, error) {
	exists, err := g.Exists(ctx, graphName)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !exists {
		return nil, nil
	}

	graph := g.db.SelectGraph(graphName)
	var query string
	params := map[string]interface{}{
		"name": functionName,
	}

	isolationFilterSource := ""
	isolationFilterDep := ""
	if filter.OrgID != "" {
		isolationFilterSource += " AND source.github_org_id = $org_id"
		isolationFilterDep += " AND dep.github_org_id = $org_id"
		params["org_id"] = filter.OrgID
	}
	if filter.WorkspaceID != "" {
		isolationFilterSource += " AND source.workspace_id = $workspace_id"
		isolationFilterDep += " AND dep.workspace_id = $workspace_id"
		params["workspace_id"] = filter.WorkspaceID
	}
	if filter.UserID != "" {
		isolationFilterSource += " AND source.user_id = $user_id"
		isolationFilterDep += " AND dep.user_id = $user_id"
		params["user_id"] = filter.UserID
	}
	if filter.MachineID != "" {
		isolationFilterSource += " AND source.machine_id = $machine_id"
		isolationFilterDep += " AND dep.machine_id = $machine_id"
		params["machine_id"] = filter.MachineID
	}
	if filter.RepoID > 0 {
		isolationFilterSource += " AND source.repo_id = $repo_id"
		isolationFilterDep += " AND dep.repo_id = $repo_id"
		params["repo_id"] = int(filter.RepoID)
	}

	if filePath != "" {
		query = fmt.Sprintf(`MATCH (source {name: $name, file_path: $file_path})
		         WHERE (source:Function OR source:Method)%s
		         MATCH path = (source)-[:CALLS*1..1]->(dep)
		         WHERE (dep:Function OR dep:Method)
		         AND dep.file_path IS NOT NULL%s
		         RETURN DISTINCT dep.name AS name,
		                dep.chunk_id AS chunk_id,
		                dep.file_path AS file_path,
		                dep.start_line AS start_line,
		                dep.end_line AS end_line,
		                length(path) AS depth
		         ORDER BY depth ASC
		         LIMIT 50`, isolationFilterSource, isolationFilterDep)
		params["file_path"] = filePath
	} else {
		query = fmt.Sprintf(`MATCH (source {name: $name})
		         WHERE (source:Function OR source:Method)%s
		         MATCH path = (source)-[:CALLS*1..1]->(dep)
		         WHERE (dep:Function OR dep:Method)
		         AND dep.file_path IS NOT NULL%s
		         RETURN DISTINCT dep.name AS name,
		                dep.chunk_id AS chunk_id,
		                dep.file_path AS file_path,
		                dep.start_line AS start_line,
		                dep.end_line AS end_line,
		                length(path) AS depth
		         ORDER BY depth ASC
		         LIMIT 50`, isolationFilterSource, isolationFilterDep)
	}

	result, err := graph.ROQuery(query, params, nil)
	if err != nil {
		return nil, fmt.Errorf("dependencies query failed: %w", err)
	}

	return collectResults(result), nil
}

// GetImporters returns files that import the given file. Matches imports whose
// Package name contains the file's basename — the indexer stores import targets
// as raw module names, not file paths, so this is a heuristic with both false
// positives (basename collisions across packages) and false negatives (renamed/
// aliased imports). Tightens once indexer emits a per-file package field.
func (g *graphRepositoryImpl) GetImporters(ctx context.Context, graphName string, filePath string, filter services.SearchFilter) ([]repositories.GraphResult, error) {
	exists, err := g.Exists(ctx, graphName)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !exists || filePath == "" {
		return nil, nil
	}

	base := fileBaseName(filePath)
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}
	if base == "" {
		return nil, nil
	}

	graph := g.db.SelectGraph(graphName)
	params := map[string]interface{}{
		"basename":  base,
		"self_path": filePath,
	}

	isolationFilter := ""
	if filter.OrgID != "" {
		isolationFilter += " AND f.github_org_id = $org_id"
		params["org_id"] = filter.OrgID
	}
	if filter.WorkspaceID != "" {
		isolationFilter += " AND f.workspace_id = $workspace_id"
		params["workspace_id"] = filter.WorkspaceID
	}
	if filter.UserID != "" {
		isolationFilter += " AND f.user_id = $user_id"
		params["user_id"] = filter.UserID
	}
	if filter.MachineID != "" {
		isolationFilter += " AND f.machine_id = $machine_id"
		params["machine_id"] = filter.MachineID
	}
	if filter.RepoID > 0 {
		isolationFilter += " AND f.repo_id = $repo_id"
		params["repo_id"] = int(filter.RepoID)
	}

	query := fmt.Sprintf(`MATCH (f:File)-[:IMPORTS]->(p:Package)
		WHERE p.name CONTAINS $basename
		  AND f.file_path <> $self_path%s
		RETURN DISTINCT f.file_path AS file_path,
		                f.file_path AS name,
		                "" AS chunk_id,
		                0 AS depth,
		                0 AS start_line,
		                0 AS end_line
		LIMIT 30`, isolationFilter)

	result, err := graph.ROQuery(query, params, nil)
	if err != nil {
		return nil, fmt.Errorf("get importers failed: %w", err)
	}
	return collectResults(result), nil
}

func (g *graphRepositoryImpl) GetFunctionsByFile(ctx context.Context, graphName string, filePath string, filter services.SearchFilter) ([]repositories.GraphResult, error) {
	exists, err := g.Exists(ctx, graphName)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !exists {
		return nil, nil
	}

	graph := g.db.SelectGraph(graphName)
	params := map[string]interface{}{
		"file_path": filePath,
	}

	isolationFilter := ""
	if filter.OrgID != "" {
		isolationFilter += " AND n.github_org_id = $org_id"
		params["org_id"] = filter.OrgID
	}
	if filter.WorkspaceID != "" {
		isolationFilter += " AND n.workspace_id = $workspace_id"
		params["workspace_id"] = filter.WorkspaceID
	}
	if filter.UserID != "" {
		isolationFilter += " AND n.user_id = $user_id"
		params["user_id"] = filter.UserID
	}
	if filter.MachineID != "" {
		isolationFilter += " AND n.machine_id = $machine_id"
		params["machine_id"] = filter.MachineID
	}
	if filter.RepoID > 0 {
		isolationFilter += " AND n.repo_id = $repo_id"
		params["repo_id"] = int(filter.RepoID)
	}

	query := fmt.Sprintf(`MATCH (n {file_path: $file_path})
		WHERE (n:Function OR n:Method OR n:Class)%s
		RETURN n.name AS name, n.chunk_id AS chunk_id, n.file_path AS file_path, 0 AS depth
		LIMIT 50`, isolationFilter)

	result, err := graph.ROQuery(query, params, nil)
	if err != nil {
		return nil, fmt.Errorf("get functions by file failed: %w", err)
	}

	return collectResults(result), nil
}

func (g *graphRepositoryImpl) SearchFunctions(ctx context.Context, graphName string, queryStr string, filter services.SearchFilter, limit int) ([]repositories.GraphResult, error) {
	exists, err := g.Exists(ctx, graphName)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !exists {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	// The caller is expected to pass pre-extracted keywords (via LLM or similar).
	// We just split into tokens and skip very short ones.
	var tokens []string
	for _, w := range strings.Fields(strings.ToLower(queryStr)) {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if len(w) < 3 {
			continue
		}
		tokens = append(tokens, w)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	graph := g.db.SelectGraph(graphName)

	// Run a query per token, collect deduplicated results.
	// Match on function name only (not file_path to avoid cross-service noise).
	seen := make(map[string]bool)
	var allResults []repositories.GraphResult

	for _, token := range tokens {
		params := map[string]interface{}{
			"token": token,
		}

		isolationFilter := ""
		if filter.OrgID != "" {
			isolationFilter += " AND n.github_org_id = $org_id"
			params["org_id"] = filter.OrgID
		}
		if filter.WorkspaceID != "" {
			isolationFilter += " AND n.workspace_id = $workspace_id"
			params["workspace_id"] = filter.WorkspaceID
		}
		if filter.UserID != "" {
			isolationFilter += " AND n.user_id = $user_id"
			params["user_id"] = filter.UserID
		}
		if filter.MachineID != "" {
			isolationFilter += " AND n.machine_id = $machine_id"
			params["machine_id"] = filter.MachineID
		}
		if filter.RepoID > 0 {
			isolationFilter += " AND n.repo_id = $repo_id"
			params["repo_id"] = int(filter.RepoID)
		}

		query := fmt.Sprintf(`MATCH (n)
			WHERE (n:Function OR n:Method)%s
			AND toLower(n.name) CONTAINS $token
			RETURN n.name AS name, n.chunk_id AS chunk_id, n.file_path AS file_path, 0 AS depth
			LIMIT %d`, isolationFilter, limit)

		result, err := graph.ROQuery(query, params, nil)
		if err != nil {
			g.logger.Debug("SearchFunctions token query failed", zap.String("token", token), zap.Error(err))
			continue
		}

		for _, r := range collectResults(result) {
			key := r.Name + "|" + r.FilePath
			if !seen[key] {
				seen[key] = true
				allResults = append(allResults, r)
			}
		}

		if len(allResults) >= limit {
			break
		}
	}

	if len(allResults) > limit {
		allResults = allResults[:limit]
	}
	return allResults, nil
}

func (g *graphRepositoryImpl) DeleteByFilePath(ctx context.Context, graphName string, filePath string) error {
	graph := g.db.SelectGraph(graphName)
	query := `MATCH (n {file_path: $path}) DETACH DELETE n`
	params := map[string]interface{}{
		"path": filePath,
	}

	if _, err := graph.Query(query, params, nil); err != nil {
		return fmt.Errorf("delete by file path failed: %w", err)
	}
	return nil
}

func (g *graphRepositoryImpl) DeleteByRepoID(ctx context.Context, graphName string, repoID uint) error {
	graph := g.db.SelectGraph(graphName)
	query := `MATCH (n {repo_id: $repo_id}) DETACH DELETE n`
	params := map[string]interface{}{
		"repo_id": int(repoID),
	}

	if _, err := graph.Query(query, params, nil); err != nil {
		return fmt.Errorf("delete by repo_id failed: %w", err)
	}
	return nil
}

func (g *graphRepositoryImpl) DeleteGraph(ctx context.Context, graphName string) error {
	graph := g.db.SelectGraph(graphName)
	if err := graph.Delete(); err != nil {
		return fmt.Errorf("failed to delete graph: %w", err)
	}
	return nil
}

func (g *graphRepositoryImpl) Exists(ctx context.Context, graphName string) (bool, error) {
	graphs, err := g.db.ListGraphs()
	if err != nil {
		return false, fmt.Errorf("failed to list graphs: %w", err)
	}
	for _, name := range graphs {
		if name == graphName {
			return true, nil
		}
	}
	return false, nil
}

func (g *graphRepositoryImpl) resolveChunkID(name, filePath string, elementsByName map[string]services.CodeElement) string {
	if el, found := elementsByName[name]; found {
		return fmt.Sprintf("%s:%d-%d", el.FilePath, el.StartLine, el.EndLine)
	}
	return filePath + ":0-0"
}

func collectResults(result *falkordb.QueryResult) []repositories.GraphResult {
	var results []repositories.GraphResult

	for result.Next() {
		record := result.Record()
		name, _ := record.Get("name")
		chunkID, _ := record.Get("chunk_id")
		filePath, _ := record.Get("file_path")
		depth, _ := record.Get("depth")
		startLine, _ := record.Get("start_line")
		endLine, _ := record.Get("end_line")

		r := repositories.GraphResult{
			Name:      toString(name),
			ChunkID:   toString(chunkID),
			FilePath:  toString(filePath),
			Depth:     toInt(depth),
			StartLine: toInt(startLine),
			EndLine:   toInt(endLine),
		}
		results = append(results, r)
	}

	return results
}

func elementLabel(kind string) string {
	switch strings.ToLower(kind) {
	case "method":
		return "Method"
	case "class":
		return "Class"
	default:
		return "Function"
	}
}

func fileBaseName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
