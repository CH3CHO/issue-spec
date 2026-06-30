---
name: issue-spec-workflow
description: Use issue-spec to run an issue-native OpenSpec-style workflow with GitHub issues, typed comments, PR review comments, final verification, and durable spec archive PRs.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Workflow

Use this skill for issue-native OpenSpec work. Active change artifacts live in GitHub issues and issue comments; durable specs are repository files created after implementation merge.

## Start

1. Run issue-spec auth status --json and confirm the active token source.
2. Run issue-spec status --repo higress-group/issue-spec --proposal <issue> --design <issue> --implement <issue> --json when issues already exist.
3. For new work, create proposal, design, and implement issues with issue-spec issue create and pass --body-file with concrete markdown content.
4. When an issue body changes, update it in place with issue-spec issue update --body-file and include --summary for the human-readable audit trail.
5. Store requirements, tasks, process ownership, review, and verify evidence as typed comments.

## Rules

- Create SPEC comments before design; each SPEC must be testable and include WHEN/THEN scenarios.
- Do not leave active proposal/design/implement issue bodies as TBD placeholders.
- Resolve blocking QUESTION comments before design/tasks, or explicitly record accepted assumptions.
- Link SPEC <-> TASK and TASK <-> PROCESS with issue-spec link.
- Link every PROCESS to the implementation PR with issue-spec pr link-process.
- Before human review, add PR rationale comments with issue-spec pr rationale for every active PROCESS.
- Use issue-spec review finding for PR line findings and issue-spec review reply to close the original thread.
- Run issue-spec review sync and issue-spec verify before declaring ready.
- After the implementation PR merges, create the separate durable spec PR with issue-spec archive durable-spec --create-pr.
