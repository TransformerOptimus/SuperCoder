package impl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	coreconfig "github.com/TransformerOptimus/SuperCoder/services/context-engine/internal/pkg/config/core"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

var validCollectionName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateCollection(name string) error {
	if name == "" {
		return fmt.Errorf("collection name must not be empty")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("collection name contains invalid characters: %q", name)
	}
	if !validCollectionName.MatchString(name) {
		return fmt.Errorf("collection name must be alphanumeric with hyphens/underscores: %q", name)
	}
	return nil
}

type merkleServiceImpl struct {
	localDir string
	s3Client *s3.Client
	bucket   string
	prefix   string
	isDev    bool
	logger   *zap.Logger
}

// NewMerkleService creates a MerkleService that persists trees to local disk
// in development, or to S3 in non-development environments.
func NewMerkleService(cfg config.IndexerConfig, envCfg *coreconfig.EnvConfig, logger *zap.Logger) (services.MerkleService, error) {
	// Fall back to source S3 bucket when merkle-specific bucket isn't set.
	bucket := cfg.MerkleS3Bucket()
	if bucket == "" {
		bucket = cfg.S3Bucket()
	}

	svc := &merkleServiceImpl{
		localDir: cfg.MerkleDir(),
		bucket:   bucket,
		prefix:   cfg.MerkleS3Prefix(),
		isDev:    envCfg.IsDevelopment(),
		logger:   logger.Named("merkle"),
	}

	// Initialize S3 client if bucket is configured (works with MinIO in dev too).
	if svc.bucket != "" {
		region := cfg.MerkleS3Region()
		if region == "" {
			region = "us-east-1"
		}

		var cfgOpts []func(*awsconfig.LoadOptions) error
		cfgOpts = append(cfgOpts, awsconfig.WithRegion(region))

		// Use explicit credentials from config when available (SUPERAGI_S3_ACCESS_KEY /
		// SUPERAGI_S3_SECRET_KEY). Falls back to the default AWS SDK chain (IRSA,
		// instance profile) when not set.
		if ak, sk := cfg.S3AccessKey(), cfg.S3SecretKey(); ak != "" && sk != "" {
			cfgOpts = append(cfgOpts, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(ak, sk, ""),
			))
		}

		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to init S3 for merkle: %w", err)
		}

		var s3Opts []func(*s3.Options)
		if endpoint := cfg.S3Endpoint(); endpoint != "" {
			s3Opts = append(s3Opts, func(o *s3.Options) {
				o.BaseEndpoint = &endpoint
				o.UsePathStyle = true
			})
		}
		svc.s3Client = s3.NewFromConfig(awsCfg, s3Opts...)
	}

	return svc, nil
}

func (m *merkleServiceImpl) BuildTree(ctx context.Context, provider services.SourceProvider, root string) (*services.MerkleNode, error) {
	files, err := provider.ListFiles(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("failed to list files for merkle tree: %w", err)
	}

	type fileEntry struct {
		relPath string
		hash    string
	}

	var entries []fileEntry
	for _, f := range files {
		if f.IsDir {
			continue
		}
		hash, err := provider.GetFileHash(ctx, f.Path)
		if err != nil {
			m.logger.Warn("Failed to hash file, skipping", zap.String("path", f.Path), zap.Error(err))
			continue
		}
		entries = append(entries, fileEntry{relPath: f.RelativePath, hash: hash})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relPath < entries[j].relPath
	})

	rootNode := &services.MerkleNode{Path: ".", IsDir: true}
	dirNodes := map[string]*services.MerkleNode{".": rootNode}

	ensureDir := func(dirPath string) *services.MerkleNode {
		if n, ok := dirNodes[dirPath]; ok {
			return n
		}
		parts := strings.Split(dirPath, "/")
		for i := 1; i <= len(parts); i++ {
			p := strings.Join(parts[:i], "/")
			if _, ok := dirNodes[p]; !ok {
				node := &services.MerkleNode{Path: p, IsDir: true}
				dirNodes[p] = node
				parentPath := "."
				if i > 1 {
					parentPath = strings.Join(parts[:i-1], "/")
				}
				parent := dirNodes[parentPath]
				parent.Children = append(parent.Children, node)
			}
		}
		return dirNodes[dirPath]
	}

	for _, e := range entries {
		dirPath := path.Dir(e.relPath)
		if dirPath == "" {
			dirPath = "."
		}
		parent := ensureDir(dirPath)
		leaf := &services.MerkleNode{
			Path:  e.relPath,
			Hash:  e.hash,
			IsDir: false,
		}
		parent.Children = append(parent.Children, leaf)
	}

	computeHash(rootNode)
	return rootNode, nil
}

func computeHash(node *services.MerkleNode) {
	if !node.IsDir {
		return
	}

	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Path < node.Children[j].Path
	})

	for _, child := range node.Children {
		computeHash(child)
	}

	h := sha256.New()
	for _, child := range node.Children {
		h.Write([]byte(child.Hash))
	}
	node.Hash = hex.EncodeToString(h.Sum(nil))
}

func (m *merkleServiceImpl) DiffTrees(old, new *services.MerkleNode) *services.MerkleDiff {
	diff := &services.MerkleDiff{}
	if old == nil {
		collectFiles(new, &diff.Added)
		return diff
	}
	diffRecursive(old, new, diff)
	return diff
}

func diffRecursive(old, new *services.MerkleNode, diff *services.MerkleDiff) {
	if old == nil && new == nil {
		return
	}
	if old == nil {
		collectFiles(new, &diff.Added)
		return
	}
	if new == nil {
		collectFiles(old, &diff.Deleted)
		return
	}

	if old.Hash == new.Hash {
		return
	}

	if !old.IsDir && !new.IsDir {
		diff.Changed = append(diff.Changed, new.Path)
		return
	}

	oldChildren := make(map[string]*services.MerkleNode)
	for _, c := range old.Children {
		oldChildren[c.Path] = c
	}
	newChildren := make(map[string]*services.MerkleNode)
	for _, c := range new.Children {
		newChildren[c.Path] = c
	}

	for path, newChild := range newChildren {
		if oldChild, ok := oldChildren[path]; ok {
			diffRecursive(oldChild, newChild, diff)
		} else {
			collectFiles(newChild, &diff.Added)
		}
	}

	for path, oldChild := range oldChildren {
		if _, ok := newChildren[path]; !ok {
			collectFiles(oldChild, &diff.Deleted)
		}
	}
}

func collectFiles(node *services.MerkleNode, paths *[]string) {
	if node == nil {
		return
	}
	if !node.IsDir {
		*paths = append(*paths, node.Path)
		return
	}
	for _, c := range node.Children {
		collectFiles(c, paths)
	}
}

// collectFileHashes walks node and populates out with {leaf.Path: leaf.Hash}.
// Directories are traversed; their Hash field (which is a rollup of children)
// is ignored. Used by ApplyChanges to flatten a stored tree into a map that
// can be mutated and rebuilt.
func collectFileHashes(node *services.MerkleNode, out map[string]string) {
	if node == nil {
		return
	}
	if !node.IsDir {
		out[node.Path] = node.Hash
		return
	}
	for _, c := range node.Children {
		collectFileHashes(c, out)
	}
}

// ApplyChanges returns a new MerkleNode tree with updates merged in and
// deletes removed. Implementation flattens tree to a {path: hash} map,
// applies the delta, and rebuilds the directory hierarchy using the same
// ensureDir/computeHash logic as BuildTree. The input tree is not mutated.
func (m *merkleServiceImpl) ApplyChanges(tree *services.MerkleNode, updates map[string]string, deletes []string) *services.MerkleNode {
	// Phase 1: flatten
	leaves := make(map[string]string)
	collectFileHashes(tree, leaves)

	// Phase 2: apply delta. Updates override (or add) leaves; deletes remove
	// them. A delete of a non-existent path is a no-op — matches the
	// finalizer's "re-request on next /diff" contract for failed files.
	for path, hash := range updates {
		leaves[path] = hash
	}
	for _, p := range deletes {
		delete(leaves, p)
	}

	// Phase 3: rebuild. Reuses the same ensureDir closure pattern as
	// BuildTree at services/impl/merkle_service_impl.go so the resulting
	// tree is bit-identical to one produced by BuildTree over the same
	// leaf set.
	type fileEntry struct {
		relPath string
		hash    string
	}
	entries := make([]fileEntry, 0, len(leaves))
	for p, h := range leaves {
		entries = append(entries, fileEntry{relPath: p, hash: h})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relPath < entries[j].relPath
	})

	rootNode := &services.MerkleNode{Path: ".", IsDir: true}
	dirNodes := map[string]*services.MerkleNode{".": rootNode}

	ensureDir := func(dirPath string) *services.MerkleNode {
		if n, ok := dirNodes[dirPath]; ok {
			return n
		}
		parts := strings.Split(dirPath, "/")
		for i := 1; i <= len(parts); i++ {
			p := strings.Join(parts[:i], "/")
			if _, ok := dirNodes[p]; !ok {
				node := &services.MerkleNode{Path: p, IsDir: true}
				dirNodes[p] = node
				parentPath := "."
				if i > 1 {
					parentPath = strings.Join(parts[:i-1], "/")
				}
				parent := dirNodes[parentPath]
				parent.Children = append(parent.Children, node)
			}
		}
		return dirNodes[dirPath]
	}

	for _, e := range entries {
		dirPath := path.Dir(e.relPath)
		if dirPath == "" {
			dirPath = "."
		}
		parent := ensureDir(dirPath)
		leaf := &services.MerkleNode{
			Path:  e.relPath,
			Hash:  e.hash,
			IsDir: false,
		}
		parent.Children = append(parent.Children, leaf)
	}

	computeHash(rootNode)
	return rootNode
}

// ── Local persistence ────────────────────────────────────────────────────────

func (m *merkleServiceImpl) localPath(collection string) string {
	return filepath.Join(m.localDir, collection+".json")
}

func (m *merkleServiceImpl) LoadTree(ctx context.Context, collection string) (*services.MerkleNode, error) {
	if err := validateCollection(collection); err != nil {
		return nil, err
	}

	// Prefer S3 when configured, fall back to local.
	if m.s3Client != nil {
		tree, err := m.loadS3(ctx, collection)
		if err == nil && tree != nil {
			return tree, nil
		}
	}

	if m.localDir != "" {
		tree, err := m.loadLocal(collection)
		if err == nil && tree != nil {
			return tree, nil
		}
	}

	return nil, nil
}

func (m *merkleServiceImpl) SaveTree(ctx context.Context, collection string, tree *services.MerkleNode) error {
	if err := validateCollection(collection); err != nil {
		return err
	}

	// Prefer S3 when configured, fall back to local.
	if m.s3Client != nil {
		if err := m.saveS3(ctx, collection, tree); err != nil {
			return fmt.Errorf("failed to save merkle tree to S3: %w", err)
		}
		return nil
	}
	if m.localDir != "" {
		return m.saveLocal(collection, tree)
	}
	return nil
}

func (m *merkleServiceImpl) DeleteTree(ctx context.Context, collection string) error {
	if m.s3Client != nil {
		key := m.s3Key(collection)
		_, err := m.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(m.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return fmt.Errorf("failed to delete merkle tree from S3: %w", err)
		}
		return nil
	}
	if m.localDir != "" {
		return m.deleteLocal(collection)
	}
	return nil
}

func (m *merkleServiceImpl) loadLocal(collection string) (*services.MerkleNode, error) {
	p := m.localPath(collection)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	var tree services.MerkleNode
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("failed to unmarshal local merkle tree: %w", err)
	}
	return &tree, nil
}

func (m *merkleServiceImpl) saveLocal(collection string, tree *services.MerkleNode) error {
	if err := os.MkdirAll(m.localDir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("failed to marshal merkle tree: %w", err)
	}

	p := m.localPath(collection)
	tmpPath := p + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, p)
}

func (m *merkleServiceImpl) deleteLocal(collection string) error {
	p := m.localPath(collection)
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete local merkle tree: %w", err)
	}
	return nil
}

// ── S3 persistence (optional) ────────────────────────────────────────────────

func (m *merkleServiceImpl) s3Key(collection string) string {
	prefix := m.prefix
	if prefix == "" {
		prefix = "merkle"
	}
	return fmt.Sprintf("%s/%s/tree.json", prefix, collection)
}

func (m *merkleServiceImpl) loadS3(ctx context.Context, collection string) (*services.MerkleNode, error) {
	key := m.s3Key(collection)
	out, err := m.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil // genuinely missing
		}
		return nil, fmt.Errorf("load merkle tree from S3: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read merkle tree from S3: %w", err)
	}

	var tree services.MerkleNode
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("failed to unmarshal S3 merkle tree: %w", err)
	}
	return &tree, nil
}

func (m *merkleServiceImpl) saveS3(ctx context.Context, collection string, tree *services.MerkleNode) error {
	data, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("failed to marshal merkle tree: %w", err)
	}

	key := m.s3Key(collection)
	_, err = m.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(m.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	return err
}

// ── CAS (Compare-and-Set) persistence ────────────────────────────────────────
//
// LoadTreeWithVersion / SaveTreeIfUnchanged implement optimistic concurrency
// control so multiple finalizers can commit Merkle updates safely without
// corrupting each other. S3 uses ETag + If-Match (the server rejects the
// write with 412 Precondition Failed when the token no longer matches).
// The local filesystem variant uses a POSIX advisory lock (flock) plus a
// sha256 hash of the serialised bytes as the version token; we re-read under
// the lock so concurrent writers that wait on the lock see each other's
// commits.
//
// emptyTree is returned when the backend has no tree for the collection —
// callers get a root they can diff against without nil-handling everywhere.

func (m *merkleServiceImpl) emptyTree() *services.MerkleNode {
	return &services.MerkleNode{Path: ".", IsDir: true}
}

func (m *merkleServiceImpl) LoadTreeWithVersion(ctx context.Context, collection string) (*services.MerkleNode, string, error) {
	if err := validateCollection(collection); err != nil {
		return nil, "", err
	}

	if m.s3Client != nil {
		return m.loadS3WithVersion(ctx, collection)
	}
	if m.localDir != "" {
		return m.loadLocalWithVersion(collection)
	}
	return m.emptyTree(), "", nil
}

func (m *merkleServiceImpl) SaveTreeIfUnchanged(ctx context.Context, collection string, tree *services.MerkleNode, expectedVersion string) error {
	if err := validateCollection(collection); err != nil {
		return err
	}
	if tree == nil {
		return fmt.Errorf("cannot save nil merkle tree")
	}

	if m.s3Client != nil {
		return m.saveS3IfUnchanged(ctx, collection, tree, expectedVersion)
	}
	if m.localDir != "" {
		return m.saveLocalIfUnchanged(collection, tree, expectedVersion)
	}
	return fmt.Errorf("no merkle backend configured")
}

func (m *merkleServiceImpl) loadS3WithVersion(ctx context.Context, collection string) (*services.MerkleNode, string, error) {
	key := m.s3Key(collection)
	out, err := m.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return m.emptyTree(), "", nil
		}
		if isS3Permanent(err) {
			return nil, "", fmt.Errorf("load merkle tree from S3: %w: %v", services.ErrStoragePermanent, err)
		}
		return nil, "", fmt.Errorf("load merkle tree from S3: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read merkle tree from S3: %w", err)
	}

	var tree services.MerkleNode
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, "", fmt.Errorf("unmarshal S3 merkle tree: %w", err)
	}
	return &tree, aws.ToString(out.ETag), nil
}

func (m *merkleServiceImpl) saveS3IfUnchanged(ctx context.Context, collection string, tree *services.MerkleNode, expectedVersion string) error {
	data, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("marshal merkle tree: %w", err)
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.bucket),
		Key:         aws.String(m.s3Key(collection)),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	}
	if expectedVersion == "" {
		// First-write: fail if any object already exists at this key.
		input.IfNoneMatch = aws.String("*")
	} else {
		input.IfMatch = aws.String(expectedVersion)
	}

	_, err = m.s3Client.PutObject(ctx, input)
	if isS3CASMiss(err) {
		return services.ErrVersionMismatch
	}
	if isS3Permanent(err) {
		return fmt.Errorf("save merkle tree to S3: %w: %v", services.ErrStoragePermanent, err)
	}
	if err != nil {
		return fmt.Errorf("save merkle tree to S3: %w", err)
	}
	return nil
}

// isS3Permanent recognises S3 error codes that cannot be fixed by
// retrying: credential/permission failures (AccessDenied,
// InvalidAccessKeyId, SignatureDoesNotMatch) and bucket-missing
// (NoSuchBucket). These surface ErrStoragePermanent so the finalizer
// stamps the session as MarkFailed immediately rather than burning the
// Asynq retry budget on a doomed task. NoSuchKey is deliberately NOT in
// this list — loadS3WithVersion treats it as "empty tree" upstream.
func isS3Permanent(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch",
		"NoSuchBucket", "AllAccessDisabled":
		return true
	}
	return false
}

// isS3CASMiss recognises both error codes S3 returns for failed conditional
// PutObject requests. PreconditionFailed (HTTP 412) is returned when IfMatch
// does not match an existing object's ETag. ConditionalRequestConflict
// (HTTP 409) is returned when IfNoneMatch="*" fails because an object
// already exists at the key, or when concurrent writers race during a
// conditional upload. Both conditions map to ErrVersionMismatch so the
// finalizer can re-read and retry.
func isS3CASMiss(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "PreconditionFailed" || code == "ConditionalRequestConflict"
	}
	return false
}

func (m *merkleServiceImpl) loadLocalWithVersion(collection string) (*services.MerkleNode, string, error) {
	p := m.localPath(collection)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m.emptyTree(), "", nil
		}
		return nil, "", fmt.Errorf("read local merkle tree: %w", err)
	}

	var tree services.MerkleNode
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, "", fmt.Errorf("unmarshal local merkle tree: %w", err)
	}
	sum := sha256.Sum256(data)
	return &tree, hex.EncodeToString(sum[:]), nil
}

func (m *merkleServiceImpl) saveLocalIfUnchanged(collection string, tree *services.MerkleNode, expectedVersion string) error {
	if err := os.MkdirAll(m.localDir, 0o755); err != nil {
		return fmt.Errorf("mkdir merkle local dir: %w", err)
	}

	p := m.localPath(collection)
	lockPath := p + ".lock"

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open merkle lock %q: %w", lockPath, err)
	}
	defer lockFile.Close()

	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("flock merkle lock %q: %w", lockPath, err)
	}
	defer func() {
		_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
	}()

	// Re-read under the lock so any writer that held the lock ahead of us
	// has flushed its state.
	existing, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if expectedVersion != "" {
			return services.ErrVersionMismatch
		}
	case err != nil:
		return fmt.Errorf("re-read local merkle tree: %w", err)
	default:
		currentSum := sha256.Sum256(existing)
		currentVersion := hex.EncodeToString(currentSum[:])
		if currentVersion != expectedVersion {
			return services.ErrVersionMismatch
		}
	}

	newData, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("marshal merkle tree: %w", err)
	}

	tmpPath := p + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0o600); err != nil {
		return fmt.Errorf("write tmp merkle tree: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		return fmt.Errorf("rename tmp merkle tree: %w", err)
	}
	return nil
}
