// Package backend defines the platform-neutral Backend interface and the
// value types that every issue-spec command consumes.
//
// A Backend implementation is responsible for translating these abstract
// operations into platform-specific calls (GitHub REST today, GitLab via
// glab in a follow-up PROCESS). Command code in internal/commands MUST
// depend only on this package and on the Backend interface itself; reaching
// into internal/github or internal/gitlab directly is a layering violation
// tracked by SPEC-001.
//
// # Stability
//
// The Backend interface is the public contract between command code and
// every platform implementation. Adding a method is a breaking change
// because both GitHub and GitLab implementations must update in lockstep.
// internal/backend/backend_contract_test.go pins the method count so any
// drift is caught at CI time instead of at code review.
package backend

import (
	"context"
	"time"
)

// Backend is the platform-neutral API surface implemented by GitHub today
// and GitLab in PROCESS-002. Every method MUST be safe to call from
// multiple goroutines; implementations that wrap an external CLI (like gh
// or glab) are responsible for serialising or rate-limiting themselves.
//
// Conventions for all methods:
//
//   - The first argument is a context.Context; implementations MUST honour
//     cancellation and the deadline.
//   - The repo argument uses the platform's local project notation
//     ("owner/name" on GitHub, "group/subgroup/project" or numeric IDs on
//     GitLab). Implementations parse and validate it themselves.
//   - Errors must wrap the underlying platform error with enough context
//     for command code to surface a useful message. Sentinel errors can
//     be added later by wrapping in this package; for now callers use
//     errors.Is/errors.As on the underlying type.
//   - Returned value types (Issue, Note, ...) MUST NOT leak platform
//     identifiers (GitHub node_id, GitLab iid mapping, etc.) on the wire.
//     Normalisation happens at the implementation boundary.
type Backend interface {
	// Issue lifecycle -------------------------------------------------------

	// CreateIssue opens a new issue with the given title, body and labels.
	// The returned CreateIssueResult exposes the platform's user-visible
	// number (GitHub issue number, GitLab iid) and an opaque ID for
	// subsequent API calls; both are required because GitHub uses
	// node_id-as-string in some endpoints while GitLab exposes a numeric
	// project ID.
	CreateIssue(ctx context.Context, repo string, opts CreateIssueOpts) (CreateIssueResult, error)

	// GetIssue returns the current state of a single issue. The returned
	// Issue uses the same field names regardless of platform.
	GetIssue(ctx context.Context, repo string, number int) (Issue, error)

	// ListIssueNotes returns every top-level note on an issue, ordered
	// chronologically (oldest first). Implementations MUST paginate so
	// callers do not have to.
	ListIssueNotes(ctx context.Context, repo string, number int) ([]Note, error)

	// CreateIssueNote posts a new top-level note on an issue.
	CreateIssueNote(ctx context.Context, repo string, number int, body string) (Note, error)

	// UpdateIssueNote edits the body of an existing top-level issue note.
	UpdateIssueNote(ctx context.Context, repo string, noteID int64, body string) (Note, error)

	// ReplyIssueDiscussion posts a reply to a discussion thread on an
	// issue. On GitHub the discussionID is empty for plain comments; on
	// GitLab it is the discussion thread id. Implementations accept the
	// empty string and route the body as a new top-level note in that
	// case so callers do not need to branch on platform.
	ReplyIssueDiscussion(ctx context.Context, repo string, issueNumber int, discussionID string, body string) (Note, error)

	// Merge request lifecycle ----------------------------------------------

	// GetMergeRequest returns the current state of a merge request / pull
	// request.
	GetMergeRequest(ctx context.Context, repo string, number int) (MergeRequest, error)

	// CreateMergeRequest opens a new merge request / pull request.
	CreateMergeRequest(ctx context.Context, repo string, opts CreateMergeRequestOpts) (MergeRequest, error)

	// UpdateMergeRequest applies changes to an existing merge request
	// (title, body, target branch, state). Pass only the fields that
	// should change; implementations leave the rest untouched.
	UpdateMergeRequest(ctx context.Context, repo string, number int, opts UpdateMergeRequestOpts) (MergeRequest, error)

	// ListMRFiles returns the diff of a merge request as a list of files
	// with patches. Implementations MUST paginate.
	ListMRFiles(ctx context.Context, repo string, number int) ([]DiffFile, error)

	// ListMRDiscussions returns the inline and general discussions on a
	// merge request. A discussion groups one or more Notes (the original
	// comment plus any replies) plus the file/line position it is anchored
	// to, if any.
	ListMRDiscussions(ctx context.Context, repo string, number int) ([]Discussion, error)

	// CreateMRDiscussion posts a new inline discussion at the given file
	// and line. To post a general (non-inline) comment, leave File/Line
	// empty and pass only Body.
	CreateMRDiscussion(ctx context.Context, repo string, number int, opts CreateDiscussionOpts) (Discussion, error)

	// ReplyMRDiscussion appends a reply note to an existing discussion
	// thread.
	ReplyMRDiscussion(ctx context.Context, repo string, number int, discussionID string, body string) (Note, error)

	// ResolveMRDiscussion marks a discussion thread as resolved. On
	// platforms where resolution is per-thread (GitLab) this is a
	// no-op aside from the state change; on platforms where each note is
	// resolved independently (GitHub) the implementation must resolve
	// every outstanding note in the thread.
	ResolveMRDiscussion(ctx context.Context, repo string, number int, discussionID string) error

	// CI / pipelines --------------------------------------------------------

	// GetMRPipelineStatus returns the latest pipeline / check-run status
	// for the merge request's head commit. The Jobs slice may be empty if
	// the platform exposes only an aggregate status.
	GetMRPipelineStatus(ctx context.Context, repo string, number int) (PipelineStatus, error)

	// ListMRPipelineJobs returns every job in the given pipeline, in the
	// order the platform returns them.
	ListMRPipelineJobs(ctx context.Context, repo string, pipelineID int64) ([]PipelineJob, error)

	// Labels ---------------------------------------------------------------

	// CreateLabel creates a label with the given name, colour (in the
	// platform's local format: hex without leading "#" on GitHub, hex
	// with leading "#" on GitLab) and description. If the label already
	// exists implementations MUST return a LabelResult with Skipped=true
	// rather than an error.
	CreateLabel(ctx context.Context, repo, name, color, description string) (LabelResult, error)
}

// -----------------------------------------------------------------------------
// Value types
// -----------------------------------------------------------------------------

// Issue is the platform-neutral representation of an issue. Platform
// implementations normalise GitHub's and GitLab's local shapes into this
// struct; callers MUST NOT assume a platform-specific field exists.
type Issue struct {
	// Number is the user-visible identifier (GitHub issue number, GitLab
	// iid).
	Number int `json:"number"`
	// Title is the issue title as the author wrote it.
	Title string `json:"title"`
	// Body is the issue body in markdown.
	Body string `json:"body"`
	// State is the platform's local state string ("open", "closed").
	State string `json:"state"`
	// URL is the user-visible HTML URL.
	URL string `json:"url"`
	// CreatedAt is when the issue was created.
	CreatedAt time.Time `json:"created_at"`
	// Author is the human-readable handle of the author.
	Author string `json:"author"`
	// Labels is the list of label names attached to the issue.
	Labels []string `json:"labels,omitempty"`
	// can extend platform-specific fields later
}

// Note is the platform-neutral representation of a single comment, reply,
// or discussion note. It covers both issue comments and merge request
// discussion notes because the shape is identical: an author, a body, a
// timestamp and a URL.
type Note struct {
	// ID is the platform-local note identifier (numeric on GitHub, numeric
	// on GitLab note IDs).
	ID int64 `json:"id"`
	// Author is the human-readable handle of the note author.
	Author string `json:"author"`
	// Body is the note body in markdown.
	Body string `json:"body"`
	// CreatedAt is when the note was first posted.
	CreatedAt time.Time `json:"created_at"`
	// URL is the user-visible HTML URL of the note.
	URL string `json:"url"`
	// can extend platform-specific fields later
}

// MergeRequest is the platform-neutral representation of a merge request
// (GitLab) or pull request (GitHub).
type MergeRequest struct {
	// Number is the user-visible identifier.
	Number int `json:"number"`
	// Title is the merge request title.
	Title string `json:"title"`
	// Body is the merge request description in markdown.
	Body string `json:"body"`
	// State is the platform's local state string ("open", "closed",
	// "merged").
	State string `json:"state"`
	// SourceBranch is the head branch.
	SourceBranch string `json:"source_branch"`
	// TargetBranch is the base branch.
	TargetBranch string `json:"target_branch"`
	// HeadSHA is the tip commit of the source branch.
	HeadSHA string `json:"head_sha"`
	// URL is the user-visible HTML URL.
	URL string `json:"url"`
	// CreatedAt is when the merge request was created.
	CreatedAt time.Time `json:"created_at"`
	// Author is the human-readable handle of the author.
	Author string `json:"author"`
	// can extend platform-specific fields later
}

// DiffFile is a single file in a merge request's diff.
type DiffFile struct {
	// Path is the file path relative to the repository root.
	Path string `json:"path"`
	// Patch is the unified diff hunk for the file; empty for binary files
	// or files that only changed mode.
	Patch string `json:"patch,omitempty"`
	// can extend platform-specific fields later
}

// Discussion is a discussion thread on a merge request. A thread may have
// one or more Notes (the original comment plus any replies) and may be
// anchored to a file/line position on the head SHA.
type Discussion struct {
	// ID is the platform-local discussion identifier. On GitLab this is a
	// string like "abc123"; on GitHub it is the numeric review comment
	// thread id rendered as a string so the field type stays uniform.
	ID string `json:"id"`
	// Notes is the ordered list of notes in the thread (oldest first).
	Notes []Note `json:"notes"`
	// Resolved is true if the thread has been marked resolved.
	Resolved bool `json:"resolved"`
	// File is the file path the discussion is anchored to, empty for
	// general (non-inline) discussions.
	File string `json:"file,omitempty"`
	// Line is the line number on the new side of the diff; zero if the
	// discussion is not anchored or is anchored on the old side only.
	Line int `json:"line,omitempty"`
	// OldLine is the line number on the old side of the diff; zero if
	// the discussion is not anchored or is anchored on the new side only.
	OldLine int `json:"old_line,omitempty"`
	// CommitID is the head SHA the discussion is anchored to; empty for
	// general discussions.
	CommitID string `json:"commit_id,omitempty"`
	// can extend platform-specific fields later
}

// PipelineStatus is the aggregate status of a merge request's pipeline.
// The Jobs slice is the optional expansion; some platforms expose only
// the aggregate state.
type PipelineStatus struct {
	// State is the high-level pipeline state ("running", "success",
	// "failed", "canceled", ...).
	State string `json:"state"`
	// Conclusion mirrors State for platforms that distinguish between
	// "in progress" (State) and "final result" (Conclusion). On platforms
	// that do not (GitLab pipelines), Conclusion is empty.
	Conclusion string `json:"conclusion,omitempty"`
	// SHA is the commit the pipeline ran against.
	SHA string `json:"sha"`
	// URL is the user-visible HTML URL of the pipeline.
	URL string `json:"url"`
	// Jobs is the optional list of jobs in the pipeline.
	Jobs []PipelineJob `json:"jobs,omitempty"`
	// can extend platform-specific fields later
}

// PipelineJob is one job inside a pipeline. Status is the in-progress
// state and Conclusion is the final result (empty while still running).
type PipelineJob struct {
	// ID is the platform-local job identifier.
	ID int64 `json:"id"`
	// Name is the job's display name (GitHub job name, GitLab stage/job
	// name).
	Name string `json:"name"`
	// Status is the in-progress state ("queued", "running", ...).
	Status string `json:"status"`
	// Conclusion is the final result ("success", "failure", ...);
	// empty while still running.
	Conclusion string `json:"conclusion,omitempty"`
	// StartedAt is when the job began executing.
	StartedAt time.Time `json:"started_at,omitempty"`
	// FinishedAt is when the job finished; zero while still running.
	FinishedAt time.Time `json:"finished_at,omitempty"`
	// can extend platform-specific fields later
}

// LabelResult is the outcome of CreateLabel. Implementations use Skipped
// rather than an error when a label already exists so callers can decide
// whether that is acceptable.
type LabelResult struct {
	// Name is the label name as returned by the platform.
	Name string `json:"name"`
	// Color is the colour in the platform's local format.
	Color string `json:"color"`
	// Skipped is true if the label already existed and CreateLabel did
	// not create a new one.
	Skipped bool `json:"skipped"`
	// can extend platform-specific fields later
}

// -----------------------------------------------------------------------------
// Option types
// -----------------------------------------------------------------------------

// CreateIssueOpts is the input to CreateIssue.
type CreateIssueOpts struct {
	// Title is the issue title.
	Title string
	// Body is the issue body in markdown.
	Body string
	// Labels is the optional list of label names to attach at creation
	// time.
	Labels []string
	// can extend platform-specific fields later
}

// CreateIssueResult is the output of CreateIssue.
type CreateIssueResult struct {
	// Number is the user-visible identifier (GitHub issue number, GitLab
	// iid).
	Number int `json:"number"`
	// ID is the platform-local database identifier (GitHub node_id as
	// int64 fallback or GitLab issue id).
	ID int64 `json:"id"`
	// URL is the user-visible HTML URL of the new issue.
	URL string `json:"url"`
	// can extend platform-specific fields later
}

// CreateMergeRequestOpts is the input to CreateMergeRequest.
type CreateMergeRequestOpts struct {
	// Title is the merge request title.
	Title string
	// Body is the merge request description in markdown.
	Body string
	// SourceBranch is the head branch (the branch that contains the
	// changes to be merged).
	SourceBranch string
	// TargetBranch is the base branch (the branch the changes will be
	// merged into).
	TargetBranch string
	// can extend platform-specific fields later
}

// UpdateMergeRequestOpts is the input to UpdateMergeRequest. A nil pointer
// means "leave this field untouched"; implementations MUST skip nil
// pointers when constructing the platform request.
type UpdateMergeRequestOpts struct {
	// Title, when non-nil, replaces the current title.
	Title *string
	// Body, when non-nil, replaces the current description.
	Body *string
	// TargetBranch, when non-nil, changes the base branch.
	TargetBranch *string
	// State, when non-nil, sets the merge request state ("open",
	// "closed").
	State *string
	// can extend platform-specific fields later
}

// CreateDiscussionOpts is the input to CreateMRDiscussion. To post a
// general (non-inline) comment, leave File, Line, OldLine, and CommitID
// at their zero values and pass only Body.
type CreateDiscussionOpts struct {
	// Body is the comment body in markdown.
	Body string
	// File is the file path the discussion is anchored to; empty for
	// general discussions.
	File string
	// Line is the line number on the new side of the diff.
	Line int
	// OldLine is the line number on the old side of the diff; zero if
	// the discussion is anchored on the new side only.
	OldLine int
	// CommitID is the head SHA the discussion is anchored to; empty for
	// general discussions.
	CommitID string
	// Side is the diff side ("new" or "old"); empty defaults to "new".
	Side string
	// can extend platform-specific fields later
}

// -----------------------------------------------------------------------------
// Severity / status enums
// -----------------------------------------------------------------------------

// Severity is a platform-neutral severity classification used by issue-spec
// findings (review comments, P0/P1/P2 tags). Platforms map their local
// labels (GitHub label names, GitLab priority labels) onto these.
type Severity int

const (
	// SeverityP0 is the most urgent severity (blocker).
	SeverityP0 Severity = iota
	// SeverityP1 is a high severity (must fix before merge).
	SeverityP1
	// SeverityP2 is a medium severity (should fix soon).
	SeverityP2
)

// FindingStatus is the platform-neutral lifecycle status of a finding
// surfaced on an issue or merge request. Both platforms converge on these
// four states; GitHub uses label colours and GitLab uses its own status
// field.
type FindingStatus int

const (
	// FindingOpen means the finding is open and unresolved.
	FindingOpen FindingStatus = iota
	// FindingResolved means the author has addressed the finding and
	// the reviewer accepted it.
	FindingResolved
	// FindingClosed means the issue carrying the finding was closed
	// without resolving the finding itself.
	FindingClosed
	// FindingFixed means the author applied a fix and merged it.
	FindingFixed
	// FindingSuperseded means a newer finding replaced this one.
	FindingSuperseded
)
