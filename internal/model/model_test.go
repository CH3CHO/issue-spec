package model

import (
	"strings"
	"testing"
)

func TestEnsureTypedBodyAddsMarkerAndHeader(t *testing.T) {
	body, err := EnsureTypedBody("SPEC", "SPEC-001", "## Requirement: X\n\nX MUST work.", BodyOptions{Agent: "Coordinator", Status: "confirmed", Scope: "workflow"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "<!-- issue-spec:type=SPEC id=SPEC-001 version=1 -->") {
		t.Fatalf("missing marker:\n%s", body)
	}
	tc := ParseTypedComment(body)
	if len(tc.Errors) > 0 {
		t.Fatalf("unexpected parse errors: %v", tc.Errors)
	}
	if tc.Type != "SPEC" || tc.ID != "SPEC-001" || tc.Status != "confirmed" || tc.Scope != "workflow" {
		t.Fatalf("unexpected typed comment: %+v", tc)
	}
}

func TestAddRelatedCommentLinkIsIdempotent(t *testing.T) {
	body, err := EnsureTypedBody("TASK", "TASK-001", "## Task\n\n- [ ] 1. Test", BodyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	updated, changed, err := AddRelatedCommentLink(body, "https://github.com/o/r/issues/1#issuecomment-1")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first link should change body")
	}
	updatedAgain, changed, err := AddRelatedCommentLink(updated, "https://github.com/o/r/issues/1#issuecomment-1")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second link should be idempotent")
	}
	if updatedAgain != updated {
		t.Fatal("idempotent update changed body")
	}
}

func TestAddPRLinkIsIdempotent(t *testing.T) {
	body, err := EnsureTypedBody("PROCESS", "PROCESS-001", "## Process\n\nDone.", BodyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	updated, changed, err := AddPRLink(body, "https://github.com/o/r/pull/4")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first PR link should change body")
	}
	updatedAgain, changed, err := AddPRLink(updated, "https://github.com/o/r/pull/4")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second PR link should be idempotent")
	}
	if updatedAgain != updated {
		t.Fatal("idempotent PR update changed body")
	}
}

func TestVerifyTraceabilityRequiresBacklinks(t *testing.T) {
	specBody, _ := EnsureTypedBody("SPEC", "SPEC-001", "## Requirement: X\n\nX MUST work.\n\n### Scenario: ok\n\n- **WHEN** x\n- **THEN** y", BodyOptions{})
	taskBody, _ := EnsureTypedBody("TASK", "TASK-001", "## Task\n\n- [ ] 1. Test", BodyOptions{})
	taskBody, _, _ = AddRelatedCommentLink(taskBody, "https://github.com/o/r/issues/1#issuecomment-1")

	report := VerifyTraceability([]Artifact{
		{URL: "https://github.com/o/r/issues/1#issuecomment-1", Comment: ParseTypedComment(specBody)},
		{URL: "https://github.com/o/r/issues/2#issuecomment-2", Comment: ParseTypedComment(taskBody)},
	})
	if report.OK {
		t.Fatal("expected missing SPEC backlink to fail")
	}

	specBody, _, _ = AddRelatedCommentLink(specBody, "https://github.com/o/r/issues/2#issuecomment-2")
	report = VerifyTraceability([]Artifact{
		{URL: "https://github.com/o/r/issues/1#issuecomment-1", Comment: ParseTypedComment(specBody)},
		{URL: "https://github.com/o/r/issues/2#issuecomment-2", Comment: ParseTypedComment(taskBody)},
	})
	if !report.OK {
		t.Fatalf("expected traceability OK, got %v", report.Errors)
	}
}

func TestSetStatusAndAppendResolution(t *testing.T) {
	body, err := EnsureTypedBody("QUESTION", "QUESTION-001", "## Question\n\nUse confirmed?\n\n## Resolution Log\n\n- Pending.", BodyOptions{Status: "blocked"})
	if err != nil {
		t.Fatal(err)
	}
	body, err = SetTypedCommentStatus(body, "confirmed")
	if err != nil {
		t.Fatal(err)
	}
	body = AppendResolutionLog(body, "Use confirmed as the default resolved status.")
	tc := ParseTypedComment(body)
	if tc.Status != "confirmed" {
		t.Fatalf("status = %s", tc.Status)
	}
	if !strings.Contains(body, "Use confirmed as the default resolved status.") {
		t.Fatalf("missing resolution log:\n%s", body)
	}
}
