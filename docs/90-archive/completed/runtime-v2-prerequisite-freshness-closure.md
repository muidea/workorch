# Runtime V2 Prerequisite Freshness System Closure

Status: completed

Type: implementation closure plan

Last Updated: 2026-07-20

Implementation runbook: [runtime-v2-prerequisite-freshness-component-closure-matrix.md](runtime-v2-prerequisite-freshness-component-closure-matrix.md)

End-to-end supplement (continuation, model, provider failure and boundary gates): [runtime-v2-system-component-closure-supplement.md](runtime-v2-system-component-closure-supplement.md)

Single implementation queue and current P5 verifier status: [runtime-v2-code-closure-execution-spec.md](runtime-v2-code-closure-execution-spec.md)

## Implementation Entry Point And Document Routing

本记录保留原实施顺序以便审计。三个 archived 文档是同一收口工作的不同
视图，禁止把其中任一份当成并列的 Runtime V2 状态机、或在不同文件中写出互相
矛盾的“完成”结论。

| When the change concerns | Start from | Must update before marking the card locally closed | Do not use as completion evidence |
| --- | --- | --- | --- |
| mutable prerequisite, receipt, stale, invalidation, refresh budget or restart | this parent plan and the matching `C0`--`C8`/`P0`--`P5` row in the [component matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md) | this plan's implementation ledger, the changed owner/orchestrator implementation contract, and the card completion record | a successful UI request, an owner-specific retry, or an in-memory-only fixture |
| Decision/Approval continuation, unified `resume`, workspace model preference, sealed target or provider timeout | the matching `R1`--`R5` and `C-R*` card in the [system supplement](runtime-v2-system-component-closure-supplement.md) | the supplement's local evidence section and the parent implementation ledger; update `runtime-stage-contract.md` only if a neutral transition changes | a raw HTTP 2xx/4xx response, a Console build, or a second application Resume |
| EventHub/base-biz/module shape, verifier portability or component ownership | the fixed interaction contract and `C0`--`C8` card in the [component matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md) | component `readme.md` when its external typed contract changes, plus the verifier/direct regression named by the card | a blanket verifier exclusion or a test that directly uses another owner's repository/service |
| a newly observed deployed symptom | the closest failed assertion in this parent plan's scenario matrix, then its owner card | a new reproducible direct regression before a deployed retry; append the trace identity and the reopened earliest card | a UI-side compensating action or a claim that another component is at fault without the persisted lineage |

The only allowed implementation flow is:

```text
pick one component delivery card
  -> declare or validate its concrete typed boundary
  -> add the owner-local legal and forbidden-transition regression
  -> implement its sole formal-state mutation
  -> prove duplicate/restart/CAS and the named budget boundary
  -> run the card's handoff gate
  -> append an evidence record and advance only to the named next card
```

`locally closed` means the card's own state writer and typed boundary have
passed this flow. `system closed` additionally requires its downstream
integration exit and, for this plan, RPF-5 fresh deployed accepted-terminal
evidence. No status may skip directly from an owner package pass to a system or
production conclusion.

## Verification Re-baseline (2026-07-20)

The File/Shell/Git positive-chain and the aggregate package are **not**
currently green as one combined gate.  Earlier local success records remain
useful historical evidence for their named narrow assertions, but they are not
evidence for P5 until the following failures are removed and the whole
orchestrator package is rerun.  This section has precedence over any earlier
wording in this plan or its runbook that says the positive chain "now reaches"
an accepted terminal.

| Failing regression | Classification | Earliest card to work | Required correction and proof | Explicitly not permitted |
| --- | --- | --- | --- | --- |
| `TestAcquireAndCommitTargetSnapshotPrerequisiteAdvancesTypedFacts` | fixture/adapter contract drift | C0/C1 | Make the target-snapshot observation use the exact canonical completed-prerequisite identity. Preserve the invariant that a mutable satisfied prerequisite cannot accept a differently named observation. | weakening prerequisite/observation identity validation |
| `TestRuntimeV2MainlineRecordsInfeasibleCandidateBeforeDispatchSeal` and `TestRuntimeV2MainlineRetriesCorrectedInfeasibleCandidateBeforeSeal` | fixture contract drift | C0/C1 | Give each File owner-admissibility fixture its matching persisted freshness observation, then retain the original infeasible-candidate assertions. | bypassing the pre-seal freshness gate so the old fixture passes |
| `TestDriveRuntimeV2FileBackedFreshFileShellGitAcceptance` | sealed-bootstrap fixture contract drift | activation/C0 contract characterization | The fixture produced `git/write` but omitted Git's canonical `operation_admissibility` prerequisite. Seal that requirement exactly as production activation derives it from the executable capability contract; then prove its observation/inventory is invalidated and reacquired after File effects, with a new candidate at attempt 0. | fixture-only inventory injection, an owner/UI retry, or classifying the missing inventory as candidate failure |

The required recovery order is deliberately narrow:

```text
B0: correct invalid fixture owner/observation identities and rerun direct tests
 -> B1: audit each sealed bootstrap so every executable capability has its
        derived owner operation-admissibility prerequisite
 -> B2: rerun the File -> Shell -> Git invalidation/reacquisition positive gate
 -> B3: rerun the complete orchestrator package, P5 crash gates and verifiers
 -> B4: only after all local gates pass, capture a newly deployed accepted trace
```

`B0` repairs test inputs only and must not be reported as a runtime behavior
fix. `B1` protects the activation invariant: capability-to-owner mapping and
admissibility requirement derivation happen before a Run is sealed, while the
Orchestrator only consumes the persisted requirement. If a future positive
trace has a correctly sealed requirement but its inventory is absent after an
effect, reopen C1/C4 and locate request, commit, merge/filter or lookup. C4
continues to own the meaning of the Git acquisition binding; only C1 decides
invalidation, pending state, refresh budget or the next action.

### Managed M2 Re-baseline (2026-07-20)

The current managed M2 failure is no longer a MCP owner, activation or
candidate-authority failure. The read-only MCP capability now follows the
intended single path:

```text
registry explicit none/safe_to_replay
  -> activation-sealed RuntimeMCPCapabilityContext
  -> validation capability inventory and candidate seal
  -> LLM EventHub revalidation with the same activation
  -> MCP owner receipt/evidence for focus-002
```

The persisted trace then fails while the managed mock constructs the following
assessment from the *whole* provider prompt:

```text
current focus coverage: cwi-002
mock response coverage: cwi-001, cwi-002
strict boundary result: cwi-001 is outside current slice coverage
```

The strict rejection is required behavior. The next card is therefore the
fixture-only M2-H1 contract in the [single execution
specification](runtime-v2-code-closure-execution-spec.md#m2-h1--managed-assessment-current-slice-isolation): derive assessment coverage from the final
structured current Focus/RequiredOutput, fail deterministically on an absent
or ambiguous slice, and retain only current-focus evidence refs. It must not
alter `RuntimeAssessmentCandidate`, reducer coverage checks, candidate budget,
Decision, Resume or a projection. Its hermetic fixture self-test and retained
artifact M2 run are now green (`ws-9370e08122ab50ee` / `run-001`, completed);
the next managed integration card is M3.

## Purpose

本方案用于系统收口 Runtime V2 中“可变 prerequisite 在 workspace 或 owner 状态发生变化后仍被继续使用”的问题，并作为后续逐批代码实现、组件独立回归和 deployed 验收的执行清单。

本方案不是新的 Runtime 核心语义 SoT。中立状态机规则仍以
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md) 为准，生产实现仍以
`internal/modules/kernel/orchestrator/readme.md` 与
`internal/pkg/workorchruntimecontract` 为准。实现每一批代码时，必须同步更新这些权威合同中受影响的条款；本方案完成后迁入 archive。

目标不是分别为 File、Shell、Git 增加新的特殊重试，而是建立 owner-neutral 的统一闭环：

```text
typed prerequisite acquisition
  -> persisted observation + resource dependencies
  -> candidate / sealed dispatch
  -> owner receipt reports durable resource changes or typed stale
  -> Orchestrator atomically invalidates intersecting observations
  -> satisfied prerequisite returns to pending
  -> reducer selects acquire_prerequisite
  -> owner reacquires a fresh observation
  -> new authority transaction, candidate attempt 0
```

## Reopened Production Fact

2026-07-19 对 workspace `ws-0f969ef3003adb2c` 的 fresh deployed 复核给出了以下反证：

- `run-005` 使用封存模型 `ai-proxy/deepseek-v4-flash`，lineage 为 `run-004 -> run-005`；
- File owner 已允许受约束的新文件目标，`main.go` 创建成功；
- owner receipt、sealed dispatch、state-change evidence 正常持久化，`cwi-026` 被关闭；
- 后续 managed validation 尚未执行，Run 以
  `candidate_retry_exhausted: runtime shell operation admissibility snapshot is stale`
  失败；
- stale 在 candidate attempt 0/1 中被当作 candidate infeasible，最终错误消耗 candidate budget 并终态化。

因此，先前“production admissibility + positive acceptance completed”的结论只证明了首次 owner inventory、selector、feasibility gate 和 bounded terminal 能工作，不能证明 durable effect 后 prerequisite freshness 已闭环。该能力重新列为 active，原完成记录保留为当时切片证据，不作为 freshness 已完成的依据。

## Root Cause

当前实现存在四类结构性缺口：

1. `WorkspaceManifest`、`TargetSnapshot`、`RuntimeOperationAdmissibility`、`RuntimeCapabilityStateSnapshot` 等值在 Run 初始化或首次需要时获取，但没有统一记录“它依赖哪些可变资源”。
2. owner receipt 记录 effect 和 evidence，却没有用 owner-neutral typed contract 声明本次操作改变了哪些资源域。
3. capability-state 有专用 `RefreshCapabilityStateAfterStaleReceipt`，manifest、target snapshot 和 operation admissibility 没有共用的 invalidation/refresh 入口。
4. owner probe 返回的 `operation_admissibility_stale` 目前属于 candidate contract failure，导致 stale 消耗 attempt 0/1；Shell 又使用 workspace 根目录 mtime 作为 inventory revision，创建任意直接子项都可能使其立即 stale。

这意味着当前系统可以正确执行并记账一个 durable effect，却无法保证下一次 authority transaction 使用的是 effect 之后的资源事实。

## Scope

本轮必须覆盖：

- workspace manifest、target snapshot；
- File、Shell、Git、MCP、SSH、Web operation admissibility；
- Git 等 owner capability state；
- provider ceiling 或 remote owner observation 后续接入同一合同所需的通用类型和预算；
- candidate seal 前 probe stale、sealed operation 执行前/执行时 stale、owner effect receipt 引起的后续 observation invalidation；
- restart、duplicate、late result、CAS conflict、refresh exhaustion；
- LLM request volatile context、Result、Eventstream、Web/TUI projection 的一致行为；
- 每个涉及组件不依赖完整服务和真实 LLM 的独立功能回归。

## Non-Goals

- 不恢复 V1 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor 或 application drive queue。
- 不允许 Application、Run owner、capability block、Result、Eventstream、Web/TUI 选择下一 `RuntimeAction`。
- 不把软件开发、LSP、Go、测试框架、`main.go` 或 Git 工作流写入 Orchestrator reducer。
- 不使用目录 mtime、进程内时间戳或 application 轮询作为统一 freshness authority。
- 不通过刷新 prerequisite 修改同一 Run 已封存的 definition、provider/model target 或 convergence policy。
- 不建立跨 owner service/repository 注入、通用 Envelope、`map[string]any`、JSON/reflect bridge 或第二套 retry 状态机。
- 不要求所有 observation 在每个 transition 都全量重取；只失效与已确认资源变化相交的 observation。

## State-Machine Impact

| 状态机 | 当前偏差 | 目标收口 |
| --- | --- | --- |
| SM-1 Run lifecycle | stale 可经 candidate exhaustion 错误终态化 | 可恢复 stale 保持同一 active Run；只有 typed refresh budget exhausted 或其他 sealed terminal decision 才能失败 |
| SM-2 authority/focus convergence | prerequisite 只有 pending/satisfied，没有可变 observation 的生命周期 | reducer 只接受 fresh satisfied observation；失效后确定性返回 `acquire_prerequisite` |
| SM-3 candidate/sealed dispatch | seal 前 stale 被记为 candidate failure；sealed 后只有 capability-state 专用 refresh | stale 使 proposal/dispatch invalidated，不产生 candidate failure、repair debt 或 rejected evidence；owner effect 后原子失效相关 observation |
| SM-4 drive/recovery/projection | restart 可恢复 aggregate，但 stale 不在通用恢复链 | 从 aggregate 中已持久化的 invalidation、refresh ledger、receipt 和 dispatch 状态幂等 re-drive；projection 只展示 |

### Required Legal Transitions

```text
satisfied observation
  -> resource change or typed stale detected
  -> invalidated
  -> prerequisite pending
  -> acquire_prerequisite
  -> fresh satisfied observation
  -> new authority transaction

candidate attempt pending
  -> seal-time probe stale
  -> invalidated (not failed)
  -> acquire_prerequisite
  -> candidate attempt 0 under new transaction

sealed operation claimed
  -> owner reports typed prerequisite stale before effect
  -> receipt committed
  -> dispatch invalidated/closed without execution evidence
  -> acquire_prerequisite

owner effect accepted
  -> receipt + evidence + ResourceChanges committed
  -> remaining affected observations/operations invalidated
  -> acquire_prerequisite before another affected operation
```

### Forbidden Transitions

- stale -> candidate attempt 1；
- stale -> repair directive / rejected operation shape；
- stale -> provider transport retry 或 operation recovery；
- stale -> rejected Result while refresh budget remains；
- owner receipt -> block/application directly schedules next action；
- projection detects drift -> rebuild/overwrite authority；
- refresh -> patch activation provider/model、Goal/DoD 或 definition revision；
- duplicate/late stale -> consume a second refresh budget；
- resource change not intersecting an observation -> unrelated prerequisite refresh。

## Target Typed Model

类型名称是本方案的目标合同名称。实现时如因 Go package 现状需要微调命名，必须保持字段语义、owner-neutral 边界和本节不变量，并在本方案中记录最终名称。

### Resource Domain

```go
type RuntimeResourceDomain string

const (
    RuntimeResourceWorkspaceTree  RuntimeResourceDomain = "workspace_tree"
    RuntimeResourceWorkspaceTarget RuntimeResourceDomain = "workspace_target"
    RuntimeResourceRepositoryState RuntimeResourceDomain = "repository_state"
    RuntimeResourceOwnerCatalog    RuntimeResourceDomain = "owner_catalog"
    RuntimeResourceProviderConfig  RuntimeResourceDomain = "provider_config"
    RuntimeResourceRemoteResource  RuntimeResourceDomain = "remote_resource"
)
```

资源域只表达中立变化类别，不表达业务动作。Orchestrator 不解释路径、Git 分支、可执行文件或 MCP server；这些由 owner 转换为规范化 `SubjectRef` 和 revision/hash。

### Freshness Dependency

```go
type RuntimeFreshnessDependency struct {
    Domain        RuntimeResourceDomain
    OwnerModuleID string
    SubjectRef    string
    Revision      string
}
```

约束：

- `OwnerModuleID + Domain + SubjectRef` 是精确资源身份；不得只靠 EventHub destination alias。
- `Revision` 是 acquisition 时 owner 观察到的稳定 revision/hash，不得使用仅表示“本次查询时间”的随机值。
- 目录树变化由 owner 显式报告 `workspace_tree`；目标变化由 owner同时报告对应 `workspace_target`。Orchestrator 不做路径 prefix 推断。
- 一个 observation 可以依赖多个资源，例如 Git admissibility 同时依赖 `workspace_tree` 和 `repository_state`。

### Prerequisite Observation

```go
type RuntimePrerequisiteObservation struct {
    PrerequisiteID string
    Kind           PrerequisiteKind
    OwnerModuleID  string
    ObservationID  string
    ContentHash    string
    Dependencies   []RuntimeFreshnessDependency
    ObservedAt     string
}
```

该类型是现有 typed resource fact 的 freshness metadata，不替代
`WorkspaceManifest`、`TargetSnapshot`、`ProviderCeiling`、
`RuntimeCapabilityStateSnapshot` 或 `RuntimeOperationAdmissibility`。每个 satisfied mutable prerequisite 必须恰好绑定一个 observation；immutable/run-sealed prerequisite 可以明确声明无可变 dependency。

`ObservationID` 必须稳定绑定 prerequisite identity、typed content hash 和规范化 dependencies。仅 `ObservedAt` 不参与身份计算。

### Resource Change

```go
type RuntimeResourceChange struct {
    Domain         RuntimeResourceDomain
    OwnerModuleID  string
    SubjectRef     string
    BeforeRevision string
    AfterRevision  string
}
```

`RuntimeOperationReceipt` 增加有界 `ResourceChanges []RuntimeResourceChange`：

- `Changed=true` 的 durable workspace/remote effect 必须报告至少一个 change；
- `NoOp=true` 不得伪造 revision change；
- owner 可报告多个 change，例如 File 创建文件同时改变 `workspace_target/main.go` 与 `workspace_tree`；
- `BeforeRevision` 未知时可以为空，但 `AfterRevision` 必须由 owner 实际观察得出；
- receipt 仍由 owner 在返回前持久化，invalidation record 通过 receipt identity 绑定 change；change 自身不反向携带 receipt ref，避免循环身份；
- Orchestrator 不根据 operation kind 猜测 change。

### Typed Stale Detail

```go
type RuntimePrerequisiteStale struct {
    PrerequisiteID   string
    ObservationID    string
    Domain           RuntimeResourceDomain
    OwnerModuleID    string
    SubjectRef       string
    ExpectedRevision string
    ObservedRevision string
    Reason           string
}
```

seal-time `ProbeRuntimeOperationFeasibilityResult` 与 execution receipt 都使用该具体类型表达 freshness conflict。现有 `RuntimeCapabilityStateStaleDetail` 应迁移/兼容到这一通用结构，最终只保留一套 invalidation 语义；普通 operation infeasible 仍使用 typed `FailureFact`，两者不得混淆。

### Invalidation Record

aggregate 必须持久化规范化 invalidation 记录，至少包含：

- invalidation identity/signature；
- source：owner receipt 或 seal-time probe；
- workspace/run、authority snapshot、candidate attempt、dispatch/operation（适用时）；
- 被失效的 prerequisite/observation IDs；
- resource change 或 stale detail；
- refresh budget 是否已消费；
- applied state revision。

该记录用于 duplicate、late delivery、restart 和 CAS reload 后幂等判定，不能依赖进程内 map。

## Freshness Intersection Algorithm

统一算法只做规范化身份匹配：

1. owner acquisition 返回 typed fact 和 observation dependencies；Orchestrator 在同一 CAS 中将 prerequisite 从 pending 移至 satisfied。
2. candidate context 只渲染当前 satisfied 且未失效的 observation。
3. owner receipt 返回 `ResourceChanges`，或 probe/execute 返回 `RuntimePrerequisiteStale`。
4. Orchestrator 以 `OwnerModuleID + Domain + SubjectRef` 求 change/stale 与 observation dependency 的交集。
5. 对每个命中的 observation：
   - 从对应 `RuntimeResourceFacts` 移除旧 typed value；
   - 从 `Prerequisites.Satisfied` 移除；
   - 将原 requirement 规范化后放回 `Prerequisites.Pending`；
   - 写入 invalidation record；
   - 清除依赖旧 observation 的 `Feasibility` 和 authority transaction 派生状态。
6. 原子提交 facts、receipt/evidence/dispatch close、invalidation 和 refresh ledger。
7. reducer 在下一次 reduce 中只能选择 `acquire_prerequisite`；重取完成后生成新的 authority snapshot/transaction，candidate 从 attempt 0 开始。

匹配必须满足：

- exact identity；无字符串 contains、路径猜测或 profile 特判；
- change 集合为空时不做全量失效；
- 同一 change 命中多个 observation 时一次 CAS 全部失效，只消费一次该 invalidation signature 的 refresh budget；
- 同一 observation 被同一 receipt/stale 重复命中时完全幂等；
- 不相交的 observation 保持 satisfied，避免全量 preflight 和不必要 LLM cache drift。

## Candidate And Dispatch Semantics

### Seal-Time Probe Stale

owner read-only probe 若发现 inventory/observation stale：

- 返回 `RuntimePrerequisiteStale`，不返回 candidate contract `FailureFact`；
- 当前 attempt 持久化为 `invalidated`，不是 `failed`；
- 不写 active rejected operation shape、failure fact、repair ledger 或 evidence；
- 不创建 sealed dispatch；
- 消费 prerequisite refresh budget 后回到 `acquire_prerequisite`；
- refresh 后使用新 transaction 和 candidate attempt 0，旧 attempt 只保留审计。

### Execution-Time Stale Before Effect

owner 在执行前发现 sealed observation 已变化：

- 按 execution key/fence 持久化带 stale detail 的 receipt；
- `ObservedEffectScope=none`、`Changed=false`，不得执行 effect；
- Orchestrator 关闭/标记该 dispatch 为 invalidated，不提交 rejected operation evidence；
- invalidation、budget 和 prerequisite pending 在同一 CAS 中提交。

### Accepted Effect Invalidates Later Work

owner operation 已 accepted 且发生 durable effect：

- 已完成 operation 的 receipt/evidence 必须正常提交，不能因后续 freshness refresh 丢失；
- `ResourceChanges` 在同一 CAS 中失效相关 observation；
- active dispatch 中尚未执行且依赖旧 observation 的 operation 不得继续；dispatch 记录为 partially committed/invalidated，未执行 operation 不产生 rejected evidence；
- reducer 重新获取 prerequisite 后再决定是否需要新 candidate，不能直接复制旧 dispatch 剩余 operation。

### Duplicate And Late Results

- exact receipt/stale/invalidation replay：返回已应用 aggregate，不递增预算或 revision；
- 旧 transaction 的 late stale：只进入 persisted diagnostic lineage，不得失效新 observation；
- 相同 prerequisite ID 但 observation ID/revision 不匹配：返回 stale diagnostic/conflict，不覆盖当前事实；
- CAS conflict：reload 后先按 invalidation identity 查重，再决定是否重试 commit。

## Recovery Budget

将 capability-state 专用预算统一为：

```go
MaxPrerequisiteRefreshes int
PrerequisiteRefreshes    int
RuntimeRecoveryBudgetPrerequisiteRefresh
```

迁移要求：

- 新 Run 只封存统一预算；bootstrap 默认值继续保持有界，初始建议为 3；
- 已有 aggregate schema 若需要读取旧字段，只允许在 Run owner persistence decode/migration boundary 做确定性映射，不能在 reducer 保留两套分支；
- 每个 distinct invalidation signature 消费一次；duplicate/late/CAS retry 不重复消费；
- stale 不消费 candidate attempt、focus repair、provider transport、operation recovery 或 terminal delivery budget；
- refresh 成功不重置累计 refresh 次数，防止外部资源持续抖动形成无限循环；
- exhausted 时 reducer 产生 typed terminal code `prerequisite_refresh_exhausted`，failure 中必须包含 resource identity、最后 observation/stale identity 和已用/上限；不得伪装为 `candidate_retry_exhausted`。

## EventHub Contracts

合同定义归 `internal/modules/kernel/orchestrator/pkg/events`，capability owner 订阅并返回具体结果：

| 合同 | 用途 | 结果 |
| --- | --- | --- |
| existing `Acquire*PrerequisiteCommand` | 获取 owner typed fact | typed fact + `RuntimePrerequisiteObservation` |
| `ProbeRuntimeOperationFeasibilityCommand` | seal 前只读校验 | feasible、ordinary failure 或 `RuntimePrerequisiteStale` 三选一 |
| existing `Execute*OperationCommand` | sealed operation 执行 | owner-persisted `RuntimeOperationReceipt`，可含 changes/stale |
| Run owner CAS commands | persist aggregate | expected revision + concrete aggregate/result |

所有同步 acquisition/probe/execute/commit 都使用 EventHub `Send` + concrete command/result，并验证：

- workspace/run/snapshot/transaction/dispatch/operation scope；
- result 存在且类型正确；
- canonical owner identity；
- observation/change/stale identity 和 revision；
- receipt exact execution key/fence。

禁止 `events.Response`、Envelope、`map[string]any`、`[]any`、JSON/reflect 类型桥、repository/service/callback/channel 进入合同。

## Component Closure Matrix

### `internal/pkg/workorchruntimecontract`

职责：只定义 owner-neutral value types、normalization、validation、stable identity/hash、budget 和 aggregate invariant，不依赖任何 module 实现。

代码收口：

- 新增 resource domain、dependency、observation、change、stale、invalidation 类型；
- `RuntimeResourceFacts` 建立 prerequisite -> observation 的稳定绑定；
- `RuntimeOperationReceipt` 增加有界 resource changes 和通用 stale；
- candidate attempt/dispatch state 增加明确 `invalidated` 语义；
- recovery budget 从 capability-state refresh 统一为 prerequisite refresh；
- 补 clone/normalize/validate/hash/backward decode tests。

独立回归：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
```

必须直接覆盖 identity normalization、change intersection fixture、invalid receipt、duplicate signature、budget limit、old aggregate decode（若保留兼容读取）和 stable hash。

### `internal/modules/application/workorchd`

职责：从 template/profile/system/workspace 配置编译 typed bootstrap 和 refresh budget；只发 Start/Resume，不选择 next action。

代码收口：

- 新 activation 封存 `MaxPrerequisiteRefreshes`；
- `/run resume` 继续由 application 判断复用同一 active/blocked Run 还是因 terminal/definition/provider-model drift 创建新 Run；
- TUI 修改 workspace model 不得 patch 已创建 Run 的 sealed provider target；freshness refresh 也不得成为 model patch 通道。

独立回归：

```bash
go test ./internal/modules/application/workorchd/... -count=1
```

必须覆盖默认/显式 refresh budget、resume 不带 patch、workspace model drift 创建新 typed activation、同 Run refresh 保持 provider target 不变。

### `internal/modules/kernel/run`

职责：CAS 持久化 Runtime V2 aggregate、schema migration 和 recovery inventory；不运行 reducer、不选择 action、不解释 resource domain。

代码收口：

- 新 freshness/invalidation/receipt 字段完整 clone、save/load；
- expected revision 冲突保持原有 CAS 语义；
- 如需旧 `CapabilityStateRefreshes` 迁移，只在 decode boundary 一次性映射；
- recovery inventory 保持 `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending` 的 typed 分类。

独立回归：

```bash
go test ./internal/modules/kernel/run/... -count=1
```

必须覆盖 round-trip、CAS conflict、restart load、old/new budget decode、terminal immutable、invalidated candidate/dispatch inventory。

### `internal/modules/kernel/orchestrator/pkg/events`

职责：拥有 prerequisite acquire、operation probe/execute 和 owner receipt commit 的具体 EventHub DTO 与校验；不执行 owner 操作。

代码收口：

- acquisition result 携带 observation；
- probe result 将 stale 与 ordinary infeasible 分离；
- receipt validation 接受有界 changes 或 stale，拒绝二者冲突组合；
- 加强 owner/scope/revision/fence 校验。

独立回归：

```bash
go test ./internal/modules/kernel/orchestrator/pkg/events -count=1
```

使用 in-memory Hub 和 fake subscriber 覆盖 nil/wrong result、wrong owner、wrong observation、duplicate result、Post result=nil 安全性；不启动完整服务。

### `internal/modules/kernel/orchestrator/biz/runtimev2`

职责：唯一 freshness intersection、invalidation、budget、candidate/dispatch 状态迁移和 reducer authority。

代码收口：

- 将 `RefreshCapabilityStateAfterStaleReceipt` 泛化为 prerequisite invalidation controller；
- commit acquisition 时持久化 typed observation；
- commit receipt/evidence 时原子应用 changes；
- seal-time stale 将 attempt 标记 invalidated；execution-time stale 关闭 dispatch 并 refresh；
- reducer 禁止使用 stale/missing observation；
- progress signature 纳入 pending/satisfied observation identity、invalidation 和 refresh ledger，但仍排除纯 bookkeeping；
- completion gate 不得跨过 pending refresh。

独立回归：

```bash
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
```

必须覆盖四状态机 legal/forbidden transition、one-change-many-observations、unrelated change、duplicate/late stale、CAS conflict、restart、budget exhaustion、stale proposal cannot seal、partial dispatch effect preservation、terminal gate。

### `internal/modules/kernel/orchestrator/biz`

职责：Runtime V2 drive、EventHub adapters 和 owner-neutral orchestration；不读取 owner repository，不直接操作 workspace。

代码收口：

- `prepareRuntimeV2Prerequisites` 统一接收 fact + observation；
- candidate probe 识别 stale result，不能调用 `FailRuntimeV2CandidateAttempt`；
- operation receipt path 在继续 dispatch 前应用 invalidation；
- restart drive 从 aggregate 继续 pending refresh；
- drive guard 将真实 refresh 视为 progress，将 duplicate bookkeeping 视为 no-progress。

独立回归：

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

使用 fake owner、fake LLM 和 in-memory/file-backed Run owner，覆盖 acquire -> candidate -> receipt/change -> reacquire -> candidate 0、每个 CAS crash point 和 terminal delivery；不使用真实 provider。

### `internal/modules/blocks/file`

职责：拥有 workspace tree/target 的 manifest、snapshot、admissibility、probe、effect execution 和 receipt revision；不选择 next action。

dependency/change 合同：

| fact/operation | dependencies | accepted effect changes |
| --- | --- | --- |
| manifest | `workspace_tree` | n/a |
| target snapshot | `workspace_target:<ref>` | n/a |
| File admissibility | `workspace_tree` + required `workspace_target:<ref>` | n/a |
| create/write/edit/patch | sealed target dependencies | changed targets + `workspace_tree` when membership/metadata inventory changes |

代码收口：

- acquisition 返回 content-derived revision 和 observation；
- probe stale 返回 typed stale，不返回 candidate failure；
- receipt 从真实 before/after manifest/target hash 生成 changes；
- new-target selector 继续由 File owner 约束，Orchestrator 不扩权。

独立回归：

```bash
go test ./internal/modules/blocks/file/... -count=1
```

使用 `t.TempDir()` 覆盖空目录创建、existing edit、patch、no-op、external mutation、stale probe、receipt replay、symlink/path escape 和 bounded inventory。

### `internal/modules/blocks/shell`

职责：拥有 executable/policy/working-directory/managed-validation 可行性和 shell receipt；不使用 workspace 目录 mtime 代表 workspace 内容。

dependency/change 合同：

- Shell inventory 的基础身份依赖 workspace root identity、owner policy revision 和 executable catalog revision；
- 具体 validation target 存在性依赖 `workspace_target:<ref>` 或 owner 返回的等价精确资源 identity；
- 若 shell operation 被允许产生 durable workspace/remote effect，owner 必须通过执行前后有界观察报告 changes；无法可靠界定变化时，该 operation 不得声明为可自动执行的 durable effect；
- managed validation 的 ephemeral session resource 不得伪装成 workspace durable change。

代码收口：

- 删除 `os.Stat(workspaceRoot).ModTime()` 作为 inventory revision 的语义；
- 将 policy/executable/root revision 分开、规范化并 hash；
- target 缺失与 observation stale 分开返回；
- receipt changes 与实际 effect scope 一致。

独立回归：

```bash
go test ./internal/modules/blocks/shell/... -count=1
```

必须证明：File 创建新文件不会仅因根目录 mtime 使 Shell inventory stale；validation target dependency 变化会精确 stale；policy/executable catalog 变化会 stale；probe 不启动进程；managed validation cleanup 幂等。

### `internal/modules/blocks/git`

职责：拥有 repository state observation、operation sequence/admissibility、Git effect 和 receipt；不向 Orchestrator 泄漏 repository 实现。

dependency/change 合同：

- capability state 依赖 `repository_state:workspace_deliverable`，必要时同时依赖 `workspace_tree`；
- Git admissibility 复用同一 repository observation identity，不再维护独立 stale 语义；
- init/add/commit 等 accepted receipt 报告新的 repository revision；会改变 workspace tree 的 operation 额外报告 workspace change。

代码收口：

- 将专用 `RuntimeCapabilityStateStaleDetail` 接入通用 stale；
- query receipt、operation receipt 和 observation ID 保持 exact workspace/run/acquisition/execution scope；
- stale refresh 后重新计算 required operation sequence，不能复制旧 sequence。

独立回归：

```bash
go test ./internal/modules/blocks/git/... -count=1
```

覆盖 repo absent -> init -> add -> commit、外部 repo mutation、query receipt crash/replay、stale before effect、duplicate execution fence 和新 sequence acquisition。

### `internal/modules/blocks/mcp`

职责：拥有 MCP server/catalog/policy/remote resource 的 admissibility 和执行 receipt；默认不依赖 workspace tree。

代码收口：

- 本地 server inventory 使用 `owner_catalog`/policy dependency；
- 只有具体 MCP operation 声明 workspace target 时才增加对应 dependency，禁止所有 MCP operation 因 workspace 变化被全量失效；
- remote revision 可确认时报告 `remote_resource` change/stale；不可确认的外部波动走 ordinary execution/provider failure，不伪造 freshness。

独立回归：

```bash
go test ./internal/modules/blocks/mcp/... -count=1
```

覆盖 catalog change、unrelated workspace change、remote resource stale、probe 不创建 session、receipt replay。

### `internal/modules/blocks/ssh`

职责：拥有 SSH operation policy、连接参数校验、remote execution 和 receipt；可确认的 owner 配置变化接入 freshness，不把不可版本化的网络波动伪装成 stale。

代码收口：

- 将当前常量 `ssh-owner` revision 替换为稳定的 owner policy/configuration contract hash；
- inventory 依赖 `owner_catalog`/policy；若后续引入受管 host catalog，则使用精确 host subject ref；
- probe 对 policy/config revision drift 返回 typed stale；DNS、连接超时、认证失败等仍返回 ordinary prerequisite/execution failure；
- remote effect 只有在 owner 能取得前后 revision 时才报告 `remote_resource` change，否则按 sealed effect/evidence 合同返回结果，不猜测 revision。

独立回归：

```bash
go test ./internal/modules/blocks/ssh/... -count=1
```

覆盖 policy/config change、unrelated workspace change、network failure 非 stale、probe 不执行远端命令、execution receipt replay。

### `internal/modules/blocks/web`

职责：拥有 Web provider catalog/configuration admissibility、read-only probe 和 remote result receipt。

代码收口：

- 将现有 provider configuration revision 映射为 `provider_config`/`owner_catalog` dependency；
- revision drift 返回 typed stale，不再返回 candidate failure `operation_admissibility_stale`；
- provider absent、参数不合法和请求失败继续使用 ordinary typed failure；
- read-only fetch/search 默认不报告 workspace change；只有明确持久化的 remote mutation 能报告 `remote_resource` change。

独立回归：

```bash
go test ./internal/modules/blocks/web/... -count=1
```

覆盖 provider catalog refresh、stale proposal invalidation、provider unavailable 非 stale、unrelated workspace change 和 fake adapter request。

### `internal/modules/blocks/llm`

职责：只消费 sealed typed candidate context 并请求 provider；不判断 freshness、不重建 authority。

代码收口：

- 只渲染 fresh observations；
- provider ceiling 若依赖的 configuration/capability revision 变化，只能针对同一 Run 已封存的 `ProviderModelTarget` 重取；目标已不可用时返回 typed prerequisite/governance failure，不得静默切换 provider/model；
- refresh 后 C0 runtime/schema、C1 Goal/DoD、C2 declarative capability/governance contract 保持稳定；
- observation、manifest、target、admissibility、failure 等变化只进入 C3/C4；
- same transaction attempt 0/1 仍复用 stable prefix；refresh 产生新 transaction，但不得因 freshness metadata 进入 C0-C2 导致 `stablePrefixHash` 漂移；
- stale 不发送第二次 same-contract candidate request。

独立回归：

```bash
go test ./internal/modules/blocks/llm/... -count=1
```

使用 fake provider 覆盖 stable prefix、volatile hash change、protected field integrity、no request while prerequisite pending、refresh 后 candidate attempt 0。

### `internal/modules/blocks/result`

职责：只保存 Orchestrator 已裁决的 accepted/rejected outcome；不解释 stale 或选择恢复。

代码收口：

- refresh 可恢复期间不得创建 rejected Result；
- 只有 `prerequisite_refresh_exhausted` 等 sealed terminal decision 才写 rejected Result；
- duplicate terminal delivery 保持幂等。

独立回归：

```bash
go test ./internal/modules/blocks/result/... -count=1
```

### `kernel/eventstream`、`application/workorchd` read model、Web/TUI

职责：审计和展示 refresh 状态，不驱动 authority。

展示最小字段：

- `prerequisite_refreshing` / `prerequisite_refresh_exhausted`；
- owner、resource domain、subject ref、refresh used/limit；
- stale 来源是 probe、execution receipt 还是 prior effect change；
- 当前 Run 仍 active/blocked/terminal 的正式状态。

约束：

- 不把 refresh 显示为 candidate retry、provider retry 或 decision ticket；
- 不提供 restart/replan 特殊按钮；TUI 运行控制继续统一为 `resume`；
- projection 丢失或 stale 只能影响 ready/display，不得反向修改 aggregate。

独立回归：

```bash
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
```

## Cross-Component Regression Matrix

| 场景 | SM | 关键断言 |
| --- | --- | --- |
| acquisition commit | SM-2/4 | fact + observation + satisfied prerequisite 同一 CAS；restart 不重复获取已 fresh 值 |
| seal-time stale | SM-2/3 | attempt invalidated；无 attempt 1、failure、repair、evidence；回到 acquire |
| execution-time stale | SM-3/4 | exact receipt/fence 持久化；无 effect/evidence；dispatch invalidated；restart 后继续 refresh |
| accepted effect changes resource | SM-2/3 | receipt/evidence 保留；相交 observation 原子失效；不相交值保留 |
| duplicate stale/change | SM-3/4 | revision、budget、attempt 均不增加 |
| late old stale | SM-3/4 | diagnostic only；不覆盖新 observation |
| CAS conflict | SM-4 | reload 后按 invalidation identity 去重；至多一次预算消费 |
| refresh exhausted | SM-1/2 | typed terminal `prerequisite_refresh_exhausted`；Result/Run/Workspace 最终交付 |
| governance blocked restart | SM-1/4 | freshness 不自动 resolve/Resume ticket；已有 resolution 才回 active |
| terminal delivery restart | SM-1/4 | 只重试 Result/Run/Workspace delivery，不重取 prerequisite |
| stable prefix | SM-2/3 | C0-C2 不漂移，C3/C4 随 fresh observation 更新 |
| budget isolation | SM-2/3/4 | stale 不消费 candidate/repair/provider/operation-recovery budget |

每个状态迁移改动必须同时提供：

- legal transition test；
- forbidden transition test；
- duplicate/stale/late test；
- restart/idempotency test；
- CAS conflict test；
- budget/guard test；
- terminal delivery test（如能到 terminal）。

## File-Backed Positive Acceptance

最终 CI-safe 正向场景使用 mock LLM/provider 和真实 file-backed owners：

```text
empty workspace
  -> acquire empty manifest/File/Shell/Git observations
  -> candidate creates main.go
  -> File receipt reports workspace_target + workspace_tree changes
  -> evidence commit and observation invalidation
  -> reacquire manifest/File/Shell/Git facts
  -> managed validation reads the fresh target
  -> candidate edits main.go
  -> second refresh
  -> git init/add/commit according to fresh repository sequence
  -> validation evidence
  -> assessment
  -> accepted terminal Result
  -> Run/Workspace terminal projection
```

必须在下列边界分别注入一次进程重启并复跑：

1. owner effect committed、receipt 尚未提交 Orchestrator；
2. receipt 已提交、evidence/invalidation CAS 尚未完成；
3. invalidation 已提交、prerequisite 尚未 reacquire；
4. reacquire 已提交、new candidate 尚未 seal；
5. terminal persisted、Result 尚未交付；
6. Result 已交付、Run/Workspace projection 尚未完成。

每个 fixture 必须证明 side effect 至多一次、refresh budget 至多一次、candidate attempt 在新 transaction 从 0 开始、最终能够 accepted terminal。

## Implementation Batches

逐组件的职责、允许的交互合同、代码工作包、独立回归、文件持久化与 deployed
验收顺序见 [component closure matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md)。
该矩阵只把本方案拆成可执行清单，不增加并列状态机或改变本文件的权威合同。

### Current Implementation Ledger (2026-07-19)

此表是逐项代码收口的执行账本；`done` 只表示已合入当前工作树并有定向回归，
不表示本方案已经完成或可以归档。

| 项 | 状态 | 已落地的合同/回归 | 收口前仍需完成的工作 |
| --- | --- | --- | --- |
| RPF-1 contract | done | owner-neutral observation/change/stale/invalidation、invalidated candidate/dispatch、统一 prerequisite refresh budget；contract 与 aggregate mapper 已接入；receipt-driven invalidation 持久化 owner-observed resource changes 以供审计/投影使用 | 在最终 schema/兼容策略确定后收紧“durable changed receipt 必须有 changes”的校验 |
| RPF-2 authority | locally complete; P5 local gate passed | seal-time stale 使 proposal invalidated；execution stale 关闭 dispatch；accepted receipt 的相交 change 在 evidence CAS 内失效 observation；partial dispatch 会保留 accepted evidence、重开 coverage 并持久化 fresh candidate 需求；duplicate signature 幂等；所有 prerequisite 重取完成且无 remaining freshness coverage 时可推进已满足 focus，已有 controller transition 回归；refresh budget 耗尽即优先生成 `prerequisite_refresh_exhausted`，不会被 pending prerequisite 重新导向 candidate | 2026-07-20 re-baseline 发现 File→Shell→Git fixture 的 bootstrap 漏列 Git operation-admissibility prerequisite，而非 C1 merge/lookup 丢失；补齐后完整 orchestrator 包、positive/convergence verifier 与 CI-safe gate 通过。仍需 fresh deployed accepted trace 与 release gate |
| RPF-3 File | done | manifest、target snapshot、admissibility acquisition 返回 observation；probe 返回 typed stale；write/edit/patch receipt 报告 target/tree changes；`TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation` 已证明 owner-observed workspace tree drift 返回 generic stale 且 probe 不写入目标；`TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite` 已证明相同 execution key/fence 只重放持久 receipt、不因不同 duplicate payload 二次写入；`TestFileMutationReceiptObservationReportsAfterRevisionForAllStateChangingHandlers` 已逐一覆盖 file edit/write/patch apply 的 target/tree `AfterRevision`，并验证 no-op/rejected 不伪造 resource change；owner 完整包测试通过 | 剩余只由 P5 File→Shell→Git 与 deployed 验收统一覆盖，不再增加 File owner 单点路径 |
| RPF-3 Shell | done | inventory revision 改为稳定 shell policy hash；根目录 mtime 不再导致 stale；policy observation/probe stale 合同和定向回归已接入；validation target dependency 使用 File 作为 `workspace_target` resource owner，`missing` target 也被记录，故 File create/change 可精确失效 Shell observation；state-changing shell command 已采集 sealed target before/after revision，并以 durable receipt 报告 File-owned target/tree change；duplicate execution key 不重放 side effect；完整 Shell block 测试通过 | 剩余只由 P5 File→Shell→Git 与 deployed 验收统一覆盖，不再增加 Shell owner 单点路径 |
| RPF-3 Git | locally complete; P5 local gate passed | capability/admissibility acquisition 返回 repository-state observation，且声明 File workspace-tree dependency；probe stale 与 capability-state stale receipt 改用 generic stale；required capability sequence 按当前 focus 过滤，避免 Git sequence 泄漏到 File-only focus；`TestCapabilityStateReacquiresSequenceAfterStageChanges` 已直接证明 `stage-changes -> create-commit` 在 add 后重取为仅 `create-commit`；`TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation` 已证明 receipt 持久化后重建 Git owner，以相同 execution key/fence 但不同 `git add` pathspec 重投时只返回原 receipt、不会二次暂存；H1 已将 persisted capability receipt 的 opaque `AcquisitionBinding` 贯穿 capability snapshot、admissibility、probe/result 与 durable feasibility ledger；`TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale` 证明 mismatch 只返回 typed stale，`TestCapabilityAcquisitionBindingParticipatesInStableIdentities` 与 `TestOperationAdmissibilityBindingRequiresCommittedOwnerCapabilityState` 覆盖 hash/commit 边界 | File→Shell→Git fixture 现以完整 sealed requirement 集合通过，并证明 Git inventory 经 effect invalidation 后重取。仍需与其余 owner 一起完成 release/deployed exit。字段级合同与完整直接 gate 见[执行矩阵 H1](runtime-v2-prerequisite-freshness-component-closure-matrix.md#h1-shared-acquisition-binding-required-field-level-contract)。 |
| RPF-4 owner admission | in progress | MCP server catalog、SSH owner policy、Web provider configuration acquisition 均已返回 typed observation；revision drift 通过 generic stale 返回；各 owner direct tests 已覆盖 helper contract；LLM `provider_ceiling` 现在持久化 `provider_config` observation，封存目标的配置 revision 在 candidate 请求前确认漂移时以 typed stale 穿过 LLM/EventHub，由 Orchestrator 失效并重取，不消耗 candidate/provider/repair budget；普通 HTTP deadline 保持 provider transport failure；LLM freshness metadata 仅进入 C3 volatile context；Runtime V2 ticket 的 replan-style ingress 在 allow-list 明确允许 continue 时会审计后归一化为具体 continue resolution；不允许时返回携带 ticket/allowed actions/remediation 的 typed conflict/HTTP 409；Decision commit 后 CLI 不再追加 scheduler Resume，继续只由 Decision owner -> Orchestrator 的 singleflight lane 发起 | 补齐 Result 与 application budget/resume/model 的端到端回归，并完成 LLM stale 主驱动的 file-backed/restart 覆盖 |
| RPF-4 presentation | in progress | read model/port/remote backend/TUI/Console 已只读投影 `prerequisite_refreshing` / `prerequisite_refresh_exhausted`、used/limit、owner/resource/source；TUI 单独呈现 refresh，不混入 candidate attempts；projection 不回写 aggregate；TUI 已移除可执行的 `/run restart` 和 `/decision replan`，只呈现 `/run resume`，但 `/decision answer` 的文本仍可由 Decision 合同归一化为审计的 replan intent；application 回归已证明 workspace 默认模型改变后 active Run 保持 sealed target，显式 resume model override 返回 conflict | 补 Result/端到端投影一致性，以及在 file-backed terminal fixture 验证 eventstream/read-model 一致性 |
| RPF-5 | system closed | Git stale refresh 的 file-backed crash/restart fixture 已迁移到 generic stale；refresh budget exhaustion 已优先成为 typed terminal，不再被 pending prerequisite 伪装为 candidate retry；Git owner query-receipt、prerequisite commit、post-init invalidation/reacquire、candidate seal 前的 file-backed restart regressions已按每个 durable Git effect 关闭旧 dispatch、重取新 transaction 的合同；`TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult` 与 `TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection` 已各自重建 Hub/Result owner/delivery Biz 并从 file-backed Run 重载 terminal aggregate，证明两条终态 delivery 边界均不重复 Result 且最终投影 Run/Workspace | 2026-07-20 fresh deployed bounded fixtures `ws-f85931b73f8d7d7b` / `ws-3d999d6a994e6c87` 均 `completed/accepted`。live verifier 仅裁决 accepted Git receipt，验证 `init -> add -> commit` 各一次且每个 durable effect 使用新的 capability-state hash；retained artifact 为 `/tmp/workorch-e2e/runtime-v2-bounded-fixtures/20260720T015428Z/terminal-verification`。 |

**Implementation evidence — 2026-07-20 / R1 recoverable refresh terminal-delivery guard.**

`deliverRuntimeV2Terminal` is now directly characterized against the
post-invalidation, recoverable aggregate shape: a pending affected prerequisite,
`prerequisite_refreshes=1` below its sealed limit, and reducer action
`acquire_prerequisite`.  `TestDeliverRuntimeV2TerminalRejectsRecoverablePrerequisiteRefresh`
proves SM-4 rejects that non-terminal authority before any Result owner command
or Run/Workspace terminal projection: Result remains absent, Run remains
`active`, and Workspace retains its active Run.  The paired existing
`TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult`
proves the opposite boundary: only the persisted
`prerequisite_refresh_exhausted` terminal may later create the rejected Result
and complete terminal delivery after restart.  This affects SM-2/SM-3/SM-4;
it forbids using Result delivery as a stale-recovery shortcut and does not add
any retry or alter terminal authority.  Direct R1 owner/Orchestrator package
gates and `make verify-boundary-contract` passed on 2026-07-20.  R1 remains
in progress for unavailable-sealed-target/no-fallback and R4 read-path exits.

**Implementation evidence — 2026-07-20 / R1 sealed-target no-fallback.**

`TestResolveProviderClientForSealedCeilingDoesNotFallbackWhenTargetUnavailable`
uses a current system default of `ai-proxy/fallback-model` while the persisted
ceiling names a missing `retired-provider/retired-model`. It proves the LLM
owner returns `ProviderModelFailureProviderNotFound` for the exact sealed
target rather than resolving the default target. This is an R1 owner-local
typed-failure proof: it changes no freshness/candidate/recovery budget and
leaves any governance or terminal decision to Orchestrator. The targeted LLM
sealed-ceiling gate passed on 2026-07-20.

**Implementation evidence — 2026-07-20 / RPF-4 LLM provider-ceiling freshness.**

`blocks/llm` now produces a `provider_ceiling` observation whose exact
dependency is `provider_config + llm owner + provider/model target +
configuration revision`.  When the sealed-ceiling resolver confirms that
revision changed before candidate generation, the typed candidate result
carries `RuntimePrerequisiteStale`; the EventHub adapter rejects stale/failure
mixing, and the Orchestrator invalidates the pending attempt (which may have no
proposal yet), consumes only prerequisite-refresh budget and reacquires the
ceiling.  The change affects SM-2/SM-3/SM-4.  It explicitly proves the legal
`pending candidate -> prerequisite reacquisition` transition and forbids a
candidate failure, provider transport charge, repair/evidence record or target
fallback.  Direct proof: `TestHTTPProviderCompleteCancelsConfiguredDeadline`,
`TestRequestCandidatePreservesTypedPrerequisiteStaleWithoutCandidateFailure`,
`TestRequestRuntimeCandidateReturnsTypedPrerequisiteStaleWithoutCandidateFailure`,
`TestPrerequisiteStaleInvalidatesPendingCandidateRequestWithoutCandidateDebt`,
`TestRequestAndSealRuntimeV2CandidateReacquiresProviderCeilingAfterTypedStale`,
and `TestAcquireAndCommitManifestAndProviderCeilingPrerequisites`.  Focused
LLM/Orchestrator and owner package gates passed on 2026-07-20; H2/RPF-4 remains
open for Result and read/presentation end-to-end exits, and P5 still requires a
fresh deployed accepted-terminal trace.

**Implementation evidence — 2026-07-20 / RPF-4 Decision replan ingress.**

`ResolutionActionConflictError` is now a Decision package typed error carrying
the ticket ID, requested action, persisted allowed actions and remediation; it
unwraps to `ErrConflict`, so HTTP emits an actionable `409` instead of the
former opaque `400 invalid argument`. For a `runtime_v2_governance` ticket,
replan-style ingress is normalized only when that ticket's persisted allow-list
permits its continue resolution. The original replan intent is retained in the
payload, while the committed resolution is `provide_missing_info`; the
Decision owner therefore sends the already supported typed governance outcome
and Orchestrator performs its existing idempotent continuation. CLI Decision
resolve/batch-resolve no longer publish an additional scheduler resume after
the commit. This affects SM-1 blocker admission, SM-2 governance fact and
SM-4 continuation; it forbids a V1 successor, a Runtime V2 `replan` action,
bare validation error and a double-drive. Direct proof:
`TestService_RuntimeGovernanceRejectsUnallowedActionBeforeProposal`,
`TestResolveDecisionViewNormalizesRuntimeV2ReplanToAllowedContinuation`,
`TestResolveDecisionViewReturnsActionableConflictForUnsupportedRuntimeV2Replan`,
`TestWriteBusinessErrorMapsTicketActionConflictToActionableHTTP409`,
`TestCLIDecisionResolveDoesNotPublishSecondSchedulerContinuation`, and
`TestRuntimeGovernanceResolutionCommitsThroughTypedEventHubCommand`.
R2/RPF-4 remains in progress for R4 remote/TUI/Console exit coverage; the
file-backed resolution-restart boundary is recorded below.

**Implementation evidence — 2026-07-20 / R2 file-backed Decision continuation restart.**

The Decision owner now recognizes an exact replay of its already committed
Runtime V2 governance proposal and returns the persisted proposal rather than
rejecting the caller before Orchestrator can deduplicate the continuation.  It
still rejects changed resolution ID/type/payload.  On the duplicate path it
does not re-emit Decision evidence or a ticket notification.  The file-backed
fixture `TestRuntimeGovernanceResolutionFileBackedRestartReplaysExactlyOnce`
persists the governed aggregate/ticket/resolution, destroys the Hub, Run owner,
Decision owner and Orchestrator Biz, then recreates all of them with new
file-repository instances.  Two typed Decision replays yield one aggregate
governance resolution/evidence, no second candidate or owner receipt, no
recovery-ledger charge and exactly one recreated-process singleflight drive.
`TestService_RuntimeGovernanceCommittedProposalReplaysExactResolution` proves
the Decision-owner legal/forbidden input boundary.  This affects SM-1/SM-2/SM-4
only; it adds neither a direct Resume nor a second runtime state machine.
Focused Decision and Orchestrator package gates passed on 2026-07-20.  R2 is
locally closed for the restart boundary but remains system-open for R4
presentation mapping and R5 deployed acceptance.

**Implementation evidence — 2026-07-20 / R2 Approval evidence boundary.**

Approval resolve is now intentionally separated from Runtime V2 governance
continuation. The CLI approval approve/reject/batch commands persist only the
Approval owner's typed state/evidence; they no longer append a
`WorkspaceRunRequested` scheduler command. This prevents an application-side
second drive and prevents a generic approval ID from being misrepresented as a
persisted Runtime V2 governance ticket. `TestCLIApprovalResolveDoesNotPublishSchedulerContinuation`
proves the forbidden scheduler counter remains zero, while the paired Decision
test continues to prove its bound owner/Orchestrator continuation is the only
automatic path. A resolved generic Approval is followed, when admissible, by
the single explicit `resume` user command. This affects SM-1 admission and
SM-4 delivery only; it does not create or rebuild SM-2/SM-3 authority. Direct
Approval and focused appservice tests passed on 2026-07-20. R2 remains open
for R4 remote/TUI command-surface coverage; the file-backed Decision-resolution
restart evidence is recorded above.

**Implementation evidence — 2026-07-20 / RPF-4 sealed model read contract.**

`RuntimeBootstrap.Activation.ProviderTarget` is now projected as the
read-only Run field `sealedLLMTarget` through `runtimecore.MapRun`, app-model,
workorch port, HTTP client and remote backend; every mapper clones the target
so a reader cannot alias or mutate the persisted bootstrap.  `Workspace.DefaultLLMTarget`
remains the separately displayed next-activation preference.  Both embedded
and remote backends strip all activation data (including `llmTarget`) when a
Resume names an existing Run, so Resume cannot reopen a provider/model patch
channel.  TUI `/model show` renders the two values independently.  This affects
SM-1 Resume admission and SM-4 read delivery only; it does not alter SM-2/SM-3
authority, provider selection or a sealed activation.  Direct proof:
`TestMapRunProjectsSealedProviderTargetWithoutAliasingBootstrap`,
`TestPortRunInfoPreservesSealedTargetWithoutAliasing`,
`TestRemoteRunCommandResumeSendsRequestID`,
`TestEventHubRunResumeForExistingRunOmitsActivationOverrides`,
`TestModelShowDistinguishesNextRunDefaultFromCurrentRunSealedTarget`, and
`TestActivateWorkspaceExecutionWithBackendResumesSealedActiveRunWhenWorkspaceModelChanges`.
The R3/RPF-4 card remains system-open for the remaining R4
projection/command-surface evidence and the R5 deployed acceptance exit.  The
R1 unavailable-sealed-target/no-fallback owner proof is now locally complete;
it does not by itself close any read or deployed boundary.

**Implementation evidence — 2026-07-20 / R3 new-Run target precedence.**

`TestCreateAndStartWorkspaceExecutionRunSealsProviderTargetPrecedence` binds
the three target sources to a new persisted Runtime V2 bootstrap: explicit
target wins over workspace/system, workspace target wins over system, and the
configured system default is used only when neither override exists.  Each
case creates and starts the Run through typed owner commands, reloads it from
the Run repository, and mutates its original source object afterward to prove
the sealed target is not aliased.  This covers SM-1 activation input and SM-4
delivery only; it cannot patch an active Run, affect SM-2/SM-3 authority, or
turn Resume into a model migration.  The active-Run immutability test remains
the paired forbidden-transition proof.  R3 is locally closed for precedence
and sealing, but stays system-open for R4 read surfaces and R5 deployed
acceptance.

**Implementation evidence — 2026-07-20 / R4 projection replay authority guard.**

`workspaceviewcore.Manager` now has an explicit counter-based regression for
both full and workspace-scoped rebuild.  It arms EventHub observers for
`DriveRuntimeV2`, governance-ticket commit, a state-changing File execution
and Result creation, then confirms a projection rebuild sends none of those
commands.  Only its derived `WorkspaceProjectionChanged` notification is
published.  The test establishes the SM-4 forbidden transitions directly;
projection recovery cannot reconstruct SM-2/SM-3 authority or use Result as a
terminalization shortcut.  R4 remains open for remote/TUI/Console fact and
command mapping, and R5 remains the process/deployed exit.

**Implementation evidence — 2026-07-20 / R4 Console sealed-versus-next target display.**

The Console `Workspace` and `Run` read types now preserve respectively
`defaultLLMTarget` and `sealedLLMTarget`.  Workspace detail renders the former
as the next-run default and the active Run's latter value as the immutable
current-run target; the Run table also exposes the sealed target for historical
inspection.  The Console sends neither value as a resume override, so this is
SM-4 descriptive delivery only and cannot become an activation patch or
provider fallback path. `npm run build` in `web/workorch-console` passed on
2026-07-20. R4 remains open for projection replay/no-drive counters, provider
failure presentation and the one-command governance surface.

**Implementation evidence — 2026-07-20 / RPF-5 truthful release gate.**

The release loop previously continued after a Darwin CLI compilation failure
(`syscall.TCGETS`/`TCSETS`) and printed success after failed `tar` commands.
`internal/services/workorch/service/cli/readline.go` now uses the portable
`golang.org/x/term` raw-terminal boundary, and the Makefile release loop is
fail-fast. The legal P5 release outcome is therefore either four successful
archives plus `checksums.txt`, or a non-zero exit with no success message;
there is no partial-success state. Direct evidence: CLI package test, Darwin
amd64/arm64 cross-compiles, an intentionally invalid platform returning
non-zero without `built into`, and a temporary-directory `make release-check`
which passed fmt, vet, all Go tests, boundary verification, Console build and
all Linux/Darwin archives. This is structural/release delivery only: it affects
no Runtime V2 SM transition or authority, and the fresh deployed accepted
terminal remains the sole P5 exit.

The deployed HTTP verifier likewise uses bounded configurable curl connect and
response timeouts for every request. An unreachable deployed service is now a
typed verification failure with retained artifacts rather than a hanging P5
command; this changes no runtime authority or retry ledger.

当前已验证的最小命令：

```bash
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/file/biz -count=1
go test ./internal/modules/blocks/shell/biz -count=1
go test ./internal/modules/blocks/git/biz -count=1
```

实现者不得把表中的 `in progress` 当作生产验收结论；RPF-4/RPF-5 仍是 active
blocker，完整的组件职责、交互合同、独立回归命令和最终验收条件以本文件前述各节为准。

### RPF-1: Characterization And Contract Types

范围：contract package、现有失败 characterization、文档 SoT 草案。

工作项：

- 固化当前 File-success -> Shell-stale -> candidate exhaustion 回归；
- 新增 resource/dependency/observation/change/stale/invalidation 类型和 validation/hash；
- 新增统一 refresh budget，确定旧字段 decode 策略；
- 不改变生产 drive 行为，先建立编译和类型门禁。

退出条件：contract package 独立测试通过；旧行为有失败 characterization；不存在 map/any 或 owner-specific core type。

### RPF-2: Orchestrator Generic Invalidation

范围：SM-2/SM-3 controller、reducer、candidate/dispatch state、Run persistence。

工作项：

- 泛化 capability-state refresh；
- acquisition observation commit；
- seal-time和 execution-time stale 迁移；
- receipt change intersection；
- duplicate/late/CAS/restart/budget isolation；
- progress signature 和 terminal code。

退出条件：Orchestrator 使用 fake owners 可完成 acquire -> change -> reacquire；stale proposal cannot seal；所有预算隔离和四状态机测试通过。

### RPF-3: File/Shell/Git Owner Adoption

范围：三个直接触发 live 问题的 owner。

工作项：

- File observation/change；
- Shell revision 重构，删除目录 mtime 内容语义；
- Git capability/admissibility stale 合同统一；
- owner receipt、probe 和 direct tests。

退出条件：三个 block 可分别独立回归；File 创建文件后 Shell 能使用 fresh target；Git sequence 在每次 repository effect 后重取。

### RPF-4: Remaining Adapters And Presentation

范围：MCP、SSH、Web、LLM、Result、Eventstream、read model、Web/TUI、application bootstrap。

工作项：

- owner catalog/remote dependency 接入；
- request cache stable-prefix 回归；
- refresh projection/result behavior；
- bootstrap budget 和 resume/model invariants。

退出条件：各组件 direct tests 通过；TUI 只显示 resume 和 refresh 状态；可恢复 stale 不生成 rejected Result。

### RPF-5: File-Backed/Restart/Deployed Acceptance

范围：跨组件 fixture、CI verifier、fresh deployed workspace。

工作项：

- 增加上述 file-backed positive acceptance；
- 六个 crash boundary fixture；
- `make verify-boundary-contract`、相关 verify target、全量 Go tests；
- 重新创建 fresh workspace/run 验证 accepted terminal。

退出条件：

- 不再出现 stale 被归类为 `candidate_retry_exhausted`；
- receipt/evidence/refresh/terminal 全链可追溯；
- workspace durable effect 后只刷新相交 observations；
- deployed Run 正向推进至 accepted terminal；
- active overview/current-state/matrix 与模块 readme 更新一致。

## Recommended Verification Commands

每批先执行直接范围，再执行边界和全量门禁：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/pkg/events -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/shell/... -count=1
go test ./internal/modules/blocks/git/... -count=1
go test ./internal/modules/blocks/mcp/... -count=1
go test ./internal/modules/blocks/ssh/... -count=1
go test ./internal/modules/blocks/web/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
make verify-boundary-contract
go test -p 1 ./... -count=1
```

涉及并发 receipt、CAS 和 recovery 的批次额外执行：

```bash
go test -race ./internal/modules/kernel/orchestrator/... -count=1
go test -race ./internal/modules/kernel/run/... -count=1
```

## Definition Of Done

本方案只有同时满足以下条件才可归档：

- mutable prerequisite 均有 persisted observation/dependency，immutable prerequisite 有明确声明；
- owner receipt 能报告实际 resource changes，probe/execute stale 使用同一 typed stale；
- Orchestrator 是唯一 intersection/invalidation/refresh/action authority；
- Application/Run/blocks/Result/Eventstream/Web/TUI 均未越权；
- stale 不消耗 candidate、repair、provider 或 operation-recovery budget；
- duplicate、late、restart、CAS conflict 下 refresh 和 side effect 均幂等；
- refresh exhaustion 使用独立 typed terminal，不再显示 candidate exhaustion；
- C0-C2 stable prefix 在 refresh 前后保持稳定，C3/C4 使用新 observation；
- File、Shell、Git、MCP、SSH、Web、LLM、Result、Orchestrator、Run、application/presentation 都有独立回归入口；
- file-backed 六类 crash fixture 和 fresh deployed accepted terminal 通过；
- `runtime-stage-contract.md`、Orchestrator/owner readme、current-state、active overview、unclosed matrix 与代码一致；
- `git diff --check`、boundary gate、targeted tests 和全量 tests 通过。

## Documentation Closure Rule

实现期间只在本文件记录未完成工作和批次证据。中立规则变化同步上提
`docs/00-core/runtime-stage-contract.md`，实现合同同步更新
`internal/modules/kernel/orchestrator/readme.md` 与相关 owner readme，类型事实落到
`internal/pkg/workorchruntimecontract`。

RPF-1～RPF-5 全部完成且 fresh deployed acceptance 通过后：

1. 将本文件迁入 `docs/90-archive/completed/`；
2. active overview 和 current-unclosed matrix 移除 RPF；
3. current-state 写入最终生产事实；
4. archive 记录保留本次 live failure、迁移决策、验证命令和最终 evidence refs；
5. 不保留第二份并列 Runtime freshness 核心合约。

## Completion Record (2026-07-20)

RPF-1--RPF-5 are system-closed. This record supersedes earlier ledger cells
marked `in progress` while the deployed trace was unavailable.

| Requirement | Current proof |
| --- | --- |
| owner-local contracts and independent regressions | full `make release-check` passed, including `go vet ./...`, `go test ./...`, console build and `verify-boundary-contract` |
| File/Shell/Git freshness, stale/replay and restart | `make verify-runtime-v2-positive-convergence-file` passed; retained artifact `/tmp/workorch-e2e/runtime-v2-positive-convergence/20260720T015735Z` |
| managed/CI-safe owner interaction | `make verify-ci-mvp-file` passed, including LLM and MCP stdio execution |
| deployed accepted terminal | fresh bounded fixtures `ws-f85931b73f8d7d7b` and `ws-3d999d6a994e6c87` both reached `completed/accepted`; artifact `/tmp/workorch-e2e/runtime-v2-bounded-fixtures/20260720T015428Z/terminal-verification/summary.json` has `errors: []` and aligned ready projection |
| Git fresh-transaction invariant | accepted receipts are exactly ordered `init`, `add`, `commit`, each has a required-operation ref, and each uses a distinct capability-state hash; old invalidated-dispatch operations remain diagnostic lineage only |
| fixture isolation | bounded fixture cleanup restored the workspace list; CI-safe verifier workspaces were subsequently purged, leaving only `ws-0f969ef3003adb2c` |

The first fresh fixture attempt at `20260720T014739Z` completed both Runs but
exposed an E8 verifier defect: it counted unexecuted operations retained from
invalidated dispatches as durable effects and incorrectly required a single
Git snapshot hash. The receipt-only verifier plus hermetic self-test corrects
that assertion; the independent fresh rerun above is the acceptance evidence.
