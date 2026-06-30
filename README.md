# issue-spec

`issue-spec` is a GitHub issue-backed OpenSpec-style workflow CLI. Active change artifacts live in proposal/design/implement issues and typed comments; durable specs remain repository files after merge/archive.

## Build

```bash
go test ./...
go build ./cmd/issue-spec
```

## MVP Commands

```bash
issue-spec auth status
issue-spec auth login --with-token
issue-spec auth logout
issue-spec auth token --plain

issue-spec init --repo owner/repo --create-labels
issue-spec issue create proposal --repo owner/repo --change my-change
issue-spec issue create design --repo owner/repo --change my-change --proposal 1
issue-spec issue create implement --repo owner/repo --change my-change --design 2

issue-spec comment upsert --repo owner/repo --issue 1 --type SPEC --id SPEC-001 --body-file spec.md
issue-spec comment list --repo owner/repo --issue 1 --json
issue-spec question create --repo owner/repo --issue 1 --id QUESTION-001 --blocking --question "What must be decided?"
issue-spec question resolve --repo owner/repo --issue 1 --id QUESTION-001 --resolution-file resolution.md
issue-spec pr rationale --repo owner/repo --pr 4 --path internal/foo.go --line 42 --process PROCESS-001 --spec SPEC-001 --spec-url https://github.com/owner/repo/issues/1#issuecomment-1 --body "Why this line changes."
issue-spec pr link-process --repo owner/repo --issue 3 --process PROCESS-001 --pr 4
issue-spec review sync --repo owner/repo --pr 4 --implement 3 --id REVIEW-001
issue-spec archive durable-spec --repo owner/repo --proposal 1 --capability issue-spec-cli
issue-spec link --repo owner/repo --from SPEC-001 --from-issue 1 --to TASK-001 --to-issue 2
issue-spec status --repo owner/repo --proposal 1 --design 2 --implement 3
issue-spec verify --repo owner/repo --proposal 1 --design 2 --implement 3 --pr 4 --durable-spec openspec/specs/issue-spec-cli/spec.md
issue-spec verify-links --repo owner/repo --proposal 1 --design 2 --implement 3
```

Token source priority is `ISSUE_SPEC_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, then the issue-spec credential store. The CLI uses GitHub REST directly and does not shell out to standalone `gh`.
