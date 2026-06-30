package templates

import (
	"fmt"
	"strings"
)

func ProposalIssue(change string) (string, string, []string) {
	title := fmt.Sprintf("issue-spec proposal: %s", change)
	body := fmt.Sprintf(`<!-- issue-spec:issue=proposal change=%s version=1 -->
# Proposal: %s

## Metadata

- Change Name: %s
- External Issue ID: N/A

## Background

TBD

## Goals

- TBD

## Scope

- TBD

## Non-Goals

- TBD

## Key Constraints

- Active change artifacts live in this GitHub proposal/design/implement issue set.

## Related Specs Analysis

TBD

## Existing Assumptions Impact

TBD

## Open Questions

- No blocking QUESTION is currently recorded.

## Capabilities

- TBD

## Impact

TBD
`, change, change, change)
	return title, body, []string{"issue-spec/proposal"}
}

func DesignIssue(change, proposalRef string) (string, string, []string) {
	title := fmt.Sprintf("issue-spec design: %s", change)
	body := fmt.Sprintf(`<!-- issue-spec:issue=design change=%s version=1 -->
# Design: %s

## Question Convergence Check

- Proposal Issue: %s
- Blocking QUESTION status: confirmed or explicitly accepted as assumptions.

## Current Implementation Locations

TBD

## Involved Modules

TBD

## Impact Scope

TBD

## Unaffected Modules

TBD

## Search Entry Points / Key Files

TBD

## Risk Hotspots

TBD

## Candidate Plans

TBD

## Decisions

TBD

## Rejected Alternatives

TBD

## Test Strategy and Acceptance Criteria

TBD

## Rollout / Rollback Notes

TBD

## Confirmation Checklist

- [ ] SPEC comments are linked and testable.
- [ ] Blocking QUESTION comments are resolved or accepted as assumptions.
`, change, change, valueOr(proposalRef, "N/A"))
	return title, body, []string{"issue-spec/design"}
}

func ImplementIssue(change, designRef string) (string, string, []string) {
	title := fmt.Sprintf("issue-spec implement: %s", change)
	body := fmt.Sprintf(`<!-- issue-spec:issue=implement change=%s version=1 -->
# Implement DAG: %s

## PR Mode Decision

TBD

## DAG Nodes and Dependencies

TBD

## Worktree / Branch Plan

TBD

## PR-owner and Review-agent Assignment

TBD

## Conflict Risk and Serialization Plan

TBD

## Global Review / Verify Status

- Design Issue: %s
- Status: draft
`, change, change, valueOr(designRef, "N/A"))
	return title, body, []string{"issue-spec/implement"}
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
