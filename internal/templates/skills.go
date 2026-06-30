package templates

import (
	"fmt"
	"strings"
)

const IssueSpecGeneratedBy = "issue-spec"

type WorkflowTemplate struct {
	Name        string
	Description string
	CommandID   string
	CommandName string
	Body        string
}

type RenderedSkill struct {
	Name    string
	Content string
}

type CommandContent struct {
	ID          string
	Name        string
	Description string
	Category    string
	Tags        []string
	Body        string
}

func IssueSpecSkills(repo string) []RenderedSkill {
	workflows := issueSpecWorkflows(repo)
	out := make([]RenderedSkill, 0, len(workflows))
	for _, tmpl := range workflows {
		out = append(out, RenderedSkill{Name: tmpl.Name, Content: renderSkill(tmpl.Name, tmpl.Description, tmpl.Body)})
	}
	return out
}

func IssueSpecCommandContents(repo string) []CommandContent {
	workflows := issueSpecWorkflows(repo)
	out := make([]CommandContent, 0, len(workflows))
	for _, tmpl := range workflows {
		if strings.TrimSpace(tmpl.CommandID) == "" {
			continue
		}
		out = append(out, CommandContent{
			ID:          tmpl.CommandID,
			Name:        tmpl.CommandName,
			Description: tmpl.Description,
			Category:    "Workflow",
			Tags:        []string{"workflow", "issue-spec"},
			Body:        tmpl.Body,
		})
	}
	return out
}

func issueSpecWorkflows(repo string) []WorkflowTemplate {
	repo = valueOr(strings.TrimSpace(repo), "owner/repo")
	workflows := []WorkflowTemplate{
		{
			Name:        "issue-spec-workflow",
			Description: "Use issue-spec to run an issue-native OpenSpec-style workflow with GitHub issues, typed comments, PR review comments, final verification, and durable spec archive PRs.",
			Body: `# Issue Spec Workflow

Use this skill for issue-native OpenSpec work. Active change artifacts live in GitHub issues and issue comments; durable specs are repository files created after implementation merge.

## Start

1. Run issue-spec auth status --json and confirm the active token source.
2. Run issue-spec status --repo {{repo}} --proposal <issue> --design <issue> --implement <issue> --json when issues already exist.
3. For new work, create proposal, design, and implement issues with issue-spec issue create.
4. Store requirements, tasks, process ownership, review, and verify evidence as typed comments.

## Rules

- Create SPEC comments before design; each SPEC must be testable and include WHEN/THEN scenarios.
- Resolve blocking QUESTION comments before design/tasks, or explicitly record accepted assumptions.
- Link SPEC <-> TASK and TASK <-> PROCESS with issue-spec link.
- Link every PROCESS to the implementation PR with issue-spec pr link-process.
- Before human review, add PR rationale comments with issue-spec pr rationale for every active PROCESS.
- Use issue-spec review finding for PR line findings and issue-spec review reply to close the original thread.
- Run issue-spec review sync and issue-spec verify before declaring ready.
- After the implementation PR merges, create the separate durable spec PR with issue-spec archive durable-spec --create-pr.
`,
		},
		{
			Name:        "issue-spec-propose",
			Description: "Create or continue proposal, SPEC, QUESTION, design, and TASK artifacts for an issue-spec change.",
			CommandID:   "propose",
			CommandName: "Issue Spec: Propose",
			Body: `# Issue Spec Propose

Use when the user asks for /issue-spec:propose, issue-spec propose, creating a change proposal, drafting SPEC comments, or preparing design/tasks after questions converge.

## Steps

1. Create the proposal issue:

       issue-spec issue create proposal --repo {{repo}} --change <change-name>

2. Add SPEC comments with issue-spec comment upsert --type SPEC. SPEC comments must use MUST/SHALL and WHEN/THEN scenarios.
3. Add QUESTION comments for unresolved behavior with issue-spec question create and resolve blocking questions before design.
4. Create the design issue after SPEC/QUESTION convergence:

       issue-spec issue create design --repo {{repo}} --change <change-name> --proposal <proposal-issue-or-url>

5. Add TASK comments with issue-spec comment upsert --type TASK and link every TASK to covered SPEC comments with issue-spec link.
6. Create the implement issue once tasks are ready.
7. Run issue-spec verify-links and fix missing backlinks before implementation.
`,
		},
		{
			Name:        "issue-spec-apply",
			Description: "Implement PROCESS comments for an issue-spec change and keep PR traceability synchronized.",
			CommandID:   "apply",
			CommandName: "Issue Spec: Apply",
			Body: `# Issue Spec Apply

Use when the user asks for /issue-spec:apply, issue-spec apply, or implementing PROCESS/TASK scopes from an issue-spec change.

## Steps

1. Read proposal/design/implement issue context and list typed comments with issue-spec comment list --json.
2. Create or update PROCESS comments with owner agent, scope, dependencies, and status.
3. Link each PROCESS to its TASK comments with issue-spec link.
4. Implement the code changes for one PROCESS scope at a time.
5. Link the PROCESS to the PR with issue-spec pr link-process.
6. Add PR rationale comments on key changed lines with issue-spec pr rationale, each linked to a SPEC comment.
7. Mark PROCESS comments done only after implementation and focused verification evidence exist.
`,
		},
		{
			Name:        "issue-spec-review",
			Description: "Review an issue-spec implementation PR, create PR line findings, reply after fixes, and sync REVIEW comments.",
			CommandID:   "review",
			CommandName: "Issue Spec: Review",
			Body: `# Issue Spec Review

Use when the user asks for /issue-spec:review, issue-spec review, or a PR review gate for an issue-spec implementation.

## Steps

1. Run issue-spec review sync --repo {{repo}} --pr <number> --implement <issue> --id REVIEW-<n> --json to capture current rationale comments, findings, and checks.
2. Create actionable PR line findings with issue-spec review finding. Use P0/P1 for blockers and P2 for non-blocking follow-up.
3. Assign every finding to a PROCESS owner.
4. After the worker fixes a finding, reply to the original thread with issue-spec review reply --status resolved.
5. Re-run review sync. P0/P1 findings must be resolved before final verify/archive.
`,
		},
		{
			Name:        "issue-spec-verify",
			Description: "Run final issue-spec verification across traceability, questions, review findings, PR rationale, PR checks, and durable spec draft.",
			CommandID:   "verify",
			CommandName: "Issue Spec: Verify",
			Body: `# Issue Spec Verify

Use when the user asks for /issue-spec:verify, issue-spec verify, or final readiness evidence before merge/archive.

## Steps

1. Run focused project tests and record evidence in VERIFY comments.
2. Run issue-spec verify-links --repo {{repo}} --proposal <issue> --design <issue> --implement <issue> --json.
3. Render a durable spec draft:

       issue-spec archive durable-spec --repo {{repo}} --proposal <issue> --capability <capability> --output /tmp/<capability>-spec.md --json

4. Run final verify:

       issue-spec verify --repo {{repo}} --proposal <issue> --design <issue> --implement <issue> --pr <pr> --durable-spec /tmp/<capability>-spec.md --json

5. Final verify must fail if blocking questions, missing links, missing PROCESS rationale, open P0/P1 findings, failed or pending PR checks, or durable spec omissions exist.
`,
		},
		{
			Name:        "issue-spec-archive",
			Description: "Create the post-merge durable spec archive PR for an issue-spec change.",
			CommandID:   "archive",
			CommandName: "Issue Spec: Archive",
			Body: `# Issue Spec Archive

Use when the user asks for /issue-spec:archive, issue-spec archive, or creating the post-merge durable spec PR.

## Steps

1. Confirm the implementation PR is merged.
2. Create the durable spec PR:

       issue-spec archive durable-spec --repo {{repo}} --proposal <issue> --capability <capability> --create-pr --branch issue-spec/durable-spec-<capability> --json

3. Review the durable spec PR for long-lived behavior only. Do not copy process records, review findings, or verification logs into durable specs.
4. After durable spec PR merge, keep proposal/design/implement issues as audit history unless the project policy says to close them.
`,
		},
	}

	for i := range workflows {
		workflows[i].Body = strings.ReplaceAll(workflows[i].Body, "{{repo}}", repo)
	}
	return workflows
}

func renderSkill(name, description, body string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
license: MIT
compatibility: Requires issue-spec CLI.
metadata:
  author: issue-spec
  version: "1.0"
  generatedBy: "%s"
---

%s`, name, description, IssueSpecGeneratedBy, strings.TrimSpace(body)+"\n")
}
