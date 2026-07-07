package backend

import (
	"context"
	"testing"
)

// TestBackendContractForbidsExtraFields asserts that the Backend interface
// stays minimal and platform-neutral; the stub below returns zero values for
// every method, so the compile-time assertion verifies both that every
// method on Backend is satisfied and that no platform implementation can
// sneak in a platform-specific method (any new method would have to be
// implemented here, which is exactly what we want reviewers to see).
func TestBackendContractForbidsExtraFields(t *testing.T) {
	var _ Backend = (*stubBackend)(nil)
}

// stubBackend is the zero-value test double for the Backend interface. It
// returns the documented zero value of each return type and never returns a
// non-nil error. Tests for individual platform implementations live in
// internal/github and (later) internal/gitlab; this stub exists solely to
// pin the interface surface.
type stubBackend struct{}

func (*stubBackend) CreateIssue(_ context.Context, _ string, _ CreateIssueOpts) (CreateIssueResult, error) {
	return CreateIssueResult{}, nil
}

func (*stubBackend) GetIssue(_ context.Context, _ string, _ int) (Issue, error) {
	return Issue{}, nil
}

func (*stubBackend) ListIssueNotes(_ context.Context, _ string, _ int) ([]Note, error) {
	return nil, nil
}

func (*stubBackend) CreateIssueNote(_ context.Context, _ string, _ int, _ string) (Note, error) {
	return Note{}, nil
}

func (*stubBackend) UpdateIssueNote(_ context.Context, _ string, _ int64, _ string) (Note, error) {
	return Note{}, nil
}

func (*stubBackend) ReplyIssueDiscussion(_ context.Context, _ string, _ int, _ string, _ string) (Note, error) {
	return Note{}, nil
}

func (*stubBackend) GetMergeRequest(_ context.Context, _ string, _ int) (MergeRequest, error) {
	return MergeRequest{}, nil
}

func (*stubBackend) CreateMergeRequest(_ context.Context, _ string, _ CreateMergeRequestOpts) (MergeRequest, error) {
	return MergeRequest{}, nil
}

func (*stubBackend) UpdateMergeRequest(_ context.Context, _ string, _ int, _ UpdateMergeRequestOpts) (MergeRequest, error) {
	return MergeRequest{}, nil
}

func (*stubBackend) ListMRFiles(_ context.Context, _ string, _ int) ([]DiffFile, error) {
	return nil, nil
}

func (*stubBackend) ListMRDiscussions(_ context.Context, _ string, _ int) ([]Discussion, error) {
	return nil, nil
}

func (*stubBackend) CreateMRDiscussion(_ context.Context, _ string, _ int, _ CreateDiscussionOpts) (Discussion, error) {
	return Discussion{}, nil
}

func (*stubBackend) ReplyMRDiscussion(_ context.Context, _ string, _ int, _ string, _ string) (Note, error) {
	return Note{}, nil
}

func (*stubBackend) ResolveMRDiscussion(_ context.Context, _ string, _ int, _ string) error {
	return nil
}

func (*stubBackend) GetMRPipelineStatus(_ context.Context, _ string, _ int) (PipelineStatus, error) {
	return PipelineStatus{}, nil
}

func (*stubBackend) ListMRPipelineJobs(_ context.Context, _ string, _ int64) ([]PipelineJob, error) {
	return nil, nil
}

func (*stubBackend) CreateLabel(_ context.Context, _ string, _ string, _ string, _ string) (LabelResult, error) {
	return LabelResult{}, nil
}
