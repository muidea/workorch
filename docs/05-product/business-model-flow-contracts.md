# WorkOrch Business Model, Flow Contracts, And Module Interaction

Status: active

Type: business-model-flow-contracts

Last Updated: 2026-07-27

## 1. Purpose

本文整理 WorkOrch 的业务模型、业务处理流程、流程间交互合同和功能模块交互。

本文是产品/业务视角的汇总，不替代字段级、流程级、模块级权威文档：

- runtime core / Runtime V2 主链：`../00-core/runtime-stage-contract.md`
- 对象字段权威：`../10-domain-model/`
- 流程权威：`../20-runtime-flow/`
- 模块权威：`../30-module-architecture/`
- API 权威：`../50-product-interface/service-api.md`

**生产主线 = Runtime V2 only。** 中立 stage 语义用于产品/profile/LLM 意图；正式推进以 V2 facts / reducer / authority / candidate / sealed dispatch / evidence 为准。不得将 `StagePacket`、`NextAction`、graph dispatch、`ExecutionContextPatch`、自动 replan successor 表述为生产路径。

## 2. Business Object Map

### 2.1 Workspace Definition Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Workspace | 顶层业务容器，绑定交付目录、内部目录、当前 active run 和上下文 | blocks/workspace | has Goal, DoD, Run, Soul binding, Memory |
| Goal | 当前要达成的目标正式定义 | blocks/workspace | belongs to Workspace, referenced by Run/Result/Coverage |
| DoD | 完成定义和验收条目正式定义 | blocks/workspace | belongs to Workspace, referenced by Run/Result/Coverage |
| DefinitionRevision | Goal/DoD 的有效版本边界 | blocks/workspace + activation/bootstrap | binds Run, CoverageFocus / coverage items, RuntimeBootstrap.Definition |

合同：

- activation 必须绑定 effective Goal + DoD + definitionRevision。
- 新定义是 cumulative latest，除显式冲突外不能弱化旧约束。
- superseded/historical definition 只能作为 lineage，不得继续作为 active definition。
- 定义变更意图通过 **新** typed activation / 新 V2 Run 表达，不自动 replan successor。

### 2.2 Runtime Process Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Run | 一次围绕当前 definition 的正式执行实例 | blocks/run | has RuntimeBootstrap, V2 aggregate projection, process evidence, Result |
| RuntimeBootstrap | 创建时封存的 Definition + Activation + Focuses + Requirements | blocks/run（持久化）+ orchestrator（消费） | seals activation; not rebuilt from caller map |
| RuntimeFacts | drive 权威事实面（focus 队列、evidence ledger、authority 相关状态） | orchestrator aggregate | inputs to reducer |
| CoverageFocus | 有序 focus 队列元素；携带 coverage work items 与 evidence/assessment 要求 | orchestrator / bootstrap | advances after required evidence (and optional assessment) |
| EvidenceLedger | accepted/rejected operation evidence 与 lineage | orchestrator aggregate | binds sealed dispatch / operation |
| SealedDispatch | 执行前持久化的 dispatch 密封 | orchestrator | unique legal entry for operation evidence |
| Step | 过程执行单元的 **非权威投影 / legacy 过程面** | blocks/run storage + projection | may reference operation/evidence; **not** V2 scheduler authority |
| ToolCall | 外部动作过程证据 | blocks/run | belongs to Run/operation projection, may support Result |
| Patch | state-changing 变更证据 | blocks/run | belongs to Run/operation projection, may support Artifact/Result |
| ValidationResult | 验证证据 | blocks/run | may support DoD assessment |
| RuntimeCheckpoint | 可恢复 **诊断** 快照（非 V2 驱动源） | blocks/run | observability / audit |
| CoverageWorkItem | coverage ledger item（可嵌于 CoverageFocus） | run/orchestrator ledger | references Goal/DoD/deliverable requirement and evidence |
| ArtifactContinuation | incremental artifact 的内部 continuation | blocks/run/orchestrator | references operation/Artifact/targetRef |

合同：

- process evidence 不等于 accepted business outcome。
- CoverageWorkItem / CoverageFocus 是 run-scoped ledger/focus，不是新的 Goal/DoD。
- ArtifactContinuation 是 runtime internal state，不是 workspace deliverable。
- Step 若仍出现在 API/storage，只作过程证据与观测投影，不得包装为 `NextAction` / graph dispatch 权威。
- 恢复 = load 已持久化 V2 aggregate；不重放 V1 progress/terminal checkpoint 驱动。

### 2.3 Outcome Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Result | 正式业务结果 | blocks/result | references Workspace/Run/Goal/DoD/evidence/artifacts；承接 Orchestrator 裁决 |
| Artifact | 结构化产物或证据产物 | blocks/result + storage/read-model | may bind runId, stepId/operation ref, resultId |
| Outcome | workspace/run 级读模型 outcome | read-model projection | derived from Result/Run/evidence |
| TaskProgress | 当前进展与缺口投影 | read-model projection | derived from active run, focus, coverage ledger |

合同：

- accepted Result 必须引用 current definition 和 accepted evidence。
- Result 不裁决；Orchestrator 只裁决并密封 terminal payload，Result owner消费 immutable delivery fact 后写入。
- Artifact 通过 logical URI/ref 暴露，不暴露本地绝对路径。
- Outcome/TaskProgress 是投影，不反向覆盖 owner state。

### 2.4 Governance Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Decision Ticket | runtime 无法自动推进时的结构化决策对象 | blocks/decision + governance | opened by `request_governance`; binding in aggregate |
| Resolution | 对 Decision Ticket 的处理结果 | blocks/decision | commit via governance resolution path → reducer；maps continue/replan-semantic/terminate intent |
| Approval | 高风险或策略要求动作的审批对象 | blocks/approval/governance | references Workspace/Run/action |
| AutoGovernance | duplicate failure / repair exhausted / policy gate 的治理建议 | kernel/governance | may create ticket/approval suggestion；不得静默创建 successor Run |

合同：

- decision/approval 只能因真实治理缺口产生。
- `request_governance` → ticket → Orchestrator persisted governance fact → Run owner-local blocked transition。
- resolution commit 后 **application 不得** 再 Resume 才能驱动；继续走 runtime resolution commit / reducer。
- resolution 只能补充治理结果，不能覆盖原 focus / requiredOutput / stage 语义合同。
- approval 只表达允许/拒绝继续，不表达业务结果 accepted。
- replan 意图 = 新 activation / 新 Run，不是同 Run 自动 successor。

### 2.5 Context And Capability Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| Soul | 阶段约束、能力边界、风险策略 | blocks/soul + profile data | selected into Activation；stage policy → C2/activation 输入 |
| Skill | 可复用任务指导单元 | blocks/skill / skill-hub integration | selected into activation / candidate context |
| MCPServer | 外部能力提供者 | blocks/mcp-registry | has capabilities |
| MCPCapability | tool/resource/prompt 能力描述 | blocks/mcp-registry | selected by Soul/profile/activation |
| Memory | 高价值可复用上下文 | blocks/memory | recalled into candidate context |
| MemoryRecallTrace | recall 过程证据 | blocks/memory/run evidence | references selected/candidate/excluded memory |

合同：

- Soul/Skill/MCP/Memory 进入 run / candidate input 必须可追溯。
- Memory 不是事件日志，不沉淀低价值 retry/diagnostic 流水。
- capability / stage policy 是结构化数据，core 不解析业务语义，**不理解 profile ID**。
- 上下文只经 typed bootstrap / activation 与 candidate context（C0–C5），不经 `ExecutionContextPatch`。

### 2.6 Event And Projection Objects

| Object | Purpose | Owner | Key Relations |
| --- | --- | --- | --- |
| EventRecord | 持久化正式事件 | blocks/eventstream | references workspace/run/subject |
| EventEnvelope | runtime event hub 消息 | internal/pkg/events | command/result/notify carrier |
| WorkspaceProjection | workspace list/detail ViewModel | workorchd read-model | derived from owner state + events |
| RunSnapshot | run detail projection | workorchd read-model | derived from run/result/evidence/V2 state/events |
| RunObservability | timeline/evidence/checkpoint projection | workorchd read-model | derived from run evidence/events |

合同：

- EventRecord 用于审计、回放、观察，不是唯一 truth source。
- Projection 是展示权威，不是业务 owner。
- 大 payload 必须 summary/ref 化。
- projection 中的 RecoveryPlan / nextAction 类字段若存在，仅为诊断展示，不驱动调度。

## 3. State Model Contract

### 3.1 Core States

| State Family | Values / Direction | Owner |
| --- | --- | --- |
| Stage semantic | exploration, planning, execution, verification, acceptance, repair, replan（语义意图，非生产状态机枚举） | stage / output policy + profile mapping |
| RuntimeDecision.Action | acquire_prerequisite, produce_state_change, produce_validation, produce_assessment, request_governance, complete, fail（及 governed_wait 投影） | orchestrator reducer |
| RunState | pending, active, blocked_on_decision, blocked_on_approval, completed, failed, cancelled, superseded | blocks/run |
| WorkspaceState | active, blocked_on_approval, superseded | blocks/workspace owner; projection may derive prompt/progress from active run |
| Focus / requiredOutput / evidence readiness | current focus queue + RequiredOutputContract + EvidenceLedger | orchestrator aggregate |
| Completion | sealed terminal after last assessment / fail path；owner-local delivery | orchestrator terminal fact → Result → Workspace → Run |
| StepState（过程/legacy 投影） | pending, executing, succeeded, failed, skipped, cancelled | run storage / observability |
| ResultState | accepted/rejected/superseded-style outcome state | blocks/result |

### 3.2 State Bridge

Allowed bridge:

```text
Start / Resume intent
  -> Run owner-local lifecycle transition -> RunStateChanged
  -> InitializeRuntimeV2FromRun (typed bootstrap → facts / load aggregate)
  -> DriveRuntimeV2
  -> RuntimeFacts → RuntimeDecision + AuthorityTransaction
  -> RequiredOutput / candidate / SealedDispatch
  -> operation evidence / assessment / governance commit
  -> RunState / ResultState / Projection
```

Forbidden bridge:

- stage semantic / outputMode 不得覆盖 reducer 权威决策。
- LLM `completionState=completed` must not create accepted Result without evidence/DoD gates。
- Web/TUI prompt state must not override backend projection。
- `StagePacket` / `NextAction` / graph dispatch 不得作为正式推进桥。
- Decision resolve 后 application 不得靠 Resume 伪装 continuation queue。
- 不得以 `runtimeContractVersion` 路由 V1/V2 生产路径。

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
  -> seal typed RuntimeBootstrap (Definition + Activation + Focuses + Requirements)
  -> EventHub / owner creates Run with bootstrap
  -> Start intent: Run owner internal pending→active then RunStateChanged → DriveRuntimeV2
  -> initial CoverageFocus queue + RuntimeFacts
  -> projection shows active execution or governed wait
```

Input contract:

- workspaceId。
- goalId/doDId or effective latest definition。
- Soul/Skill/MCP/Memory bindings（进入 Activation，非 patch）。
- priority and operator input。

Output contract:

- Run with workspaceId/goalId/doDId/definitionRevision + sealed RuntimeBootstrap。
- CoverageFocus / coverage baseline。
- first reducer decision（非 StagePacket）。
- lightweight projection / command accepted response。

### 4.3 Planning / Execution Flow

```text
Run active / DriveRuntimeV2
  -> reducer(RuntimeFacts)
  -> acquire_prerequisite or produce_state_change / produce_validation / ...
  -> BeginCandidateAttempt (attempt 0/1, same authority transaction)
  -> RuntimeCandidateContext (C0–C5) + requiredOutput
  -> LLM candidate output
  -> Accept/Fail candidate; seal SealedDispatch
  -> ExecuteRuntimeOperation (dependency graph of sealed operations, not V1 graph dispatcher)
  -> evidence commit → reducer
```

Contracts:

- LLM candidate must match requiredOutput。
- State-changing output must bind target refs / coverage targets when required。
- Read-only path cannot satisfy state-change requirement。
- Managed validation must use declared managed_session contract when required。
- 无 `StagePacket` / `NextAction` 包装 candidate 或 evidence。

### 4.4 Verification / Acceptance Flow

```text
State change evidence available
  -> produce_validation / validation evidence commit
  -> coverage / focus accepts or rejects evidence
  -> RequireAssessment focus → produce_assessment
  -> assessment ledger close
  -> Result accepted/rejected projection or same-focus repair reconverge
```

Contracts:

- validation evidence must be concrete, scoped and accepted。
- claim-to-evidence alignment applies when profile requires it。
- accepted Result requires current definition + accepted evidence refs。
- failed assessment can be recoverable evidence gap rather than terminal failure。
- Result owner does not adjudicate。

### 4.5 Repair Flow

```text
Contract violation / failed validation / evidence gap
  -> rejected diagnostic evidence recorded
  -> same focus / same authority contract retry budget (attempt 0 + at most 1 same-contract retry where applicable)
  -> candidate regenerates targeted correction under inherited requiredOutput / stage semantic
  -> evidence gate repeats
```

Contracts:

- rejected output cannot have state-changing side effect。
- repair inherits focus, requiredOutput and stage semantic constraints。
- repeated same failure escalates to governance only after budget/policy conditions。
- repair 不是自动 replan successor。

### 4.6 Replan Flow（语义 only）

```text
Decision resolution or product intent determines current plan/definition invalid
  -> old run lineage captured
  -> **new** typed activation / new RuntimeBootstrap
  -> **independent** V2 Run created
  -> current effective Goal/DoD retained or explicitly redefined
  -> new CoverageFocus baseline; carry-over only via explicit definition/activation rules
  -> independent DriveRuntimeV2
```

Contracts:

- Runtime V2 **不自动创建** replan successor Run。
- replan cannot drop active Goal/DoD constraints without explicit new definition。
- new run must carry lineage refs when product requires audit continuity。
- old run terminal/superseded state must not remain activeRunId。
- 无 `ExecutionContextPatch` 在旧 run 上伪装 replan。

### 4.7 Decision / Approval Flow

```text
Runtime detects real governance need
  -> request_governance → Decision Ticket (and/or Approval)
  -> Orchestrator records governance wait fact; Run owner internally enters blocked_on_decision / blocked_on_approval
  -> projection shows waiting/blocker state
  -> user resolves or approves/rejects
  -> CommitRuntimeGovernanceTicketResolution / resolution commit
  -> reducer continues, fails (e.g. governance_rejected), or product starts new activation
```

Contracts:

- ticket/approval must be scoped by workspaceId/runId。
- TUI/Web submit resolution, not state mutation。
- resolved governance must be reflected in owner state and projection。
- application **不得** 因 resolve 再 Resume 驱动。

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
- UI 不得从 projection 反推 RuntimeDecision 并本地调度。

## 5. Cross-Flow Interaction Contracts

| From Flow | To Flow | Contract |
| --- | --- | --- |
| Workspace setup | Activation | effective Goal/DoD/template/Soul/Skill/MCP/Memory bindings |
| Activation | Drive | Run + RuntimeBootstrap + definitionRevision + CoverageFocus baseline |
| Drive produce_state_change | Verification | accepted state-changing evidence + target refs |
| Verification | Acceptance | accepted validation evidence + coverage/focus update |
| Acceptance | Result | assessment ledger + accepted evidence refs + artifact refs |
| Any runtime gate | Repair | rejected diagnostic evidence + same-focus reconverge |
| Repair | Drive | inherited focus/requiredOutput + narrowed candidate contract |
| Governance | Continuation | resolution commit → reducer；or new activation for replan semantic |
| Replan semantic | Activation | **new** Run + bootstrap + lineage + current definition |
| Owner state | Projection | event/owner read + projection revision |
| Projection | Web/TUI | ViewModel only, no business state reconstruction |

## 6. Functional Module Interaction

### 6.1 Workspace / Run / Orchestrator

```text
workspace owner
  -> defines Workspace/Goal/DoD
run owner
  -> creates Run, persists RuntimeStateRecord / process evidence
orchestrator
  -> Initialize + DriveRuntimeV2
  -> reducer / authority / candidate / sealed dispatch / evidence
  -> projects Run / Result / governance bindings through owner services
```

Key contract:

- Orchestrator may coordinate lifecycle but must not bypass owner object invariants。
- 唯一同步推进合同：`DriveRuntimeV2Command/Result`。

### 6.2 Orchestrator / Blocks

```text
orchestrator
  -> RequestRuntimeCandidate or ExecuteRuntimeOperation (sealed)
blocks
  -> normalized result / error / raw ref / operation evidence payload
orchestrator
  -> Accept/Fail candidate, evidence commit, reducer
```

Key contract:

- Blocks execute side effects; orchestrator interprets business meaning and authority。
- 无 V1 generic step graph dispatcher。

### 6.3 Appservice / Kernel

```text
HTTP/TUI command
  -> appservice validates request and scope
  -> EventHub command (activation/start/resume/governance resolve) or owner service call
  -> kernel owner / orchestrator updates formal state
  -> appservice returns DTO/projection / command accepted
```

Key contract:

- appservice composes use cases; kernel owns formal state。
- Start/Resume 不注入 ExecutionContextPatch；Decision resolve 不触发 application Resume。

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
  -> structured policy fields (stage policy, evidence policy, capability allowlist)
runtime core (V2)
  -> neutral interpretation as activation / C2 / requiredOutput / evidence gate inputs
  -> focus / sealed dispatch / evidence enforcement
```

Key contract:

- profile owns strategy; runtime core owns deterministic enforcement。
- kernel does not understand profile IDs。

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
| V2 state / focus / ledger summary | bounded diagnostic fields | snapshot/observability |

Constraints:

- default projection must be bounded。
- raw ref must be logical and scoped。
- truncated/partial content must not be treated as safe full file basis。

## 8. Consistency Checklist

Use this checklist when adding or changing product, flow, or module behavior:

- Does the change identify the owner object?
- Does every state-changing command carry workspace scope?
- Does the flow preserve current Goal / DoD definition revision?
- Does candidate context include requiredOutput, current gap, accepted/rejected evidence and target snapshots?
- Does contract violation stay rejected until a valid retry succeeds?
- Does accepted Result cite accepted evidence refs?
- Does projection consume owner state rather than raw event guesses?
- Does Web/TUI avoid duplicating runtime decision logic?
- Does profile-specific behavior remain in template/Soul/profile/capability data?
- Does large payload use summary/ref rather than default inline expansion?
- Is production path Runtime V2 only (no StagePacket/NextAction/graph dispatch/ExecutionContextPatch/auto replan successor)?
- Does Start/Resume enter DriveRuntimeV2 without execution-context patch?
- Does governance wait return without application auto-Resume?

## 9. Known Boundary Decisions

- Web Console is not “read-only only”; it is “projection display + controlled operation entry”。It may create/update workspace resources and submit governance/configuration operations through formal API, but it does not own business state.
- Workspace owner state is intentionally small: `active`, `blocked_on_approval`, `superseded`。`ActiveRunID` 与 Workspace state
  只由 Workspace owner 消费 immutable Run/terminal fact 后在本地 reducer 中更新。Decision blockers do not mutate
  workspace owner state; they are expressed by `RunState=blocked_on_decision`, open decision ticket, `blockingTicketId`,
  prompt mode and governance projection. Approval blockers use `WorkspaceState=blocked_on_approval` only when the owner has an active approval blocker.
- `validation/managed_session` is a runtime block kind owned by orchestrator/shell-backed managed validation flow and must be declared through central runtime vocabulary.
- `internal/pkg/events` may aggregate event contracts, but aggregation must stay contract-level and must not become business outcome logic.
- Step / graph / RecoveryPlan.nextAction 等字段可保留在读模型诊断面，但不得重新声明为生产调度权威。
- Production runtime is V2 only; dual-layer stage semantic + V2 implementation is the only product-facing runtime story.
