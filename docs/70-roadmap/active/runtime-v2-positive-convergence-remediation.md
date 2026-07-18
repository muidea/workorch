# Runtime V2 Positive Convergence Systemic Closure Plan

Status: active — code reopened by 2026-07-18 deployed Live; RVC-R17～R22 pending before final RVC-R16 rerun

Type: roadmap / implementation closure specification

Last Updated: 2026-07-18

## 1. Purpose

本文是 Runtime V2 正向收敛的唯一 active 实现收口计划。它承接最新 deployed/file-backed 双 workspace Live 证据，对四状态机交界处暴露的系统性偏差进行整体收口，避免继续通过 prompt、单一 operation kind、单一 workspace 或单一失败文本做片面堵漏。2026-07-18 Live 已证明 RVC-R5～R15 的 effect/receipt/recovery 主链成立，但同时暴露 governance resolution 跨 owner 提交、blocked Resume 权威门禁、端侧 allowed action、candidate parameter contract 和 Live fixture 边界仍未关闭；因此“只剩 RVC-R16 Live”的旧判断正式作废。

核心语义权威仍是：

- [`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)
- [`internal/modules/kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md)
- `internal/pkg/workorchruntimecontract`

本文只定义待实现的数据合同、状态迁移、组件落点、测试矩阵、迁移与验收顺序，不建立第二份 Runtime 核心语义。凡本文要求改变四状态机正式语义的内容，必须先更新 core contract，再进入代码实现。

## 2. Why The Previous Closure Was Reopened

此前 remediation 已完成 typed validation policy、rejected-shape context、Result closeout、focus cost guard、provider ceiling、candidate attempt 0/1、projection retry 以及 ai-proxy optional bearer authentication 等基础能力。但 2026-07-17 最新实例 Live 证明“仅剩 deployed E2E”的判断不成立。

### 2.1 Live evidence

| Workspace | Run | 已验证正确 | 最终偏差 |
| --- | --- | --- | --- |
| `ws-1138d6b3bfddb4fd` / LSP-Bridge | `run-022` | ai-proxy chat 成功、candidate/dispatch/evidence 均真实发生、terminal 投影完成 | read-only/session MCP 被记为 state-change；SM-2 要求 state-change repair，而 SM-3 gate 又要求 validation，最终 `candidate_retry_exhausted` |
| `ws-bea4fee67e459bcc` / ai-proxy | `run-021` | validation 正确发现 X-Request-ID 测试失败，reducer 正确进入 repair | `shell/command stateChanging=false` 的同一测试命令被 seal 为 state-change，执行失败后 anti-replay terminal |

共同结论：

1. provider 认证与模型发现不再是当前主阻塞；
2. Start/Resume、CAS、sealed dispatch、terminal delivery 和 projection 主骨架可工作；
3. 当前根因位于 SM-2 action、SM-3 operation effect、owner receipt 与 evidence commit 的交界合同；
4. 现有测试证明了实现内部一致，但没有证明跨状态机不变量完整。

### 2.2 2026-07-18 post-RVC-R15 Live evidence

| Workspace | Run | 已验证正确 | 新暴露偏差 |
| --- | --- | --- | --- |
| `ws-1138d6b3bfddb4fd` / LSP-Bridge | `run-023` | provider transient 正确进入 `request_governance`；aggregate 保持 blocked，不会因 owner ticket resolved 自动消费无效 resolution | ticket 只允许 `provide_missing_info/reject_current_path`，TUI 却提交 `replan`；Decision owner 已将 ticket/resolution 持久化为 resolved，Orchestrator aggregate 无 matching governance resolution，形成永久 split-brain |
| `ws-bea4fee67e459bcc` / ai-proxy | `run-023` | focus-001/002 已 accepted，durable File receipt 为 `workspace_durable`，focus-003 正确经历 validation → repair → assessment；最终 typed rejected Result/Run failed/Workspace release 收敛 | attempt 0/1 均生成缺少 `targetRef/path` 的 `patch/apply`，被 contract gate 正确拒绝并 `candidate_retry_exhausted`；说明 candidate producer 的 action-specific parameter schema/repair context 仍不足 |

本轮额外确认：

- `/readyz` source/projected revision 一致，projection 不是阻塞根因；
- ai-proxy request 使用封存 provider/model，stable prefix 未漂移，context budget 未超限；
- 普通 Resume 会在 Decision ticket 已 resolved 时调用 `ActivateRuntimeV2Run` 清除 Run blocker，即使 aggregate 尚无 matching typed resolution；drive 随后又根据 aggregate 回到 governed wait，形成 `blocked → active → blocked` 的非权威抖动；
- 真实 ai-proxy DoD 编译为 396 coverage items / 27 focuses；focus 数虽未超过 neutral guard 64，但整体规模不适合作为 180 秒 terminal SLA fixture。

因此本轮不是修改现有四状态机语义，而是补齐已经写入 core contract 的以下执行门禁：

1. SM-1 `blocked → active` 必须有 persisted Runtime V2 governance resolution；
2. SM-2/SM-3 governance outcome 只能由 Orchestrator aggregate 接受后生效；
3. Decision owner 的 ticket/resolution 只能投影已被 Runtime V2 接受的治理结论；
4. TUI/Web/HTTP 的动作集合必须从 ticket contract 派生；
5. action-specific candidate schema 必须在 provider 输出边界尽量消除机械参数缺失；
6. deployed terminal gate 与真实长任务 liveness observation 必须分层。

## 3. Scope And Non-Goals

### 3.1 In scope

- SM-1 Run lifecycle 的 cancel/supersede 正式语义。
- SM-2 repair directive、failure taxonomy、budget 与 action authority。
- SM-3 candidate effect gate、operation execution receipt、evidence authority 与 crash recovery。
- SM-4 startup recovery、多实例 fencing、terminal delivery 和 projection 观测。
- Application activation、LLM candidate renderer、MCP/Shell/File/Git/SSH/Web owner、Run/Result/read model 的 typed contract 联动。
- Runtime state/bootstrap/result closeout schema v2 固化与版本门禁。
- deterministic file-backed、crash injection、双 workspace deployed Live。

### 3.2 Out of scope

- 恢复 V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch 或自动 replan successor。
- 在 Orchestrator core 中写入软件开发、Go、测试框架、LSP、ai-proxy 或具体 workspace 业务判断。
- 引入通用 RuntimeAdapter registry、map-based authority rebuild 或 projection-driven reducer。
- 把 provider base URL、credential、client 或动态路由状态序列化进 workspace/run authority。
- 修改已终态历史 Run 使其重新 active。

## 4. Current Architecture Judgment

| Area | Judgment | Closure requirement |
| --- | --- | --- |
| Application typed bootstrap | 符合 | 保留；只补新 schema/effect/governance policy 编译 |
| Run owner CAS aggregate | 符合 | 保留；增加 execution/termination records 与 schema validation |
| SM-2 reducer authority | 基本符合 | repair action 必须由单一 persisted directive 驱动 |
| Candidate attempt 0/1 | 符合 | 保留；与 provider transport retry 分离 |
| Sealed dispatch | 基本符合 | 补 per-operation execution transaction 与 recovery receipt |
| Typed EventHub owner I/O | 形式符合 | owner 必须返回真实 observed effect，而非回显 Orchestrator 指定 evidence kind |
| Evidence ledger | 不充分 | evidence class 必须由 sealed expectation + owner receipt 联合裁决 |
| Governance | 不完整 | Decision/Approval/config/external dependency 必须统一为 typed governance outcome |
| Restart recovery | 不完整 | 有 load/re-drive，无 startup inventory；active sealed dispatch 会盲目重执行 |
| Terminal delivery/projection | 符合 | 保留；补重启 delivery inventory 和新观测字段 |

## 5. Target Invariants

以下不变量是本轮代码完成门禁。

### INV-01 Action authority

只有 SM-2 reducer 能产生当前 `requiredAction`。candidate renderer、anti-replay、capability owner、projection 和 application 均不得建议或切换另一个 RuntimeAction。

### INV-02 Effect compatibility

- `produce_state_change` 的 sealed candidate 至少包含一个与 RequiredOutput 相容的 durable effect operation。
- `produce_validation` 的所有 operation 必须为 non-durable effect（`none` 或显式允许的 `ephemeral_session`），不得包含 workspace/remote durable effect。
- ephemeral session operation 不得关闭 durable state-change requirement。
- unknown effect 不得作为 accepted state-change 或 accepted validation。

### INV-03 Owner receipt authority

Capability owner 必须在返回前持久化 execution receipt。Orchestrator 只能根据 sealed expectation 与 owner receipt 提交 evidence，不能把 decision action 直接当成 evidence kind。

### INV-04 Crash safety

进程崩溃发生在 owner side effect 完成和 aggregate evidence CAS 之间时，重启不得盲目重复副作用。存在 receipt 时只重提 evidence；结果未知且不可安全重放时进入治理或 bounded failure。

### INV-05 Repair coherence

同一 persisted revision 下只能存在一个 `RepairDirective.RequiredAction`。原始 evidence gap、repair target、anti-replay shapes 和 LLM prompt 必须服从该 action。

### INV-06 Governance before effect

需要 approval/decision/configuration 的 operation 必须在副作用发生前返回 typed governance requirement，不得先执行再以普通 execution failure 表达审批缺失。

### INV-07 Recovery ownership

启动恢复由 Orchestrator 通过 Run owner typed inventory/load 驱动；projection/watchdog 只观测，不产生 action。多实例通过 persisted claim/fence + CAS 合并。

### INV-08 Terminal authority

- completed/failed 只来自 SM-2 sealed terminal。
- cancelled/superseded 属于 SM-1 显式行政终态，但 active V2 Run 必须先在 aggregate 中 CAS 持久化 termination record，禁止直接修改 Run owner 状态。
- terminal Run 不 Resume、不改 aggregate、不回退 active。

## 6. Canonical Effect And Evidence Model

### 6.1 Effect scopes

新增中立 effect scope：

```go
type RuntimeEffectScope string

const (
    RuntimeEffectNone             RuntimeEffectScope = "none"
    RuntimeEffectEphemeralSession RuntimeEffectScope = "ephemeral_session"
    RuntimeEffectWorkspaceDurable RuntimeEffectScope = "workspace_durable"
    RuntimeEffectRemoteDurable    RuntimeEffectScope = "remote_durable"
    RuntimeEffectGovernanceOnly   RuntimeEffectScope = "governance_only"
    RuntimeEffectUnknown          RuntimeEffectScope = "unknown"
)
```

`workspace_durable` 和 `remote_durable` 都是 state-changing，但是否可关闭当前 focus 由 `RequiredOutput.AcceptedEffectScopes` 决定。core 不按业务领域推断 effect。

### 6.2 Operation descriptor

`RuntimeOperationDescriptor` 扩展：

```go
type RuntimeOperationDescriptor struct {
    Kind                  string
    OwnerModuleID         string
    DefaultEffectScope    RuntimeEffectScope
    AllowedActions        []RuntimeAction
    CapabilityClass       RuntimeCapabilityClass
    IdempotencyClass      RuntimeOperationIdempotencyClass
    RequiresOwnerReceipt  bool
}
```

`shell/command`、`mcp/tool_call`、`ssh/command` 不得继续只靠 kind-level 一刀切。concrete operation 必须结合 sealed capability metadata、operation parameters 和 owner contract 得到 `ExpectedEffectScope`。

### 6.3 Required output

`RequiredOutputContract` 扩展：

```go
AcceptedEffectScopes  []RuntimeEffectScope
RequiredEffectScopes  []RuntimeEffectScope
RequiredEvidenceKinds []string
```

合法映射：

| Action | Required effect | Evidence |
| --- | --- | --- |
| `produce_state_change` | 至少一个 accepted durable effect | `state_change` |
| `produce_validation` | 全部为 `none` 或显式允许的 `ephemeral_session`；不得有 durable effect | `validation` |
| `produce_assessment` | 无 operation effect | assessment fact |
| `request_governance` | `governance_only` | governance resolution |

### 6.4 Evidence derivation

Evidence 不再由 `evidenceKindForDecision` 单独决定，改为：

```text
sealed RequiredOutput
  + sealed operation ExpectedEffectScope
  + owner receipt ObservedEffectScope / outcome
  + coverage/target binding
  = committed EvidenceFact.Kind / status
```

Owner receipt 为 `no_op` 时可以作为 diagnostic/validation observation，但不能关闭 required durable state-change。

## 7. Per-Operation Execution Transaction

### 7.1 Persisted states

在 sealed dispatch 中增加 operation execution records：

```go
type RuntimeOperationExecutionState string

const (
    OperationExecutionSealed            RuntimeOperationExecutionState = "sealed"
    OperationExecutionClaimed           RuntimeOperationExecutionState = "claimed"
    OperationExecutionOwnerCommitted    RuntimeOperationExecutionState = "owner_committed"
    OperationExecutionEvidenceCommitted RuntimeOperationExecutionState = "evidence_committed"
    OperationExecutionOutcomeUnknown    RuntimeOperationExecutionState = "outcome_unknown"
)
```

```go
type RuntimeOperationExecutionRecord struct {
    OperationID         string
    ExecutionKey       string
    ExpectedEffectScope RuntimeEffectScope
    State               RuntimeOperationExecutionState
    Fence               uint64
    OwnerReceiptRef     string
    OwnerReceiptHash    string
    ClaimedAt           string
}
```

`ExecutionKey` 必须稳定绑定 `workspace/run/source-dispatch/operation`。治理 resume
dispatch 保持该 key，并通过新 fence 表达同一逻辑 operation 的下一次获批执行。

### 7.2 Owner receipt

```go
type RuntimeOperationReceipt struct {
    ExecutionKey        string
    Fence               uint64
    OperationID         string
    Outcome             RuntimeOperationOutcome
    ObservedEffectScope RuntimeEffectScope
    Changed             bool
    NoOp                bool
    TargetRefs          []string
    BeforeHash          string
    AfterHash           string
    Evidence            RuntimeOperationEvidence
    Failure             *FailureFact
    Governance          *RuntimeGovernanceRequirement
    IdempotentReplay    bool
    CompletedAt         string
}
```

每个 capability owner 继续使用自己的 concrete EventHub topic/command/result，但嵌入共享 receipt 类型。不得引入通用 executor registry。

### 7.3 Recovery transition

```text
sealed
  -> CAS claim + fence
  -> query owner receipt by ExecutionKey + Fence
       receipt accepted/rejected/governance -> commit receipt ref -> commit evidence
       receipt absent + safe to execute      -> owner execute and persist receipt
       receipt claimed/in-flight             -> bounded wait/query
       outcome unknown                       -> governance or structured fail
```

禁止行为：

- 仅因为 `CompletedOperationIDs` 缺失就重放 operation；
- 对 unknown/non-idempotent shell/git/MCP/SSH operation 自动重放；
- owner 已返回 receipt 后再次执行相同 `ExecutionKey + Fence`。

## 8. Repair Directive

### 8.1 Persisted contract

新增：

```go
type RuntimeRepairDirective struct {
    ID                     string
    FocusID                string
    Trigger                RuntimeRepairTrigger
    CurrentFailure         *FailureFact
    RequiredAction         RuntimeAction
    RequiredEffectScopes   []RuntimeEffectScope
    RequiredEvidenceKinds  []string
    AllowedOperationKinds  []string
    RejectedOperationShapes []RuntimeRejectedOperationShape
    CoverageWorkItemIDs    []CoverageWorkItemID
    TargetRefs             []string
    SupersedesEvidenceRevision string
    RepairAttempt          int
}
```

### 8.2 Reducer rules

1. rejected validation 若 policy 表明 implementation repair，生成 `RequiredAction=produce_state_change`。
2. 在 durable state-change accepted 前，directive 不得被 raw `MissingRequiredEvidenceKinds` 覆盖。
3. durable state-change accepted 后，原子更新 directive 为 `produce_validation`。
4. anti-replay 只能更新 forbidden shapes 或返回 candidate contract failure，不得返回另一 action 的自然语言指令。
5. focus advance 时清除旧 directive、failure、shape 和 per-focus budget；历史记录保留审计。

### 8.3 Candidate projection

LLM volatile context 必须同时携带：

- `requiredAction`
- `requiredEffectScopes`
- `requiredEvidenceKinds`
- `admissibleOperationKinds`
- `rejectedOperationShapes`
- `currentFailure`
- `targetRefs/targetSnapshots`

原始 `missingEvidenceKinds` 只能作为 completion audit 字段，不能再以“produce only those kinds”覆盖 repair directive。

## 9. Action-Specific Candidate Contracts

### 9.1 Separate schemas

`produce_state_change` 与 `produce_validation` 使用独立 schema/catalog 文案。

State-change candidate 必须：

- 至少一个 operation；
- 至少一个 operation 的 `ExpectedEffectScope` 命中 `RequiredEffectScopes`；
- shell state-change 必须显式声明 durable effect 和 target refs；
- read-only/session/no-op operation 只能作为依赖节点，不能单独满足 candidate。

Validation candidate 必须：

- 所有 operation effect 为 `none` 或 RequiredOutput 显式允许的 `ephemeral_session`，且不得包含 durable effect；
- 禁止 candidate 自声明改变权威 effect；
- 只允许 owner-sealed read-only capability。

### 9.2 Seal order

```text
decode schema
 -> validate action-specific candidate
 -> resolve concrete ExpectedEffectScope
 -> validate capability/action/effect/coverage compatibility
 -> validate rejected shape
 -> persist candidate attempt accepted + sealed dispatch + operation records
```

任何 effect 不可证明的 candidate 必须在 seal 前失败，不得消耗 execution/repair budget。

### 9.3 Cache stability

- 新 action/effect/schema contract 进入稳定 C0/C2 后必须升级 Runtime candidate contract version。
- 同一 Run、同一 stable contract 的 attempt 0/1 必须保持 stable prefix hash 不变。
- 新版本不得在 Resume/restart 时按当前系统配置重算旧 Run stable prefix。

## 10. MCP Capability Normalization

### 10.1 Activation boundary

Registry wire kind 与 Runtime operation kind 分层：

```text
registry Kind=tool
  -> RuntimeOperationKind=mcp/tool_call
```

Runtime MCP capability context 增加：

```go
RegistryKind       string
OperationKind      string
EffectScope        RuntimeEffectScope
IdempotencyClass   RuntimeOperationIdempotencyClass
ApprovalPolicy     string
```

### 10.2 Required mappings

- `read_only` → `none`
- session initialization/cache lifecycle → `ephemeral_session`
- workspace mutation → `workspace_durable`
- external mutation → `remote_durable`
- unknown/missing metadata → `unknown`，不得进入 validation，也不得作为 accepted state-change

Live 中的 `lsp_initialize` 必须归为 `ephemeral_session` 或 `none`，不得满足 durable state-change。

## 11. Governance Unification

### 11.1 Governance kinds

```go
type RuntimeGovernanceKind string

const (
    GovernanceDecision      RuntimeGovernanceKind = "decision"
    GovernanceApproval      RuntimeGovernanceKind = "approval"
    GovernanceConfiguration RuntimeGovernanceKind = "configuration"
    GovernanceExternal      RuntimeGovernanceKind = "external_dependency"
)
```

### 11.2 Owner outcome

Owner 在执行前发现权限/审批需求时返回：

```go
RuntimeOperationOutcomeGovernanceRequired
RuntimeGovernanceRequirement{
    Kind,
    SubjectRef,
    Reason,
    RequestedAction,
    RiskLevel,
}
```

Orchestrator 将其绑定当前 authority/dispatch/operation，分别向 Decision 或 Approval owner 创建 concrete ticket，不得包装为 execution failure。

### 11.3 Resolution

Resolution 必须绑定：

- workspace/run；
- authority snapshot；
- dispatch/operation；
- ticket/approval；
- governance kind；
- outcome/evidence refs。

continue 后回到同一 operation transaction；若 owner receipt 已存在则提交 evidence，否则按新 fence 执行。reject 由 reducer 产生 governance terminal。

## 12. SM-1 Lifecycle Alignment

### 12.1 Formal lifecycle

core contract 在代码前更新为：

```text
pending
  -> active
       <-> blocked_on_decision / blocked_on_approval
       -> completed
       -> failed
       -> cancelled
       -> superseded

all terminal states are immutable
```

### 12.2 Authority rules

- completed/failed：仅 SM-2 reducer terminal。
- cancelled/superseded：仅显式 typed administrative command；active V2 Run 必须先 CAS 持久化 `RuntimeTerminationRecord`，随后 SM-1 投影 Run/Workspace。
- pending 且尚无 aggregate 的 activation failure 可以直接 failed，但必须保留明确 administrative source。
- active aggregate 不允许直接调用 `closeRun` 绕过 termination record。

### 12.3 Termination record

```go
type RuntimeTerminationRecord struct {
    Kind       RuntimeTerminationKind
    RequestedBy string
    Reason     string
    Revision   uint64
    RequestedAt string
}
```

取消/替换不得自动创建 successor；新 Run 仍由产品层显式 activation 创建并写 lineage。

## 13. Failure Taxonomy And Budgets

### 13.1 Failure classes

至少拆分：

| Class | Reducer disposition |
| --- | --- |
| `candidate_schema_failure` | same-contract attempt 0/1 |
| `candidate_contract_failure` | same-contract attempt 0/1 |
| `provider_transient` | LLM adapter bounded transport retry；耗尽后 typed provider failure |
| `provider_authentication` / `provider_configuration` | configuration governance，不进入普通 repair |
| `capability_unavailable` | governance 或 bounded fail，不请求 LLM 猜测 |
| `owner_transient_unavailable` | operation recovery budget，不立即标 contract defect |
| `execution_rejected` | structured same-focus repair |
| `operation_outcome_unknown` | governance 或 bounded fail，禁止盲重放 |
| `governance_required/rejected` | blocked 或 terminal |
| `cas_conflict` | bounded reload/retry，不作为业务 failure |
| `runtime_invariant_defect` | typed terminal contract defect |
| `projection_delivery_failure` | delivery retry，不改 reducer terminal |

### 13.2 Independent budgets

- provider transport retry；
- candidate contract attempt 0/1；
- per-focus repair attempts；
- validation-only retries；
- operation receipt/recovery attempts；
- CAS reload attempts；
- terminal delivery attempts。

预算上限必须由 typed bootstrap/system policy 封存；credential、base URL、client 等动态配置仍不得持久化。

## 14. SM-4 Startup And Multi-Instance Recovery

### 14.1 Recovery inventory

Orchestrator `Start()` 通过 Run owner concrete typed query 获取：

- active Runtime V2 aggregate；
- active sealed dispatch；
- persisted terminal aggregate 但 Result/Run/Workspace delivery 未完成；
- blocked aggregate 仅用于核对 ticket binding，不自动 Resume。

Application server 和 projection 不负责扫描或选择 action。

### 14.2 Recovery actions

| Persisted state | Recovery |
| --- | --- |
| active / no dispatch | schedule drive |
| active / sealed operation | claim fence，query owner receipt，再决定 commit/execute/wait/unknown |
| governed wait | verify binding，继续等待 |
| terminal aggregate / Run 非终态 | retry Result → Run/Workspace delivery |
| terminal Run | no-op |

### 14.3 Multi-instance fencing

- process singleflight 保留为本地优化；
- 正式互斥使用 aggregate CAS claim/fence；
- owner receipt 拒绝旧 fence；
- CAS conflict 重新 load，不生成 `runtime_v2_drive_failed`；
- 相同 ExecutionKey 只能存在一个 owner terminal receipt。

## 15. Schema Version Policy

### 15.1 Versioning

本轮目标版本：

- `RuntimeBootstrap.SchemaVersion = 2`
- `RuntimeStateRecord.SchemaVersion = 2`
- `RuntimeResultCloseout.SchemaVersion = 2`
- candidate runtime contract version 显式升级

### 15.2 Version gate

基于当前项目没有需要保留的 schema 0/1 Runtime V2 正式数据，本轮不引入旧数据语义迁移分支：

- production persistence 只写入并接受 schema v2；
- schema 0/1 与 future schema 均 fail-fast，不原位升级、不推断 effect、不重写 evidence；
- 测试与开发环境若存在旧 fixture，直接重建当前 schema 的 Run，不把兼容逻辑写入 production core；
- provider target 仍来自 Run activation 时封存的 provider/model identity，版本门禁不得重新读取系统配置覆盖旧 Run。

## 16. Component Change Matrix

| Component | Required changes |
| --- | --- |
| `internal/pkg/workorchruntimecontract` | effect scope、descriptor、RequiredOutput、repair directive、operation receipt/state、governance、termination、failure taxonomy、schema v2 |
| `kernel/orchestrator/biz/runtimev2/reducer.go` | directive-driven action；删除 raw missing/anti-replay action 冲突 |
| `controller.go` / `dispatch_commit.go` | operation claim/receipt/evidence CAS；effect compatibility；schema gate |
| `executor.go` | 不再按 action 注入最终 EvidenceKind；先 claim/query receipt |
| `runtime_v2_mainline.go` | action-specific seal、receipt recovery、typed provider/owner error routing |
| `module.go` | startup recovery inventory；CAS conflict 分类；delivery retry |
| `kernel/run` | schema v2 validation、execution fence、termination record、recovery inventory query |
| `application/workorchd/activationports` | effect/governance/retry policy 编译；MCP kind normalization；不得参与 recovery action；blocked Runtime V2 Resume 不得只依据 ticket owner state 清 blocker |
| `blocks/llm` | action-specific schema/prompt；repair directive projection；stable contract version |
| File/Git/Shell/MCP/SSH/Web owners | concrete execute/query receipt command；ObservedEffectScope；governance-required preflight；ExecutionKey 幂等 |
| Decision/Approval | operation-scoped governance binding/resolution；Runtime V2 resolution proposal/commit saga；allowed action owner validation；禁止先 resolved 后提交 aggregate |
| Result | closeout v2，包含 expected/observed effect、receipt refs、unknown outcomes |
| read model/TUI/Web | 展示 current directive、effect mismatch、operation recovery、separate budgets；动作从 `AllowedRecoveryActions` 派生；不产生 authority |
| `blocks/llm/pkg/events` / eventstream | usage/context/cache audit 使用 concrete typed DTO，清除跨 owner `map[string]any` |

## Implementation Progress (2026-07-18)

| Batch | Status | Notes |
| --- | --- | --- |
| RVC-R5 | code-landed | effect/receipt/directive/termination/failure types; Runtime State/Bootstrap/Result closeout 固化为 schema v2 |
| RVC-R6 | code-landed | ResolveExpectedEffectScope; seal forward/reverse effect gates; validation feasibility only admits fixed non-durable kinds or explicit sealed MCP capability effects |
| RVC-R7 | code-landed | single RepairDirective authority; anti-replay cannot switch action; hints obey directive |
| RVC-R8 | code-landed | registry Kind=tool → mcp/tool_call; capability owner explicitly persists effect/idempotency metadata, activation seals missing metadata as unknown with no risk/tool-name inference |
| RVC-R9 | code-landed | owner 返回前持久化 receipt；current-schema evidence 强制 receipt；ObservedEffect/no-op/effect mismatch 联合裁决 |
| RVC-R10 | code-landed | 真实 file-backed crash matrix 已覆盖 claim CAS 前/后、execute 前、owner receipt 后、evidence/dispatch-close 原子 CAS 前/后、governance grant CAS 后、terminal delivery；safe replay 消耗 operation budget，at-most-once/unknown 保持 outcome_unknown |
| RVC-R11 | code-landed | exact-fence receipt、typed grant、continue resume dispatch、owner preflight grant verification/consumption、suspended binding dependency 保真与 restart/duplicate/stale 回归已落地 |
| RVC-R12 | code-landed | Cancel/Supersede require CAS TerminationRecord for active V2 |
| RVC-R13 | code-landed | provider/operation/CAS/terminal-delivery 独立 policy/ledger 已封存并接入 retry routing、Result closeout 与 read model |
| RVC-R14 | code-landed | Run owner typed recovery inventory、blocked binding verification、terminal delivery retry inventory、schema v2-only 与 read model 已落地 |
| RVC-R15 | code-landed | 真实 file-backed 主场景与 crash matrix 已覆盖两 focus、provider transient、CAS reload、File approval/grant、same key/next fence、stale/duplicate、validation、assessment、focus advance、terminal delivery retry、accepted Result、Run completed 与 Workspace release |
| RVC-R16 | failed / reopened | 2026-07-18 dual-workspace Live 暴露 resolved-ticket/uncommitted-aggregate split、blocked Resume bypass、candidate patch target omission 与 180s fixture sizing 偏差；不得以超时或合法 terminal 代替 accepted-result gate |
| RVC-R17 | code-landed | Decision ticket 内 durable pending proposal → Orchestrator typed commit → formal Resolution/ticket finalize；启动 reconciliation 只重放同一 commit |
| RVC-R18 | code-landed | blocked Runtime V2 Resume/Activate 均要求 aggregate matching resolution + request binding，普通 ticket resolved 不再清 blocker |
| RVC-R19 | code-landed | Decision owner server-first allowed-action gate；read model/TUI 提供并预检同一集合，batch 仍以服务端全量预检为准 |
| RVC-R20 | code-landed | provider output schema 在 C3 从 sealed RuntimeOperationContract 动态生成 patch target/parameter constraint；final seal 保留最终一致性裁决 |
| RVC-R21 | code-landed | ContextBudget/CacheUsage 改为 owner-neutral concrete audit DTO，LLM candidate/usage EventHub 与 eventstream payload 无 `map[string]any` bridge |
| RVC-R22 | pending | deployed bounded terminal fixture + real-workspace liveness observation 分层；最终重跑 RVC-R16 |

## 17. Implementation Batches

已有 completed foundations 保留，但 RVC-1 正式重新打开。后续按以下批次执行，禁止跨批次只改 prompt 或只改某个 owner。

### RVC-R5 — Core contract and schema gate

Scope: SM-1～SM-4

- 更新 core contract 四状态机正式语义。
- 新增 effect/receipt/directive/termination/failure 类型及 schema v2。
- 更新 canonical validation/hash/clone/same contract。
- 暂不改变生产执行路径。

Exit gate:

- 所有新类型 canonical round-trip、future-schema fail-fast、stable hash 测试通过。
- core verifier 能检索新 contract mapping。

### RVC-R6 — Action/effect feasibility and seal

Scope: SM-2/SM-3

- RequiredOutput 增加 effect scopes。
- FeasibleOperationKinds 使用 action + capability + effect 交集；`shell/command`、`ssh/command` 这类仅能由 operation parameter 收窄的 dual-use kind 不得仅凭 kind 进入 validation candidate request。
- `mcp/tool_call` 只在 activation 已密封至少一个 enabled、显式 effect metadata 且与 action 相容的 capability 时进入 candidate request；缺失 metadata 必须无候选并经 typed feasibility fact 收敛，不能让 LLM 猜测。
- 分离 state-change/validation candidate schema。
- seal 前正向/反向 effect gate。

Exit gate:

- `stateChanging=false` shell 无法进入 state-change。
- read-only/session MCP 无法关闭 durable state-change。
- mutating operation 无法进入 validation。

### RVC-R7 — Repair directive and anti-replay alignment

Scope: SM-2/SM-3

- reducer 持久化 single repair directive。
- candidate context 以 directive 为唯一 next-action input。
- anti-replay 仅处理 shape，不切换 action。
- accepted repair effect 后原子转 validation。

Exit gate:

- 精确复现 `run-022` lineage 时不再出现 reducer/state-change 与 candidate/validation 相互否定。

### RVC-R8 — MCP and capability normalization

Scope: Activation/SM-2/SM-3

- registry kind 与 operation kind 分层。
- capability owner 显式保存 effect/idempotency/approval policy，activation 只做 canonical seal。
- `RiskLevel`、tool 名称和 MCP session 初始化名称都只是展示/治理提示，绝不可推断 Runtime effect 或 idempotency；缺失 metadata 必须封存为 `unknown`。
- feasibility 与 candidate seal 只消费 exact server/tool 的 sealed capability effect。

Exit gate:

- `Kind=tool` 只有在 owner 声明 `EffectScope=none|ephemeral_session` 后才可用于 validation；只有声明 durable scope 后才可用于 state-change。
- 缺失 metadata、仅 `RiskLevel=read_only` 或仅 `lsp_initialize` 名称都不得产生 non-durable 或 durable effect evidence。

### RVC-R9 — Owner receipt and evidence authority

Scope: SM-3

- 各 owner 增加 execute/query receipt typed command/result。
- owner 持久化 receipt 后再响应。
- evidence commit 根据 observed effect 裁决。
- no-op/unknown/governance outcome 正式处理。

Exit gate:

- decision action 不再被 owner简单回显为 EvidenceKind。
- receipt effect mismatch 在 evidence commit 前拒绝。

### RVC-R10 — Crash recovery and fencing

Scope: SM-3/SM-4

- per-operation claim/fence。
- crash injection 覆盖 claim CAS 前后、execute 前、owner receipt commit 后、evidence CAS 前后、dispatch close 前后、governance grant CAS 后、terminal delivery 各阶段。
- 多实例同 ExecutionKey 合并。

Crash hook 只能作为测试注入点存在，不进入业务策略。每个 hook 必须在真实
file-backed aggregate 上重启新的 controller/module 实例，禁止仅复用进程内 fake
状态模拟 restart。

每个 crash point 必须断言：同一 `ExecutionKey + Fence` 最多一份 owner terminal
receipt、同一 operation/fence 最多一份 committed evidence、durable side effect 不重复、
stale fence 与 duplicate drive 幂等或 conflict，且 restart 后不要求用户手动 Resume。

Exit gate:

- 每个 crash point 最多一个 owner terminal receipt、最多一份 committed evidence、无重复副作用。

### RVC-R11 — Governance and approval

Scope: SM-1/SM-2/SM-3

- typed decision/approval/config/external governance。
- owner pre-effect governance requirement。
- operation-scoped ticket/approval resolution continuation。

#### Operation-scoped continuation contract

普通治理 dispatch 与 owner 在 operation 执行前返回的 `governance_required`
receipt 不是同一种 continuation：后者必须恢复同一个 operation transaction，不能
在 resolution 后重新请求 candidate，也不能只清除 failure 后让原 receipt 被当作
ordinary evidence。

正式持久化 binding 至少必须包含 ticket、governance dispatch/snapshot、source
operation dispatch/operation ID、`ExecutionKey`、governance fence、expected effect、
dependency identity、governance receipt hash 与 binding state。

- binding 由 Orchestrator 在接受 owner 已持久化的 `governance_required` receipt
  后创建；execution key、fence、receipt hash 必须与原 sealed execution ledger
  完全一致。
- ticket、governance dispatch、suspended operation 必须一对一原子绑定；应用层
  不得自行拼 source operation identity。
- `continue` 只能写入不可变 typed governance evidence 与 grant。它不得删除
  receipt、重置 repair ledger、创建 candidate attempt，或把 operation 标记为
  evidence committed。
- 恢复时 Orchestrator 建立当前 authority snapshot 下的 resume dispatch，保持原
  `ExecutionKey` 与 expected effect，但必须通过 CAS 将 governance fence `N`
  推进为 resume fence `N+1`。不得用同一个 `ExecutionKey + Fence` 保存治理前与
  获批后两份 receipt。
- owner request 同时携带经 binding 核验的 typed governance grant。owner 只能在
  grant 与 ticket/dependency/execution identity 全部匹配时执行副作用。
- 若 owner 已返回新的 terminal receipt，Orchestrator 只做 receipt/evidence
  continuation；若 receipt 仍为 `governance_required`、grant 不匹配、fence stale
  或 outcome unknown，必须 structured conflict/governance/failure，禁止盲重放。
- `fail` resolution 只生成 governance-rejected terminal；不得创建 resume dispatch。

实现顺序固定为：Runtime state/clone/repository binding → operation governance
record → ticket adapter 回填 governance dispatch → continue 原子 grant/resume
dispatch → executor/owner concrete command grant → crash/duplicate/stale/file-backed
regression。

#### Receipt identity and fence model

```text
first execution
  ExecutionKey = K
  Fence = N
  Receipt(K, N) = governance_required

governance continue
  Grant binds K/N and immutable governance receipt hash
  resume claim CAS advances Fence to N+1
  owner executes with K/N+1 + Grant(K/N -> N+1)
  Receipt(K, N+1) = accepted / rejected / no_op
```

- receipt persistence identity 是 `ExecutionKey + Fence`，不得只按 ExecutionKey
  去重或恢复。
- executor recovery、evidence commit、query、Result closeout/read model 必须使用
  exact fence lookup；单 ExecutionKey 查询不得进入生产恢复主链。
- governance receipt 与 resume receipt 均不可变保留，不得覆盖、删除或修改。

#### Typed governance grant

共享值类型落到 `internal/pkg/workorchruntimecontract`：

```go
type RuntimeOperationGovernanceGrant struct {
    ResolutionID          common.ResolutionID
    TicketID              common.TicketID
    ExecutionKey          string
    GovernanceFence       uint64
    ResumeFence           uint64
    DependencyKind        string
    SubjectRef            string
    GovernanceReceiptHash string
    EvidenceRefs          []string
}
```

grant 必须验证 `ResumeFence == GovernanceFence + 1`，并匹配 binding、ticket、
resolution、dependency、subject 与 governance receipt hash。

File/Git/Shell/MCP/SSH/Web 继续使用各自 concrete EventHub execute command；只在
这些 command 中增加可选 typed `GovernanceGrant`。禁止新增通用 executor registry
或跨 owner service/repository 调用。

#### Continue/fail atomic transition

`continue` 必须在同一次 aggregate CAS 中写 accepted governance evidence、将
binding 变为 `granted`、关闭 governance dispatch、创建当前 authority 下的 resume
dispatch、保持原 ExecutionKey、推进 resume fence 并持久化 typed grant。不得创建
candidate attempt 或重置 repair ledger。

`fail` 只允许 binding → `rejected`、写 governance-rejected evidence 并生成 terminal；
不得创建 resume dispatch 或执行 source operation。

Exit gate:

- `approval_required` 不再进入普通 execution repair。
- duplicate/stale resolution 幂等/diagnostic。
- continue 后恢复同一 ExecutionKey 的下一 fence，owner 只在 typed grant 有效时执行。
- crash/restart 后不会把 `Receipt(K,N)=governance_required` 当作 ordinary evidence。

### RVC-R12 — Lifecycle cancellation/supersede

Scope: SM-1/SM-4

- termination record CAS。
- active V2 Cancel/Supersede 禁止 direct close。
- terminal projection/release/lineage/read model 对齐。

Exit gate:

- 合法与禁止迁移、重复 command、terminal immutable 测试通过。

### RVC-R13 — Failure taxonomy and retry budgets

Scope: SM-2/SM-4/LLM/owners

- 拆分 provider、owner、CAS、unknown outcome、contract defect、delivery failure。
- 独立预算与 read model。
- 删除 catch-all drive error → runtime contract defect。

新增并持久化独立 policy/ledger：

```go
type RuntimeRecoveryPolicy struct {
    MaxProviderTransportAttempts int
    MaxOperationRecoveryAttempts int
    MaxCASReloadAttempts         int
    MaxTerminalDeliveryAttempts  int
}

type RuntimeRecoveryLedger struct {
    ProviderTransportAttempts int
    OperationRecoveryAttempts int
    CASReloadAttempts         int
    TerminalDeliveryAttempts  int
}
```

- policy 在 bootstrap/activation 时封存；credential、base URL、client 不持久化。
- candidate contract attempt 固定 0/1，不进入 recovery policy。
- per-focus repair ledger 与 recovery ledger 分离，任何一个成功/重置不得清零另一个。
- CAS conflict 只消耗 CAS reload budget，不写业务 failure/evidence。
- terminal delivery failure 只消耗 delivery budget，不改变 reducer terminal。
- provider auth/configuration 直接进入 configuration governance，不消耗普通 repair。
- operation recovery 耗尽后才进入 structured governance/fail，禁止直接标记
  runtime contract defect。

Exit gate:

- owner transient/CAS conflict 不终态化业务 Run。
- provider auth/config 进入 configuration governance。
- bootstrap/state/result/read model 能独立展示每类 policy、used、remaining、exhausted。

### RVC-R14 — Startup recovery, migration and read model

Scope: SM-4/Run owner/projection

- Run owner 提供 concrete typed Runtime V2 recovery inventory；Orchestrator 不再使用
  通用 Run `List()` 自行推断 recovery class。
- schema v2-only startup/load gate；旧 fixture 直接重建，不提供 schema 迁移路径。
- terminal delivery restart retry。
- read model 展示 directive/effect/receipt/budgets。

Run owner 的 `pkg/events` 必须定义分页 query/result。inventory item 至少包含
workspace/run identity、aggregate revision、active dispatch ID、ticket ID、terminal
delivery state/attempts，并显式给出以下 recovery class：

- `active_no_dispatch`；
- `active_sealed_dispatch`；
- `blocked_governance`；
- `terminal_delivery_pending`。

Orchestrator 只按 typed class 执行：

- active/no dispatch → schedule drive；
- active/sealed dispatch → exact receipt/fence recovery；
- blocked governance → 验证 binding/ticket，不自动 Resume；
- terminal delivery pending → retry Result → Run/Workspace delivery；
- terminal delivered → no-op。

Application/projection 不得扫描 aggregate 或选择 recovery action。

read model 至少投影：RepairDirective、required/expected/observed effect、execution
key/fence/state、receipt outcome/ref/hash、governance binding/grant、outcome_unknown、
独立预算、terminal delivery pending/attempts。TUI/Web 只消费该 projection。

Exit gate:

- 服务重启无需用户手动 Resume 即可继续安全 active Run。
- projection 仍不参与 authority。

### RVC-R15 — Deterministic file-backed E2E

Scope: all

- 使用 mock provider 和真实 file backend。
- 至少两 focus：state-change → validation → assessment → focus advance。
- 注入 crash、duplicate、stale、approval、provider transient、CAS conflict。

必须增加一个真实 file-backed 主场景：

```text
focus-1
  durable state-change
  validation
  satisfied assessment
  atomic focus advance

focus-2
  approval-required operation
  governance continue
  same ExecutionKey / next Fence resume
  durable state-change
  provider transient retry
  validation
  assessment
  terminal delivery
```

脚本必须检查实际持久化 aggregate、owner receipt、evidence、Result、Run 与 Workspace
状态；不得只通过 `go test -run` 聚合已有测试名称作为 RVC-R15 完成证明。

Exit gate:

- 全部四状态机 cross-contract assertions 通过。
- crash、duplicate、stale、approval、provider transient、CAS conflict 与 terminal
  delivery retry 在同一 file-backed 场景中通过。

### RVC-R16 — Deployed dual-workspace Live

Scope: all

- 部署 clean binary。
- 先创建两个全新 bounded positive fixtures，旧 terminal/污染 Run 不 Resume。
- bounded fixtures 通过后，再分别观察 LSP-Bridge 与 ai-proxy 真实 canary；需要继续原目标时只创建显式 typed successor activation，不自动 successor。
- bounded fixtures 负责 accepted terminal gate；真实 canary 负责 liveness 与状态机合法性，不共用 180 秒 terminal SLA。
- Live 证据只写 `/tmp`，不污染仓库。

Exit gate 见第 20 节。

### RVC-R17 — Runtime V2 governance durable proposal/commit saga

Scope: SM-1/SM-2/SM-3、Decision owner、Orchestrator、kernel/governance

#### RVC-R17.1 Authority split

| Object | Formal authority |
| --- | --- |
| ticket question、allowed actions、operator input | Decision owner |
| Runtime V2 request binding（ticket/dispatch/snapshot/dependency/subject） | Orchestrator aggregate |
| continue/fail outcome、governance evidence、next reducer action | Orchestrator aggregate |
| resolved ticket / resolution read model | Decision owner 对已提交 Runtime V2 outcome 的投影 |
| Run blocked/active/terminal | SM-1 owner projection，输入只能来自 Orchestrator committed aggregate |

`runtime_v2_governance` 不得继续沿用“先调用 Decision owner 普通 resolve，再尝试
Orchestrator commit”的顺序。跨 owner 没有数据库事务，必须建立可重放、可审计的
durable saga。

#### RVC-R17.2 Proposal model

Decision owner 增加 focused persisted proposal，不新增 Module/Block：

```go
type RuntimeGovernanceResolutionProposalState string

const (
    RuntimeGovernanceResolutionProposalPending   RuntimeGovernanceResolutionProposalState = "pending"
    RuntimeGovernanceResolutionProposalCommitted RuntimeGovernanceResolutionProposalState = "committed"
    RuntimeGovernanceResolutionProposalRejected  RuntimeGovernanceResolutionProposalState = "rejected"
)

type RuntimeGovernanceResolutionProposal struct {
    ResolutionID   common.ResolutionID
    TicketID       common.TicketID
    WorkspaceID    common.WorkspaceID
    RunID          common.RunID
    ResolutionType string
    Outcome        runtimecontract.RuntimeGovernanceResolutionOutcome
    Summary        string
    PayloadHash    string
    State          RuntimeGovernanceResolutionProposalState
    FailureClass   string
    FailureReason  string
    CreatedAt      string
    UpdatedAt      string
}
```

规则：

- proposal ID 使用 deterministic resolution ID；相同 ID + payload hash 幂等，不同 payload conflict；
- proposal 是 Decision owner 内部恢复事实，不是正式 resolved resolution；
- `pending` 时 ticket 必须保持 open，Run 必须保持 blocked；
- `committed` 只允许在 Orchestrator 返回 committed/duplicate 后写入；
- contract/stale/rejected failure 可写 `rejected`，ticket 仍 open；owner transient 保持 pending 并重试；
- 不为 2026-07-18 以前已产生的错误 resolution 增加兼容迁移；旧 Run 保持审计，新部署后通过新 typed activation 验证。

#### RVC-R17.3 Command sequence

```text
TUI/Web/CLI
  -> Decision EnsureResolved request
  -> Decision owner load ticket + validate scope/state/allowed action/payload
  -> persist pending proposal (ticket still open)
  -> Send CommitRuntimeGovernanceTicketResolutionCommand
       -> Orchestrator load aggregate
       -> validate ticket binding + outcome + snapshot/dispatch/dependency
       -> CAS commit governance resolution/evidence/reducer decision
       -> return committed | duplicate | stale_diagnostic
  -> committed/duplicate:
       Decision owner create/ensure formal Resolution
       Decision owner mark ticket resolved
       mark proposal committed
       publish resolved notification
  -> failure:
       do not mark ticket resolved
       persist retryable/rejected proposal state
```

Orchestrator committed 后可以按自身 aggregate 继续 drive/terminal delivery；Decision
owner finalize 失败只造成 read-model lag，不得回滚 aggregate。Decision block 在 Start
或 BackgroundRoutine 中扫描 pending proposals，重新发送同一 typed commit；duplicate
返回后完成本 owner resolution/ticket finalize。禁止 application watcher代替该 saga
发送普通 Resume。

#### RVC-R17.4 Allowed outcome mapping

对 `runtime_v2_governance` 固化：

| Resolution type | Runtime outcome | Meaning |
| --- | --- | --- |
| `provide_missing_info` | `continue` | 写 accepted governance evidence，回到同 reducer/operation continuation |
| `reject_current_path` | `fail` | 写 governance rejected evidence，产生 typed terminal |
| `replan` | invalid for current ticket | 产品层新 typed activation/restart，不是当前 Run resolution |

generic Decision 可以保留自己的 replan 产品语义，但不得映射为当前 Runtime V2 aggregate
的 continue/fail。Runtime V2 需要重新定义目标时，先按正式治理结论终止/保留当前
Run，再由显式 activation 创建 successor；不得自动 successor。

Exit gate:

- invalid/unallowed resolution 在任何 Resolution/ticket resolved 写入前被拒绝；
- proposal persisted 后任意 crash/restart 均可收敛到 committed/duplicate 或 rejected；
- Orchestrator commit failure 不产生 resolved ticket；
- Decision finalize failure 不丢失已提交 aggregate，重启后由 proposal 幂等完成；
- 同一 resolution 最多一份 governance evidence、一次 reducer effect、一个正式 Resolution。

### RVC-R18 — SM-1 blocked Resume authority gate

Scope: SM-1/SM-4、Orchestrator lifecycle、HTTP scheduler、activationports

`ResumeRunCommand` 必须区分“调度 active Run”与“消费 blocked governance resolution”：

```text
active Run
  -> repair active-run projection if needed
  -> schedule same DriveRuntimeV2

blocked Runtime V2 Run
  -> load aggregate binding
  -> matching committed governance resolution exists?
       no  -> ErrBlocked / no state mutation
       yes -> projection-only blocked -> active repair
              schedule same DriveRuntimeV2

terminal Run
  -> conflict; new activation only
```

实现要求：

- `CanResumeRun` 不得以 `ticket.State != open` 或 `approval.State != pending` 作为 Runtime V2 sufficient proof；
- Runtime V2 proof 必须来自 aggregate 中匹配 current blocker 的 resolution record，至少匹配 ticket、dispatch、authority snapshot、dependency、subject 与 outcome；
- 将当前无条件 `ActivateRuntimeV2Run` 拆为内部 projection helper，或要求传入并验证 typed continuation proof；不得保留可由普通 command 直接清 blocker 的 public facade；
- `continueRuntimeV2Governance` 使用 commit result/aggregate proof；ordinary Resume 只能调度 active Run，或修复“aggregate 已 committed、owner projection 尚 blocked”的单边投影；
- execution activation `intent=resume` 遇到 unresolved blocked Runtime V2 时返回当前 governed wait/409，不得产生 `blocked → active → blocked` 抖动；
- startup `blocked_governance` inventory 只验证 binding/proposal/ticket，不自动 Resume；只有 aggregate committed resolution 才进入 continuation/recovery class。

Exit gate:

- ticket resolved 但 aggregate 无 resolution：Resume 返回 blocked/conflict，Run/Workspace 不变；
- aggregate committed 但 Run 仍 blocked：Resume/startup recovery 可幂等修复 projection；
- duplicate Resume 不重复 drive/evidence；
- terminal、stale ticket、mismatched workspace/run/snapshot 均拒绝。

### RVC-R19 — Allowed recovery action end-to-end contract

Scope: Decision owner、application governanceports、HTTP、CLI/TUI、Web、client/port

`AllowedRecoveryActions` 是 ticket owner 的正式输入合同，不是展示提示。动作校验必须
在服务端执行，端侧过滤只是 UX。

规则：

- Decision owner 在 proposal/resolution 写入前执行最终校验；
- application `ValidateDecisionResolutionPayloadForTicket` 同时校验 normalized resolution type 与 allowed actions，尽早返回 400/invalid argument；
- 若 ticket 没有 allowed actions，generic ticket 使用明确的 owner default policy；`runtime_v2_governance` 不允许缺省为任意 supported resolution；
- TUI `/decision` 帮助、补全和命令执行按当前 ticket actions 动态生成；不允许的 `/decision replan` 在本地提示并由服务端再次拒绝；
- Web 每行按钮按 allowed actions 渲染；batch action 必须属于所有选中 ticket 的交集，否则整批拒绝，禁止部分提交；
- CLI 显式 `--type` 同样由服务端校验；agent 默认 `provide_missing_info` 仅在 ticket 允许时使用；
- read model/client/port 必须完整携带 allowed actions，不从 question/reason 自然语言推断；
- `replan` 对 Runtime V2 blocker 显示为独立“创建新 activation/restart”操作时，必须走 activation API，不调用 resolve endpoint。

Exit gate:

- TUI/Web/CLI/REST/batch 对同一 ticket 得到一致动作集合；
- unallowed action 无 proposal、Resolution、ticket update、aggregate update；
- batch mixed-actions 全拒绝且零部分写入；
- action projection missing/stale 时端侧不猜测，服务端仍保护 authority。

### RVC-R20 — Descriptor-driven candidate parameter closure

Scope: SM-3、`workorchruntimecontract`、blocks/llm candidate renderer/provider

当前 final validator 对 `patch/apply` 缺 target 的拒绝是正确行为，不得在 Orchestrator
里按 operation ID、文件名或 failure text 自动补参数。收口目标是在 provider 输出边界
提前暴露 descriptor 的机械参数合同，并让 attempt 1 能针对 exact contract failure 修正。

实现要求：

- 从 `RuntimeOperationDescriptor` / operation parameter contract 生成 action-specific provider schema/context；不得维护第二份手写 operation 规则表；
- `patch/apply|patch/preview` 至少要求 `patchType`，并要求 `targetRef` 或 `path` 之一；resolved target 必须存在于 operation `TargetRefs`；
- `file/edit` 要求 path/oldText/newText；`file/write` 保持独立 durable descriptor；shell state-changing target、MCP capability effect 等继续由现有 typed contract校验；
- JSON/schema 无法表达的 cross-field membership 仍由 final validator裁决，不得降低 seal gate；
- attempt 1 使用同一 authority transaction/stable prefix，在 volatile context 中携带 exact candidate contract failure：operation ID、kind、missing/conflicting parameter、descriptor rules 和 rejected shape hash；
- attempt 1 不复用 malformed raw response，不把失败正文复制为新 candidate；
- 两次均 invalid 时继续 `candidate_retry_exhausted` typed terminal，这属于正确 bounded failure；不得转普通 execution repair或第三次 candidate。

Exit gate:

- attempt 0 缺 target、attempt 1 补齐 target 后可 seal/execute；
- attempt 0/1 均缺 target 时只产生两个 persisted attempt 并 terminal；
- stable prefix attempt 0/1 不漂移；
- descriptor/schema/final validator 对所有 operation kinds 一致，无 domain-specific inference。

### RVC-R21 — Cross-owner typed LLM audit contract

Scope: blocks/llm、eventstream、read model

将 `LLMUsageRecordedData.ContextBudget` 与 `CacheUsage` 的 `map[string]any` 替换为
concrete typed DTO，例如：

```go
type LLMContextBudgetAudit struct {
    ContextWindowTokens      int
    EffectiveMaxOutputTokens int
    ActualTokens             int
    ActualBytes              int
    MaxTokens                int
    MaxBytes                 int
    CacheStrategy            string
    StablePrefixHash         string
    ProviderSubmitted        bool
    OverBudget               bool
    Truncated                bool
}

type LLMCacheUsageAudit struct {
    CacheReadTokens         int
    CacheWriteTokens        int
    CacheCreationTokens     int
    CacheableInputTokens    int
    NonCacheableInputTokens int
    Estimated               bool
    Source                  string
}
```

类型应复用 `workorchruntimecontract` 已有 provider audit value，或在 LLM owner
`pkg/events` 定义 owner-owned DTO；不得通过 JSON/map bridge 传输。eventstream wire
payload 可以在持久化边界编码，但组件内 EventHub data 必须保持 concrete typed。

Exit gate:

- EventHub contract 扫描不再命中 LLM usage `map[string]any`；
- native/estimated/no-cache usage round-trip；
- read model、SSE/HTTP 输出字段保持兼容的产品 JSON 形状，但不把 wire shape反向作为组件合同。

### RVC-R22 — Bounded deployed gate and real-workspace liveness observation

Scope: verification scripts、deployed fixtures、RVC-R16 final rerun

将 deployed 验收拆为两层：

1. **bounded terminal gate**：专用双 workspace fixture，每 workspace ≤2 focuses、每 focus
   ≤4 coverage items，180 秒内必须 accepted Result、Run completed、activeRun release；
2. **real-workspace liveness observation**：LSP-Bridge/ai-proxy 等大型任务验证 revision、
   accepted/rejected evidence、focus、governance/terminal 是否按合同正向变化，不要求在固定
   180 秒内完成数百 coverage items。

脚本要求：

- terminal gate 不复用历史污染 Run；每次创建新 workspace/Goal/DoD/Run 或确定性 clean fixture；
- real-workspace observation 不对 terminal Run Resume，不对 unresolved governance 自动提交动作；
- timeout 输出必须保存完整 final snapshot、ticket/approval、aggregate operation/receipt、usage audit 与 readyz；
- 合法 governed wait 计为 liveness 合规但不计为 accepted terminal pass；
- typed rejected terminal 计为状态机收敛，但不计为 positive-convergence accepted pass；
- large workspace 需声明 focus/coverage 规模和预计 observation policy，禁止用统一 180 秒 terminal SLA；
- artifacts 只写 `/tmp`。

Exit gate:

- bounded dual workspace terminal gate 连续通过；
- LSP/ai-proxy observation 无非法迁移、无 authority split、无 projection lag、无重复副作用；
- 最终重新执行 RVC-R16，两个指定 positive fixtures 均 accepted，真实 canary 另行记录 liveness 结论。

### 17.1 Mandatory closure order

后续代码必须按以下依赖顺序逐项收口，前一项 exit gate 未通过时不得把后一项标记
completed：

1. RVC-R17：Decision durable proposal → Orchestrator commit → owner finalize/reconcile；
2. RVC-R18：blocked Resume aggregate proof gate 与 projection-only continuation；
3. RVC-R19：server-first allowed action validation，再收口 TUI/Web/CLI/batch；
4. RVC-R20：descriptor-driven candidate schema 与 exact attempt-1 contract failure；
5. RVC-R21：LLM audit typed EventHub DTO；
6. RVC-R22：bounded deployed fixture 与 real-workspace liveness 分层；
7. RVC-R16 final rerun：先 bounded dual terminal gate，再真实 canary observation。

RVC-R11～R15 保持 code-landed foundation；RVC-R17～R22 不得回退 exact fence、typed
grant、independent budget、startup inventory、terminal delivery lane 或 schema v2-only。

每批完成后依次运行 focused package tests、boundary verifier、全量测试及对应 file/live
gate，并同步第 21 节列出的 SoT。单纯 `go test ./...` 通过不等于该批完成。

## 18. Required Test Matrix

### 18.1 SM-1

- pending→active；active↔blocked；terminal immutable。
- completed/failed 只能由 reducer terminal 投影。
- active V2 cancel/supersede 必须先有 termination record。
- stale terminal release 不抢占新 active run。
- blocked Runtime V2 ticket resolved 但 aggregate 无 matching resolution 时，Resume 不改变 Run/Workspace。
- aggregate resolution committed 但 Run 仍 blocked 时，projection repair 幂等恢复 active/terminal。
- execution activation `intent=resume` 不产生 `blocked→active→blocked` 抖动。

### 18.2 SM-2

- rejected validation → persisted state-change directive。
- accepted durable repair → validation directive。
- session/no-op evidence 不算 repair progress。
- repair/candidate/provider/operation budgets 互不污染。
- partial assessment 原子关闭 item 并 advance focus。

### 18.3 SM-3

- action/effect compatibility。
- exact transaction/candidate hash/operation execution key。
- owner receipt accepted/rejected/governance/no-op/unknown。
- stale/duplicate receipt 和 evidence。
- crash/fence/multi-instance。
- governance `Receipt(K,N)` → grant → resume `Receipt(K,N+1)`。
- stale/mismatched grant、duplicate resolution、再次 governance-required。
- dependency graph 在 recovery 后不重复已完成节点。
- Decision proposal pending → Orchestrator committed → Decision finalized saga。
- crash after proposal、after aggregate commit、before ticket update、after Resolution create 的逐点恢复。
- invalid/unallowed resolution 零正式写入；same proposal duplicate 幂等、different payload conflict。
- `patch/apply` attempt 0 缺 target、attempt 1 修正；两次缺失 bounded terminal。

### 18.4 SM-4

- startup inventory active/no-dispatch。
- active/sealed receipt recovery。
- blocked wait 不自动继续。
- terminal delivery restart retry。
- CAS conflict reload，不生成伪 terminal。
- provider/operation/CAS/delivery budget 各自独立消耗与耗尽。
- projection high-watermark/readiness 不反向驱动 reducer。
- pending governance proposal startup reconciliation 不依赖 application Resume watcher。

### 18.5 Cross-component regressions

- ai-proxy optional bearer empty/non-empty。
- stable prefix attempt 0/1。
- MCP registry `tool` normalization。
- shell `stateChanging=false` seal rejection。
- `lsp_initialize` 仅在 capability owner 显式声明 `ephemeral_session` 时具有 session effect；名称本身不产生推断。
- web/file approval-required governance。
- exact `run-021` / `run-022` failure lineage fixtures。
- TUI/Web/CLI/HTTP/batch 只允许 ticket `AllowedRecoveryActions`；mixed batch 零部分写入。
- Runtime V2 `replan` 只能进入显式 activation/restart，不能 resolve current governance ticket。
- LLM usage ContextBudget/CacheUsage typed EventHub round-trip，无 `map[string]any`。
- bounded deployed fixture terminal SLA 与 real-workspace liveness policy 分离。

## 19. Verification Commands

每批先执行 focused tests，再逐步扩大：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/blocks/{file,git,shell,mcp,ssh,web,llm}/... -count=1
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/kernel/governance/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/service/tui/... -count=1
go test ./internal/services/workorch/biz/backends/... -count=1
make fmt-check
make vet
make verify-boundary-contract
make test
```

升级现有 file/live gate，并按需增加 effect/crash 专用 gate：

```bash
make verify-runtime-v2-effect-contract
make verify-runtime-v2-crash-recovery
make verify-runtime-v2-positive-convergence-file
make verify-runtime-v2-positive-convergence-live
```

`verify-runtime-v2-positive-convergence-file/live` 必须升级为本方案定义的真实行为验证，不得继续只聚合已有测试名称。

新增结构扫描必须检查：

```bash
rg -n 'ContextBudget map\[string\]any|CacheUsage map\[string\]any' internal/modules/blocks/llm/pkg/events
rg -n 'case "replan"|batchResolve\(.replan' internal/services/workorch/service/tui web/workorch-console/src
rg -n 'ActivateRuntimeV2Run|CanResumeRun' internal/modules/kernel/orchestrator
```

扫描命中不直接代表失败，但每处必须由新 typed gate/test 证明不会绕过 authority。

## 20. Deployed Live Acceptance

### 20.1 Service/provider preflight

- deployed binary hash 与 clean build 一致；
- `/readyz` 为 ready；
- source/projected revision 追平；
- ai-proxy catalog fresh；
- workspace target 为预期 `providerId/modelId`；
- credential/base URL 未进入 workspace/run persisted authority。

### 20.2 Bounded terminal fixture proof

每个 bounded positive fixture 必须证明：

1. 至少跨越两个 focus；
2. 至少一个 durable owner receipt；
3. 对应 accepted state-change evidence，且 observed effect 相容；
4. 至少一个 accepted validation evidence；
5. 进入 `produce_assessment`；
6. assessment 关闭明确 coverage items；
7. focus 原子 advance；
8. 最终 accepted Result；
9. Run terminal、workspace activeRun 释放；
10. projection revision 追平。

fixture 额外限制：

- ≤2 focuses；
- 每 focus ≤4 coverage items；
- 使用全新 Run，不 Resume 历史 terminal/invalid governance Run；
- 180 秒 terminal SLA 只适用于该 bounded fixture。

### 20.3 Real-workspace liveness proof

真实 LSP-Bridge/ai-proxy canary 至少证明：

1. source/projected revision 追平；
2. 一段 observation window 内 state revision、evidence、focus 或合法 governed/terminal 状态至少一项可解释变化；
3. blocked governance 的 ticket action 与 aggregate binding 一致；
4. resolved ticket 必须有 matching aggregate resolution，或明确处于 pending proposal reconciliation；
5. candidate contract failure、execution failure、provider transient 各自进入正确预算/终态；
6. 不把大型 DoD 未在 180 秒完成误判为状态机死锁。

### 20.4 Negative assertions

- 无 `stateChanging=false` operation 被记为 state-change；
- 无 session/read-only MCP 被记为 durable state-change；
- 无 reducer/candidate action contradiction；
- 无相同 ExecutionKey 重复副作用；
- 无 CAS/owner transient 被误记为 runtime contract defect；
- 无 terminal Run Resume；
- 无 stable prefix unexpected drift。
- 无 ticket resolved / aggregate unresolved split；
- 无 blocked Runtime V2 被普通 Resume 清 blocker；
- 无端侧提交不在 allowed actions 中的 resolution；
- 无 typed rejected terminal 被误报为 positive accepted pass。

## 21. Documentation Closure

代码收口期间必须同步：

- `docs/00-core/runtime-stage-contract.md`：四状态机正式语义、effect/receipt/recovery/lifecycle。
- `docs/00-core/runtime-request-cache-note.md`：candidate contract version 与 prefix stability。
- `internal/modules/kernel/orchestrator/readme.md`：实现落点与 EventHub topic。
- capability owner `readme.md`：receipt/effect/governance contract。
- `docs/01-overview/current-state.md`、`current-unclosed-matrix.md`、active overview：真实进度。

任何批次未同步上述 SoT，不得标记 completed。

## 22. Prohibited Partial Fixes

- 只修改 prompt，未修改 seal/evidence gate。
- 只禁止某个 shell 命令或 MCP tool 名称。
- 在 reducer 中识别 LSP、Go、测试命令、文件名或 workspace ID。
- 仅增加 anti-replay 文案，未消除 action authority 冲突。
- 仅在 projection/watchdog 自动 Resume。
- 只在 TUI/Web 隐藏 `replan`，但服务端仍接受 unallowed action。
- 只给 `CanResumeRun` 增加 ticket/resolution 文件判断，不核对 aggregate binding/resolution。
- Decision owner 继续先写 resolved，再依赖失败补偿删除 resolution/ticket。
- 用进程内 retry 代替 durable governance proposal。
- Orchestrator 根据 operation ID/文件名自动补 `targetRef/path`。
- 用真实 396-item workspace 的 180 秒超时作为唯一 terminal gate。
- crash recovery 直接重放 sealed operation。
- 继续让 owner 回显 Orchestrator 指定 EvidenceKind。
- 将所有 owner/provider/CAS 错误统一终态化。
- 对任何非 current-schema aggregate/evidence 做语义升级或默认回填。
- 用单 workspace 单 focus dispatch 证明“正向收敛完成”。

## 23. Definition Of Done

RVC-1 只有同时满足以下条件才能关闭：

1. RVC-R5～R15 foundation 保持通过，RVC-R17～R22 全部完成，并最终重新通过 RVC-R16；
2. core contract、module readme、contract types 与代码一致；
3. effect、receipt、repair directive、governance、termination 和 recovery 均为 persisted typed authority；
4. 四状态机合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试齐备；
5. boundary verifier、full tests、file-backed E2E 通过；
6. 两个 bounded positive fixtures 均跨 focus并产生 accepted Result；LSP-Bridge/ai-proxy 真实 canary 均满足独立 liveness 断言且无非法迁移；
7. 无 credential/runtime client state 序列化回归；
8. active roadmap、current-state 和 unclosed matrix 不再保留虚假 code-closed 状态；
9. Runtime V2 governance 不存在 resolved-ticket/uncommitted-aggregate 或 aggregate-committed/unrecoverable-owner-projection 状态；
10. 普通 Resume、TUI/Web/CLI/HTTP/batch 均不能绕过 aggregate authority 与 allowed action contract；
11. bounded deployed terminal gate 与真实长任务 liveness observation 均有独立证据。

在上述条件满足前，本项状态保持：**active — code reopened**。
