package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/higress-group/issue-spec/internal/github"
	"github.com/higress-group/issue-spec/internal/model"
)

func TestBuildReviewSyncReportClassifiesRationaleFindingsAndChecks(t *testing.T) {
	rationale, err := model.RenderRationaleBody("Worker", "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "why", "a.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	report := buildReviewSyncReport(github.PullRequest{Number: 4, HTMLURL: "https://github.com/o/r/pull/4"}, []github.PullRequestReviewComment{
		{ID: 1, Body: rationale, Path: "a.go", Line: 10},
		{ID: 2, Body: "P1: fix this", Path: "b.go", Line: 20, HTMLURL: "https://github.com/o/r/pull/4#discussion_r2"},
	}, []github.Comment{{ID: 3}}, github.CombinedStatus{Statuses: []github.Status{
		{Context: "license/cla", State: "success"},
		{Context: "ci/test", State: "failure"},
	}}, []github.CheckRun{
		{Name: "DCO", Status: "completed", Conclusion: "success"},
		{Name: "build", Status: "queued"},
	})
	if report.OK {
		t.Fatal("finding and failed/pending checks should block review sync")
	}
	if report.RationaleComments != 1 || len(report.ActionableFindings) != 1 || len(report.FailedChecks) != 1 || len(report.PendingChecks) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.BlockingFindings) != 1 {
		t.Fatalf("expected one blocking finding: %+v", report.BlockingFindings)
	}
	if report.ActionableFindings[0].Severity != "P1" {
		t.Fatalf("severity = %s", report.ActionableFindings[0].Severity)
	}
}

func TestBuildReviewSyncReportP2FindingDoesNotBlock(t *testing.T) {
	report := buildReviewSyncReport(github.PullRequest{Number: 4, HTMLURL: "https://github.com/o/r/pull/4"}, []github.PullRequestReviewComment{
		{ID: 2, Body: "P2: polish this before follow-up", Path: "b.go", Line: 20, HTMLURL: "https://github.com/o/r/pull/4#discussion_r2"},
	}, nil, github.CombinedStatus{}, nil)
	if !report.OK {
		t.Fatalf("P2-only findings should not block review sync: %+v", report)
	}
	if len(report.ActionableFindings) != 1 || len(report.BlockingFindings) != 0 {
		t.Fatalf("unexpected finding classification: %+v", report)
	}
}

func TestBuildReviewSyncReportResolvedFindingReply(t *testing.T) {
	finding, err := model.RenderFindingBody("Review", "FINDING-001", "P1", "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Fix this before merge.", "open", "b.go", 20)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := model.RenderFindingReplyBody("Worker", "FINDING-001", "PROCESS-001", "resolved", "Fixed in the latest patch.")
	if err != nil {
		t.Fatal(err)
	}
	report := buildReviewSyncReport(github.PullRequest{Number: 4, HTMLURL: "https://github.com/o/r/pull/4"}, []github.PullRequestReviewComment{
		{ID: 2, Body: finding, Path: "b.go", Line: 20, HTMLURL: "https://github.com/o/r/pull/4#discussion_r2"},
		{ID: 3, InReplyToID: 2, Body: reply, Path: "b.go", Line: 20, HTMLURL: "https://github.com/o/r/pull/4#discussion_r3"},
	}, nil, github.CombinedStatus{}, nil)
	if !report.OK {
		t.Fatalf("resolved finding should not block review sync: %+v", report)
	}
	if len(report.ActionableFindings) != 0 || len(report.BlockingFindings) != 0 || len(report.ResolvedFindings) != 1 {
		t.Fatalf("unexpected finding classification: %+v", report)
	}
}

func TestRenderReviewSyncComment(t *testing.T) {
	body, err := renderReviewSyncComment("REVIEW-001", "Coordinator", "pr-review", "https://github.com/o/r/pull/4", reviewSyncReport{
		OK:                true,
		PR:                4,
		PRURL:             "https://github.com/o/r/pull/4",
		RationaleComments: 2,
		PassedChecks:      []reviewCheck{{Name: "DCO", State: "completed", Conclusion: "success"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Type: REVIEW", "ID: REVIEW-001", "Status: done", "Review sync passed", "DCO"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestCreateReviewFindingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := &fakeReviewClient{
		files: []github.PullRequestFile{{
			Filename: "internal/foo.go",
			Patch: `@@ -1,2 +1,3 @@
 package foo
+var X = 1
`,
		}},
		pr: github.PullRequest{Number: 7},
	}
	client.pr.Head.SHA = "abc123"
	result, err := createReviewFinding(ctx, client, "o/r", 7, "internal/foo.go", 2, "FINDING-001", "P1", "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Review Agent", "Fix this.")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.CommentID == 0 || result.Severity != "P1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	result, err = createReviewFinding(ctx, client, "o/r", 7, "internal/foo.go", 2, "FINDING-001", "P1", "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Review Agent", "Fix this.")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatalf("expected idempotent existing result: %+v", result)
	}
}

func TestReplyReviewFindingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := &fakeReviewClient{
		comments: []github.PullRequestReviewComment{{
			ID:      10,
			HTMLURL: "https://github.com/o/r/pull/7#discussion_r10",
			Body:    "P1: fix this",
			Path:    "internal/foo.go",
			Line:    2,
		}},
	}
	result, err := replyReviewFinding(ctx, client, "o/r", 7, 10, "FINDING-001", "PROCESS-001", "resolved", "Worker Agent", "Fixed.")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.CommentID == 0 || result.ParentCommentID != 10 {
		t.Fatalf("unexpected result: %+v", result)
	}
	result, err = replyReviewFinding(ctx, client, "o/r", 7, 10, "FINDING-001", "PROCESS-001", "resolved", "Worker Agent", "Fixed.")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatalf("expected idempotent existing reply: %+v", result)
	}
}

type fakeReviewClient struct {
	pr       github.PullRequest
	files    []github.PullRequestFile
	comments []github.PullRequestReviewComment
}

func (f *fakeReviewClient) GetPullRequest(context.Context, string, int) (github.PullRequest, error) {
	return f.pr, nil
}

func (f *fakeReviewClient) ListPullRequestFiles(context.Context, string, int) ([]github.PullRequestFile, error) {
	return f.files, nil
}

func (f *fakeReviewClient) ListPullRequestReviewComments(context.Context, string, int) ([]github.PullRequestReviewComment, error) {
	return f.comments, nil
}

func (f *fakeReviewClient) CreatePullRequestReviewComment(_ context.Context, _ string, _ int, body, commitID, path string, line int, side string) (github.PullRequestReviewComment, error) {
	if commitID == "" || path == "" || line == 0 || side != "RIGHT" {
		panic("invalid create review comment args")
	}
	marker, ok, err := model.FindFindingMarker(body)
	if err != nil || !ok || marker.ID != "FINDING-001" || marker.Severity != "P1" {
		panic("missing finding marker")
	}
	comment := github.PullRequestReviewComment{
		ID:       int64(len(f.comments) + 1),
		HTMLURL:  "https://github.com/o/r/pull/7#discussion_r1",
		Body:     body,
		Path:     path,
		Line:     line,
		CommitID: commitID,
	}
	f.comments = append(f.comments, comment)
	return comment, nil
}

func (f *fakeReviewClient) ReplyPullRequestReviewComment(_ context.Context, _ string, prNumber int, parentCommentID int64, body string) (github.PullRequestReviewComment, error) {
	if prNumber != 7 {
		panic("invalid pull request number")
	}
	marker, ok, err := model.FindFindingReplyMarker(body)
	if err != nil || !ok || marker.Finding != "FINDING-001" || marker.Status != "resolved" {
		panic("missing finding reply marker")
	}
	comment := github.PullRequestReviewComment{
		ID:          int64(len(f.comments) + 1),
		InReplyToID: parentCommentID,
		HTMLURL:     "https://github.com/o/r/pull/7#discussion_r2",
		Body:        body,
	}
	f.comments = append(f.comments, comment)
	return comment, nil
}
