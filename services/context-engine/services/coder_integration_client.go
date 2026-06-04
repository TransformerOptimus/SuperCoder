package services

import "context"

// CoderIntegrationClient abstracts gRPC calls to the coder-integrations CoderIntegrationService.
type CoderIntegrationClient interface {
	GetPRDiff(ctx context.Context, req *GetPRDiffRequest) (*GetPRDiffResponse, error)
	GetFileContent(ctx context.Context, req *GetFileContentRequest) (*GetFileContentResponse, error)
	ListPRFiles(ctx context.Context, req *ListPRFilesRequest) (*ListPRFilesResponse, error)
	GetPullRequest(ctx context.Context, req *GetPullRequestRequest) (*GetPullRequestResponse, error)
	CreatePRReview(ctx context.Context, req *CreatePRReviewRequest) (*CreatePRReviewResponse, error)
	AddPRComment(ctx context.Context, req *AddPRCommentRequest) (*AddPRCommentResponse, error)
	UpdatePullRequest(ctx context.Context, req *UpdatePullRequestRequest) (*UpdatePullRequestResponse, error)
	ListPRComments(ctx context.Context, req *ListPRCommentsRequest) (*ListPRCommentsResponse, error)
	UpdatePRComment(ctx context.Context, req *UpdatePRCommentRequest) (*UpdatePRCommentResponse, error)
	ListUnresolvedComments(ctx context.Context, req *ListUnresolvedCommentsRequest) (*ListUnresolvedCommentsResponse, error)
	SaveReview(ctx context.Context, req *SaveReviewRequest) (*SaveReviewResponse, error)
	LinkReviewThreads(ctx context.Context, req *LinkReviewThreadsRequest) error
	ResolveComments(ctx context.Context, req *ResolveCommentsRequest) (*ResolveCommentsResponse, error)
	CreateCheckRun(ctx context.Context, req *CreateCheckRunRequest) (*CreateCheckRunResponse, error)
	UpdateCheckRun(ctx context.Context, req *UpdateCheckRunRequest) (*UpdateCheckRunResponse, error)
	ArchiveRepoToS3(ctx context.Context, req *ArchiveRepoToS3Request) (*ArchiveRepoToS3Response, error)
	Close() error
}

type GetPRDiffRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
}

type GetPRDiffResponse struct {
	Diff string
}

type GetFileContentRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Path        string
	Ref         string
}

type GetFileContentResponse struct {
	Content []byte
}

type ListPRFilesRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
}

type ListPRFilesResponse struct {
	Files []PRFileInfo
}

type PRFileInfo struct {
	Filename  string
	Status    string
	Additions int32
	Deletions int32
	Patch     string
}

type CreatePRReviewRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
	Body        string
	Event       string // APPROVE, REQUEST_CHANGES, COMMENT
	Comments    []PRReviewCommentInput
}

type PRReviewCommentInput struct {
	Path      string
	Line      int32
	Body      string
	Side      string // LEFT, RIGHT
	StartLine int32
}

type CreatePRReviewResponse struct {
	ID      int64
	State   string
	HTMLURL string
}

type AddPRCommentRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
	Body        string
}

type AddPRCommentResponse struct {
	ID      int64
	HTMLURL string
}

type GetPullRequestRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
}

type GetPullRequestResponse struct {
	ID   int64
	Body string
}

type UpdatePullRequestRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
	Body        string
}

type UpdatePullRequestResponse struct {
	ID      int64
	HTMLURL string
}

type ListPRCommentsRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Number      int32
}

type ListPRCommentsResponse struct {
	Comments []PRCommentInfo
}

type PRCommentInfo struct {
	ID      int64
	Body    string
	HTMLURL string
}

type UpdatePRCommentRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	CommentID   int64
	Body        string
}

type UpdatePRCommentResponse struct {
	ID      int64
	HTMLURL string
}

type ListUnresolvedCommentsRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	PRNumber    int32
}

type ListUnresolvedCommentsResponse struct {
	Comments []ExistingComment
}

type CreateCheckRunRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	HeadSHA     string
	Name        string
	Status      string // queued, in_progress
}

type CreateCheckRunResponse struct {
	ID int64
}

type UpdateCheckRunRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	CheckRunID  int64
	Status      string // in_progress, completed
	Conclusion  string // success, failure, neutral
	Summary     string
}

type UpdateCheckRunResponse struct {
	ID int64
}

type SaveReviewRequest struct {
	WorkspaceID  uint64
	Provider     string
	Repo         string
	PRNumber     int32
	PRURL        string
	AuthorHandle string
	Verdict      string
	BaseSHA      string
	HeadSHA      string
	Comments     []SaveReviewComment
}

type SaveReviewComment struct {
	FilePath    string
	LineNumber  int32
	Severity    string
	Category    string
	Body        string
	IssueTypeID string
}

type SaveReviewResponse struct {
	ReviewID           string
	TotalUnresolved    int
	PreviousUnresolved int
}

type ArchiveRepoToS3Request struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	Ref         string
	S3Bucket    string
	S3Region    string
	S3Endpoint  string
}

type ArchiveRepoToS3Response struct {
	S3Bucket  string
	S3Prefix  string
	CommitSHA string
	FileCount int32
}

type LinkReviewThreadsRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	PRNumber    int32
	ReviewID    string
}

type ResolveCommentsRequest struct {
	WorkspaceID uint64
	Provider    string
	Repo        string
	PRNumber    int32
	CommentIDs  []string
}

type ResolveCommentsResponse struct {
	ResolvedCount   int
	FailedCount     int
	NoThreadIDCount int
}
