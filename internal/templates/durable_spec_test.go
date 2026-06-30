package templates

import (
	"strings"
	"testing"
)

func TestDurableSpecRendersFinalFormat(t *testing.T) {
	out, err := DurableSpec(DurableSpecOptions{
		Capability:       "issue-spec-cli",
		ProposalIssueURL: "https://github.com/o/r/issues/1",
		SpecificationList: []SpecSource{{
			ID:  "SPEC-001",
			URL: "https://github.com/o/r/issues/1#issuecomment-1",
			Body: `<!-- issue-spec:type=SPEC id=SPEC-001 version=1 -->
Agent: Coordinator
Type: SPEC
ID: SPEC-001
Status: confirmed
Scope: cli
Links:
- Proposal Issue: https://github.com/o/r/issues/1
- Design Issue: N/A
- Implement Issue: N/A
- Related Comments: N/A
- PR: N/A

## Requirement: Question lifecycle

The CLI MUST manage questions.

### Scenario: Create question

- **WHEN** Coordinator creates a question
- **THEN** the CLI records it.
`,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# issue-spec-cli",
		"## Purpose",
		"Proposal Issues:\n- https://github.com/o/r/issues/1",
		"## Requirements",
		"### Requirement: Question lifecycle",
		"#### Scenario: Create question",
		"Source SPEC comment: https://github.com/o/r/issues/1#issuecomment-1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ADDED Requirements") {
		t.Fatalf("durable spec kept delta-only heading:\n%s", out)
	}
}

func TestDurableSpecRejectsUntestableSpec(t *testing.T) {
	_, err := DurableSpec(DurableSpecOptions{
		Capability:       "issue-spec-cli",
		ProposalIssueURL: "https://github.com/o/r/issues/1",
		SpecificationList: []SpecSource{{
			ID:   "SPEC-001",
			Body: "## Requirement: Bad\n\nThis is vague.",
		}},
	})
	if err == nil {
		t.Fatal("expected untestable spec to fail")
	}
}
