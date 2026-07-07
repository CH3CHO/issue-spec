// Package gitlab — Backend implements internal/backend.Backend by wrapping
// glab CLI calls. The Runner interface (defined in glab.go) is the seam
// for tests; production code uses ExecRunner, tests use the fakeRunner in
// backend_test.go.
//
// The methods follow a small pattern:
//
//  1. Build the argv slice (with --repo, --output json, -X, -f as needed).
//  2. Call runner.Run.
//  3. On non-nil err, wrap via ClassifyStderr and return.
//  4. On success, json.Unmarshal stdout into a domain struct, then copy
//     into the platform-neutral types defined in internal/backend.
//
// glab's iid is renamed to Backend.Number when copying into the platform
// types so the rest of issue-spec never sees a GitLab-specific identifier.
package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/higress-group/issue-spec/internal/backend"
)

// -----------------------------------------------------------------------------
// Backend
// -----------------------------------------------------------------------------

// Backend is the GitLab implementation of internal/backend.Backend. It is
// safe to share across goroutines: every method calls Runner.Run with the
// caller's context and does not mutate any internal state.
type Backend struct {
	// repo is the default GitLab project path used when callers pass
	// "o/r" to the Backend methods without specifying --repo. Most
	// callers pass it explicitly; the field exists as a safety net so
	// callers do not have to repeat themselves.
	repo string
	// host is the GitLab host (gitlab.com, or a self-hosted instance).
	// Empty defaults to gitlab.com at runner-config time.
	host string
	// token is the GITLAB_TOKEN used for API calls. The Backend never
	// logs it; ClassifyStderr redacts via the runner layer.
	token string
	// runner is the injected glab invocation seam. New() defaults to an
	// ExecRunner when nil.
	runner Runner
}

// New returns a Backend wired to the given Runner. repo is the default
// project (may be overridden per call); token is passed through to glab's
// environment (in the Runner, not here). When runner is nil an ExecRunner
// pointing at "glab" on PATH is created.
func New(repo, token string, runner Runner) *Backend {
	if runner == nil {
		runner = &ExecRunner{}
	}
	return &Backend{
		repo:   repo,
		token:  token,
		runner: runner,
	}
}

// Compile-time guard for the platform-neutral interface.
var _ backend.Backend = (*Backend)(nil)

// projectPath returns the URL-encoded GitLab REST project segment.
// GitLab's REST API expects "group/subgroup/project" with each path
// component URL-encoded and joined by "%2F". We do NOT encode "/" as
// "%2F" blindly: each segment is escaped individually so user-supplied
// characters (spaces, punctuation) are preserved.
func projectPath(repo string) string {
	parts := strings.Split(repo, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "%2F")
}

// run is the small helper that all Backend methods funnel through. It
// invokes runner.Run, classifies stderr on non-nil exit, and returns the
// raw stdout (already classified error if any).
func (b *Backend) run(ctx context.Context, args ...string) ([]byte, error) {
	out, stderr, err := b.runner.Run(ctx, args...)
	if err != nil {
		return nil, ClassifyStderr(stderr, err)
	}
	return out, nil
}

// runJSON is run + json.Unmarshal into target. The target is reset by the
// caller (typically as &result).
func (b *Backend) runJSON(ctx context.Context, target any, args ...string) error {
	out, err := b.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, target); err != nil {
		return &Error{
			Kind:    ErrorUnknown,
			Message: fmt.Sprintf("glab: decode: %s: %s", err, truncate(string(out), 200)),
			RawErr:  err,
		}
	}
	return nil
}

// truncate keeps error messages short.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// isAlreadyExists recognises GitLab's "label already exists" / "name has
// already been taken" 409 response. The Backend returns Skipped rather
// than error for this case per the CreateLabel contract.
func isAlreadyExists(stderr []byte) bool {
	raw := strings.ToLower(string(stderr))
	return strings.Contains(raw, "already exists") ||
		strings.Contains(raw, "has already been taken") ||
		(strings.Contains(raw, "409") && strings.Contains(raw, "conflict"))
}

// -----------------------------------------------------------------------------
// Issues
// -----------------------------------------------------------------------------

// CreateIssue opens a new issue via `glab issue create`. The CLI returns
// iid (project-local number) and id (global ID) — both are surfaced via
// CreateIssueResult.
func (b *Backend) CreateIssue(ctx context.Context, repo string, opts backend.CreateIssueOpts) (backend.CreateIssueResult, error) {
	args := []string{"issue", "create", "--repo", repo}
	if title := strings.TrimSpace(opts.Title); title != "" {
		args = append(args, "--title", title)
	}
	if body := opts.Body; body != "" {
		args = append(args, "--description", body)
	}
	for _, label := range opts.Labels {
		if label = strings.TrimSpace(label); label != "" {
			args = append(args, "--label", label)
		}
	}
	args = append(args, "--output", "json")

	var raw struct {
		IID    int    `json:"iid"`
		ID     int64  `json:"id"`
		WebURL string `json:"web_url"`
	}
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.CreateIssueResult{}, err
	}
	return backend.CreateIssueResult{
		Number: raw.IID,
		ID:     raw.ID,
		URL:    raw.WebURL,
	}, nil
}

// GetIssue fetches a single issue via `glab issue view`.
func (b *Backend) GetIssue(ctx context.Context, repo string, number int) (backend.Issue, error) {
	args := []string{"issue", "view", strconv.Itoa(number), "--repo", repo, "--output", "json"}
	var raw gitlabIssue
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.Issue{}, err
	}
	return raw.toBackend(), nil
}

// ListIssueNotes returns the chronologically-ordered notes on an issue
// via the REST API because glab has no first-class `glab issue note list`.
func (b *Backend) ListIssueNotes(ctx context.Context, repo string, number int) ([]backend.Note, error) {
	endpoint := fmt.Sprintf("projects/%s/issues/%d/notes", projectPath(repo), number)
	var raw []gitlabNote
	if err := b.runJSON(ctx, &raw, "api", "--method", "GET", endpoint); err != nil {
		return nil, err
	}
	out := make([]backend.Note, 0, len(raw))
	for i := range raw {
		out = append(out, raw[i].toBackend())
	}
	return out, nil
}

// CreateIssueNote posts a new top-level note via the REST API.
func (b *Backend) CreateIssueNote(ctx context.Context, repo string, number int, body string) (backend.Note, error) {
	endpoint := fmt.Sprintf("projects/%s/issues/%d/notes", projectPath(repo), number)
	var raw gitlabNote
	if err := b.runJSON(ctx, &raw,
		"api", "--method", "POST", endpoint, "-f", "body="+body); err != nil {
		return backend.Note{}, err
	}
	return raw.toBackend(), nil
}

// UpdateIssueNote edits an existing top-level issue note.
func (b *Backend) UpdateIssueNote(ctx context.Context, repo string, noteID int64, body string) (backend.Note, error) {
	// The repo is needed to encode the project path. The Backend stores
	// the default repo at construction time so callers that do not pass
	// it still get the right path.
	effectiveRepo := strings.TrimSpace(repo)
	if effectiveRepo == "" {
		effectiveRepo = b.repo
	}
	endpoint := fmt.Sprintf("projects/%s/issues/notes/%d", projectPath(effectiveRepo), noteID)
	var raw gitlabNote
	if err := b.runJSON(ctx, &raw,
		"api", "--method", "PUT", endpoint, "-f", "body="+body); err != nil {
		return backend.Note{}, err
	}
	return raw.toBackend(), nil
}

// ReplyIssueDiscussion on an issue routes to CreateIssueNote when
// discussionID is empty (top-level comment) and to the threaded discussion
// endpoint when not. GitLab issue discussions are conceptually weaker than
// MR discussions, so a plain note is the right representation either way.
func (b *Backend) ReplyIssueDiscussion(ctx context.Context, repo string, issueNumber int, discussionID string, body string) (backend.Note, error) {
	if strings.TrimSpace(discussionID) == "" {
		return b.CreateIssueNote(ctx, repo, issueNumber, body)
	}
	endpoint := fmt.Sprintf("projects/%s/issues/%d/discussions/%s/notes",
		projectPath(repo), issueNumber, url.PathEscape(discussionID))
	var raw gitlabNote
	if err := b.runJSON(ctx, &raw,
		"api", "--method", "POST", endpoint, "-f", "body="+body); err != nil {
		return backend.Note{}, err
	}
	return raw.toBackend(), nil
}

// -----------------------------------------------------------------------------
// Merge requests
// -----------------------------------------------------------------------------

// GetMergeRequest fetches a single MR via `glab mr view`.
func (b *Backend) GetMergeRequest(ctx context.Context, repo string, number int) (backend.MergeRequest, error) {
	args := []string{"mr", "view", strconv.Itoa(number), "--repo", repo, "--output", "json"}
	var raw gitlabMergeRequest
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.MergeRequest{}, err
	}
	return raw.toBackend(), nil
}

// CreateMergeRequest opens a new MR via `glab mr create`.
func (b *Backend) CreateMergeRequest(ctx context.Context, repo string, opts backend.CreateMergeRequestOpts) (backend.MergeRequest, error) {
	args := []string{"mr", "create", "--repo", repo}
	if t := strings.TrimSpace(opts.Title); t != "" {
		args = append(args, "--title", t)
	}
	if d := opts.Body; d != "" {
		args = append(args, "--description", d)
	}
	if s := strings.TrimSpace(opts.SourceBranch); s != "" {
		args = append(args, "--source-branch", s)
	}
	if t := strings.TrimSpace(opts.TargetBranch); t != "" {
		args = append(args, "--target-branch", t)
	}
	args = append(args, "--output", "json")

	var raw gitlabMergeRequest
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.MergeRequest{}, err
	}
	return raw.toBackend(), nil
}

// UpdateMergeRequest applies changes to an existing MR. Only the fields
// that were set (non-nil pointer) are sent.
func (b *Backend) UpdateMergeRequest(ctx context.Context, repo string, number int, opts backend.UpdateMergeRequestOpts) (backend.MergeRequest, error) {
	args := []string{"mr", "update", strconv.Itoa(number), "--repo", repo}
	if opts.Title != nil {
		args = append(args, "--title", *opts.Title)
	}
	if opts.Body != nil {
		args = append(args, "--description", *opts.Body)
	}
	if opts.TargetBranch != nil {
		args = append(args, "--target-branch", *opts.TargetBranch)
	}
	// Note: glab mr update does not support --state in every version. We
	// omit it here rather than emit a flag the CLI does not recognise;
	// state changes should go through `glab mr close` / `glab mr reopen`
	// in a follow-up patch.
	args = append(args, "--output", "json")

	var raw gitlabMergeRequest
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.MergeRequest{}, err
	}
	return raw.toBackend(), nil
}

// ListMRFiles returns the diff via the REST API. glab mr diff would also
// work but its output is unified-diff text; the changes endpoint gives us
// per-file patches ready to copy into DiffFile.
func (b *Backend) ListMRFiles(ctx context.Context, repo string, number int) ([]backend.DiffFile, error) {
	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/changes", projectPath(repo), number)
	var raw struct {
		Changes []struct {
			NewPath string `json:"new_path"`
			OldPath string `json:"old_path"`
			Diff    string `json:"diff"`
		} `json:"changes"`
	}
	if err := b.runJSON(ctx, &raw, "api", "--method", "GET", endpoint); err != nil {
		return nil, err
	}
	out := make([]backend.DiffFile, 0, len(raw.Changes))
	for _, c := range raw.Changes {
		path := c.NewPath
		if path == "" {
			path = c.OldPath
		}
		out = append(out, backend.DiffFile{
			Path:  path,
			Patch: c.Diff,
		})
	}
	return out, nil
}

// ListMRDiscussions lists inline + general discussions on an MR.
func (b *Backend) ListMRDiscussions(ctx context.Context, repo string, number int) ([]backend.Discussion, error) {
	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/discussions", projectPath(repo), number)
	var raw []gitlabDiscussion
	if err := b.runJSON(ctx, &raw, "api", "--method", "GET", endpoint); err != nil {
		return nil, err
	}
	out := make([]backend.Discussion, 0, len(raw))
	for i := range raw {
		out = append(out, raw[i].toBackend())
	}
	return out, nil
}

// CreateMRDiscussion posts a new inline or general note. When File/Line
// are both zero the call becomes a general note (no position); otherwise
// glab mr note create posts an inline note.
func (b *Backend) CreateMRDiscussion(ctx context.Context, repo string, number int, opts backend.CreateDiscussionOpts) (backend.Discussion, error) {
	args := []string{"mr", "note", "create", strconv.Itoa(number), "--repo", repo}
	if body := strings.TrimSpace(opts.Body); body != "" {
		args = append(args, "--message", body)
	}
	if file := strings.TrimSpace(opts.File); file != "" {
		args = append(args, "--file", file)
		// glab mr note create accepts --line for the new side and
		// --old-line for the old side. When both are set we send both.
		if opts.Line > 0 {
			args = append(args, "--line", strconv.Itoa(opts.Line))
		}
		if opts.OldLine > 0 {
			args = append(args, "--old-line", strconv.Itoa(opts.OldLine))
		}
	}
	args = append(args, "--output", "json")

	var raw gitlabDiscussion
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.Discussion{}, err
	}
	return raw.toBackend(), nil
}

// ReplyMRDiscussion appends a reply note to an existing MR discussion.
func (b *Backend) ReplyMRDiscussion(ctx context.Context, repo string, number int, discussionID string, body string) (backend.Note, error) {
	args := []string{"mr", "note", "create", strconv.Itoa(number), "--repo", repo}
	if discussionID = strings.TrimSpace(discussionID); discussionID != "" {
		args = append(args, "--reply", discussionID)
	}
	if body = strings.TrimSpace(body); body != "" {
		args = append(args, "--message", body)
	}
	args = append(args, "--output", "json")

	var raw gitlabNote
	if err := b.runJSON(ctx, &raw, args...); err != nil {
		return backend.Note{}, err
	}
	return raw.toBackend(), nil
}

// ResolveMRDiscussion marks a discussion thread as resolved via
// `glab mr note resolve`.
func (b *Backend) ResolveMRDiscussion(ctx context.Context, repo string, number int, discussionID string) error {
	args := []string{"mr", "note", "resolve", strconv.Itoa(number), discussionID, "--repo", repo}
	_, err := b.run(ctx, args...)
	return err
}

// -----------------------------------------------------------------------------
// CI / pipelines
// -----------------------------------------------------------------------------

// GetMRPipelineStatus fetches the head pipeline status via the REST API.
// We use the `/merge_requests/:iid/pipelines` endpoint and pick the most
// recent pipeline (sorted by ID desc by GitLab). This is more reliable
// than `glab ci status` because the latter requires a ref and produces a
// different shape per version.
func (b *Backend) GetMRPipelineStatus(ctx context.Context, repo string, number int) (backend.PipelineStatus, error) {
	// Fetch MR head SHA first so we can disambiguate the most recent
	// pipeline for the MR's branch.
	var mr struct {
		SHA string `json:"sha"`
	}
	mrEndpoint := fmt.Sprintf("projects/%s/merge_requests/%d", projectPath(repo), number)
	if err := b.runJSON(ctx, &mr, "api", "--method", "GET", mrEndpoint); err != nil {
		return backend.PipelineStatus{}, err
	}

	pipesEndpoint := fmt.Sprintf("projects/%s/merge_requests/%d/pipelines", projectPath(repo), number)
	var pipes []struct {
		Status string `json:"status"`
		SHA    string `json:"sha"`
		WebURL string `json:"web_url"`
		ID     int64  `json:"id"`
	}
	if err := b.runJSON(ctx, &pipes, "api", "--method", "GET", pipesEndpoint); err != nil {
		return backend.PipelineStatus{}, err
	}
	// Pick the pipeline whose SHA matches the MR head. If none matches,
	// fall back to the most recently created one.
	var chosen *struct {
		Status string `json:"status"`
		SHA    string `json:"sha"`
		WebURL string `json:"web_url"`
		ID     int64  `json:"id"`
	}
	for i := range pipes {
		if pipes[i].SHA == mr.SHA {
			chosen = &pipes[i]
			break
		}
	}
	if chosen == nil && len(pipes) > 0 {
		chosen = &pipes[0]
	}
	if chosen == nil {
		return backend.PipelineStatus{}, &Error{
			Kind:    ErrorNotFound,
			Message: fmt.Sprintf("glab: no pipeline found for %s!%d", repo, number),
		}
	}
	return backend.PipelineStatus{
		State: chosen.Status,
		SHA:   chosen.SHA,
		URL:   chosen.WebURL,
	}, nil
}

// ListMRPipelineJobs lists the jobs of a pipeline via the REST API.
func (b *Backend) ListMRPipelineJobs(ctx context.Context, repo string, pipelineID int64) ([]backend.PipelineJob, error) {
	endpoint := fmt.Sprintf("projects/%s/pipelines/%d/jobs", projectPath(repo), pipelineID)
	var raw []struct {
		ID         int64     `json:"id"`
		Name       string    `json:"name"`
		Status     string    `json:"status"`
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
	}
	if err := b.runJSON(ctx, &raw, "api", "--method", "GET", endpoint); err != nil {
		return nil, err
	}
	out := make([]backend.PipelineJob, 0, len(raw))
	for _, j := range raw {
		out = append(out, backend.PipelineJob{
			ID:         j.ID,
			Name:       j.Name,
			Status:     j.Status,
			Conclusion: j.Status, // GitLab collapses status+conclusion
			StartedAt:  j.StartedAt,
			FinishedAt: j.FinishedAt,
		})
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Labels
// -----------------------------------------------------------------------------

// CreateLabel creates a project label. If the label already exists the
// implementation returns LabelResult{Skipped: true} without an error, as
// required by the contract.
func (b *Backend) CreateLabel(ctx context.Context, repo, name, color, description string) (backend.LabelResult, error) {
	endpoint := fmt.Sprintf("projects/%s/labels", projectPath(repo))
	args := []string{
		"api", "--method", "POST", endpoint,
		"-f", "name=" + name,
		"-f", "color=" + color,
	}
	if description != "" {
		args = append(args, "-f", "description="+description)
	}
	out, stderr, err := b.runner.Run(ctx, args...)
	if err != nil {
		// GitLab returns 409 Conflict when the label already exists.
		// Treat that as a successful no-op per the contract.
		if isAlreadyExists(stderr) {
			return backend.LabelResult{Name: name, Color: color, Skipped: true}, nil
		}
		return backend.LabelResult{}, ClassifyStderr(stderr, err)
	}
	var raw struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return backend.LabelResult{}, &Error{
			Kind:    ErrorUnknown,
			Message: fmt.Sprintf("glab: decode label response: %s", err),
			RawErr:  err,
		}
	}
	return backend.LabelResult{Name: raw.Name, Color: raw.Color}, nil
}

// -----------------------------------------------------------------------------
// GitLab JSON shapes
// -----------------------------------------------------------------------------

// gitlabAuthor mirrors the nested {"author": {"username": ...}} shape.
type gitlabAuthor struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// gitlabIssue is the glab issue view JSON shape.
type gitlabIssue struct {
	IID         int          `json:"iid"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	State       string       `json:"state"`
	WebURL      string       `json:"web_url"`
	CreatedAt   time.Time    `json:"created_at"`
	Author      gitlabAuthor `json:"author"`
	Labels      []string     `json:"labels"`
}

// toBackend copies the GitLab shape into the platform-neutral Issue,
// normalising "opened" -> "open" because the contract documents "open".
func (g gitlabIssue) toBackend() backend.Issue {
	return backend.Issue{
		Number:    g.IID,
		Title:     g.Title,
		Body:      g.Description,
		State:     normaliseMRState(g.State),
		URL:       g.WebURL,
		CreatedAt: g.CreatedAt,
		Author:    authorName(g.Author),
		Labels:    g.Labels,
	}
}

// gitlabNote is the GitLab note JSON shape (issue notes + MR discussion
// notes share this structure).
type gitlabNote struct {
	ID        int64        `json:"id"`
	Body      string       `json:"body"`
	Author    gitlabAuthor `json:"author"`
	CreatedAt time.Time    `json:"created_at"`
	WebURL    string       `json:"web_url"`
}

func (g gitlabNote) toBackend() backend.Note {
	return backend.Note{
		ID:        g.ID,
		Body:      g.Body,
		Author:    authorName(g.Author),
		CreatedAt: g.CreatedAt,
		URL:       g.WebURL,
	}
}

// gitlabMergeRequest is the glab mr view JSON shape.
type gitlabMergeRequest struct {
	IID          int          `json:"iid"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	State        string       `json:"state"`
	SourceBranch string       `json:"source_branch"`
	TargetBranch string       `json:"target_branch"`
	SHA          string       `json:"sha"`
	WebURL       string       `json:"web_url"`
	CreatedAt    time.Time    `json:"created_at"`
	Author       gitlabAuthor `json:"author"`
}

func (g gitlabMergeRequest) toBackend() backend.MergeRequest {
	return backend.MergeRequest{
		Number:       g.IID,
		Title:        g.Title,
		Body:         g.Description,
		State:        normaliseMRState(g.State),
		SourceBranch: g.SourceBranch,
		TargetBranch: g.TargetBranch,
		HeadSHA:      g.SHA,
		URL:          g.WebURL,
		CreatedAt:    g.CreatedAt,
		Author:       authorName(g.Author),
	}
}

// gitlabDiscussion is the GitLab MR discussion shape (each discussion is
// a thread of notes anchored to a position).
type gitlabDiscussion struct {
	ID         string       `json:"id"`
	Notes      []gitlabNote `json:"notes"`
	Resolved   bool         `json:"resolved"`
	NotesCount int          `json:"notes_count"`
	CommitID   string       `json:"commit_id"`
	Position   *struct {
		NewPath string `json:"new_path"`
		NewLine int    `json:"new_line"`
		OldLine int    `json:"old_line"`
		OldPath string `json:"old_path"`
	} `json:"position"`
}

func (g gitlabDiscussion) toBackend() backend.Discussion {
	d := backend.Discussion{
		ID:       g.ID,
		Resolved: g.Resolved,
		CommitID: g.CommitID,
		Notes:    make([]backend.Note, 0, len(g.Notes)),
	}
	for i := range g.Notes {
		d.Notes = append(d.Notes, g.Notes[i].toBackend())
	}
	if g.Position != nil {
		d.File = g.Position.NewPath
		if d.File == "" {
			d.File = g.Position.OldPath
		}
		d.Line = g.Position.NewLine
		d.OldLine = g.Position.OldLine
	}
	return d
}

// normaliseMRState maps GitLab's "opened" to the platform-neutral "open"
// used by the Backend contract. "closed" and "merged" pass through.
func normaliseMRState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "opened":
		return "open"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

// authorName picks the most user-friendly of username vs. display name.
// GitLab returns both; we prefer username because it is the unique handle.
func authorName(a gitlabAuthor) string {
	if a.Username != "" {
		return a.Username
	}
	return a.Name
}

// -----------------------------------------------------------------------------
// helpers (compile-time-only): error wrapping consistency
// -----------------------------------------------------------------------------

// wrap is reserved for future use to keep error wrapping centralised.
var _ = fmt.Errorf
var _ = errors.New
