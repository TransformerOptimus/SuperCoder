package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/producers"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type IndexRetrievalServiceImpl struct {
	openAICfg  config.OpenAIConfig
	indexerCfg config.IndexerConfig
	store      repositories.VectorRepository
	graph      repositories.GraphRepository
	textRepo   repositories.TextSearchRepository
	repoRepo   repositories.RepoRepository
	shardRepo  repositories.ShardRepository
	embedder   services.EmbedderService
	indexer    services.IndexerService
	merkle     services.MerkleService
	indexProd  *producers.IndexProducer
	logger     *zap.Logger
}

func NewIndexRetrievalService(
	openAICfg config.OpenAIConfig,
	indexerCfg config.IndexerConfig,
	store repositories.VectorRepository,
	graph repositories.GraphRepository,
	textRepo repositories.TextSearchRepository,
	repoRepo repositories.RepoRepository,
	shardRepo repositories.ShardRepository,
	embedder services.EmbedderService,
	indexer services.IndexerService,
	merkle services.MerkleService,
	indexProd *producers.IndexProducer,
	logger *zap.Logger,
) services.IndexRetrievalService {
	return &IndexRetrievalServiceImpl{
		openAICfg:  openAICfg,
		indexerCfg: indexerCfg,
		store:      store,
		graph:      graph,
		textRepo:   textRepo,
		repoRepo:   repoRepo,
		shardRepo:  shardRepo,
		embedder:   embedder,
		indexer:    indexer,
		merkle:     merkle,
		indexProd:  indexProd,
		logger:     logger.Named("index_retrieval"),
	}
}

func (s *IndexRetrievalServiceImpl) resolveCollection(ctx context.Context, userID string, workspaceID uint64, machineID, repoPath, repoURL string) (string, uint, error) {
	repo, err := s.repoRepo.FindOrCreate(ctx, userID, workspaceID, machineID, repoPath, repoURL)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve repo: %w", err)
	}

	assignment, err := s.shardRepo.AssignShard(ctx, repo.ID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to assign shard: %w", err)
	}

	return assignment.CollectionName, repo.ID, nil
}

func (s *IndexRetrievalServiceImpl) buildRetriever(strategy, collection, repoPath string) services.RetrieverService {
	provider := NewLocalSourceProvider(s.indexerCfg)
	if strategy == "" {
		strategy = "multi"
	}
	return NewRetriever(strategy, s.embedder, s.store, s.graph, s.textRepo, provider, collection, repoPath, 0.7, s.openAICfg, s.logger)
}

func (s *IndexRetrievalServiceImpl) Search(ctx context.Context, req *dto.SearchRequest) (*dto.SearchResponse, error) {
	collection, repoID, err := s.resolveCollection(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoPath, req.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("resolve collection failed: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	filter := services.SearchFilter{
		UserID:    req.UserID,
		RepoID:    repoID,
		OrgID:     req.GithubOrgID,
		MachineID: req.MachineID,
	}

	retriever := s.buildRetriever(req.Strategy, collection, req.RepoPath)
	results, err := retriever.Search(ctx, req.Query, limit, filter)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	items := make([]dto.SearchResultItem, len(results))
	for i, r := range results {
		items[i] = dto.SearchResultItem{
			ChunkID:  r.ChunkID,
			Content:  r.Content,
			FilePath: r.FilePath,
			Language: r.Language,
			Score:    r.Score,
			Source:   r.Source,
		}
	}

	return &dto.SearchResponse{
		Query:   req.Query,
		Total:   len(items),
		Results: items,
	}, nil
}

func (s *IndexRetrievalServiceImpl) GraphQuery(ctx context.Context, req *dto.GraphQueryRequest) (*dto.GraphQueryResponse, error) {
	collection, repoID, err := s.resolveCollection(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoPath, "")
	if err != nil {
		return nil, fmt.Errorf("resolve collection failed: %w", err)
	}

	// Auto-index if graph doesn't exist yet.
	graphExists, err := s.graph.Exists(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("failed to check graph existence: %w", err)
	}
	if !graphExists {
		s.logger.Warn("Graph not found, triggering async index", zap.String("repo", req.RepoPath), zap.String("collection", collection))
		_, enqErr := s.indexProd.EnqueueIndex(ctx, req.RepoPath, "", false, req.UserID, req.WorkspaceID, req.MachineID, "", "", "")
		if enqErr != nil {
			s.logger.Warn("Failed to enqueue index", zap.Error(enqErr))
		}
		return &dto.GraphQueryResponse{
			Indexing: true,
			Message:  "Repository is being indexed. Please retry in a few minutes.",
		}, nil
	}

	filter := services.SearchFilter{
		UserID:    req.UserID,
		RepoID:    repoID,
		OrgID:     req.GithubOrgID,
		MachineID: req.MachineID,
	}

	// Step 1: Resolve function names to query.
	// If a free-text query is provided, search text+vector first, then find functions in matched files.
	type functionTarget struct {
		name     string
		filePath string
	}

	var targets []functionTarget

	if req.FunctionName != "" {
		targets = append(targets, functionTarget{name: req.FunctionName, filePath: req.FilePath})
	} else if req.Query != "" {
		// Run vector and keyword search directly (not multi) to collect ALL unique file paths
		// without truncation from multi-retriever merging.
		vectorRetriever := NewVectorRetriever(s.embedder, s.store, collection)
		keywordRetriever := NewKeywordRetriever(s.textRepo, collection)

		var vectorResults, keywordResults []services.RetrieverResult
		searchGroup, searchCtx := errgroup.WithContext(ctx)
		searchGroup.Go(func() error {
			res, err := vectorRetriever.Search(searchCtx, req.Query, 20, filter)
			if err == nil {
				vectorResults = res
			}
			return nil
		})
		searchGroup.Go(func() error {
			res, err := keywordRetriever.Search(searchCtx, req.Query, 20, filter)
			if err == nil {
				keywordResults = res
			}
			return nil
		})
		_ = searchGroup.Wait()

		// Collect unique file paths from ALL search results.
		seenFiles := make(map[string]bool)
		for _, r := range vectorResults {
			if r.FilePath != "" {
				seenFiles[r.FilePath] = true
			}
		}
		for _, r := range keywordResults {
			if r.FilePath != "" {
				seenFiles[r.FilePath] = true
			}
		}

		// Find all functions in those files from the graph.
		seenFuncs := make(map[string]bool)
		for filePath := range seenFiles {
			funcs, funcErr := s.graph.GetFunctionsByFile(ctx, collection, filePath, filter)
			if funcErr != nil {
				s.logger.Warn("Failed to get functions for file", zap.String("file", filePath), zap.Error(funcErr))
				continue
			}
			for _, f := range funcs {
				key := f.Name + "|" + f.FilePath
				if !seenFuncs[key] {
					seenFuncs[key] = true
					targets = append(targets, functionTarget{name: f.Name, filePath: f.FilePath})
				}
			}
		}

		s.logger.Info("Graph query via search",
			zap.String("query", req.Query),
			zap.Int("vector_results", len(vectorResults)),
			zap.Int("keyword_results", len(keywordResults)),
			zap.Int("files_matched", len(seenFiles)),
			zap.Int("functions_found", len(targets)))
	} else {
		return nil, fmt.Errorf("either 'query' or 'function_name' is required")
	}

	// Step 2: For each function, traverse graph in the requested direction(s).
	seen := make(map[string]bool)
	var items []dto.GraphResultItem

	for _, t := range targets {
		if req.QueryType == "" || req.QueryType == "blast_radius" {
			callers, bErr := s.graph.GetBlastRadius(ctx, collection, t.name, t.filePath, filter)
			if bErr != nil {
				s.logger.Warn("Blast radius query failed", zap.String("function", t.name), zap.Error(bErr))
			}
			for _, r := range callers {
				key := r.Name + "|" + r.FilePath + "|caller"
				if !seen[key] {
					seen[key] = true
					items = append(items, dto.GraphResultItem{
						Name:      r.Name,
						FilePath:  r.FilePath,
						ChunkID:   r.ChunkID,
						Depth:     r.Depth,
						Direction: "caller",
					})
				}
			}
		}

		if req.QueryType == "" || req.QueryType == "dependencies" {
			deps, dErr := s.graph.GetDependencies(ctx, collection, t.name, t.filePath, filter)
			if dErr != nil {
				s.logger.Warn("Dependencies query failed", zap.String("function", t.name), zap.Error(dErr))
			}
			for _, r := range deps {
				key := r.Name + "|" + r.FilePath + "|dependency"
				if !seen[key] {
					seen[key] = true
					items = append(items, dto.GraphResultItem{
						Name:      r.Name,
						FilePath:  r.FilePath,
						ChunkID:   r.ChunkID,
						Depth:     r.Depth,
						Direction: "dependency",
					})
				}
			}
		}
	}

	return &dto.GraphQueryResponse{
		Total:   len(items),
		Results: items,
	}, nil
}

func (s *IndexRetrievalServiceImpl) GetContext(ctx context.Context, req *dto.ContextRequest) (*dto.ContextResponse, error) {
	collection, repoID, err := s.resolveCollection(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoPath, req.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("resolve collection failed: %w", err)
	}

	dim := uint64(s.openAICfg.EmbeddingDimensions())

	// Ensure collection exists.
	if err := s.store.EnsureCollection(ctx, collection, dim); err != nil {
		return nil, fmt.Errorf("ensure collection failed: %w", err)
	}

	// Auto-index if empty.
	isEmpty, err := s.store.IsEmpty(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("failed to check collection status: %w", err)
	}

	if isEmpty {
		s.logger.Warn("Collection empty, triggering async index", zap.String("repo", req.RepoPath))
		_, enqErr := s.indexProd.EnqueueIndex(ctx, req.RepoPath, req.RepoURL, false, req.UserID, req.WorkspaceID, req.MachineID, req.SourceType, req.S3Bucket, req.S3Prefix)
		if enqErr != nil {
			s.logger.Warn("Failed to enqueue index", zap.Error(enqErr))
		}
		return &dto.ContextResponse{
			Query:    req.Query,
			Context:  "Repository is being indexed. Please retry in a few minutes.",
			Indexing: true,
			Message:  "Repository is being indexed. Please retry in a few minutes.",
		}, nil
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	filter := services.SearchFilter{
		UserID:    req.UserID,
		RepoID:    repoID,
		OrgID:     req.GithubOrgID,
		MachineID: req.MachineID,
	}

	// Search codebase with higher limit to collect more files for graph expansion.
	searchLimit := 20
	retriever := s.buildRetriever("multi", collection, req.RepoPath)
	allResults, err := retriever.Search(ctx, req.Query, searchLimit, filter)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Display only top `limit` results, but use all for graph expansion.
	displayResults := allResults
	if len(displayResults) > limit {
		displayResults = displayResults[:limit]
	}

	// Format search results like tool_executor.go.
	var contextText string
	if len(displayResults) > 0 {
		contextText += fmt.Sprintf("## Code Search Results for: %s\n\n", req.Query)
		for i, res := range displayResults {
			content := truncate(res.Content, maxToolResultChars)
			contextText += fmt.Sprintf("Result %d (File: %s, Score: %.2f, Source: %s):\n%s\n\n",
				i+1, res.FilePath, res.Score, res.Source, content)
		}
	}

	// Second-pass: expand search results through the graph.
	// For each file found by text/vector, find its functions and traverse callers + dependencies.
	if s.graph != nil && len(allResults) > 0 {
		graphFilter := services.SearchFilter{
			UserID:    req.UserID,
			RepoID:    repoID,
			OrgID:     req.GithubOrgID,
			MachineID: req.MachineID,
		}

		// Collect unique files from ALL search results (not just displayed).
		seenFiles := make(map[string]bool)
		for _, r := range allResults {
			if r.FilePath != "" {
				seenFiles[r.FilePath] = true
			}
		}

		// Find functions from search result files + graph search, then traverse.
		type graphEntry struct {
			name      string
			filePath  string
			depth     int
			direction string
		}
		seen := make(map[string]bool)
		var allDeps, allCallers []graphEntry

		// Collect functions from search result files.
		type funcTarget struct{ name, filePath string }
		seenFuncs := make(map[string]bool)
		var targets []funcTarget

		for filePath := range seenFiles {
			funcs, fErr := s.graph.GetFunctionsByFile(ctx, collection, filePath, graphFilter)
			if fErr != nil {
				continue
			}
			for _, f := range funcs {
				key := f.Name + "|" + f.FilePath
				if !seenFuncs[key] {
					seenFuncs[key] = true
					targets = append(targets, funcTarget{f.Name, f.FilePath})
				}
			}
		}

		// Also search graph for functions matching the query (catches functions in sibling files).
		// Use LLM to extract meaningful keywords from the natural language query first.
		var graphFuncCount int
		keywords, kwErr := extractGraphKeywords(ctx, s.openAICfg, req.Query)
		if kwErr == nil && keywords != "" {
			graphFuncs, gErr := s.graph.SearchFunctions(ctx, collection, keywords, graphFilter, 10)
			if gErr == nil {
				graphFuncCount = len(graphFuncs)
				for _, f := range graphFuncs {
					key := f.Name + "|" + f.FilePath
					if !seenFuncs[key] {
						seenFuncs[key] = true
						targets = append(targets, funcTarget{f.Name, f.FilePath})
					}
				}
			}
		}

		s.logger.Info("Context graph expansion",
			zap.Int("files_from_search", len(seenFiles)),
			zap.Int("functions_from_files", len(targets)),
			zap.Int("functions_from_graph_search", graphFuncCount))

		// Traverse graph for each target function.
		for _, t := range targets {
			deps, dErr := s.graph.GetDependencies(ctx, collection, t.name, t.filePath, graphFilter)
			if dErr == nil {
				for _, d := range deps {
					key := d.Name + "|" + d.FilePath + "|dep"
					if !seen[key] {
						seen[key] = true
						allDeps = append(allDeps, graphEntry{d.Name, d.FilePath, d.Depth, "dependency"})
					}
				}
			}

			callers, bErr := s.graph.GetBlastRadius(ctx, collection, t.name, t.filePath, graphFilter)
			if bErr == nil {
				for _, c := range callers {
					key := c.Name + "|" + c.FilePath + "|caller"
					if !seen[key] {
						seen[key] = true
						allCallers = append(allCallers, graphEntry{c.Name, c.FilePath, c.Depth, "caller"})
					}
				}
			}
		}

		if len(allDeps) > 0 {
			contextText += fmt.Sprintf("## Dependencies — %d downstream function(s):\n\n", len(allDeps))
			for i, r := range allDeps {
				if i >= 20 {
					contextText += fmt.Sprintf("... and %d more\n", len(allDeps)-20)
					break
				}
				contextText += fmt.Sprintf("%d. %s (file: %s, depth: %d)\n", i+1, r.name, r.filePath, r.depth)
			}
			contextText += "\n"
		}

		if len(allCallers) > 0 {
			contextText += fmt.Sprintf("## Blast Radius — %d upstream caller(s):\n\n", len(allCallers))
			for i, r := range allCallers {
				if i >= 20 {
					contextText += fmt.Sprintf("... and %d more\n", len(allCallers)-20)
					break
				}
				contextText += fmt.Sprintf("%d. %s (file: %s, depth: %d)\n", i+1, r.name, r.filePath, r.depth)
			}
			contextText += "\n"
		}
	}

	if contextText == "" {
		contextText = "No relevant context found."
	}

	return &dto.ContextResponse{
		Query:   req.Query,
		Context: contextText,
	}, nil
}

func (s *IndexRetrievalServiceImpl) GetIndexStatus(ctx context.Context, repoPath, userID string, workspaceID uint64, machineID string) (*dto.IndexStatusResponse, error) {
	repo, err := s.repoRepo.FindOrCreate(ctx, userID, workspaceID, machineID, repoPath, "")
	if err != nil {
		return &dto.IndexStatusResponse{Exists: false}, nil
	}

	assignment, err := s.shardRepo.GetAssignment(ctx, repo.ID)
	if err != nil {
		return &dto.IndexStatusResponse{Exists: false, RepoID: repo.ID}, nil
	}

	isEmpty, err := s.store.IsEmpty(ctx, assignment.CollectionName)
	if err != nil {
		return &dto.IndexStatusResponse{
			Exists:         true,
			CollectionName: assignment.CollectionName,
			RepoID:         repo.ID,
			Empty:          true,
		}, nil
	}

	return &dto.IndexStatusResponse{
		Exists:         true,
		CollectionName: assignment.CollectionName,
		RepoID:         repo.ID,
		Empty:          isEmpty,
	}, nil
}

func (s *IndexRetrievalServiceImpl) DeleteIndex(ctx context.Context, req *dto.IndexDeleteRequest) (*dto.IndexDeleteResponse, error) {
	repo, err := s.repoRepo.FindOrCreate(ctx, req.UserID, req.WorkspaceID, req.MachineID, req.RepoPath, "")
	if err != nil {
		return &dto.IndexDeleteResponse{Deleted: false, Message: "repo not found"}, nil
	}

	assignment, err := s.shardRepo.GetAssignment(ctx, repo.ID)
	if err != nil {
		return &dto.IndexDeleteResponse{Deleted: false, Message: "no index found for repo"}, nil
	}

	collection := assignment.CollectionName

	_ = s.store.DeleteByRepoID(ctx, collection, repo.ID)
	_ = s.graph.DeleteByRepoID(ctx, collection, repo.ID)
	_ = s.textRepo.DeleteByRepoID(ctx, collection, repo.ID)

	// Delete merkle tree so re-indexing doesn't skip unchanged files.
	merkleKey := fmt.Sprintf("repo_%d", repo.ID)
	if err := s.merkle.DeleteTree(ctx, merkleKey); err != nil {
		s.logger.Warn("Failed to delete merkle tree", zap.String("key", merkleKey), zap.Error(err))
	}

	return &dto.IndexDeleteResponse{
		Deleted: true,
		Message: fmt.Sprintf("index data deleted for repo %s from collection %s", req.RepoPath, collection),
	}, nil
}

func (s *IndexRetrievalServiceImpl) TriggerIndex(ctx context.Context, req *dto.IndexRequest) (string, error) {
	taskID, err := s.indexProd.EnqueueIndex(
		ctx,
		req.RepoPath,
		req.RepoURL,
		req.Reindex,
		req.UserID,
		req.WorkspaceID,
		req.MachineID,
		req.SourceType,
		req.S3Bucket,
		req.S3Prefix,
	)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue index: %w", err)
	}
	return taskID, nil
}
