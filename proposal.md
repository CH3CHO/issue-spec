# Issue-native OpenSpec Workflow Proposal

## Metadata

- **Change Name**: `issue-native-openspec-workflow`
- **External Issue ID**: N/A
- **Reference Workflow**: `/Users/zhangty/work/istio` OpenSpec workflow
- **Primary Goal**: 对标当前 Istio 仓库 OpenSpec 规范完善程度，但将 active change spec 从本地 `openspec/changes/<change>/...` 文件改为 GitHub issue body 和 typed issue comments。

## Background

当前 Istio 仓库的 OpenSpec 机制已经形成较完整的 spec-first agent 工作流：

- `openspec/schemas/istio-agent-workflow/schema.yaml` 定义 artifact DAG：`proposal -> specs/questions -> design -> tasks -> review -> verify`，`apply` 依赖 `tasks/review/verify`。
- `openspec/config.yaml` 定义项目上下文、语言规则、代码标记规则、测试命令、durable spec 生命周期和每个 artifact 的项目级规则。
- `openspec/schemas/istio-agent-workflow/templates/*.md` 定义 proposal、spec、questions、design、tasks、review、verify 的结构模板。
- `.codex/skills` 和 `.agents/skills` 将 proposal、questions、design、tasks、apply、review、verify、archive 等阶段拆成可独立调用的 agent workflow。
- `openspec status --json` 和 `openspec instructions <artifact> --json` 能提供机器可读的阶段状态、依赖、模板和规则。

这套机制的强点是需求、设计、任务、实现、review、verify 和 durable spec 之间有明确阶段门禁。但原生 OpenSpec 以文件为 active change storage，会在仓库内写入：

```text
openspec/changes/<change>/proposal.md
openspec/changes/<change>/questions.md
openspec/changes/<change>/design.md
openspec/changes/<change>/tasks.md
openspec/changes/<change>/review.md
openspec/changes/<change>/verify.md
openspec/changes/<change>/specs/<capability>/spec.md
```

本项目希望保留同等规范强度和 agent 可执行性，但 active change artifacts 不进入仓库，而是落到 GitHub issues 和 issue comments。仓库只持久化 merge 后的 durable specs。

## Goals

- 建立一套 issue-native change spec 工作流，对标 Istio OpenSpec 的阶段 DAG、模板、质量门和 archive 语义。
- 使用三类 GitHub issue 承载 active change：
  - proposal issue: proposal body + `SPEC` / `QUESTION` comments。
  - design issue: design body + `TASK` / `QUESTION` comments。
  - implement issue: multi-agent DAG body + `PROCESS` / `QUESTION` / `REVIEW` / `VERIFY` comments。
- 保留 spec-first 原则：初始 `SPEC` 必须在 design 前产生，blocking `QUESTION` 必须在 design/tasks 前收敛或显式接受为假设。
- 支持 Coordinator、worker agent、review agent 通过同一个 GitHub 账号协作，但每条 comment 必须声明 agent identity、type、scope、status 和 links。
- 维护 `SPEC <-> TASK <-> PROCESS <-> PR` 的双向 traceability，最终 verify 阶段必须检查链路一致性。
- 支持简单任务的单 PR 模式，也支持复杂任务的 multi-PR DAG、独立 worktree、PR-owner agent 和 per-PR review agent。
- 在 PR review 阶段，review agent 使用 GitHub PR line comments 记录问题，Coordinator 分派给 owner worker，worker 修复后回复原 review thread。
- 在进入最终人工 review 前，负责功能实现的 worker agent 必须在关键代码行添加 inline rationale comments，解释修改原因并链接相关 `SPEC` comments。
- 在实现 PR merge 后，另起 durable-spec PR，将最终行为契约持久化到仓库 durable spec，并在需要长期语义指针的代码处引用 durable spec 条目。
- 提供足够的机器可读结构，便于后续实现 Go 单二进制 `issue-spec` CLI，自动创建/更新 issue、comments、双向链接、状态和校验报告。

## Non-Goals

- 不把 active change artifacts 默认写入仓库的 `openspec/changes/<change>/...`。
- 不要求 OpenSpec CLI 原生承担 GitHub issue/comment storage。
- 不为所有任务强制拆分多个 PR；只有功能完全解耦、review 可独立完成时才使用 multi-PR DAG。
- 不用 token 量预测作为 agent 拆分依据；拆分依据是预估改动文件、模块、测试面和 review 大小。
- 不替代最终人工 review 和人工产品决策。
- 不在本 proposal 中交付完整 CLI 代码、GitHub App、bot service 或 durable spec 目录结构；但本 proposal 会约束未来 `issue-spec` CLI 的用户语义、auth 能力和 traceability 行为。

## Key Constraints

- Core narrative 使用中文；保留 artifact type、status、MUST、SHALL、Requirement、Scenario、WHEN、THEN、PR、CI、DCO 等保留词和工程符号原文。
- Active change issue/comment 必须可被人直接 review，也必须可被脚本稳定解析。
- Issue body 和 typed comments 采用原地更新为主；每次关键更新后，Coordinator 必须回复一条变更摘要，保留人工可读审计线索。
- Blocking `QUESTION` 未解决前，不得进入依赖该答案的后续阶段。
- Worker 和 review agent 使用同一 GitHub 账号时，comment header 是身份边界，不得省略。
- GitHub comment URL 或 comment id 是 traceability 的稳定锚点。任何自动化都必须避免重复创建同一 logical artifact。
- PR inline comments 必须使用合法 diff line，通过 GitHub API 创建，且要避免重复 explanatory comments。
- Durable specs 是长期行为契约；proposal/design/task/review/verify 的过程记录不应混入 durable spec，除非后续明确扩展 durable spec schema。
- Bootstrap 阶段在 `issue-spec` CLI 尚不存在时 MAY 使用本机 `gh` 手工创建 repo、issue、comment 和 PR；CLI 完成后，正常用户路径 MUST NOT 依赖 standalone `gh` 安装。
- `issue-spec` CLI SHOULD 使用 Go 实现为单二进制，并通过内置 `issue-spec auth` 管理 GitHub 认证。
- `issue-spec` CLI MAY 参考 `/Users/zhangty/git/cli` 和 `github.com/cli/go-gh/v2` 的公开包能力，但 MUST NOT import `github.com/cli/cli/v2/internal/...`。

## Requirement Summary

### Requirement: Workflow MUST Preserve OpenSpec Artifact Gates

issue-native 工作流 MUST 保留 OpenSpec 的阶段依赖和 gate：

- proposal 可先创建。
- `SPEC` comments MUST 在 design 前创建，并包含可测试场景。
- `QUESTION` comments MUST 在 design/tasks 前收敛，或者被人明确接受为假设。
- design issue MUST 在 proposal/spec/question gate 后创建。
- `TASK` comments MUST 在 design 之后创建，并从 `SPEC` scenarios 推导测试设计和实现任务。
- implement issue MUST 在 design/task gate 后创建。
- review 和 verify MUST 在实现过程中持续维护，并在最终交付前通过 gate。

#### Scenario: Blocking question exists before design

- **WHEN** proposal issue 或 `SPEC` comment 仍有关联 open blocking `QUESTION`
- **THEN** Coordinator MUST 阻止 design issue 进入 confirmed 状态，并等待人类答复或显式接受默认假设

#### Scenario: Implementation exposes requirement flaw

- **WHEN** worker 或 review agent 发现实现无法满足现有 `SPEC`
- **THEN** Coordinator MUST 回溯更新 proposal issue、相关 `SPEC` comment、design issue 和 `TASK` comment，再继续实现

### Requirement: Active Change Artifacts MUST Use Three Issue Classes

每个复杂 change SHOULD 使用三类 issue 承载 active artifacts：

1. proposal issue
   - Issue body 是 proposal。
   - 每个需求是独立 `SPEC` comment。
   - 阻塞或非阻塞问题使用 `QUESTION` comment。

2. design issue
   - Issue body 是整体设计。
   - 每个任务是独立 `TASK` comment。
   - `TASK` comment MUST 链接相关 `SPEC` comment。

3. implement issue
   - Issue body 是 multi-agent implementation/review DAG。
   - 每个 agent、worker scope 或 PR node 是独立 `PROCESS` comment。
   - review 总结和 final verification 可以使用 `REVIEW` / `VERIFY` comments。

轻量 change MAY 省略独立 implement issue，将 implement coordination 放在 design issue 下，但必须保留 typed comments 和 traceability。

### Requirement: Typed Comments MUST Be Structured And Idempotent

每个 typed comment MUST 包含稳定 header，供人和自动化读取：

```text
Agent: <Coordinator | Worker Agent A | Review Agent B>
Type: <SPEC | TASK | PROCESS | QUESTION | REVIEW | VERIFY>
ID: <SPEC-001 | TASK-001 | PROCESS-001 | QUESTION-001 | REVIEW-001 | VERIFY-001>
Status: <draft | blocked | confirmed | in-progress | ready | done | superseded>
Scope: <owned behavior or module>
Links:
- Proposal Issue: <url>
- Design Issue: <url>
- Implement Issue: <url>
- Related Comments: <urls>
- PR: <url or N/A>
```

Typed comment body SHOULD include a hidden machine marker, for example:

```text
<!-- issue-spec:type=SPEC id=SPEC-001 version=1 -->
```

The marker allows `issue-spec` CLI tools to update comments in place instead of appending duplicates.

### Requirement: SPEC Comments MUST Model Testable Behavior

`SPEC` comments MUST preserve the OpenSpec spec discipline:

- Requirement statements MUST use SHALL or MUST。
- 每个 Requirement MUST 至少包含一个 Scenario。
- Scenario MUST 使用 `WHEN` / `THEN` 表达可验证条件。
- 行为不明确时，MUST 记录默认假设，并链接或创建 `QUESTION` comment。
- `SPEC` comments 描述行为和验收条件，不描述实现调用链，除非调用链本身是契约。

### Requirement: TASK Comments MUST Trace To SPEC Scenarios

`TASK` comments MUST 从 proposal、`SPEC` comments、resolved `QUESTION` comments 和 design issue 推导，且 MUST 包含：

- Test Case Design。
- Implementation。
- Unit Test Implementation。
- Integration Test Implementation。
- Review and Fixup。
- Verification。

每个 task item MUST 是 checkbox，并带稳定编号。测试设计 MUST 先于实现任务完成。每个 `TASK` comment MUST 链接覆盖的 `SPEC` comment URLs，并且相关 `SPEC` comments MUST 增加 backlink。

### Requirement: PROCESS Comments MUST Define Agent Ownership

`PROCESS` comments MUST 记录 agent 或 PR node 的执行边界：

- Owner agent。
- PR-owner agent when applicable。
- Review agent when applicable。
- Branch and worktree。
- Base branch or parent PR。
- Dependencies。
- Owned files/functions。
- Expected tests。
- Estimated diff size。
- Linked `TASK` comments。
- Current status and blockers。

Coordinator MUST 先确认 PROCESS DAG，再启动 worker agents。worker 遇到语义问题时 MUST 发布或更新 `QUESTION` comment，并将自己的 `PROCESS` status 置为 blocked。

### Requirement: Traceability MUST Be Bidirectional And Verifiable

系统 MUST 维护以下双向链接：

- `SPEC` comment links to related `TASK` comments。
- `TASK` comment links back to related `SPEC` comments。
- `TASK` comment links to related `PROCESS` comments。
- `PROCESS` comment links back to related `TASK` comments。
- PR inline rationale comments link to related `SPEC` comments。
- durable spec entries link to proposal issue and later proposal issues that updated the behavior。

Final verify MUST 检查这些链接不存在断链、遗漏、冲突或实现与 `SPEC` 不一致的问题。

### Requirement: Review MUST Use PR Line Comments For Code Findings

Review agent MUST 在 PR diff 的具体代码行上评论代码问题。Coordinator MUST 拉取 PR review comments、PR issue comments、review decision 和 checks，区分：

- worker explanatory comments。
- actionable human/review-agent findings。
- CI/check failures。
- unresolved review threads。

每个 actionable finding MUST 分派给对应 PROCESS owner。worker 修复后 MUST 回复原 review thread，并更新 `PROCESS` comment。

### Requirement: Verify MUST Record Evidence And Coverage

`VERIFY` comment 或 implement issue verify section MUST 包含：

- Exit criteria。
- Unit test command evidence。
- Integration test evidence or blocker。
- Generated-code/lint/precommit evidence when applicable。
- Skipped expensive checks with explicit reasons。
- Requirement/scenario coverage table。
- Final verdict。

没有证据或 blocker 的任务不得标记为 verified。P0/P1 review finding 未关闭时不得进入 archive/durable spec 阶段。

### Requirement: Durable Specs MUST Be Archived After Merge In A Separate PR

实现 PR merge 后，Coordinator MUST 创建单独 durable-spec PR，将最终行为契约写入仓库 durable spec。Durable spec MUST：

- 使用最终格式，而不是 delta-only headings。
- 包含 proposal issue URL。
- 在后续行为更新时追加新的 proposal issue URL。
- 只记录长期行为契约，不混入 design、task、review、verify 过程记录。

当实现代码需要长期行为指针时，worker MUST 将临时 issue comment 引用更新为 durable spec 条目引用。

### Requirement: Automation MUST Provide A GitHub-backed `issue-spec` CLI

OpenSpec CLI 可以复用 artifact DAG、templates 和 phase instructions，但不应作为 issue-native active storage。后续 MUST 提供 Go 单二进制 `issue-spec` CLI 作为 GitHub-backed issue-native workflow CLI，至少支持：

- 内置 GitHub auth，不要求用户预先安装 standalone `gh`。
- 创建 proposal/design/implement issues。
- 创建或更新 typed comments。
- 根据 hidden marker 和 comment id 做 idempotent upsert。
- 添加和校验双向 links。
- 渲染 workflow status。
- 检查 unresolved `QUESTION` / P0/P1 review findings / CI failures。
- 校验 SPEC/TASK/PROCESS/PR/durable spec traceability。
- 从 issue-native artifacts 生成 durable spec PR 草稿。

CLI MVP MUST 包含以下命令组：

```bash
issue-spec auth status
issue-spec auth login --with-token
issue-spec auth logout
issue-spec auth token

issue-spec init --repo owner/repo
issue-spec issue create proposal --repo owner/repo --change <change-name>
issue-spec issue create design --repo owner/repo --change <change-name> --proposal <issue>
issue-spec issue create implement --repo owner/repo --change <change-name> --design <issue>

issue-spec comment upsert --repo owner/repo --issue <issue> --type SPEC --id SPEC-001 --body-file spec.md
issue-spec comment list --repo owner/repo --issue <issue> --type SPEC --json
issue-spec link --repo owner/repo --from SPEC-001 --from-issue <proposal-issue> --to TASK-001 --to-issue <design-issue>

issue-spec status --repo owner/repo --proposal <issue> --design <issue> --implement <issue>
issue-spec verify-links --repo owner/repo --proposal <issue> --design <issue> --implement <issue>
```

`issue-spec auth` MUST 支持以下 token source 优先级：

1. `ISSUE_SPEC_TOKEN`。
2. `GH_TOKEN`。
3. `GITHUB_TOKEN`。
4. issue-spec 自己持久化的 credential store。

`issue-spec auth login --with-token` MUST 从 stdin 读取 token，调用 GitHub `GET /user` 校验 token，并优先保存到 OS keyring。若需要明文/config fallback，MUST 要求显式 flag 并在 human output 中警告。环境变量 token MUST 不被写入持久化 credential store。

GitHub API 调用 SHOULD 通过 Go HTTP client 或 `github.com/cli/go-gh/v2` 的稳定公开包完成。`issue-spec` MUST 将 GitHub client 包装在自身 `internal/github.Client` 边界内，避免未来替换 API 库时影响命令和业务模型。

## Proposed Issue Types

### Proposal Issue

Issue body SHOULD follow proposal template:

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

`SPEC` comments are the source of truth for behavior requirements during active change.

### Design Issue

Issue body SHOULD follow design template:

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

`TASK` comments are the source of truth for executable work items.

### Implement Issue

Issue body SHOULD include:

- PR mode decision: single integrated PR or multi-PR DAG。
- DAG nodes and dependencies。
- Worktree/branch plan。
- PR-owner and review-agent assignment。
- Conflict risk and serialization plan。
- Global review/verify status。

`PROCESS` comments are the source of truth for agent execution status.

## Skill And Agent Requirements

对标 Istio 的阶段 skill 机制，本项目 SHOULD 保留一个 Coordinator skill 作为入口，并在流程稳定后按阶段拆分：

- proposal/spec skill。
- questions skill。
- design/task skill。
- apply/process skill。
- review skill。
- verify skill。
- archive/durable-spec skill。

拆分条件不是任务大小本身，而是某个阶段已经稳定、可独立调用、且单一 skill 内容过长或会让 agent 上下文过宽。

## CLI Reuse Assessment

OpenSpec CLI 可复用的部分：

- `openspec status --json` 暴露 artifact DAG 和 gate 状态模型。
- `openspec instructions <artifact> --json` 暴露 artifact instruction、rules、template、dependencies 和 unlocks。
- `openspec validate` 可用于 durable spec 或临时 draft 校验。

OpenSpec CLI 不可直接承担的部分：

- 它默认管理 file-backed change directory。
- 它不维护 GitHub issue body/comment ids。
- 它不维护 typed comment upsert、双向链接、review thread 分派、PR line comments 和 multi-agent process status。

因此，本机制 SHOULD 将 OpenSpec CLI 视为 schema/template/instruction provider，而不是 issue-native storage engine。真正的 active state management SHOULD 由 GitHub-backed `issue-spec` CLI 实现。

GitHub CLI 相关代码的复用边界：

- `/Users/zhangty/git/cli` 可作为 GitHub auth、API client、scope 检测、host 配置和 UX 行为的参考实现。
- `github.com/cli/go-gh/v2` 的稳定公开包 MAY 被 `issue-spec` CLI 直接依赖。
- `github.com/cli/cli/v2/internal/...` MUST NOT 被新项目 import；这些 internal 包在 Go 模块边界外不可用，也不应成为长期公共 API 假设。
- standalone `gh` CLI MAY 在 bootstrap 阶段由 agent 手工调用，但 MUST NOT 是最终 `issue-spec` CLI 的运行时依赖。

推荐实现方向：

- 使用 Go 实现 `issue-spec`，分发为单 binary。
- 在 MVP 中实现 `issue-spec auth`，先支持 env token 和 `--with-token` 登录，后续再支持 web/device flow。
- 将 auth、GitHub REST client、typed comment model、link verifier、status renderer 分成独立包，便于 worker agent 并行开发和 review。

## Open Questions

### Blocking Questions

- [ ] Durable spec 在 Higress 仓库中的目录和格式是否沿用 `openspec/specs/<capability>/spec.md`？
- [ ] 三类 issue 是否需要固定 labels，例如 `issue-spec/proposal`、`issue-spec/design`、`issue-spec/implement`？
- [ ] 是否允许轻量 change 只创建一个 issue，但仍使用 typed comments 表达 `SPEC` / `TASK` / `PROCESS`？
- [ ] Typed comment 的 hidden marker 格式是否需要与未来 CLI schema 先行固定？
- [ ] Durable spec PR 是否由 Coordinator 创建，还是由专门 archive agent 创建？

### Non-Blocking Questions

- [ ] 是否需要 GitHub Projects 或 milestone 承载跨 issue 状态看板？
- [ ] 是否需要把 issue-native artifacts 导出成本地 markdown snapshot 供离线审计？
- [ ] 是否需要在 PR merge 后自动关闭三类 issue，还是保持 open 直到 durable-spec PR merge？
- [ ] 是否需要把 OpenSpec CLI 的 schema/templates vendored 到本仓库，还是只在 skill 中引用外部规范？
- [ ] `issue-spec auth` 是否在 MVP 就支持 GitHub Enterprise hostname，还是先实现 `github.com` 并保留 host abstraction？

## Capabilities

### New Capabilities

- `issue-native-change-spec`: 使用 GitHub issue/comment 承载 active proposal/spec/design/task/question/review/verify。
- `multi-agent-process-traceability`: 使用 PROCESS comments 管理 worker/review agent 的状态、职责、DAG 和 PR 关系。
- `durable-spec-archive-pr`: 在实现 PR merge 后，以单独 PR 持久化最终 spec。
- `issue-spec-cli`: 使用 Go 单二进制和内置 auth 管理 issue-native workflow 的创建、更新、链接、状态和校验。

### Modified Capabilities

- `multi-agent-pr-coordination`: 从简单 coordination issue 扩展为 proposal/design/implement 三 issue 流，并增加 OpenSpec-like gates。
- `pr-review-explanation`: 从一般 inline rationale comments 扩展为必须链接 `SPEC` comments 和 durable spec 条目。

## Impact

### Affected Areas

- Project-local skills under `skills/`。
- Future Go `issue-spec` CLI。
- GitHub issue/PR operating conventions。
- Durable spec directory and archive process。

### Expected Benefits

- active change 过程不污染仓库。
- 人类可以直接在 issue/comment 上参与需求、设计和 review。
- agent 可通过 typed comments 保持上下文同步。
- `issue-spec auth` 让 agent、CI 和本地用户在不安装 standalone `gh` 的情况下使用同一套 workflow。
- merge 后仍保留 durable spec，避免长期行为契约只存在 issue 历史中。
- review/verify 可追溯到每条 requirement/scenario。

### Risks

- GitHub issue comments 比本地文件更难做批量重构和 diff review。
- comment 原地更新需要可靠的 idempotent tooling，否则容易产生重复或断链。
- 三类 issue 可能增加简单任务的流程成本。
- CLI 自行管理 GitHub auth 后，需要正确处理 token 存储、scope、host 和错误脱敏。
- 如果没有 durable spec archive gate，行为契约可能停留在 issue 中而未进入仓库。

## Acceptance Criteria

- 能从一个用户需求创建 proposal/design/implement 三类 issue，并形成完整 `SPEC` / `TASK` / `PROCESS` 链路。
- Blocking `QUESTION` 能阻止后续阶段推进。
- `issue-spec auth status/login --with-token/logout/token` 的 MVP 语义明确，且正常 CLI 路径不依赖 standalone `gh`。
- `issue-spec` CLI 能 idempotently 创建或更新 typed comments，并能维护/校验双向 links。
- Multi-agent implement DAG 能明确并行与串行关系、worktree/branch/PR owner/review owner。
- Review findings 能通过 PR line comments 分派、修复、回复和复核。
- Verify 阶段能证明每个 requirement/scenario 有测试、手工验证或明确 blocker。
- Final human review 前，关键代码行有 worker rationale comments 并链接相关 `SPEC` comments。
- Merge 后能创建 durable-spec PR，durable spec 包含 proposal issue URL，并且需要长期语义引用的代码指向 durable spec。
