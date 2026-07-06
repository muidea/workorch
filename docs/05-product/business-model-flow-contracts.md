# WorkOrch Business Model, Flow Contracts, And Module Interaction

Status: active

Type: business-model-flow-contracts

Last Updated: 2026-07-01

## 1. Purpose

本文整理 WorkOrch 的业务模型、业务处理流程、流程间交互合同和功能模块交互。

本文是产品/业务视角的汇总，不替代字段级、流程级、模块级权威文档：

- 对象字段权威：`../10-domain-model/`
- 流程权威：`../20-runtime-flow/`
- 模块权威：`../30-module-architecture/`
- API 权威：`../50-product-interface/service-api.md`

## 2. Business Object Map

### 2.1 Workspace Definition Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Workspace | 顶层业务容器，绑定交付目录、内部目录、当前 active run 和上下文 | kernel/workspace | has Goal, DoD, Run, Soul binding, Memory |
| Goal | 当前要达成的目标正式定义 | kernel/workspace | belongs to Workspace, referenced by Run/Result/Coverage |
| DoD | 完成定义和验收条目正式定义 | kernel/workspace | belongs to Workspace, referenced by Run/Result/Coverage |
| DefinitionRevision | Goal/DoD 的有效版本边界 | kernel/workspace + run ledger | binds Run, CoverageWorkItem, StagePacket |

合同：

- activation 必须绑定 effective Goal + DoD + definitionRevision。
- 新定义是 cumulative latest，除显式冲突外不能弱化旧约束。
- superseded/historical definition 只能作为 lineage，不得继续作为 active definition。

### 2.2 Runtime Process Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Run | 一次围绕当前 definition 的正式执行实例 | kernel/run | has Steps, CoverageWorkItems, evidence, Result |
| Step | Run 的基本执行单元 | kernel/run | has input/output, references runtimeStage/outputMode |
| ToolCall | 外部动作过程证据 | kernel/run | belongs to Step/Run, may support Result |
| Patch | state-changing 变更证据 | kernel/run | belongs to Step/Run, may support Artifact/Result |
| ValidationResult | 验证证据 | kernel/run | belongs to Step/Run, may support DoD assessment |
| RuntimeCheckpoint | 可恢复诊断快照 | kernel/run | belongs to Run/Step |
| CoverageWorkItem | 当前 definition 下的 coverage ledger item | kernel/run | references Goal/DoD/deliverable requirement and evidence |
| ArtifactContinuation | incremental artifact 的内部 continuation | kernel/run/orchestrator | references Step/Artifact/targetRef |

合同：

- process evidence 不等于 accepted business outcome。
- CoverageWorkItem 是 run-scoped ledger，不是新的 Goal/DoD。
- ArtifactContinuation 是 runtime internal state，不是 workspace deliverable。

### 2.3 Outcome Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Result | 正式业务结果 | kernel/result | references Workspace/Run/Goal/DoD/evidence/artifacts |
| Artifact | 结构化产物或证据产物 | kernel/result + storage/read-model | may bind runId, stepId, resultId |
| Outcome | workspace/run 级读模型 outcome | read-model projection | derived from Result/Run/evidence |
| TaskProgress | 当前进展与缺口投影 | read-model projection | derived from active run and coverage ledger |

合同：

- accepted Result 必须引用 current definition 和 accepted evidence。
- Artifact 通过 logical URI/ref 暴露，不暴露本地绝对路径。
- Outcome/TaskProgress 是投影，不反向覆盖 owner state。

### 2.4 Governance Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Decision Ticket | runtime 无法自动推进时的结构化决策对象 | kernel/decision + governance | references Workspace/Run/Step |
| Resolution | 对 Decision Ticket 的处理结果 | kernel/decision | maps to resume/replan/terminate |
| Approval | 高风险或策略要求动作的审批对象 | kernel/approval/governance | references Workspace/Run/Step/action |
| AutoGovernance | duplicate failure / repair exhausted / policy gate 的治理建议 | kernel/governance | may create ticket/approval/resolution |

合同：

- decision/approval 只能因真实治理缺口产生。
- resolution 只能补充恢复上下文，不能覆盖原 step stage contract。
- approval 只表达允许/拒绝继续，不表达业务结果 accepted。

### 2.5 Context And Capability Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Soul | 阶段约束、能力边界、风险策略 | kernel/soul + profile data | selected by Workspace/Run |
| Skill | 可复用任务指导单元 | kernel/skill / skill-hub integration | selected into executionContext |
| MCPServer | 外部能力提供者 | kernel/mcp | has capabilities |
| MCPCapability | tool/resource/prompt 能力描述 | kernel/mcp | selected by Soul/profile/run |
| Memory | 高价值可复用上下文 | kernel/memory | recalled into Run/Step context |
| MemoryRecallTrace | recall 过程证据 | kernel/memory/run evidence | references selected/candidate/excluded memory |

合同：

- Soul/Skill/MCP/Memory 进入 run input 必须可追溯。
- Memory 不是事件日志，不沉淀低价值 retry/diagnostic 流水。
- capability policy 是结构化数据，core 不解析业务语义。

### 2.6 Event And Projection Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| EventRecord | 持久化正式事件 | kernel/eventstream | references workspace/run/step/subject |
| EventEnvelope | runtime event hub 消息 | internal/pkg/events | command/result/notify carrier |
| WorkspaceProjection | workspace list/detail ViewModel | workorchd read-model | derived from owner state + events |
| RunSnapshot | run detail projection | workorchd read-model | derived from run/result/evidence/events |
| RunObservability | timeline/evidence/checkpoint projection | workorchd read-model | derived from run evidence/events |

合同：

- EventRecord 用于审计、回放、观察，不是唯一 truth source。
- Projection 是展示权威，不是业务 owner。
- 大 payload 必须 summary/ref 化。

## 3. State Model Contract

### 3.1 Core States

| State Family | Values / Direction | Owner |
| --- | --- | --- |
| RuntimeStage | exploration, planning, execution, verification, acceptance, repair, replan, decision, approval | orchestrator stage contract |
| RunState | pending, active, blocked_on_decision, blocked_on_approval, completed, failed, cancelled, superseded | kernel/run |
| WorkspaceState | active, blocked_on_approval, superseded | kernel/workspace owner; projection may derive prompt/progress from active run |
| ConvergenceContract.State | need_evidence, need_state_change, need_validation, need_assessment, need_governance, blocked | orchestrator |
| Completion | DeterministicComplete + CompletionReason | orchestrator |
| StepState | pending, executing, succeeded, failed, skipped, cancelled | kernel/run |
| ResultState | accepted/rejected/superseded-style outcome state | kernel/result |

### 3.2 State Bridge

Allowed bridge:

```text
RunState
  -> ConvergenceContract
  -> RuntimeStage + RequiredOutput
  -> OutputMode / StagePacket
  -> LLM candidate or block execution
  -> Contract gate / Evidence recorder / Governance gate
  -> RunState / ResultState / Projection
```

Forbidden bridge:

- `outputMode` must not overwrite `ConvergenceContract.State`。
- `runtimeStage=decision` must not imply `RunState=blocked_on_decision` until ticket is created and run is updated。
- LLM `completionState=completed` must not create accepted Result without evidence/DoD gates。
- Web/TUI prompt state must not override backend projection。

## 4. Main Business Flows

### 4.1 Workspace Setup Flow

```text
User input
  -> workspace setup draft or direct workspace create
  -> template/Soul/Skill/MCP/default policy selection
  -> Workspace created
  -> Goal / DoD defined
  -> directories created
  -> projection updated
```

Input contract:

- title/description/deliverableDirectory。
- optional templateId/defaultSoulId/stage bindings/skills/MCP/memory。

Output contract:

- Workspace with deliverableDirectory/internalDirectory。
- active Goal/DoD definitions if supplied。
- projection revision update。

### 4.2 Activation / Start Flow

```text
User start / activation command
  -> appservice validates workspace + Goal + DoD + bindings
  -> EventHub or owner service creates Run
  -> initial coverage baseline created
  -> executionContext assembled
  -> orchestrator dispatches first Step
  -> projection shows active execution or governed wait from run/ticket/approval state
```

Input contract:

- workspaceId。
- goalId/doDId or effective latest definition。
- Soul/Skill/MCP/Memory bindings。
- priority and operator input。

Output contract:

- Run with workspaceId/goalId/doDId/definitionRevision。
- CoverageWorkItem baseline。
- StagePacket for first dispatch。
- lightweight projection response。

### 4.3 Planning / Execution Flow

```text
Run active
  -> BuildConvergenceContract
  -> ApplyConvergenceContract
  -> StagePacket + requiredOutput
  -> LLM candidate output
  -> execution contract validation
  -> block graph dispatch or artifact continuation
  -> evidence recorded
  -> next action recomputed
```

Contracts:

- LLM candidate must match requiredOutput。
- State-changing output must bind target refs / coverage targets when required。
- Read-only graph cannot satisfy state-change requirement。
- Managed validation must use declared managed_session contract when required。

### 4.4 Verification / Acceptance Flow

```text
State change evidence available
  -> validation requirement computed
  -> validation block / managed validation executes
  -> ValidationResult recorded
  -> coverage ledger accepts/rejects evidence
  -> DoD assessment computed
  -> Result accepted/rejected or next repair/replan selected
```

Contracts:

- validation evidence must be concrete, scoped and accepted。
- claim-to-evidence alignment applies when profile requires it。
- accepted Result requires current definition + accepted evidence refs。
- failed assessment can be recoverable evidence gap rather than terminal failure。

### 4.5 Repair Flow

```text
Contract violation / failed validation / evidence gap
  -> rejected diagnostic evidence recorded
  -> repair budget and signature checked
  -> repairActionPackage built
  -> retry inherits original stage contract
  -> LLM generates targeted correction
  -> evidence gate repeats
```

Contracts:

- rejected output cannot have state-changing side effect。
- repair inherits runtimeStage, runtimeStageMode, requiredOutput and current focus。
- repeated same failure escalates to governance only after budget/policy conditions。

### 4.6 Replan Flow

```text
Decision resolution or runtime determines current plan invalid
  -> old run lineage captured
  -> successor run proposal or replacement activation
  -> current effective Goal/DoD retained
  -> new coverage baseline / carry-over evidence calculated
  -> successor run starts
```

Contracts:

- replan cannot drop active Goal/DoD constraints。
- successor run must carry lineage refs。
- old run terminal/superseded state must not remain activeRunId。

### 4.7 Decision / Approval Flow

```text
Runtime detects real governance need
  -> Decision Ticket or Approval created
  -> Run enters blocked_on_decision / blocked_on_approval
  -> projection shows waiting/blocker state
  -> user resolves or approves/rejects
  -> scoped continuation event submitted
  -> run resumes/replans/terminates
```

Contracts:

- ticket/approval must be scoped by workspaceId/runId/stepId。
- TUI/Web submit resolution, not state mutation。
- resolved governance must be reflected in owner state and projection。

### 4.8 Projection / Observation Flow

```text
Owner state or event changes
  -> event persisted / projection update scheduled
  -> workspace/run ViewModel rebuilt or patched
  -> SSE/event stream notifies clients
  -> Web/TUI display projection
```

Contracts:

- main views consume projection store。
- detail/raw views open bounded payload refs on demand。
- projection health/revision must be visible。

## 5. Cross-Flow Interaction Contracts

| From Flow | To Flow | Contract |
| --- | --- | --- |
| Workspace setup | Activation | effective Goal/DoD/template/Soul/Skill/MCP/Memory bindings |
| Activation | Execution | Run + definitionRevision + coverage baseline + executionContext |
| Execution | Verification | accepted state-changing evidence candidate + target refs |
| Verification | Acceptance | accepted validation evidence + coverage update |
| Acceptance | Result | DoD assessment + accepted evidence refs + artifact refs |
| Any runtime gate | Repair | rejected diagnostic evidence + repairActionPackage |
| Repair | Execution/Verification | inherited stage contract + narrowed requiredOutput |
| Governance | Continuation | Resolution/Approval result + scoped continuation action |
| Replan | Activation/Execution | successor run + lineage + current definition |
| Owner state | Projection | event/owner read + projection revision |
| Projection | Web/TUI | ViewModel only, no business state reconstruction |

## 6. Functional Module Interaction

### 6.1 Workspace / Run / Orchestrator

```text
workspace owner
  -> defines Workspace/Goal/DoD
run owner
  -> creates Run/Step/evidence objects
orchestrator
  -> reads Run/coverage/evidence
  -> dispatches next Step
  -> updates state through owner services/repositories
```

Key contract:

- Orchestrator may coordinate lifecycle but must not bypass owner object invariants。

### 6.2 Orchestrator / Blocks

```text
orchestrator
  -> StepExecutionRequest
blocks
  -> normalized result / error / raw ref
orchestrator
  -> contract validation / evidence recording / next action
```

Key contract:

- Blocks execute side effects; orchestrator interprets business meaning。

### 6.3 Appservice / Kernel

```text
HTTP/TUI command
  -> appservice validates request and scope
  -> EventHub command or owner service call
  -> kernel owner updates formal state
  -> appservice returns DTO/projection
```

Key contract:

- appservice composes use cases; kernel owns formal state。

### 6.4 Workorchd Projection / UI

```text
owner state + event records
  -> observed/readmodelprojection
  -> presenter/port DTO
  -> Web/TUI/API
```

Key contract:

- UI consumes DTO/projection and submits commands; it does not infer next action。

### 6.5 Profile / Runtime Core

```text
template/Soul/profile/capability
  -> structured policy fields
runtime core
  -> neutral interpretation
  -> stage/requiredOutput/evidence gate
```

Key contract:

- profile owns strategy; runtime core owns deterministic enforcement。

## 7. Data And Payload Contracts

| Data Kind | Default Projection | Detail / Raw Access |
| --- | --- | --- |
| Workspace / Run summary | compact fields + projection metadata | snapshot/detail endpoint |
| EventRecord | small payload or payloadSummary | payloadRef open endpoint |
| LLM request/response | usage/audit summary | runtime payload raw ref |
| Tool output | stdout/stderr preview + truncation flag | raw output ref |
| Patch/diff | affected files + stats + summary | raw diff/content ref |
| Artifact | logical URI + metadata | artifact content endpoint |
| Memory | title/type/summary/source refs | memory detail |

Constraints:

- default projection must be bounded。
- raw ref must be logical and scoped。
- truncated/partial content must not be treated as safe full file basis。

## 8. Consistency Checklist

Use this checklist when adding or changing product, flow, or module behavior:

- Does the change identify the owner object?
- Does every state-changing command carry workspace scope?
- Does the flow preserve current Goal / DoD definition revision?
- Does LLM-visible context include requiredOutput, current gap, accepted/rejected evidence and target snapshots?
- Does contract violation stay rejected until a valid retry succeeds?
- Does accepted Result cite accepted evidence refs?
- Does projection consume owner state rather than raw event guesses?
- Does Web/TUI avoid duplicating runtime decision logic?
- Does profile-specific behavior remain in template/Soul/profile/capability data?
- Does large payload use summary/ref rather than default inline expansion?

## 9. Known Boundary Decisions

- Web Console is not “read-only only”; it is “projection display + controlled operation entry”。It may create/update workspace resources and submit governance/configuration operations through formal API, but it does not own business state.
- Workspace owner state is intentionally small: `active`, `blocked_on_approval`, `superseded`。Decision blockers do not mutate workspace owner state; they are expressed by `RunState=blocked_on_decision`, open decision ticket, `blockingTicketId`, prompt mode and governance projection. Approval blockers use `WorkspaceState=blocked_on_approval` only when the owner has an active approval blocker.
- `validation/managed_session` is a runtime block kind owned by orchestrator/shell-backed managed validation flow and must be declared through central runtime vocabulary.
- `internal/pkg/events` may aggregate event contracts, but aggregation must stay contract-level and must not become business outcome logic.
