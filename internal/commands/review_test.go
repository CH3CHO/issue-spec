package commands

import (
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
	if report.ActionableFindings[0].Severity != "P1" {
		t.Fatalf("severity = %s", report.ActionableFindings[0].Severity)
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
