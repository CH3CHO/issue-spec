---
name: issue-spec-apply
description: Implement PROCESS comments for an issue-spec change and keep PR traceability synchronized.
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "issue-spec"
---

# Issue Spec Apply

Use when the user asks for /issue-spec:apply, issue-spec apply, or implementing PROCESS/TASK scopes from an issue-spec change.

## Steps

1. Read proposal/design/implement issue context and list typed comments with issue-spec comment list --json.
2. Create or update PROCESS comments with owner agent, scope, dependencies, and status.
3. Link each PROCESS to its TASK comments with issue-spec link.
4. Implement the code changes for one PROCESS scope at a time.
5. Link the PROCESS to the PR with issue-spec pr link-process.
6. Add PR rationale comments on key changed lines with issue-spec pr rationale, each linked to a SPEC comment.
7. Mark PROCESS comments done only after implementation and focused verification evidence exist.
