# Issue-spec CLI Implementation Plan

## Purpose

本文档用于指导全新上下文中的 agent 初始化并实现一个新的 GitHub 项目：`issue-spec` CLI。

目标不是马上依赖一个已经存在的 CLI，而是先用 agent + `gh` 手工执行 issue-native OpenSpec 工作流，完成 `issue-spec` 项目自身的 proposal、spec、design、task、implement、review、verify，再逐步把其中重复且容易出错的操作固化成 CLI。

## Background

当前参考对象有三类：

- `/Users/zhangty/work/istio` 中成熟的 OpenSpec 工作流。
- `/Users/zhangty/git/openspec` 中 OpenSpec CLI 源码。
- `/Users/zhangty/git/cli` 中 GitHub CLI 源码和 `github.com/cli/go-gh/v2` 依赖边界。
- 当前仓库根目录的 `proposal.md`，描述了 issue-native OpenSpec 工作流需求。

OpenSpec 的强项：

- artifact DAG: `proposal -> specs/questions -> design -> tasks -> review -> verify`。
- schema/templates/rules 能产出高质量阶段指令。
- durable spec 使用仓库文件长期保存。

OpenSpec 不适合直接承担的部分：

- active change artifacts 默认写入 `openspec/changes/<change>/...`。
- 状态依赖本地文件是否存在。
- `store` 概念是 git planning repo，不是 GitHub issue/comment storage provider。
- 不管理 issue comment id、typed comment upsert、双向链接、review thread、PROCESS 状态和 PR line comments。

因此，`issue-spec` CLI 应该是 GitHub-backed adapter，而不是 OpenSpec 的替代品。

## Target Outcome

新项目最终应提供一个 CLI，帮助 agent 和人类在 GitHub issues/PRs 中执行 issue-native OpenSpec 流程：

- 创建 proposal/design/implement 三类 issue。
- 创建或原地更新 typed comments。
- 维护 `SPEC <-> TASK <-> PROCESS <-> PR <-> durable spec` traceability。
- 检查 unresolved `QUESTION`、P0/P1 review finding、CI failure、断链和 spec/implementation 冲突。
- 内置 GitHub authentication 能力，最终 CLI 不要求用户预先安装 `gh`。
- 在实现 PR merge 后，生成 durable spec PR 草稿。

第一阶段不需要完全实现所有能力。MVP 应先解决最容易出错的部分：

- typed comment idempotent upsert。
- issue/comment 状态读取。
- 双向链接维护和校验。
- status/verify-links 输出。
- `issue-spec auth` 登录、登出、状态检查和 token source 管理。

## Bootstrap Rule

在 `issue-spec` CLI 实现前，初始化项目必须纯基于 agent + `gh` 完成，不依赖自研 CLI。

允许使用：

```bash
gh repo create
gh issue create
gh issue view
gh issue edit
gh api
gh pr create
gh pr checks
gh pr view
gh pr diff
```

不允许假设：

- 已经存在 `issue-spec` CLI。
- OpenSpec CLI 可以直接写 GitHub issue/comment。
- GitHub issue comments 可以靠人工重复创建而不记录 comment id。

Bootstrap 阶段允许依赖本机 `gh`，因为 CLI 尚不存在。`issue-spec` 项目实现完成后，正常用户路径 MUST NOT require standalone `gh` installation；GitHub API、auth、repo/issue/comment/PR 操作应由 Go 代码直接完成。

## Recommended Repository

建议新建独立 GitHub 仓库：

```text
<owner>/issue-spec
```

推荐技术栈：

- Language: Go。
- CLI framework: `spf13/cobra` or lightweight standard-library command dispatcher. Prefer `cobra` if subcommand help and shell completion are needed early。
- Markdown parsing: 先用轻量正则 + structured markers；复杂 spec parsing 后续可接 OpenSpec parser。
- GitHub API: use Go HTTP client plus GitHub REST v3 endpoints. Prefer `github.com/cli/go-gh/v2` for reusable auth/config/API helpers where it has a stable public package boundary。
- Authentication: implement `issue-spec auth` in MVP. Support env tokens first, then persisted token login。
- Test runner: Go `testing` package。
- Distribution: single binary via Go builds。

选择 Go 的原因：

- 不要求用户安装 `gh` CLI。
- 单 binary 分发简单，适合 agent、CI 和本地脚本调用。
- GitHub API、Markdown 状态机、文件生成和并发抓取 comments/PR 数据都适合 Go。
- 可以参考 `/Users/zhangty/git/cli` 的 auth/API 设计，但避免直接依赖其 `internal/` 包。

GitHub integration boundary:

- SHOULD use `github.com/cli/go-gh/v2` public packages when they cover auth/config/API needs。
- MAY参考 `github.com/cli/cli/v2` 源码中的 API/auth 设计。
- MUST NOT import `github.com/cli/cli/v2/internal/...` from the new project, because Go `internal` packages are not importable outside their parent tree。
- SHOULD wrap any GitHub client dependency behind `internal/github.Client` so future replacement is localized。

## Issue-native Workflow Model

每个复杂 change 使用三类 issue。

### Proposal Issue

Issue body 是 proposal。

必须包含：

- Metadata。
- Background。
- Goals。
- Scope。
- Non-Goals。
- Key Constraints。
- Related Specs Analysis。
- Existing Assumptions Impact。
- Open Questions summary。
- Capabilities。
- Impact。

Issue comments：

- `SPEC`: 每个 requirement 一个 comment。
- `QUESTION`: blocking/non-blocking decision。

Gate：

- 至少一个 `SPEC` comment 存在后，才能进入 design。
- Blocking `QUESTION` 必须解决，或被人类显式接受为假设。

### Design Issue

Issue body 是整体设计。

必须包含：

- Question Convergence Check。
- Current Implementation Locations。
- Involved Modules。
- Impact Scope。
- Unaffected Modules。
- Search Entry Points / Key Files。
- Risk Hotspots。
- Candidate Plans。
- Decisions。
- Rejected Alternatives。
- Test Strategy and Acceptance Criteria。
- Rollout / Rollback Notes。
- Confirmation Checklist。

Issue comments：

- `TASK`: 每个 implementation task 一个 comment。
- `QUESTION`: 设计期间新发现的决策点。

Gate：

- 每个 `TASK` 必须链接相关 `SPEC` comment。
- 相关 `SPEC` comment 必须 backlink 到 `TASK` comment。
- 如果设计发现需求问题，必须回溯更新 proposal issue 和 `SPEC` comments。

### Implement Issue

Issue body 是实现 DAG 和 multi-agent 协作计划。

必须包含：

- PR mode decision: single integrated PR or multi-PR DAG。
- DAG nodes and dependencies。
- Worktree/branch plan。
- PR-owner and review-agent assignment。
- Conflict risk and serialization plan。
- Global review/verify status。

Issue comments：

- `PROCESS`: 每个 worker/agent/PR node 一个 comment。
- `QUESTION`: 实现期阻塞问题。
- `REVIEW`: review gate summary or finding tracker。
- `VERIFY`: final verification evidence。

Gate：

- Coordinator 确认 DAG 后才能启动 worker agents。
- 每个 `PROCESS` 必须链接相关 `TASK` comments。
- 相关 `TASK` comments 必须 backlink 到 `PROCESS` comments。
- 进入最终人工 review 前，每个实现 worker 必须在自己负责的关键代码行添加 PR inline rationale comments，并链接相关 `SPEC` comments。

## Typed Comment Format

所有 typed comments 使用统一 header。这个格式是 CLI MVP 的解析对象。

```markdown
<!-- issue-spec:type=SPEC id=SPEC-001 version=1 -->
Agent: Coordinator
Type: SPEC
ID: SPEC-001
Status: draft
Scope: <behavior or module>
Links:
- Proposal Issue: <url>
- Design Issue: N/A
- Implement Issue: N/A
- Related Comments: N/A
- PR: N/A

## Requirement: <name>

<Requirement body using SHALL or MUST.>

### Scenario: <name>

- **WHEN** <condition>
- **THEN** <expected behavior>
```

Common fields:

- `Agent`: logical agent identity, not GitHub account identity。
- `Type`: one of `SPEC`, `TASK`, `PROCESS`, `QUESTION`, `REVIEW`, `VERIFY`。
- `ID`: stable logical id。
- `Status`: `draft`, `blocked`, `confirmed`, `in-progress`, `ready`, `done`, `superseded`。
- `Scope`: owned behavior/module。
- `Links`: issue/comment/PR links。

Hidden marker rules:

- Marker must appear near the top of the comment body。
- Upsert searches by `issue-spec:type=<TYPE> id=<ID>`。
- If found, update the existing comment in place。
- If not found, create a new comment。
- Do not create duplicate logical artifacts。

## Manual Bootstrap Commands

These examples assume:

```bash
OWNER=<owner>
REPO=issue-spec
```

### Create Repository

```bash
gh repo create "$OWNER/$REPO" --public --description "GitHub issue-backed spec workflow CLI" --clone
cd issue-spec
```

If the repository should be private, use `--private`.

### Create Labels

```bash
gh label create issue-spec/proposal --color 0969da --description "Issue-native proposal artifact"
gh label create issue-spec/design --color 8250df --description "Issue-native design artifact"
gh label create issue-spec/implement --color 1a7f37 --description "Issue-native implementation coordination"
gh label create issue-spec/question --color fbca04 --description "Blocking or non-blocking workflow question"
gh label create issue-spec/review --color cf222e --description "Review gate or finding"
gh label create issue-spec/verify --color 57606a --description "Verification evidence"
```

### Create Proposal Issue

```bash
gh issue create \
  --repo "$OWNER/$REPO" \
  --title "issue-spec CLI proposal" \
  --label issue-spec/proposal \
  --body-file proposal-body.md
```

Capture the issue number:

```bash
PROPOSAL_ISSUE=<number>
```

### Create Or Update Typed Comment Manually

Create:

```bash
gh api "repos/$OWNER/$REPO/issues/$PROPOSAL_ISSUE/comments" \
  -f body="$(cat spec-001.md)"
```

Update:

```bash
COMMENT_ID=<id>
gh api "repos/$OWNER/$REPO/issues/comments/$COMMENT_ID" \
  -X PATCH \
  -f body="$(cat spec-001.md)"
```

Find existing comments:

```bash
gh api "repos/$OWNER/$REPO/issues/$PROPOSAL_ISSUE/comments" --paginate \
  --jq '.[] | {id, url, body}'
```

Agents should manually search hidden markers before posting a new comment.

## CLI MVP Scope

The MVP should implement only the primitives needed to make the manual process reliable.

### Command Group: `issue-spec auth`

Purpose:

- Provide GitHub authentication without requiring standalone `gh` installation。
- Support non-interactive agent/CI usage through environment tokens。
- Support local user login and persisted credentials。
- Make token source explicit in status output。

Token source priority:

1. `ISSUE_SPEC_TOKEN`。
2. `GH_TOKEN`。
3. `GITHUB_TOKEN`。
4. persisted issue-spec credential store。

MVP commands:

```bash
issue-spec auth status
issue-spec auth login --with-token
issue-spec auth logout
issue-spec auth token
```

`auth status` requirements:

- Report hostname, authenticated user if known, token source, and required scopes if detectable。
- Exit non-zero when no token is available。
- Support `--hostname`, default `github.com`。
- Support `--json`。

`auth login --with-token` requirements:

- Read token from stdin, not command-line args。
- Validate token by calling `GET /user`。
- Store token in OS keyring when available。
- Fall back to an issue-spec config file only with an explicit confirmation flag or documented warning。
- Never print the token。

`auth logout` requirements:

- Remove persisted token for the host。
- Do not unset environment variables; report when env token is still active。

`auth token` requirements:

- Print the active token only for scripting compatibility。
- Refuse to print in human mode unless `--plain` or equivalent explicit flag is provided。
- In JSON mode, include token source metadata but avoid accidental token leakage unless explicitly requested。

Future auth commands:

```bash
issue-spec auth login --web
issue-spec auth refresh
```

These can use OAuth device/browser flow after MVP. Reference `/Users/zhangty/git/cli/internal/authflow` and `github.com/cli/oauth`, but do not import GitHub CLI `internal` packages.

Auth implementation details:

- Token resolution MUST be deterministic and visible in `auth status`。
- Environment tokens MUST never be written into the persisted credential store。
- Persisted credentials SHOULD be keyed by hostname, so `github.com` and enterprise hosts do not collide。
- The preferred credential backend is OS keyring. A plaintext/config-file token fallback requires an explicit flag such as `--insecure-storage` and a warning in human output。
- Config files may store non-secret metadata such as default hostname, default repo, last authenticated user, and token source。
- API clients MUST receive tokens through an interface, not by reading environment variables directly inside every command。
- The MVP SHOULD validate scopes from GitHub response headers when available. For private repositories, `repo` scope is expected; for public repositories, `public_repo` or broader scope is expected。
- Error messages MUST identify the hostname, operation, and token source, but MUST redact token values。

Suggested Go boundary:

```go
type Token struct {
    Value  string
    Source string
    User   string
    Scopes []string
}

type TokenSource interface {
    Token(ctx context.Context, host string) (Token, error)
}
```

### Command: `issue-spec init`

Purpose:

- Create default config in repo, if needed。
- Validate issue-spec auth status。
- Optionally create labels。

Suggested usage:

```bash
issue-spec init --repo owner/repo
issue-spec init --repo owner/repo --create-labels
```

Output:

- JSON and human modes。
- Report repo, labels, auth status, token source, and API hostname。

### Command: `issue-spec issue create`

Purpose:

- Create proposal/design/implement issues using templates。

Suggested usage:

```bash
issue-spec issue create proposal --repo owner/repo --change issue-spec-cli
issue-spec issue create design --repo owner/repo --change issue-spec-cli --proposal <url>
issue-spec issue create implement --repo owner/repo --change issue-spec-cli --design <url>
```

Requirements:

- Issue body should include hidden marker。
- Labels should be applied。
- Output issue number and URL。

### Command: `issue-spec comment upsert`

Purpose:

- Create or update a typed comment by `(type, id)` marker。

Suggested usage:

```bash
issue-spec comment upsert \
  --repo owner/repo \
  --issue 12 \
  --type SPEC \
  --id SPEC-001 \
  --body-file spec-001.md
```

Requirements:

- Fetch all comments。
- Find marker。
- PATCH existing comment if marker exists。
- POST new comment if marker does not exist。
- Output comment id and URL。

### Command: `issue-spec comment list`

Purpose:

- List typed comments on one issue。

Suggested usage:

```bash
issue-spec comment list --repo owner/repo --issue 12 --json
issue-spec comment list --repo owner/repo --issue 12 --type SPEC
```

Requirements:

- Parse hidden markers and visible headers。
- Return type/id/status/scope/url/links。
- Flag malformed typed comments。

### Command: `issue-spec link`

Purpose:

- Maintain bidirectional links。

Suggested usage:

```bash
issue-spec link \
  --repo owner/repo \
  --from SPEC-001 --from-issue 12 \
  --to TASK-001 --to-issue 13
```

Requirements:

- Add `to` URL to `from` comment links。
- Add `from` URL to `to` comment links。
- Preserve other body content。
- Avoid duplicate links。

### Command: `issue-spec status`

Purpose:

- Render change-level status across proposal/design/implement issues。

Suggested usage:

```bash
issue-spec status --repo owner/repo --proposal 12 --design 13 --implement 14
```

Status should include:

- SPEC count and statuses。
- Blocking QUESTION count。
- TASK count and statuses。
- PROCESS count and statuses。
- Open REVIEW findings。
- VERIFY verdict。
- Missing backlinks。
- PR links and CI status if available。

### Command: `issue-spec verify-links`

Purpose:

- Check traceability integrity。

Suggested usage:

```bash
issue-spec verify-links --repo owner/repo --proposal 12 --design 13 --implement 14
```

Checks:

- Every `TASK` links at least one `SPEC`。
- Every linked `SPEC` backlinks the `TASK`。
- Every `PROCESS` links at least one `TASK`。
- Every linked `TASK` backlinks the `PROCESS`。
- Every PR rationale comment links a `SPEC` when PR is known。
- No unknown comment URLs。
- No duplicated logical ids。
- No typed comment with missing marker/header mismatch。

## Phase 2 CLI Scope

After MVP is stable, implement:

### `issue-spec question`

- Create blocking/non-blocking QUESTION。
- Mark resolved。
- Record resolution log。
- Update dependent SPEC/TASK/PROCESS comments。

### `issue-spec pr rationale`

- Find PR diff line numbers。
- Add inline rationale comments by PROCESS owner。
- Avoid duplicate rationale comments。
- Require SPEC links in comment body。

### `issue-spec review sync`

- Fetch PR review comments。
- Classify actionable vs explanatory comments。
- Map findings to PROCESS owner。
- Create/update REVIEW comment summary。

### `issue-spec verify`

- Collect CI status。
- Collect PROCESS statuses。
- Validate SPEC/TASK/PROCESS graph。
- Generate/update VERIFY comment。
- Block final verdict when P0/P1 finding, unresolved QUESTION, missing rationale comment, or failing CI exists。

### `issue-spec archive durable-spec`

- Generate durable spec draft from final SPEC comments。
- Include proposal issue URL。
- Include later proposal issue URLs for modified behavior。
- Create branch and PR, or generate files only for agent review。

## Project Architecture

Recommended initial tree:

```text
issue-spec/
├── go.mod
├── go.sum
├── README.md
├── cmd/
│   └── issue-spec/
│       └── main.go
├── internal/
│   ├── auth/
│   │   ├── token_source.go
│   │   ├── env.go
│   │   ├── store.go
│   │   ├── keyring.go
│   │   └── login.go
│   ├── commands/
│   │   ├── auth.go
│   │   ├── init.go
│   │   ├── issue_create.go
│   │   ├── comment_upsert.go
│   │   ├── comment_list.go
│   │   ├── link.go
│   │   ├── status.go
│   │   └── verify_links.go
│   ├── github/
│   │   ├── client.go
│   │   ├── rest.go
│   │   ├── issues.go
│   │   ├── comments.go
│   │   ├── pulls.go
│   │   └── types.go
│   ├── model/
│   │   ├── marker.go
│   │   ├── typed_comment.go
│   │   ├── links.go
│   │   ├── status.go
│   │   └── validation.go
│   ├── templates/
│   │   ├── proposal.go
│   │   ├── design.go
│   │   ├── implement.go
│   │   └── typed_comments.go
│   └── output/
│       ├── json.go
│       └── human.go
└── testdata/
    ├── comments/
    ├── graphs/
    └── github/
```

### Module Responsibilities

`auth/`:

- Resolve token source in this priority: `ISSUE_SPEC_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, persisted issue-spec credential。
- Implement `issue-spec auth status/login/logout/token`。
- Store persisted credentials in OS keyring when available。
- Support explicit config fallback only with a warning and user confirmation。
- Never print tokens except for explicit `issue-spec auth token`。
- Redact tokens from logs and errors。
- Support `--hostname`, defaulting to `github.com`。
- Validate login token with `GET /user` and report the authenticated user。

`github/`:

- Direct GitHub REST client; do not shell out to standalone `gh`。
- Inject tokens through `auth.TokenSource`。
- Support JSON request/response, pagination, rate-limit metadata, and explicit error context。
- Prefer stable public packages from `github.com/cli/go-gh/v2` when useful。
- MAY reference `/Users/zhangty/git/cli` source for behavior, but MUST NOT import `github.com/cli/cli/v2/internal/...` packages。

`model/marker.go`:

- Parse hidden markers。
- Render hidden markers。
- Validate allowed type/id/status。

`model/typed_comment.go`:

- Parse visible header。
- Merge updates without dropping body。
- Detect header/marker mismatch。

`model/links.go`:

- Parse `Links:` block。
- Add/remove links idempotently。
- Verify backlinks。

`model/status.go`:

- Aggregate issue/comment/PR status。
- Derive ready/blocked/done state。

`model/validation.go`:

- Validate typed comment shape。
- Validate graph invariants。

## Suggested Agent Workflow For Bootstrapping This Project

Use Coordinator + workers even before CLI exists.

### Coordinator Agent

Responsibilities:

- Create proposal/design/implement issues manually with `gh`。
- Create initial typed comments。
- Maintain comment URLs and ids。
- Decide single PR vs multi-PR。
- Assign workers。
- Run final verify。
- Ensure worker rationale comments exist before human review。

### Worker A: CLI Skeleton

Owns:

- `go.mod` and initial dependencies。
- `cmd/issue-spec/main.go`。
- command registration under `internal/commands`。
- README quickstart。

Initial PR scope:

- `issue-spec --help` works。
- JSON/human output convention established。
- `go test ./...` works。
- `go build ./cmd/issue-spec` works。

### Worker B: Typed Comment Model

Owns:

- marker parser。
- typed comment parser。
- links parser/updater。
- unit tests。

Initial PR scope:

- parse/render/upsert body primitives。
- no GitHub dependency。

### Worker C: Auth And GitHub Adapter

Owns:

- `internal/auth` token source and credential store。
- `issue-spec auth status/login/logout/token`。
- GitHub REST client。
- issue/comment fetch。
- comment create/update。
- PR file/comment helper primitives。
- label creation helper。
- mocked tests。

Initial PR scope:

- no workflow business logic beyond auth and GitHub transport。
- no standalone `gh` dependency in normal CLI execution。
- env token path works before persisted login path。

### Worker D: Status And Verify-links

Owns:

- status aggregation。
- verify-links checks。
- issue/comment graph validation。
- output formatting。

Initial PR scope:

- use mocked GitHub data。
- CLI commands work against fixtures。

### Review Agent

Responsibilities:

- Review per PR。
- Comment on code lines for findings。
- Check tests and CLI behavior。
- Check typed comment schema does not drift from proposal。

For small bootstrap, use one integrated PR. For larger implementation, split into PR DAG:

```text
PR A: project skeleton and command framework
PR B: typed comment parser and link model, depends on PR A
PR C: auth and GitHub adapter, depends on PR A
PR D: status/verify-links, depends on PR B and PR C
PR E: docs and dogfood examples, depends on PR D
```

Do not force parallel PRs if the repository is still tiny and interfaces are unstable.

## Review And Rationale Comment Policy

Before human review:

- Each implementation worker must add 2-4 PR inline rationale comments on key lines they own。
- Each rationale comment must start with agent identity。
- Each rationale comment must link relevant `SPEC` comment。
- Coordinator must verify every `PROCESS` owner has at least one rationale comment unless its scope is docs-only or config-only and explicitly waived。

Example:

```text
Agent: Worker B
Scope: typed comment parser
SPEC: <SPEC-003 comment URL>

This parser treats the hidden marker as the idempotency key, so repeated agent runs update the same logical artifact instead of creating duplicate comments.
```

## Verification Requirements

Every PR should have:

- Unit tests for changed logic。
- `go test ./...` evidence。
- `go build ./cmd/issue-spec` evidence。
- `go vet ./...` evidence, or `golangci-lint run` evidence if the project configures golangci-lint。
- CLI smoke test evidence。
- SPEC coverage mapping in `VERIFY` comment。

`VERIFY` comment should include:

```markdown
<!-- issue-spec:type=VERIFY id=VERIFY-001 version=1 -->
Agent: Verify Agent
Type: VERIFY
ID: VERIFY-001
Status: ready
Scope: project bootstrap
Links:
- Implement Issue: <url>
- PR: <url>

## Exit Criteria

- [x] No open P0/P1 review findings
- [x] Build passes
- [x] Unit tests pass
- [x] Every SPEC scenario has coverage or documented blocker
- [x] Worker rationale comments are present

## Command Evidence

| Command | Result | Summary |
| --- | --- | --- |
| `go test ./...` | PASS | ... |
| `go build ./cmd/issue-spec` | PASS | ... |
| `go run ./cmd/issue-spec --help` | PASS | ... |

## Spec Coverage

| SPEC | Scenario | Evidence |
| --- | --- | --- |
| SPEC-001 | comment upsert is idempotent | `internal/model/typed_comment_test.go` |

## Final Verdict

Ready for human review.
```

## Durable Spec Plan

After the implementation PR merges:

1. Coordinator creates a separate durable-spec PR。
2. Durable spec files are added under the chosen durable spec location。
3. Durable spec includes:
   - Purpose。
   - Requirements。
   - Scenarios。
   - Proposal issue URL。
   - Later proposal issue URLs that modify the same behavior。
4. Code comments should link durable spec entries only when they capture long-lived behavior assumptions。

Open question: durable spec path for the new repo should be decided in proposal stage. Candidate:

```text
openspec/specs/issue-native-workflow/spec.md
openspec/specs/typed-comment-storage/spec.md
openspec/specs/github-adapter-cli/spec.md
```

## Risks And Mitigations

Risk: issue comments become hard to diff。

- Mitigation: use structured headers, hidden markers, status command, and optional snapshot export later。

Risk: duplicate comments。

- Mitigation: all typed comments must use hidden idempotency marker; `comment upsert` is MVP priority。

Risk: agents forget backlinks。

- Mitigation: `verify-links` is MVP priority; Coordinator blocks final review when missing backlinks exist。

Risk: workflow too heavy for small changes。

- Mitigation: allow lightweight single issue mode, but keep typed comments and traceability。

Risk: OpenSpec reuse creates dependency instability。

- Mitigation: first copy or reimplement minimal parser logic; only import OpenSpec modules after public exports are stable。

Risk: GitHub API line comments fail due to invalid diff line。

- Mitigation: CLI should call `GET /repos/{owner}/{repo}/pulls/{pull_number}/files` or an equivalent `go-gh` client path to find valid changed lines before posting。

## OpenSpec Reuse Strategy

Do not fork OpenSpec as the first step.

Short term:

- Reuse concepts and templates。
- Copy minimal logic if needed with attribution/license compliance。
- Keep issue-spec CLI independent。

Medium term:

- Contribute public exports to OpenSpec for:
  - artifact graph。
  - schema resolver。
  - template loader。
  - validator。
  - delta spec parser。

Long term:

- If upstream is interested, propose `ArtifactStore` abstraction:
  - `FileSystemArtifactStore`。
  - `GitHubIssueArtifactStore`。
- This would allow native OpenSpec issue-backed workflows, but it is a larger architectural change。

## Initial SPEC Suggestions For The New Project

These can become initial `SPEC` comments.

### SPEC-001: Typed Comment Upsert

The CLI MUST create or update typed issue comments by `(Type, ID)` hidden marker, without creating duplicate logical artifacts.

Scenarios:

- **WHEN** no matching marker exists
- **THEN** the CLI MUST create a new issue comment

- **WHEN** a matching marker exists
- **THEN** the CLI MUST update that comment in place

### SPEC-002: Typed Comment Parsing

The CLI MUST parse hidden markers, visible headers, status, scope, and links from issue comments.

Scenarios:

- **WHEN** marker and header agree
- **THEN** the comment is valid

- **WHEN** marker and header conflict
- **THEN** the CLI MUST report a validation error

### SPEC-003: Bidirectional Link Verification

The CLI MUST verify bidirectional links between SPEC, TASK, and PROCESS comments.

Scenarios:

- **WHEN** TASK links SPEC and SPEC backlinks TASK
- **THEN** verify-links passes for that pair

- **WHEN** TASK links SPEC but SPEC does not backlink TASK
- **THEN** verify-links MUST report a missing backlink

### SPEC-004: Change Status Rendering

The CLI MUST render proposal/design/implement workflow status from GitHub issues and typed comments.

Scenarios:

- **WHEN** blocking QUESTION is open
- **THEN** status MUST show the next dependent stage as blocked

- **WHEN** all required comments are confirmed and verified
- **THEN** status MUST show ready for final review

### SPEC-005: Verification Gate

The CLI MUST block final ready verdict when required evidence is missing.

Scenarios:

- **WHEN** there is an open P0/P1 review finding
- **THEN** verify MUST not return ready

- **WHEN** worker rationale comments are missing
- **THEN** verify MUST report a human-review blocker

### SPEC-006: Built-in GitHub Authentication

The CLI MUST provide first-class GitHub authentication through `issue-spec auth` without requiring standalone `gh` installation for normal use.

Scenarios:

- **WHEN** a token is provided through `ISSUE_SPEC_TOKEN`, `GH_TOKEN`, or `GITHUB_TOKEN`
- **THEN** `issue-spec auth status` MUST report the active token source and authenticated user if the token is valid

- **WHEN** the user runs `issue-spec auth login --with-token`
- **THEN** the CLI MUST read the token from stdin, validate it with GitHub, persist it securely when possible, and avoid printing the token

- **WHEN** the user runs `issue-spec auth logout`
- **THEN** the CLI MUST remove persisted issue-spec credentials without modifying environment variables

## Initial TASK Suggestions

These can become initial `TASK` comments.

### TASK-001: Repository Skeleton

Links: `SPEC-001`, `SPEC-002`, `SPEC-003`, `SPEC-004`, `SPEC-005`, `SPEC-006`

- [ ] Create Go module skeleton。
- [ ] Add `cmd/issue-spec/main.go` CLI entry。
- [ ] Add test runner。
- [ ] Add build and smoke-test commands。
- [ ] Add README with dogfood workflow。

### TASK-002: Typed Comment Model

Links: `SPEC-001`, `SPEC-002`

- [ ] Implement marker parser。
- [ ] Implement header parser。
- [ ] Implement typed comment renderer。
- [ ] Implement validation for marker/header mismatch。
- [ ] Add unit tests。

### TASK-003: Link Model

Links: `SPEC-003`

- [ ] Implement Links block parser。
- [ ] Implement idempotent add link。
- [ ] Implement backlink verification。
- [ ] Add unit tests。

### TASK-004: Auth And GitHub Adapter

Links: `SPEC-001`, `SPEC-004`, `SPEC-006`

- [ ] Implement token source priority: `ISSUE_SPEC_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, persisted issue-spec credential。
- [ ] Implement `issue-spec auth status`。
- [ ] Implement `issue-spec auth login --with-token`。
- [ ] Implement `issue-spec auth logout`。
- [ ] Implement `issue-spec auth token`。
- [ ] Implement GitHub REST client without shelling out to standalone `gh`。
- [ ] Implement issue create。
- [ ] Implement comment list。
- [ ] Implement comment create/update。
- [ ] Add mocked tests。

### TASK-005: Status And Verify-links Commands

Links: `SPEC-003`, `SPEC-004`, `SPEC-005`

- [ ] Implement `issue-spec status`。
- [ ] Implement `issue-spec verify-links`。
- [ ] Add fixture-based tests。
- [ ] Add JSON output contract。

## Handoff Checklist For A Fresh Agent

Before starting implementation, a fresh agent should:

1. Read this document completely。
2. Read the current `proposal.md` in the Higress repo if available。
3. Inspect `/Users/zhangty/work/istio/openspec/schemas/istio-agent-workflow/schema.yaml` for the reference artifact DAG。
4. Inspect `/Users/zhangty/git/openspec/src/core/artifact-graph` for reusable graph concepts。
5. Confirm GitHub owner/repo name with the user。
6. Inspect `/Users/zhangty/git/cli/go.mod` and prefer stable public `github.com/cli/go-gh/v2` packages where they fit。
7. Confirm Go implementation and built-in `issue-spec auth` scope unless the user changes preference。
8. Create proposal/design/implement issues manually with `gh` during bootstrap only。
9. Only then scaffold code。

## Decision Summary

- Bootstrap is feasible without CLI。
- Use agent + `gh` to dogfood the issue-native workflow。
- Build a small independent Go `issue-spec` CLI first。
- Implement first-class `issue-spec auth` in the MVP, with env-token support and persisted login。
- The final normal user path must not require standalone `gh` installation。
- Do not try to make OpenSpec CLI store active artifacts in GitHub issues in the MVP。
- Keep OpenSpec as workflow/template/parser inspiration, and optionally as a future library dependency after public exports are stable。
