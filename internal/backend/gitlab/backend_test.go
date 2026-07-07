package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/issue-spec/internal/backend"
)

// Compile-time guard: Backend must satisfy the platform-neutral interface.
// If a new method is added without an implementation, this file fails to
// compile, surfacing the drift immediately at CI time.
var _ backend.Backend = (*Backend)(nil)

// fakeRunner is the unit-test replacement for ExecRunner. It returns
// successive responses in FIFO order so a test that expects N glab calls
// queues N responses. Each response records the args it saw so tests can
// assert the Backend passed the right CLI flags.
type fakeRunner struct {
	responses []fakeResponse
	calls     [][]string
}

type fakeResponse struct {
	stdout []byte
	stderr []byte
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, append([]string{}, args...))
	if len(f.responses) == 0 {
		return nil, nil, errors.New("fakeRunner: no response queued")
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return r.stdout, r.stderr, r.err
}

// newBackend builds a Backend wired to a fakeRunner. repo and token are
// the typical inputs; tests tweak the runner per case.
func newBackend(repo, token string) (*Backend, *fakeRunner) {
	r := &fakeRunner{}
	b := New(repo, token, r)
	return b, r
}

// -----------------------------------------------------------------------------
// Issues
// -----------------------------------------------------------------------------

func TestBackendCreateIssue(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"iid": 42, "id": 999, "web_url": "https://gitlab.com/o/r/-/issues/42"}`),
	}}
	got, err := b.CreateIssue(context.Background(), "o/r", backend.CreateIssueOpts{
		Title:  "hello",
		Body:   "world",
		Labels: []string{"bug", "p1"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if got.Number != 42 {
		t.Errorf("Number = %d, want 42", got.Number)
	}
	if got.ID != 999 {
		t.Errorf("ID = %d, want 999", got.ID)
	}
	if got.URL == "" {
		t.Errorf("URL is empty")
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 glab call, got %d: %v", len(r.calls), r.calls)
	}
	args := r.calls[0]
	if args[0] != "issue" || args[1] != "create" {
		t.Errorf("first args = %v, want issue create", args[:2])
	}
	if !containsArg(args, "--repo") || !containsArgValue(args, "--repo", "o/r") {
		t.Errorf("missing --repo o/r: %v", args)
	}
	if !containsArgValue(args, "--title", "hello") {
		t.Errorf("missing --title hello: %v", args)
	}
	if !containsArgValue(args, "--description", "world") {
		t.Errorf("missing --description world: %v", args)
	}
	// labels
	if !containsArgValue(args, "--label", "bug") || !containsArgValue(args, "--label", "p1") {
		t.Errorf("missing labels: %v", args)
	}
}

func TestBackendGetIssue(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"iid": 7,
			"title": "fix",
			"description": "stuff",
			"state": "opened",
			"web_url": "https://gitlab.com/o/r/-/issues/7",
			"created_at": "2026-07-01T10:00:00Z",
			"author": {"username": "alice"},
			"labels": ["bug", "p1"]
		}`),
	}}
	got, err := b.GetIssue(context.Background(), "o/r", 7)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Number != 7 {
		t.Errorf("Number = %d, want 7", got.Number)
	}
	if got.Title != "fix" || got.Body != "stuff" || got.State != "open" {
		t.Errorf("unexpected issue fields: %+v", got)
	}
	if got.URL == "" || got.Author != "alice" {
		t.Errorf("missing url/author: %+v", got)
	}
	if len(got.Labels) != 2 {
		t.Errorf("Labels = %v, want 2 entries", got.Labels)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--repo", "o/r") {
		t.Errorf("missing --repo o/r: %v", args)
	}
	// glab uses "issue view <iid>"
	wantIIDPos := -1
	for i, a := range args {
		if a == "view" && i+1 < len(args) {
			if args[i+1] == "7" {
				wantIIDPos = i
				break
			}
		}
	}
	if wantIIDPos == -1 {
		t.Errorf("expected positional arg '7' after 'view', got %v", args)
	}
}

func TestBackendListIssueNotes(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`[
			{"id": 11, "body": "first", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/issues/7#note_11"},
			{"id": 12, "body": "second", "author": {"username": "bob"}, "created_at": "2026-07-02T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/issues/7#note_12"}
		]`),
	}}
	notes, err := b.ListIssueNotes(context.Background(), "o/r", 7)
	if err != nil {
		t.Fatalf("ListIssueNotes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes count = %d, want 2", len(notes))
	}
	if notes[0].ID != 11 || notes[0].Author != "alice" {
		t.Errorf("note 0 wrong: %+v", notes[0])
	}
	if notes[1].Body != "second" {
		t.Errorf("note 1 wrong: %+v", notes[1])
	}
	// The endpoint must use the URL-encoded project path.
	if !strings.Contains(strings.Join(r.calls[0], " "), "projects/o%2Fr/issues/7/notes") {
		t.Errorf("expected encoded projects/o%%2Fr/issues/7/notes path in args, got %v", r.calls[0])
	}
}

func TestBackendCreateIssueNote(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"id": 99, "body": "thanks", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/issues/7#note_99"}`),
	}}
	got, err := b.CreateIssueNote(context.Background(), "o/r", 7, "thanks")
	if err != nil {
		t.Fatalf("CreateIssueNote: %v", err)
	}
	if got.ID != 99 || got.Body != "thanks" || got.Author != "alice" {
		t.Errorf("note = %+v", got)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--method", "POST") {
		t.Errorf("missing POST: %v", args)
	}
	if !containsArgValue(args, "-f", "body=thanks") {
		t.Errorf("missing body= field: %v", args)
	}
}

func TestBackendUpdateIssueNote(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"id": 99, "body": "edited", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/issues/7#note_99"}`),
	}}
	got, err := b.UpdateIssueNote(context.Background(), "o/r", 99, "edited")
	if err != nil {
		t.Fatalf("UpdateIssueNote: %v", err)
	}
	if got.Body != "edited" {
		t.Errorf("Body = %q, want edited", got.Body)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--method", "PUT") {
		t.Errorf("missing PUT: %v", args)
	}
	if !containsArgValue(args, "-f", "body=edited") {
		t.Errorf("missing body=edited: %v", args)
	}
	if !containsArg(args, "projects/o%2Fr/issues/notes/99") {
		t.Errorf("missing encoded note path: %v", args)
	}
}

func TestBackendReplyIssueDiscussion(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"id": 100, "body": "reply", "author": {"username": "bob"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/issues/7#note_100"}`),
	}}
	// With an empty discussionID the implementation should route the body as
	// a new top-level note (CreateIssueNote semantics).
	got, err := b.ReplyIssueDiscussion(context.Background(), "o/r", 7, "", "reply")
	if err != nil {
		t.Fatalf("ReplyIssueDiscussion: %v", err)
	}
	if got.ID != 100 {
		t.Errorf("ID = %d, want 100", got.ID)
	}
}

// -----------------------------------------------------------------------------
// Merge requests
// -----------------------------------------------------------------------------

func TestBackendGetMergeRequest(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"iid": 5,
			"title": "feat",
			"description": "body",
			"state": "opened",
			"source_branch": "feat/x",
			"target_branch": "main",
			"sha": "abc123",
			"web_url": "https://gitlab.com/o/r/-/merge_requests/5",
			"created_at": "2026-07-01T10:00:00Z",
			"author": {"username": "alice"}
		}`),
	}}
	mr, err := b.GetMergeRequest(context.Background(), "o/r", 5)
	if err != nil {
		t.Fatalf("GetMergeRequest: %v", err)
	}
	if mr.Number != 5 || mr.Title != "feat" || mr.Body != "body" {
		t.Errorf("unexpected mr: %+v", mr)
	}
	if mr.State != "open" {
		t.Errorf("State = %q, want open", mr.State)
	}
	if mr.SourceBranch != "feat/x" || mr.TargetBranch != "main" {
		t.Errorf("branches wrong: %+v", mr)
	}
	if mr.HeadSHA != "abc123" || mr.Author != "alice" {
		t.Errorf("sha/author wrong: %+v", mr)
	}
}

func TestBackendCreateMergeRequest(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"iid": 6,
			"title": "feat",
			"description": "body",
			"state": "opened",
			"source_branch": "feat/x",
			"target_branch": "main",
			"sha": "def456",
			"web_url": "https://gitlab.com/o/r/-/merge_requests/6",
			"created_at": "2026-07-01T10:00:00Z",
			"author": {"username": "alice"}
		}`),
	}}
	mr, err := b.CreateMergeRequest(context.Background(), "o/r", backend.CreateMergeRequestOpts{
		Title:        "feat",
		Body:         "body",
		SourceBranch: "feat/x",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateMergeRequest: %v", err)
	}
	if mr.Number != 6 {
		t.Errorf("Number = %d, want 6", mr.Number)
	}
	args := r.calls[0]
	if args[0] != "mr" || args[1] != "create" {
		t.Errorf("expected mr create, got %v", args[:2])
	}
	if !containsArgValue(args, "--title", "feat") {
		t.Errorf("missing --title feat: %v", args)
	}
	if !containsArgValue(args, "--description", "body") {
		t.Errorf("missing --description body: %v", args)
	}
	if !containsArgValue(args, "--source-branch", "feat/x") {
		t.Errorf("missing --source-branch: %v", args)
	}
	if !containsArgValue(args, "--target-branch", "main") {
		t.Errorf("missing --target-branch: %v", args)
	}
}

func TestBackendUpdateMergeRequest(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"iid": 6,
			"title": "feat (edited)",
			"description": "body",
			"state": "opened",
			"source_branch": "feat/x",
			"target_branch": "main",
			"sha": "def456",
			"web_url": "https://gitlab.com/o/r/-/merge_requests/6",
			"created_at": "2026-07-01T10:00:00Z",
			"author": {"username": "alice"}
		}`),
	}}
	title := "feat (edited)"
	mr, err := b.UpdateMergeRequest(context.Background(), "o/r", 6, backend.UpdateMergeRequestOpts{
		Title: &title,
	})
	if err != nil {
		t.Fatalf("UpdateMergeRequest: %v", err)
	}
	if mr.Title != "feat (edited)" {
		t.Errorf("Title = %q", mr.Title)
	}
	args := r.calls[0]
	if args[0] != "mr" || args[1] != "update" {
		t.Errorf("expected mr update, got %v", args[:2])
	}
	if !containsArgValue(args, "--title", "feat (edited)") {
		t.Errorf("missing --title: %v", args)
	}
}

func TestBackendListMRFiles(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	// GitLab's /merge_requests/:iid/changes returns {"changes": [...]}.
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"changes": [
				{"new_path": "a.go", "diff": "@@ -1 +1 @@\n-old\n+new\n"},
				{"new_path": "b.go", "diff": "@@ -10 +10 @@\n-x\n+y\n"}
			]
		}`),
	}}
	files, err := b.ListMRFiles(context.Background(), "o/r", 5)
	if err != nil {
		t.Fatalf("ListMRFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files count = %d, want 2", len(files))
	}
	if files[0].Path != "a.go" || !strings.Contains(files[0].Patch, "+new") {
		t.Errorf("file 0 = %+v", files[0])
	}
	if files[1].Path != "b.go" {
		t.Errorf("file 1 = %+v", files[1])
	}
}

func TestBackendListMRDiscussions(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`[
			{
				"id": "d1",
				"notes": [
					{"id": 1, "body": "first", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/merge_requests/5#note_1"}
				],
				"resolved": false,
				"position": {"new_path": "a.go", "new_line": 10, "old_line": 9},
				"commit_id": "abc123"
			}
		]`),
	}}
	discussions, err := b.ListMRDiscussions(context.Background(), "o/r", 5)
	if err != nil {
		t.Fatalf("ListMRDiscussions: %v", err)
	}
	if len(discussions) != 1 {
		t.Fatalf("discussions = %d, want 1", len(discussions))
	}
	d := discussions[0]
	if d.ID != "d1" || d.Resolved {
		t.Errorf("id/resolved wrong: %+v", d)
	}
	if d.File != "a.go" || d.Line != 10 || d.OldLine != 9 {
		t.Errorf("position wrong: %+v", d)
	}
	if d.CommitID != "abc123" || len(d.Notes) != 1 || d.Notes[0].Body != "first" {
		t.Errorf("notes/commit wrong: %+v", d)
	}
}

func TestBackendCreateMRDiscussion(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"id": "d2",
			"notes": [
				{"id": 2, "body": "inline", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/merge_requests/5#note_2"}
			],
			"resolved": false,
			"position": {"new_path": "a.go", "new_line": 12, "old_line": 11},
			"commit_id": "abc123"
		}`),
	}}
	got, err := b.CreateMRDiscussion(context.Background(), "o/r", 5, backend.CreateDiscussionOpts{
		Body: "inline",
		File: "a.go",
		Line: 12,
	})
	if err != nil {
		t.Fatalf("CreateMRDiscussion: %v", err)
	}
	if got.ID != "d2" || got.File != "a.go" || got.Line != 12 {
		t.Errorf("unexpected discussion: %+v", got)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--file", "a.go") {
		t.Errorf("missing --file: %v", args)
	}
	if !containsArgValue(args, "--line", "12") {
		t.Errorf("missing --line 12: %v", args)
	}
}

func TestBackendCreateMRDiscussionGeneralComment(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"id": "d3",
			"notes": [
				{"id": 3, "body": "general", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/merge_requests/5#note_3"}
			],
			"resolved": false
		}`),
	}}
	got, err := b.CreateMRDiscussion(context.Background(), "o/r", 5, backend.CreateDiscussionOpts{
		Body: "general",
	})
	if err != nil {
		t.Fatalf("CreateMRDiscussion: %v", err)
	}
	if got.ID != "d3" || got.File != "" {
		t.Errorf("unexpected: %+v", got)
	}
	args := r.calls[0]
	// General comments must NOT carry --file/--line flags.
	if containsArg(args, "--file") || containsArg(args, "--line") {
		t.Errorf("general comment leaked position flags: %v", args)
	}
}

func TestBackendReplyMRDiscussion(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"id": 4, "body": "reply", "author": {"username": "alice"}, "created_at": "2026-07-01T10:00:00Z", "web_url": "https://gitlab.com/o/r/-/merge_requests/5#note_4"}`),
	}}
	got, err := b.ReplyMRDiscussion(context.Background(), "o/r", 5, "d1", "reply")
	if err != nil {
		t.Fatalf("ReplyMRDiscussion: %v", err)
	}
	if got.Body != "reply" {
		t.Errorf("body = %q", got.Body)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--reply", "d1") {
		t.Errorf("missing --reply d1: %v", args)
	}
}

func TestBackendResolveMRDiscussion(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{stdout: []byte("")}}
	if err := b.ResolveMRDiscussion(context.Background(), "o/r", 5, "d1"); err != nil {
		t.Fatalf("ResolveMRDiscussion: %v", err)
	}
	args := r.calls[0]
	if !containsArg(args, "resolve") {
		t.Errorf("missing resolve subcommand: %v", args)
	}
	// The discussion ID must be present in the args list.
	foundID := false
	for _, a := range args {
		if a == "d1" {
			foundID = true
			break
		}
	}
	if !foundID {
		t.Errorf("discussion ID 'd1' missing from args: %v", args)
	}
}

// -----------------------------------------------------------------------------
// CI / pipelines
// -----------------------------------------------------------------------------

func TestBackendGetMRPipelineStatus(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	// Backend calls the MR endpoint first (to get head SHA) then the
	// pipelines endpoint. Queue two responses.
	r.responses = []fakeResponse{
		{stdout: []byte(`{"sha": "abc123"}`)},
		{stdout: []byte(`[{"status": "success", "sha": "abc123", "web_url": "https://gitlab.com/o/r/-/pipelines/1", "id": 1}]`)},
	}
	got, err := b.GetMRPipelineStatus(context.Background(), "o/r", 5)
	if err != nil {
		t.Fatalf("GetMRPipelineStatus: %v", err)
	}
	if got.State != "success" || got.SHA != "abc123" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestBackendListMRPipelineJobs(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`[
			{"id": 100, "name": "build", "status": "success", "started_at": "2026-07-01T10:00:00Z", "finished_at": "2026-07-01T10:05:00Z"},
			{"id": 101, "name": "test",  "status": "failed",  "started_at": "2026-07-01T10:00:00Z", "finished_at": "2026-07-01T10:06:00Z", "failure_reason": "script_failure"}
		]`),
	}}
	jobs, err := b.ListMRPipelineJobs(context.Background(), "o/r", 42)
	if err != nil {
		t.Fatalf("ListMRPipelineJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	if jobs[0].Name != "build" || jobs[0].Conclusion != "success" {
		t.Errorf("job 0 = %+v", jobs[0])
	}
	if jobs[1].Conclusion != "failed" {
		t.Errorf("job 1 = %+v", jobs[1])
	}
	// Timestamps should round-trip.
	if jobs[0].StartedAt.IsZero() {
		t.Errorf("started_at not parsed: %+v", jobs[0])
	}
}

// -----------------------------------------------------------------------------
// Labels
// -----------------------------------------------------------------------------

func TestBackendCreateLabelNew(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"name": "bug", "color": "#FF0000"}`),
	}}
	got, err := b.CreateLabel(context.Background(), "o/r", "bug", "#FF0000", "things")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if got.Skipped {
		t.Errorf("Skipped = true, want false on new label")
	}
	if got.Name != "bug" || got.Color != "#FF0000" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestBackendCreateLabelAlreadyExists(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	// Simulate GitLab's "already exists" 409 response.
	r.responses = []fakeResponse{{
		stderr: []byte("409 Conflict - Label 'bug' already exists"),
		err:    errors.New("exit status 1"),
	}}
	got, err := b.CreateLabel(context.Background(), "o/r", "bug", "#FF0000", "things")
	if err != nil {
		t.Fatalf("CreateLabel must swallow 409, got error: %v", err)
	}
	if !got.Skipped {
		t.Errorf("Skipped = false, want true for already-exists")
	}
	if got.Name != "bug" {
		t.Errorf("Name = %q, want bug", got.Name)
	}
}

// -----------------------------------------------------------------------------
// Error surface
// -----------------------------------------------------------------------------

func TestBackendCreateIssueReportsClassifiedError(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	r.responses = []fakeResponse{{
		stderr: []byte("403 Forbidden - insufficient_scope"),
		err:    errors.New("exit status 1"),
	}}
	_, err := b.CreateIssue(context.Background(), "o/r", backend.CreateIssueOpts{Title: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if gerr.Kind != ErrorPermission {
		t.Errorf("Kind = %v, want ErrorPermission", gerr.Kind)
	}
}

func TestBackendUsesRepoFromConstructor(t *testing.T) {
	// The constructor takes a repo; calls without explicit --repo should
	// default to that repo so commands that pass only "o/r" via ctx do not
	// need to repeat it.
	b, r := newBackend("group/sub/proj", "glpat-x")
	r.responses = []fakeResponse{{
		stdout: []byte(`{"iid": 1, "id": 1, "web_url": ""}`),
	}}
	if _, err := b.GetIssue(context.Background(), "group/sub/proj", 1); err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	args := r.calls[0]
	if !containsArgValue(args, "--repo", "group/sub/proj") {
		t.Errorf("missing --repo group/sub/proj: %v", args)
	}
}

func TestBackendCreatedAtRoundTrip(t *testing.T) {
	b, r := newBackend("o/r", "glpat-x")
	want := "2026-07-01T10:00:00Z"
	r.responses = []fakeResponse{{
		stdout: []byte(`{
			"iid": 1,
			"title": "t",
			"description": "b",
			"state": "opened",
			"web_url": "",
			"created_at": "` + want + `",
			"author": {"username": "alice"}
		}`),
	}}
	got, err := b.GetIssue(context.Background(), "o/r", 1)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.CreatedAt.Format(time.RFC3339) != want {
		t.Errorf("CreatedAt = %v, want %s", got.CreatedAt, want)
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// containsArgValue returns true when --flag VALUE pair exists in args.
// For "--flag=value" single-arg forms, also accepted.
func containsArgValue(args []string, flag, value string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return true
		}
		if strings.HasPrefix(a, flag+"=") && a == flag+"="+value {
			return true
		}
	}
	return false
}
