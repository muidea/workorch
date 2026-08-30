# Runtime Orchestrator Module

Status: active

Type: kernel-module-implementation-contract

Last Verified: 2026-08-30

## 功能范围与边界

`kernel/orchestrator` 是 Runtime SM-2、SM-3、SM-4 的唯一 owner。它在一个 persisted Runtime aggregate 和一个 CAS 边界内
完成 authority/focus convergence、semantic proof、candidate transaction、sealed dispatch、receipt/evidence judgment、
governance continuation、bounded recovery 和 terminal delivery。它不拥有 Workspace、Run、Result、Decision、Approval 或
capability resource state。

| 状态机 | 本模块拥有 | 本模块不得 |
| --- | --- | --- |
| SM-1 Run lifecycle | 无；只消费 Run owner notification/ack | 创建 Run、选择 lifecycle target、持久化 Run terminal 或 successor |
| SM-2 authority/focus | focus、CWI、binding、prerequisite、evidence credit、assessment、repair、terminal decision | 调 provider、执行 capability effect、修改其它 owner state |
| SM-3 candidate/dispatch | request session、document admission、attempt 0/1、candidate、feasibility transaction、sealed dispatch、receipt transaction | 选择 next action、绕过 seal、把 LLM/owner fact 当 transition command |
| SM-4 drive/recovery/delivery | bounded drive、singleflight、lease、budget settlement、startup recovery、Result→Workspace→Run delivery ack | 从 projection/Eventstream/text 重建 authority，或代写 SM-1 |

规范入口是 [`docs/00-core/runtime-stage-contract.md`](../../../../docs/00-core/runtime-stage-contract.md)。专题语义分别位于
[`state-machines.md`](../../../../docs/00-core/runtime/state-machines.md)、
[`semantic-convergence.md`](../../../../docs/00-core/runtime/semantic-convergence.md)、
[`candidate-and-dispatch.md`](../../../../docs/00-core/runtime/candidate-and-dispatch.md)、
[`execution-budget.md`](../../../../docs/00-core/runtime/execution-budget.md) 和
[`recoverable-wait.md`](../../../../docs/00-core/runtime/recoverable-wait.md)。本 README 只固定该规范在 Orchestrator 的 owner、
交互和可执行验收落点，不复制专题全文。

### Single-owner mutation contract

- 外部 concrete result、owner receipt、governance resolution 和 delivery acknowledgement 必须先映射为
  `RuntimeInternalEvent`。只有 `RuntimeController.ApplyInternalEvent` 可以分派 mutation；handler 为 package-private。
- private event 只允许 `Activation | Authority | Transaction | Delivery` 四个 concrete payload family，且 kind 与非空 payload
  一一对应。missing/wrong/multiple payload、scope/identity drift 在 aggregate load/CAS 前拒绝。
- `runtimeMutationKernel` 唯一持有 repository port 与 `RuntimeTransitionCoordinator`；authority、planning transaction、
  candidate dispatch、drive delivery component 只共享该 kernel，不持有 sibling、Hub 或第二份 aggregate。
- `RuntimeAggregate.Validate` 顺序验证 SM-2、SM-3、SM-4 及 cross-machine fence。所有合法变化经一次 semantic CAS；
  projection、timer、adapter 和 shared contract package都不能直接写 aggregate。
- `internal/pkg/workorchruntimecontract` 只包含 DTO、enum、normalize/validate/clone/hash、manifest compiler 和纯 matcher；
  不持有 Hub/I/O/CAS，也不选择 retry、terminal 或 Runtime action。

### Semantic and candidate authority

- activation 只消费 Run owner 已封存的 `RuntimeBootstrap`。Definition startup input、Profile contract、provider selection、
  Soul/Skill/MCP/Memory snapshot 和 `StartupInputHash` 在既有 Run 中不可改写。
- required CWI 在 operation candidate 前必须由 SM-2 从 immutable Definition seed、typed owner catalog、SM-3 semantic document
  与 matching owner feasibility 封存唯一 `CoverageEvidenceBinding`。不存在 binding-free operation authority。
- Goal/DoD、prospective constraint、required sequence 和 collective receipt 只支持 conjunctive `all_of`；不存在
  `one_of`/`any_of` terminal escape。Definition gap 只能进入专用 clarification transaction。
- provider-facing manifest、closed JSON Schema、strict decoder、single evaluator 和 owner feasibility 共享一个 sealed
  `RuntimeCandidateContractManifest`。LLM wire 不包含 operation ID/dependency；本地 compiler 唯一派生 identity/graph。
- JSON Schema response format 是可选 transport acceleration。format on/off 都必须经过同一 local decoder/evaluator；format
  failure不授权第二次无 format physical call。
- coherent empty/invalid、schema rejection 与 semantic rejection 共用 attempt 0/1。只有 persisted typed correction witness
  可开启 attempt 1；attempt 1 任意再次拒绝 final。transport/incomplete recovery、owner feasibility、repair 与 governance budget
  不得借用该 attempt ledger。
- accepted candidate 在 effect 前必须同时持久化 candidate attempt、admission target/binding/recipe commitment、
  `SealedDispatch`、operation execution key/fence 与 `DispatchAuthorityHash`。fresh load 从原始 manifest/document/owner fact
  preimage 反证，不从 current mutable facts重编译。

### Receipt, evidence and repair

- capability owner 只返回 factual catalog/feasibility/receipt。receipt 必须绑定 exact dispatch、operation、execution key/fence、
  target authorization、effect、binding 与 recipe/check；只有 SM-2 private event 可以裁决 evidence kind、credit、CWI settlement
  或 terminal。
- receipt Query/Commit 的 producer 必须与 persisted sealed dispatch 中 exact operation kind 的 descriptor owner 一致；source
  identity 不做 whitespace normalization。empty、whitespace/noncanonical、wrong 或 cross-capability source 在 receipt lookup 或
  CAS 前拒绝。Commit adapter 只把 observed source 映射为 `OperationReceiptObserved` private event；source/operation 校验与 receipt
  mutation 使用同一次 aggregate load/CAS，并统一经 `submitRuntimeInternalEvent` 的 Workspace-retirement fence。该 gate 是 SM-3
  receipt admission；`found=false` 虽由 SM-4 drive 中的 owner executor 使用，也不能成为独立 SM-4 transition authority。
- `rejected` receipt 先保留 owner exact diagnostic；只有 `accepted|no_op` 进入 effect/quality gate。raw stdout/stderr、provider
  reasoning 和 decoder message不能进入 aggregate、Eventstream、projection、memory 或 C4。
- claimed/no-receipt 只在 sealed idempotency 允许时消费 SM-4 operation-recovery budget后重放；at-most-once/unknown effect 进入
  `outcome_unknown`。exact receipt duplicate零 effect、零 evidence revision、零 credit。
- `Facts.RepairDirective` 是同 focus 唯一 repair action authority。LLM assessment 的文本、missing claim 和 owner diagnostic
  只作为 observation，不能选择 state change、validation、retry 或 terminal。
- owner-grounded repair 必须区分 diagnostic observer 与 effect owner：File-confirmed observation 直接绑定 File；MCP
  receipt-evidence observation 只有在 exact targets 已有完整 File snapshot，且 File 自己提供 fresh exact-target admissibility 时，
  才能暴露 File-owned durable operation。Shell/Git/MCP 等其它 durable catalog 即使同时存在也不能进入该 manifest。
- validation failure target 必须来自 sealed finite scope 内的 structured owner observation。Shell candidate先经 File read-only
  confirmation；pending/stale/rejected confirmation均无 concrete repair authority。confirmed all-of 的每个 File receipt都必须
  形成连续 workspace revision edge，最后一项后才生成 target-free fresh-validation directive。

### Recovery, governance and delivery

- provider、operation、CAS、repair、governance、terminal delivery 使用独立 persisted budget/guard。allocation 在 external
  call/effect 前与授权同 CAS 创建；submitted outcome必须 settlement，unknown conservatively占用预算。
- `RuntimeDriveLease` 是 delayed recovery 的唯一 authority。timer只 claim exact due generation；process-local gate/timer 仅合并
  persisted identity。Stop 清理本地等待但不删除 durable lease，fresh startup从 Run owner inventory恢复。
- startup 只用 Run owner 的 bounded recovery inventory/readiness/opaque record query。unknown/retired current JSON、cursor cycle、
  page/item limit、nil/wrong result直接使启动失败，零 provider/capability/governance side effect。
- Decision/Approval owner 持久化 resolution；Orchestrator用 exact ticket/approval、resolution、dispatch/dependency/grant fence
  提交 continuation或terminal。Application和Governance都不发布 Resume。
- terminal outcome先由 SM-2 seal，再由 SM-4按 Result→Workspace→Run顺序交付。各 owner只执行自身 private reducer并返回 exact
  acknowledgement；partial/response-loss从同一 delivery ledger幂等续驱，不改写 terminal decision。
- Workspace deletion只允许在 Run owner `draining` fence后排空既有 terminal delivery。Orchestrator独占自身 quiescence/purge
  private reducer，不终止 Run、不推进 Workspace deletion phase、不跨 owner删除资源。

## 组件交互及合同

| 交互方 | 方向 | concrete contract | owner 边界 |
| --- | --- | --- | --- |
| `blocks/run` | 双向 | state notification、bounded recovery inventory/readiness、opaque Runtime record read/CAS、lifecycle/terminal ack | Run拥有SM-1与物理opaque store；不解释SM-2/3/4 |
| `blocks/llm` | Orchestrator → LLM | provider ceiling、preflight、semantic/operations request | LLM返回provider/document fact，不创建attempt或transition |
| File / Git / Shell / MCP / Web / SSH | 双向 | 各 owner 支持的 capability/admissibility/feasibility/semantic/reconciliation 子集，以及 execute、receipt query/commit | capability owner拥有effect/receipt；不选择next action；Web/SSH不提供CWI reconciliation |
| Decision / Approval | 双向 | governance/clarification open、resolution/evidence | 资源owner拥有ticket/approval lifecycle；Orchestrator拥有Runtime continuation |
| Result / Workspace / Run | Orchestrator → owner | sealed terminal delivery fact与exact ack | 三个owner各自迁移，SM-4只记delivery ledger |
| `application/workorchd` | 双向只读 | immutable drive observation、runtime audit query | Application不Drive、不持有aggregate |
| `kernel/governance` | 无直接I/O | 经Decision/Approval/Run owner fact间接协作 | 不存在Governance↔Orchestrator生产topic |
| Skill / Soul / MCP Registry / Memory | 无直接I/O | 只消费Run activation中的detached snapshot | registry或memory更新不改变既有Run |
| Eventstream | 无直接订阅 | owner notification形成审计 | 审计不重建authority |

## 不变量与非职责

- 核心必须领域中立，不写死 profile、语言、测试框架、MCP tool 或业务领域语义。
- 每个 formal state 只有一个 owner；外部 EventHub metadata 只描述 participant，不授权外部 reducer。
- SM-1不选择 action，SM-2不执行 effect，SM-3不选择 next action，SM-4不重建 authority。
- duplicate幂等；stale duplicate、result 或 trigger 不得越过当前 fence，只能进入其 typed refresh、retirement 或 diagnostic
  route。CAS前中断、CAS后响应丢失和fresh-process reload不得重复provider、effect、receipt、evidence、budget或delivery。
- projection/read model/metrics/Eventstream、LLM文本和process-local cache均非authority。
- 禁止 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、
  application continuation queue、auto replan successor、checkpoint authority replay与map/JSON/reflect authority bridge。
- Orchestrator不暴露Start/Resume service facade；生产execution ingress是Workspace-scoped intent，经Run owner SM-1 notification
  触发Drive。
- current persistence只接受current shape；没有legacy decoder、migration/backfill或由projection修复invalid aggregate的路径。
- provider-facing cache保持C0 system/schema/runtime、C1 Goal/DoD、C2 declarative capability/governance、C3～C5 volatile runtime
  分层；unexpected stable-prefix drift是rendering defect。

## 能力验收

| 验收面 | 自动化证据 | 必需维度 |
| --- | --- | --- |
| owner-private dispatcher/CAS/aggregate | `biz/runtime/internal_event_*_test.go`、`component_facades_test.go`、`aggregate_invariant_test.go`、`structure_contract_test.go` | exactly-one payload、legal/illegal transition、single mutation root、pre/post-CAS failure |
| SM-2 semantic/CWI/binding | `semantic_*_test.go`、`cwi_*_test.go`、`coverage_*_test.go`、`assessment_*_test.go` | all-of、owner fact、duplicate/stale/ABA、reopen/settle、proof preimage/restart |
| SM-3 document/candidate/seal | `runtime_document_conformance_test.go`、`candidate_*_test.go`、`dispatch_*_test.go` | empty/invalid/schema/semantic、attempt 0/1、target/binding/effect/dependency、fresh-load reverse proof |
| capability effect/receipt/evidence | `runtime_operation_receipt_source_test.go`、`*_owner_receipt_integration_test.go`、`operation_receipt_test.go`、`candidate_evidence_*_test.go` | 六类 owner 的 descriptor/source/query 边界矩阵（除 File 外使用最小 sealed fixture）；empty/whitespace/noncanonical/wrong/cross 的 Query/Commit 零 CAS、aggregate 不变；真实 File admission→seal→claim 的 accepted/no-op/rejected exact Commit 正向 CAS、retirement fence、receipt-loss recovery；其余五类 owner 的 production-valid admission→seal→claim→Commit 全矩阵未覆盖 |
| owner-grounded repair | `repair_surface_confirmation_test.go`、`candidate_evidence_repair_transaction_test.go`、`mcp_owner_receipt_integration_test.go`、`reducer_test.go` | Shell failure→File confirmation、MCP receipt→File snapshot、effect-owner filter、single/multi all-of、partial/rejected/restart/governance |
| governance/clarification | `governance*_test.go`、`definition_clarification_test.go`、`runtime_lifecycle_handoff_test.go` | ticket/approval identity、continue/fail、duplicate/stale、response loss、blocked Run ack |
| SM-4 wait/recovery/delivery | `biz/runtime/drive_lease_test.go`、`biz/runtime_lifecycle_handoff_test.go`、`biz/runtime_drive_result_test.go`、`biz/runtime_result_projection_test.go`、`biz/runtime_state_repository_test.go` | singleflight、timer/Stop epoch、CAS cut points、fresh process、budget、three-owner partial ack |
| Workspace deletion | `biz/workspace_retirement_test.go`、`biz/workspace_runtime_quiescence_adversarial_test.go` | draining/quiescing fence、in-flight writer、retain/purge、pending journal、restart/tombstone |
| LLM request/cache/audit | `biz/runtime/provider_request_recovery_test.go`、`biz/runtime/provider_request_terminal_test.go`、`biz/runtime_candidate_event_adapter_test.go` 与 `blocks/llm/biz/runtime_*_test.go` | ceiling、identity、format on/off、submission settlement、raw isolation、C0～C5 stability |
| static owner boundary | `make verify-boundary-contract` | no forbidden surface、no cross-owner service/repository、state owner manifest consistency |

真实 provider、远端 endpoint 和长时间网络故障由
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；部署事实不能替代
本表的 deterministic contract tests。
