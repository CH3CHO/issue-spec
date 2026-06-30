# issue-spec

`issue-spec` is a GitHub issue-backed OpenSpec-style workflow CLI. Active change artifacts live in proposal/design/implement issues and typed comments; durable specs remain repository files after merge/archive.

## Why issue-spec

`issue-spec` keeps the working habits that make OpenSpec useful for agents:

- phase-oriented change work, from proposal to specs, design, tasks, review, verify, and archive
- reusable agent skills and slash-command style workflows
- explicit gates before implementation, review, and archive
- durable specs after a change is merged

It changes where active change specs live and adds GitHub-native coordination on top.

### Active specs stay out of the code repository

OpenSpec active changes are usually repository files under `openspec/changes/<change>/...`. That works well for local spec-driven development, but it also means draft, superseded, or abandoned change specs can be found by `grep`, `rg`, code search, or an agent reading the repository later.

`issue-spec` keeps active change artifacts in GitHub issues instead:

- proposal issue: proposal body plus `SPEC` and `QUESTION` comments
- design issue: design body plus `TASK` and `QUESTION` comments
- implement issue: implementation DAG plus `PROCESS`, `REVIEW`, and `VERIFY` comments

This keeps the repository focused on current code and durable specs. Draft change history remains reviewable in GitHub, with comment threads, edits, links, and human approval points. Human-in-the-loop decisions are first-class: blocking questions, accepted assumptions, review findings, and verification evidence are all issue or PR comments instead of unreviewed local files.

### Native multi-agent DAG coordination

`issue-spec` treats implementation as a native multi-agent workflow. Work is split into small `TASK` and `PROCESS` units, linked back to the relevant `SPEC` comments and PR work.

The goal is to keep each model invocation inside its effective reasoning zone: narrow scope, clear context, explicit ownership, focused tests, and small review surfaces. The implement issue records the DAG:

- worker owner and review owner
- branch/worktree or PR node
- dependencies
- owned files and scope
- linked TASK/SPEC comments
- status, blockers, and verification evidence

The CLI does not act as a scheduler that launches agents automatically. It provides the shared state, links, and gates that let a coordinator safely split work across multiple agents without losing traceability.

### PR-native review flow

OpenSpec already encourages review and verification as workflow phases. `issue-spec` connects that discipline directly to GitHub PR review comments:

- `pr rationale` records why a worker changed a specific PR diff line and links it to a `SPEC` and `PROCESS`
- `review finding` creates actionable PR line findings with severity, owner process, and linked spec context
- `review reply` lets the worker close the original review thread after a fix
- `review sync` summarizes rationale comments, findings, resolved findings, PR checks, and review status back into `REVIEW` comments

This makes review easier for humans: findings are attached to the exact code line, while the issue comments preserve the workflow state and assignment context. Final verification checks unresolved blocking questions, traceability, P0/P1 findings, PR rationale coverage, PR checks, and durable spec coverage before archive.

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
issue-spec init --repo owner/repo --tools codex,claude --delivery both
issue-spec issue create proposal --repo owner/repo --change my-change
issue-spec issue create design --repo owner/repo --change my-change --proposal 1
issue-spec issue create implement --repo owner/repo --change my-change --design 2

issue-spec comment upsert --repo owner/repo --issue 1 --type SPEC --id SPEC-001 --body-file spec.md
issue-spec comment list --repo owner/repo --issue 1 --json
issue-spec question create --repo owner/repo --issue 1 --id QUESTION-001 --blocking --question "What must be decided?"
issue-spec question resolve --repo owner/repo --issue 1 --id QUESTION-001 --resolution-file resolution.md
issue-spec pr rationale --repo owner/repo --pr 4 --path internal/foo.go --line 42 --process PROCESS-001 --spec SPEC-001 --spec-url https://github.com/owner/repo/issues/1#issuecomment-1 --body "Why this line changes."
issue-spec pr link-process --repo owner/repo --issue 3 --process PROCESS-001 --pr 4
issue-spec review finding --repo owner/repo --pr 4 --path internal/foo.go --line 42 --id FINDING-001 --severity P1 --process PROCESS-001 --spec SPEC-001 --spec-url https://github.com/owner/repo/issues/1#issuecomment-1 --body "What must be fixed."
issue-spec review reply --repo owner/repo --pr 4 --comment-id 123456 --finding FINDING-001 --process PROCESS-001 --status resolved --body "Fixed in the latest patch."
issue-spec review sync --repo owner/repo --pr 4 --implement 3 --id REVIEW-001
issue-spec archive durable-spec --repo owner/repo --proposal 1 --capability issue-spec-cli
issue-spec archive durable-spec --repo owner/repo --proposal 1 --capability issue-spec-cli --create-pr --branch issue-spec/durable-spec-issue-spec-cli
issue-spec link --repo owner/repo --from SPEC-001 --from-issue 1 --to TASK-001 --to-issue 2
issue-spec status --repo owner/repo --proposal 1 --design 2 --implement 3
issue-spec verify --repo owner/repo --proposal 1 --design 2 --implement 3 --pr 4 --durable-spec openspec/specs/issue-spec-cli/spec.md
issue-spec verify-links --repo owner/repo --proposal 1 --design 2 --implement 3
```

Token source priority is `ISSUE_SPEC_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, then the issue-spec credential store. The CLI uses GitHub REST directly and does not shell out to standalone `gh`.

## Agent Skills And Slash Commands

`issue-spec init` can generate OpenSpec-style agent workflow artifacts for a project:

```bash
issue-spec init --repo owner/repo --tools codex,claude --delivery both
```

- Codex skills are written to `.agents/skills/issue-spec-*`, the current Codex repo skill location.
- Claude skills are written to `.claude/skills/issue-spec-*`.
- Claude slash commands are written to `.claude/commands/issue-spec/*.md`, invoked like `/issue-spec:propose`.
- Codex slash prompts are written to `${CODEX_HOME:-~/.codex}/prompts/issue-spec-*.md` for compatibility with Codex custom prompts. Codex custom prompts are deprecated by current Codex docs; prefer skills for shared workflows.
- `--delivery skills` writes only skills; `--delivery commands` writes only slash commands.

If `--tools` is omitted, init detects existing `.agents` or `.claude` directories and refreshes those workflows. Use `--tools none` to initialize only `.issue-spec/config.json` and optional labels.
