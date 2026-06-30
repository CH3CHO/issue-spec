# issue-spec-cli

## Purpose

记录 issue-native workflow CLI 的长期行为契约。

Proposal Issues:
- https://github.com/higress-group/issue-spec/issues/1

## Requirements

### Requirement: QUESTION lifecycle commands

`issue-spec question` MUST create and resolve structured `QUESTION` comments without requiring standalone `gh`, and blocking questions MUST participate in existing design/task gates.

#### Scenario: Create a blocking QUESTION

- **WHEN** Coordinator runs `issue-spec question create --blocking --id QUESTION-001` for a proposal/design/implement issue
- **THEN** the CLI MUST upsert one typed `QUESTION` comment with hidden marker, `Status: blocked`, explicit blocking flag, default assumption, links, and resolution log section.

#### Scenario: Resolve a blocking QUESTION

- **WHEN** Coordinator runs `issue-spec question resolve --id QUESTION-001 --resolution-file <file>`
- **THEN** the CLI MUST update the existing `QUESTION` comment in place, record resolution evidence, and set `Status: confirmed` unless another valid terminal status is requested.

#### Scenario: Gate observes QUESTION status

- **WHEN** a proposal issue has a `QUESTION` comment whose status is `blocked`
- **THEN** `issue-spec issue create design` and `issue-spec status` MUST report the blocking gate until the question is resolved or explicitly accepted as an assumption.

Source SPEC comment: https://github.com/higress-group/issue-spec/issues/1#issuecomment-4842970374

### Requirement: Durable spec draft generation

`issue-spec archive durable-spec` MUST generate a durable spec draft from confirmed `SPEC` comments without copying proposal/design/task/review/verify process records into the durable spec.

#### Scenario: Generate durable spec from proposal SPEC comments

- **WHEN** Coordinator runs `issue-spec archive durable-spec --repo <repo> --proposal <issue> --capability <name>`
- **THEN** the CLI MUST fetch active `SPEC` comments, validate that each requirement has testable scenarios, and write `openspec/specs/<capability>/spec.md` in final durable format.

#### Scenario: Durable spec keeps proposal traceability

- **WHEN** a durable spec draft is generated
- **THEN** the durable spec MUST include the proposal issue URL and source `SPEC` comment URLs, and MUST NOT retain delta-only headings such as `ADDED Requirements`.

Source SPEC comment: https://github.com/higress-group/issue-spec/issues/1#issuecomment-4843072386

### Requirement: Final verify gate

`issue-spec verify` MUST provide a read-only final delivery gate across proposal/design/implement issues and optional durable spec files.

#### Scenario: Final verify blocks incomplete workflow state

- **WHEN** any active `TASK`, `PROCESS`, or `REVIEW` comment is not done, any blocking `QUESTION` remains, or any active `SPEC` lacks `VERIFY` coverage
- **THEN** `issue-spec verify` MUST return `ok:false` and exit non-zero with actionable error details.

#### Scenario: Final verify checks durable spec traceability

- **WHEN** `issue-spec verify --durable-spec <path>` is provided
- **THEN** the CLI MUST confirm the durable spec has final durable format, includes the proposal issue URL, includes source `SPEC` comment URLs, and has no delta-only headings.

Source SPEC comment: https://github.com/higress-group/issue-spec/issues/1#issuecomment-4843133992

### Requirement: PR inline rationale comments

`issue-spec pr rationale` MUST create idempotent GitHub PR review comments on valid changed diff lines, and final verify MUST be able to require rationale coverage for active `PROCESS` comments when a PR is supplied.

#### Scenario: Create rationale on valid PR diff line

- **WHEN** Worker runs `issue-spec pr rationale --pr <number> --path <file> --line <right-line> --process <PROCESS-ID> --spec <SPEC-ID> --spec-url <url>`
- **THEN** the CLI MUST validate the line is present in the PR file patch, create a PR review comment with agent identity, rationale marker, process id, and SPEC comment URL, and avoid duplicate comments for the same logical rationale.

#### Scenario: Final verify requires rationale coverage when PR is supplied

- **WHEN** Coordinator runs `issue-spec verify --pr <number>`
- **THEN** the CLI MUST fail final verify unless every active `PROCESS` comment has at least one rationale review comment linked to an active `SPEC` comment.

Source SPEC comment: https://github.com/higress-group/issue-spec/issues/1#issuecomment-4843179415

### Requirement: PR review sync

`issue-spec review sync` MUST fetch PR review comments, PR issue comments, review rationale comments, and check results, then upsert a structured `REVIEW` comment on the implement issue.

#### Scenario: Sync clean PR review state

- **WHEN** Coordinator runs `issue-spec review sync --pr <number> --implement <issue> --id <REVIEW-ID>` on a PR with only rationale line comments and passing checks
- **THEN** the CLI MUST upsert a `REVIEW` comment with `Status: done`, rationale counts, check evidence, and no actionable findings.

#### Scenario: Sync blocking PR review state

- **WHEN** PR line comments without rationale markers, failed checks, or pending checks are present
- **THEN** the CLI MUST mark the synced `REVIEW` comment as `blocked` and report actionable findings or check blockers.

Source SPEC comment: https://github.com/higress-group/issue-spec/issues/1#issuecomment-4843317232
