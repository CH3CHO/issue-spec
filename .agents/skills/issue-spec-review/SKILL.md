---
name: issue-spec-review
description: Review an issue-spec implementation PR, create PR line findings, reply after fixes, and sync REVIEW comments.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Review

Use when the user asks for /issue-spec:review, issue-spec review, or a PR review gate for an issue-spec implementation.

## Steps

1. Run issue-spec review sync --repo higress-group/issue-spec --pr <number> --implement <issue> --id REVIEW-<n> --json to capture current rationale comments, findings, and checks.
2. Create actionable PR line findings with issue-spec review finding. Use P0/P1 for blockers and P2 for non-blocking follow-up.
3. Assign every finding to a PROCESS owner.
4. After the worker fixes a finding, reply to the original thread with issue-spec review reply --status resolved.
5. Re-run review sync. P0/P1 findings must be resolved before final verify/archive.
