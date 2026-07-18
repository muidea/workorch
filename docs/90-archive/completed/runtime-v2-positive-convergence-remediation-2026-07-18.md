# Runtime V2 Positive Convergence Systemic Closure Plan

Status: completed — RVC-R17～R26 code-landed; RVC-R16 bounded terminal and canonical canary liveness passed

Type: roadmap / implementation closure specification

Last Updated: 2026-07-18

## 1. Purpose

本文是 Runtime V2 正向收敛的唯一 active 实现收口计划。它承接最新 deployed/file-backed 双 workspace Live 证据，对四状态机交界处暴露的系统性偏差进行整体收口，避免继续通过 prompt、单一 operation kind、单一 workspace 或单一失败文本做片面堵漏。RVC-R5～R22 已关闭 effect/receipt/recovery、governance saga、blocked Resume、allowed action、基础 candidate parameter schema、typed LLM audit 与 Live 分层；2026-07-18 `run-024` 进一步证明，capability 执行前状态与 operation 子操作变体仍未成为 sealed authority，因此最终 RVC-R16 之前新增 RVC-R23/R24。

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

### 2.3 2026-07-18 post-RVC-R22 capability-state / variant evidence

| Workspace | Run | 已验证正确 | 新暴露偏差 |
| --- | --- | --- | --- |
| `ws-1138d6b3bfddb4fd` / LSP-Bridge | `run-024` | focus-001 已 satisfied 并原子推进；Git owner 对每次执行持久化 exact receipt；`not_git_repository`、effect mismatch、candidate attempt 0/1 与 typed rejected terminal 均按合同收敛；activeRun 已释放 | version-control focus 要求 `git/write` 的 `commit`，但 candidate 前没有 owner-confirmed repository state；模型连续生成 `add/commit`，直到执行后才发现 workspace 未初始化。最后 attempt 0 的 `commit` 非法携带 `pathspecs`，attempt 1 又缺少 `commit`，说明 provider schema 只有 kind 级参数集合，没有 sub-operation variant 条件 |
| `ws-bea4fee67e459bcc` / ai-proxy | `run-024` | validation → state-change repair → assessment → repair budget terminal 全链合法；Result rejected、Run failed、activeRun release 与 projection 对齐 | 大型真实任务证明 liveness/terminal 分层有效；其实现/测试失败属于 workspace 交付结果，不应反向触发 core 特判 |

本轮结论：

1. SM-1、SM-3 receipt/evidence、SM-4 terminal delivery 均按预期工作；合法 rejected terminal 不构成 Runtime 停滞。
2. SM-2 prerequisite 目前只确认“能力存在”，没有确认“能力执行所需的当前 owner state 已获取”。这使可在 candidate 前确定的环境状态延迟到 execution failure。
3. `RuntimeOperationContract` 可列出 kind 的通用参数，却不能用同一权威描述 `patchType=file_write/text_replace/unified_diff`、`operation=init/add/commit` 等 discriminated variants；LLM schema 与 final validator 因而存在双份、强弱不一致的规则。
4. 修复必须抽象为 owner-neutral capability-state snapshot + descriptor-driven operation variants；禁止在 reducer、prompt 或 Git executor 中增加 workspace/错误文本特判。

### 2.4 2026-07-18 RVC-R16 terminal rerun: profile owner identity drift

| Fixture | 已验证正确 | 新暴露偏差 |
| --- | --- | --- |
| bounded general | system default target corrected to `ai-proxy/DeepSeek-V4-Flash` 后，双 focus 的 validation → assessment → durable state change → accepted Result 正常完成 | 无 |
| bounded Git | activation、reducer `acquire_prerequisite`、Run 及 projection 均可观测 | profile 将 capability-state owner 写作 `git`，但 canonical Git block owner 为 `blocks/git`；sealed prerequisite 因而投递到不存在的 EventHub destination，停在 revision 7，180 秒 terminal gate 超时 |

该偏差不能通过在 Git block 注册 `git` 别名修复：profile 直接持久化 owner identity，别名会重新引入双合同和不可审计的 authority 路由。RVC-R25 必须：

1. 在 `RuntimeProfileContract` validation 中验证每个 capability-state owner 等于其 accepted operation descriptor 的 canonical owner；
2. 将内置 `software-dev` profile 与 bounded live fixture 统一为 `blocks/git`；
3. fixture creator 在创建 workspace/template 前验证将被 activation 封存的 system/explicit `providerId/modelId` 是 fresh、ready 的 chat target，避免坏系统默认产生 HTTP 500 后留下无效 fixture；
4. 不迁移或默认投影已经持久化的 `git` owner；旧 workspace 必须通过显式的新 activation/新 workspace 采用 current profile contract。

RVC-R25 exit gate：legacy `git` owner 在 activation contract validation 前被拒绝；canonical `blocks/git` 通过；内置模板与 live fixture 均不再含 legacy owner；file matrix 与 fresh deployed Git fixture 能取得 owner snapshot，并按 `init → add → commit` 继续。

### 2.5 2026-07-18 RVC-R16 rerun: candidate DTO/JSON schema drift

RVC-R25 修复部署后，fresh bounded general fixture 再次 accepted；Git fixture 已正确取得 canonical owner snapshot，
但第二个 focus 的 `produce_state_change` 在 candidate 边界以
`json: unknown field "requiredOperationRef"` 耗尽同合同 retry。

根因不是 Git owner、provider 或 reducer：provider-facing action schema 已声明该字段，sealed
`RuntimeOperation`、required-sequence matcher 与 Git EventHub command 也都持有该字段；唯独 LLM
candidate strict-decode DTO 漏了它。因 `DisallowUnknownFields` 正确 fail-closed，合法的 required
sequence candidate 被错误归类为 schema failure。

RVC-R26 收口要求：

1. strict DTO 显式声明并将 `requiredOperationRef` 映射到 typed `RuntimeOperation`；不得放宽
   strict unknown-field policy；
2. 回归测试覆盖 JSON candidate → typed operation 的字段保持；
3. positive-convergence file verifier 必须执行该回归，避免 schema、DTO、seal、owner contract
   再次只在 deployed terminal gate 才发现断裂；
4. 不为已失败 workspace 做兼容投影；fresh fixture 必须用于下一次 RVC-R16 gate。

RVC-R26 exit gate：DTO mapping、focused/full Go tests、boundary verifier 与 file-backed
positive-convergence verifier 均通过；带该代码的新部署随后通过 fresh bounded terminal gate 与
独立 canonical canary liveness。

### 2.6 2026-07-18 RVC-R16 rerun result: terminal passed, legacy canary excluded

带 RVC-R26 的新部署已通过 fresh bounded dual-fixture terminal gate：general 与 Git fixture 都跨越
两个 focus 并写入 accepted Result；Git fixture 在 candidate 前取得 canonical `blocks/git`
snapshot，并完成 sealed `init → add → commit`（每项都有 required-operation ref）。

同次 liveness 观察不能使用现有 `LSP-Bridge` / `ai-proxy` 的 `run-025` 作为通过证据：它们在
RVC-R25 前已持久化 destination `git`，30 秒观察保持 `stateRevision=10`、dispatch/evidence=0，
且报历史 `missing observer ... destination-git`。按 schema v2-only 与 canonical-owner 规约，
不得为它们注册 alias、重写 aggregate 或 Resume 以掩盖该历史错误。

随后以当前 canonical profile 和隔离 `/tmp` 交付目录创建 LSP-Bridge/ai-proxy 的真实定义副本。
官方 liveness verifier 从无 Run baseline 观察到两者进入 `run-001/focus-001/produce_state_change`，
并通过 readiness、scope、lifecycle 与 governance assertions。该路径也修复了观察器把无 Run baseline
的 `acceptedEvidence=null` 与数值比较而触发 Python `TypeError` 的边界问题。

首组副本随后都以 `candidate_retry_exhausted: unexpected EOF` 安全终态化：attempt 0/1、typed
rejected Result、Run failed 与 workspace active-run release 均完整，证明大型真实 DoD 的 provider
输出失败不会伪装成状态机停滞。第二组官方 liveness gate 已通过，运行中 provider 候选仍可继续由
其自身预算/终态收敛。旧 `run-025` 保留为 nonconforming historical diagnostic，不参与 RVC-R16
positive liveness gate。

## 3. Scope And Non-Goals

### 3.1 In scope

- SM-1 Run lifecycle 的 cancel/supersede 正式语义。
- SM-2 repair directive、failure taxonomy、budget 与 action authority。
- SM-3 candidate effect gate、operation execution receipt、evidence authority 与 crash recovery。
- SM-4 startup recovery、多实例 fencing、terminal delivery 和 projection 观测。
- Application activation、LLM candidate renderer、MCP/Shell/File/Git/SSH/Web owner、Run/Result/read model 的 typed contract 联动。
- Runtime state/bootstrap/result closeout schema v2 固化与版本门禁。
- deterministic file-backed、crash injection、双 workspace deployed Live。
- owner-confirmed capability-state prerequisite、freshness/invalidation 与 candidate projection。
- discriminated operation variant contract，同时驱动 provider schema、candidate validation、seal 与 owner adapter preflight。

### 3.2 Out of scope

- 恢复 V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch 或自动 replan successor。
- 在 Orchestrator core 中写入软件开发、Go、测试框架、LSP、ai-proxy 或具体 workspace 业务判断。
- 引入通用 RuntimeAdapter registry、map-based authority rebuild 或 projection-driven reducer。
- 把 provider base URL、credential、client 或动态路由状态序列化进 workspace/run authority。
- 修改已终态历史 Run 使其重新 active。
- 让 prerequisite acquisition 自动执行 `git init`、创建文件、修改配置或产生其他副作用；prerequisite 只能读取 owner state。
- 在 Orchestrator core 识别 Git repository、commit、patchType、语言栈或具体业务语义。

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
| Capability-state prerequisite | 未落地 | 需由 owner 返回持久化、可校验 freshness 的 typed state snapshot；core 只验证 identity/revision，不解释 condition 名称 |
| Operation variant schema | 部分落地 | kind 级 contract 已有；需把 discriminator、required/forbidden parameters、effect/idempotency 与 sequence requirements 统一为 descriptor authority |

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

### INV-09 Owner state before candidate

- 某 operation kind 的可执行性依赖 owner 当前状态时，activation/profile 必须声明 typed capability-state prerequisite。
- profile capability-state `OwnerModuleID` 必须等于其 `AcceptedStepKinds` 所对应 operation descriptor 的 canonical owner；不得把 EventHub destination alias 当作 persisted owner identity。
- SM-2 只能在 matching owner state snapshot 已 acquisition 并持久化后请求 candidate；snapshot 缺失、stale 或 identity 不匹配时只能 `acquire_prerequisite`。
- acquisition 是只读观察，不得隐式执行初始化、修复或其他 side effect。
- candidate、seal、restart 必须引用同一 snapshot revision/hash；owner state 变化后必须失效并重新获取，不能依赖 LLM 根据失败文本猜测。

### INV-10 One variant contract authority

- 同一 operation kind 的 discriminator variants 必须由 `RuntimeOperationContract` 单一描述；provider schema、candidate validator、seal、owner adapter preflight 均从该描述派生。
- variant 必须定义 discriminator、required/forbidden parameters、effect scope、idempotency class 与可选 dependency/sequence requirement。
- profile/focus/RequiredOutput 使用 typed `AcceptedOperationVariants{Kind, VariantID}`，不得继续用脱离 kind 的 `AcceptedOperations=["commit"]`；声明 required variant 时 candidate 至少包含一条匹配并绑定 requirement identity 的 operation。
- provider schema 可以提前拒绝机械错误，但 final seal 仍是最终权威；二者不得维护独立的手写规则集合。

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

### 6.5 Capability-state prerequisite

当 capability 是否可执行或需要哪些 preparatory operation 可以由 owner 在调用 LLM 前确定时，使用新的只读 prerequisite：

```go
const PrerequisiteKindCapabilityState PrerequisiteKind = "capability_state"

type RuntimeCapabilityStateRequirement struct {
    ID             string
    OwnerModuleID  string
    CapabilityKind string
    SubjectRef     string
    Reason         string
}

type RuntimeCapabilityStateSnapshot struct {
    SchemaVersion             string
    SnapshotID                string
    AcquisitionKey            string
    OwnerModuleID             string
    CapabilityKind            string
    SubjectRef                string
    ObservedRevision          string
    ContentHash               string
    Disposition               RuntimeCapabilityStateDisposition
    Conditions                []RuntimeCapabilityCondition
    RequiredOperationSequence []RuntimeRequiredOperationVariant
    QueryReceiptRef           string
    ObservedAt                string
    InvalidatedByReceiptRefs  []string
}

type RuntimeCapabilityStateDisposition string

const (
    RuntimeCapabilityStateReady             RuntimeCapabilityStateDisposition = "ready"
    RuntimeCapabilityStateSequenceRequired  RuntimeCapabilityStateDisposition = "operation_sequence_required"
    RuntimeCapabilityStateGovernanceRequired RuntimeCapabilityStateDisposition = "governance_required"
    RuntimeCapabilityStateUnavailable       RuntimeCapabilityStateDisposition = "unavailable"
    RuntimeCapabilityStateUnknown           RuntimeCapabilityStateDisposition = "unknown"
)

type RuntimeCapabilityCondition struct {
    ID           string
    State        RuntimeConditionState // satisfied / unsatisfied / unknown
    Reason       string
    EvidenceRefs []string
}

type RuntimeRequiredOperationVariant struct {
    RequirementID             string
    Kind                      string
    VariantID                 string
    DependsOnRequirementIDs   []string
    Reason                    string
}

type RuntimeCapabilityStateRef struct {
    OwnerModuleID  string
    CapabilityKind string
    SubjectRef     string
}

type RuntimeCapabilityStateStaleDetail struct {
    StateRef        RuntimeCapabilityStateRef
    SnapshotID      string
    ExpectedHash    string
    ObservedRevision string
    ObservedHash    string
    Reason          string
}
```

约束：

- `RuntimeRequirement` 只声明 owner/kind/subject；具体 condition 与 required sequence 只能由 owner 查询结果产生，application、LLM、Orchestrator 不得构造。
- `RuntimeFacts.Resources` 持久化 normalized active snapshot 与 immutable history；snapshot identity、acquisition key、owner revision、content hash、query receipt 和 required sequence 纳入 aggregate validation/clone/same/hash。
- SM-2 只解释“required snapshot 是否存在、fresh、scope matching”；不解释 `repository_initialized` 等 condition ID。
- SM-2 可以解释 owner-neutral disposition：`ready` 允许继续且 sequence 必须为空；
  `operation_sequence_required` 允许继续且 sequence 必须非空；`governance_required` 进入
  typed governance；`unavailable/unknown` 进入独立 refresh/governance/bounded failure。core
  不得自行从 conditions 推导 disposition。
- snapshot 中的 `RequiredOperationSequence` 是 owner 给 core 的中立 variant 约束。每项使用稳定 `RequirementID`，依赖也引用 requirement identity，禁止只以可能重复的 `VariantID` 作为图节点。SM-3 必须证明 candidate 中存在一对一 matching operation，且 dependency graph 满足顺序。
- prerequisite acquisition 只读；`init/add/commit`、创建目录或修改配置仍必须作为 candidate operation 经 seal/claim/receipt 执行。

Git owner 的实例映射仅存在于 Git block：

```text
repository_initialized=false + unstaged_changes=true
  -> required sequence: repo-init(git/write:init)
                        -> stage-changes(git/write:add)
                        -> create-commit(git/write:commit)

repository_initialized=true + unstaged_changes=true
  -> required sequence: stage-changes(git/write:add)
                        -> create-commit(git/write:commit)

repository_initialized=true + staged_changes=true
  -> required sequence: create-commit(git/write:commit)
```

core 只看到 variant ID 与依赖，不包含 Git 分支。

### 6.6 Operation variant contract

`RuntimeOperationContract` 扩展为 discriminated variant authority：

```go
type RuntimeOperationVariantContract struct {
    ID                              string
    Discriminators                  []RuntimeOperationParameterPredicate
    RequiredParameters              []string
    RequireOneOfParameters          []string
    ForbiddenParameters             []string
    EffectScope                     RuntimeEffectScope
    IdempotencyClass                RuntimeOperationIdempotencyClass
    InvalidatesCapabilityStateKinds []string
}

type RuntimeOperationParameterPredicate struct {
    Name         string
    EqualsString string
}

type RuntimeOperationContract struct {
    Kind                   string
    Parameters             []RuntimeOperationParameterContract
    RequireOneOfParameters []string
    RequireTargetRefs      bool
    ForbiddenParameters    []string
    Variants               []RuntimeOperationVariantContract
    Rules                  []string
}

type RuntimeOperationVariantRef struct {
    Kind      string
    VariantID string
}
```

normalized operation catalog 必须有稳定 hash/revision：kind、parameter type、variant
discriminator、required/forbidden、effect、idempotency、invalidation metadata 任一变化都必须
改变 `OperationContractRevision`。描述文案若不参与 executable rule 可以不进入 revision，
但 provider schema 与 final matcher 不得依赖未入 hash 的自由文本 `Rules`。

首批必须迁移：

| Kind | Variants |
| --- | --- |
| `git/write` | `init`、`add`、`commit`；`add` requires `pathspecs`，`commit` requires `message` and forbids `pathspecs` |
| `git/read` | `status`、`diff`、`log`、`show`、`branch` |
| `patch/apply` / `patch/preview` | `file_write`、`text_replace`、`unified_diff`，各自复用当前参数规则 |

每个 concrete operation 必须匹配且只匹配一个 variant；zero/multiple match 都在 provider output decode 后、seal 前拒绝。discriminator 参数仍保留在 operation parameters 中，variant ID 由 matcher 派生，不允许 LLM 另报一个可与参数冲突的 variant 字段。

profile → focus → `RequiredOutputContract` 的正式约束统一改为
`AcceptedOperationVariants []RuntimeOperationVariantRef`。旧的脱离 kind 的
`AcceptedOperations []string` 不做运行时默认投影或按 kind 猜测；仓库内当前 profile/
fixture 必须原位改为 typed variant ref，非终态旧 activation 若缺少新 contract revision，
只能 typed fail 并创建新 activation，不能在 Resume 时补字段。

`RuntimeOperation` 增加可选 `RequiredOperationRef`。当 capability-state snapshot 声明
required sequence 时：

- 每个 requirement ID 必须且只能被一个 candidate operation 引用；
- 引用 operation 的 kind/derived variant 必须与 requirement 完全相同；
- 一个 operation 不得同时满足多个 requirement；
- 无 required sequence 的普通 operation 不得伪造 requirement ref；
- requirement dependency `A -> B` 必须由 operation `B.DependsOn` 直接包含 operation A，
  final DAG validation 仍负责 cycle/unknown dependency 拒绝。

### 6.7 Provider schema and final seal

```text
static RuntimeOperationContract.Variants
  -> C2 declarative operation catalog

current RequiredOutput + capability-state required sequence
  -> C3 JSON schema oneOf/if-then branches
  -> candidate attempt 0/1
  -> final variant validation + sequence/dependency validation
  -> SealedDispatch
```

- provider schema 必须为每个 variant 生成 discriminator `const`、required parameters 与 forbidden parameters，不能只限制 parameter name 总集合。
- required sequence 必须生成 array-level `contains` 约束；每个分支同时固定 requirement ref、kind 与 discriminator。例如当前 state 要求 `init/add/commit` 时，schema 必须要求三个不同 requirement ref 的 matching operations。
- final seal 复用同一 variant matcher，再验证 operation dependency graph；provider 是否严格支持 JSON Schema 不能成为安全前提。
- attempt 1 的 retry context 必须包含 exact variant validation failure，例如 `git/write:commit forbids pathspecs`，而不是只给出自然语言 wrapper 提示。
- static variant catalog 属于稳定 C2；owner state snapshot、required sequence、failure 与 rejected shape 属于 C3/C4。capability state 变化不得造成 C0～C2 `stablePrefixHash` 漂移。

### 6.8 Freshness, invalidation and recovery

- capability-state query 使用 `workspace/run/requirement/refresh-generation` 组成的 deterministic `AcquisitionKey`。owner 在返回前持久化 query receipt；同 key duplicate 返回同 snapshot/receipt，不同 refresh generation 才允许观察新 revision。
- `AuthoritySnapshot` / `AuthorityTransaction` 增加 normalized `CapabilityStateHash`，绑定 candidate、seal 与 execution 使用的 state snapshot 集合；`StableAuthorityContractID` / same-contract 必须包含该 hash，但 C0～C2 `stablePrefixHash` 不包含动态 snapshot。
- variant descriptor 可声明其成功 receipt 允许使哪些 capability-state kinds 失效；owner
  accepted receipt 必须返回 exact `RuntimeCapabilityStateRef{owner, kind, subject}`，且只能是
  descriptor 声明集合的子集。Orchestrator 按 exact ref 失效 matching snapshot，禁止只按 kind
  粗暴清除其他 subject。失效只影响下一次 reducer/candidate，不中断同一 sealed dispatch 内的依赖执行。
- owner 在 execution preflight 发现实际状态与 sealed snapshot 不一致时返回 typed
  `capability_state_stale` receipt + exact stale detail；但同一 sealed dispatch 内由已 accepted
  predecessor receipt 造成的预期状态变化不是 stale，owner 必须按 sealed dependency +
  predecessor receipt 验证预期推进。Orchestrator 只有在 stale detail 匹配 exact
  transaction/snapshot/state ref/hash 时才失效 matching snapshot 并回到
  `acquire_prerequisite`，不得按普通 implementation repair 消耗 focus repair budget。
- capability-state refresh 使用独立 persisted recovery ledger；超过上限进入 governance 或 typed bounded failure，禁止无限 query/candidate 循环。
- restart 必须从 aggregate 中恢复 snapshot、hash、required sequence 与 active dispatch；已完成 `init` receipt 后不得重复初始化，后续 `add/commit` 按原 ExecutionKey/fence 继续。

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
| `capability_state_stale` | 校验 exact snapshot/state ref/hash 后失效并走独立 refresh；不进入普通 repair/attempt 1 |
| `capability_state_refresh_exhausted` | capability/external governance 或 typed bounded fail |
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
- capability-state query/refresh attempts；
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
- Runtime V2 persisted schema 继续为 v2，不因 RVC-R23/R24 新增 schema-1 compatibility
- activation/definition 增加 sealed `OperationContractRevision`
- provider prompt/schema ID 从 `workorch.runtime_candidate*.v2` 显式升级为
  `workorch.runtime_candidate*.v3`

### 15.2 Version gate

基于当前项目没有需要保留的 schema 0/1 Runtime V2 正式数据，本轮不引入旧数据语义迁移分支：

- production persistence 只写入并接受 schema v2；
- schema 0/1 与 future schema 均 fail-fast，不原位升级、不推断 effect、不重写 evidence；
- current schema v2 但缺少 RVC-R23/R24 必需 `OperationContractRevision` 的非终态 Run 不做
  default projection；以 typed contract-revision failure 终止并创建新 activation；
- 测试与开发环境若存在旧 fixture，直接重建当前 schema 的 Run，不把兼容逻辑写入 production core；
- provider target 仍来自 Run activation 时封存的 provider/model identity，版本门禁不得重新读取系统配置覆盖旧 Run。

`OperationContractRevision` 必须由 normalized static descriptor/variant catalog 计算，不是手工
配置字符串。activation、authority transaction、provider C2 projection、seal 与 owner preflight
必须核对同一 revision；binary catalog 变化时旧非终态 Run 不允许静默采用新规则。

## 16. Component Change Matrix

| Component | Required changes |
| --- | --- |
| `internal/pkg/workorchruntimecontract/canonical_runtime.go` | capability-state requirement/snapshot/ledger、typed accepted variant refs、required variant sequence、`CapabilityStateHash`、clone/normalize/validate/hash/same contract |
| `internal/pkg/workorchruntimecontract/runtime_operation_contract.go` | discriminated variants、唯一 matcher、variant required/forbidden parameters、effect/idempotency、state invalidation metadata；删除 kind 与 variant 双份规则 |
| `internal/pkg/workorchruntimecontract/runtime_candidate.go` / `runtime_state.go` | candidate operation requirement binding、variant/sequence/DAG seal、authority contract identity、attempt 0/1 exact failure、restart invariant |
| `internal/pkg/workorchruntimecontract/runtime_recovery_budget.go` | 独立 capability-state refresh policy/ledger/budget kind，不复用 focus repair、candidate、provider 或 operation recovery 预算 |
| `kernel/orchestrator/biz/runtimev2/prerequisite.go` / `reducer.go` | capability-state 缺失/失效只进入 `acquire_prerequisite`；core 只判断 identity/freshness/hash，不解释 owner condition |
| `controller.go` / `dispatch_commit.go` | operation claim/receipt/evidence CAS；variant requirement 一对一绑定；sequence/DAG/effect compatibility；seal 前 state hash gate |
| `executor.go` / `runtime_v2_mainline.go` | exact receipt recovery、typed `capability_state_stale` routing、同 sealed dispatch 已完成 predecessor 不重放、refresh 不消耗普通 repair |
| `prerequisite_event_adapter.go` / owner event contracts | 通过 EventHub 调用 concrete typed capability-state query；禁止通用 owner service/repository 或 map bridge |
| `module.go` | startup recovery inventory；CAS conflict 分类；capability-state refresh 与 terminal delivery retry 调度 |
| `kernel/run` aggregate/state repository | schema v2 validation、snapshot/sequence/refresh ledger 持久化、execution fence、termination record、recovery inventory query |
| `application/workorchd/biz/appservice/activationcore/runtime_v2_bootstrap.go` / activation ports | 从 profile 显式 capability-state requirement 编译 prerequisite；封存 refresh policy/operation contract revision；不从 accepted operation、错误文本或旧 Run 默认推断 |
| `blocks/llm/runtime_candidate_provider.go` / candidate boundary | C2 静态 variant catalog；C3 state snapshot summary/required sequence；按同一 descriptor 生成 JSON Schema；attempt 1 投影 exact variant failure；稳定前缀不漂移 |
| Git owner（首批） | concrete query command/result、owner-persisted query receipt、repository state → opaque conditions/required sequence 映射、variant preflight、state stale receipt、init/add/commit receipt 幂等 |
| File/Patch owner（首批） | `file_write` / `text_replace` / `unified_diff` 统一迁移到 variant matcher；apply/preview 复用同一参数合同并保持 effect 差异 |
| Shell/MCP/SSH/Web owners | 保持 concrete execute/query receipt、ObservedEffectScope、governance preflight 与 ExecutionKey 幂等；后续有 discriminator 时只能扩展同一 variant authority |
| Decision/Approval | operation-scoped governance binding/resolution；Runtime V2 resolution proposal/commit saga；allowed action owner validation；禁止先 resolved 后提交 aggregate |
| Result | closeout v2，包含 expected/observed effect、receipt refs、unknown outcomes |
| `application/workorchd/.../readmodelprojection` / TUI/Web | 展示 snapshot identity/freshness、required variant sequence、operation→requirement binding、stale refresh 与 separate budgets；只读 projection，不产生 authority |
| `scripts/verify-runtime-v2-positive-convergence-{file,live}.sh` | 增加 absent/unstaged/staged Git fixture、variant schema negative case、restart/stale/prefix assertions；产物仍只写 `/tmp` |
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
| RVC-R13 | code-landed foundation | provider/operation/CAS/terminal-delivery 独立 policy/ledger 已封存；RVC-R23 在同一模型上新增 capability-state refresh budget，不回退既有语义 |
| RVC-R14 | code-landed | Run owner typed recovery inventory、blocked binding verification、terminal delivery retry inventory、schema v2-only 与 read model 已落地 |
| RVC-R15 | code-landed | 真实 file-backed 主场景与 crash matrix 已覆盖两 focus、provider transient、CAS reload、File approval/grant、same key/next fence、stale/duplicate、validation、assessment、focus advance、terminal delivery retry、accepted Result、Run completed 与 Workspace release |
| RVC-R16 | verified | 2026-07-18 Live 先后暴露 governance/Resume/parameter/fixture 偏差；`run-024` 又暴露 capability-state 与 variant authority 缺口。RVC-R23～R26、file-backed matrix 与 bounded fixture creator 已完成本地收口；带 RVC-R26 的 fresh bounded Git/general terminal gate 已 passed。新的 canonical LSP-Bridge/ai-proxy canary 以无 Run baseline 通过官方 liveness verifier；pre-R25 `run-025` 保持不可迁移 historical diagnostic。 |
| RVC-R17 | code-landed | Decision ticket 内 durable pending proposal → Orchestrator typed commit → formal Resolution/ticket finalize；启动 reconciliation 只重放同一 commit |
| RVC-R18 | code-landed | blocked Runtime V2 Resume/Activate 均要求 aggregate matching resolution + request binding，普通 ticket resolved 不再清 blocker |
| RVC-R19 | code-landed | Decision owner server-first allowed-action gate；read model/TUI 提供并预检同一集合，batch 仍以服务端全量预检为准 |
| RVC-R20 | code-landed | provider output schema 已从 sealed RuntimeOperationContract 动态生成 patch target/parameter constraint；RVC-R24 已将 discriminator variant、stable rule ID 与 owner enumeration 收口为同一 descriptor authority。 |
| RVC-R21 | code-landed | ContextBudget/CacheUsage 改为 owner-neutral concrete audit DTO，LLM candidate/usage EventHub 与 eventstream payload 无 `map[string]any` bridge |
| RVC-R22 | code-landed / deployed-live pending | terminal gate 已强制恰好 2 focus、每 focus ≤4 coverage items，activation 后会再次读取权威 slice，避免 pre-activation 空 projection 绕过 bound，并核对两 slice 全部 accepted 与至少一个 accepted durable owner receipt；`create-runtime-v2-bounded-positive-fixtures.sh` 默认创建两个 script-owned 最小 `must_produce_state_change` template：A 通过一个 DoD validation focus 加一个 `git/write:commit` deliverable focus，显式要求 Git capability-state snapshot 并从 repository absent 开始；B 为通用 durable state-change/validation 双 focus。live verifier 现同时保存 snapshot/required sequence 观察，并对 A 的 sealed `init/add/commit`、required-operation ref、单一 capability-state hash、receipt fence 唯一性、outcome-unknown 和 blocked ticket/aggregate governance binding 做失败闭合；所有 template/workspace/deliverable/artifact 仅由脚本创建且可选清理，deliverable/artifact 均落在 `/tmp`。real-workspace liveness mode 已独立。仍待在最新部署实例创建 fixture 并最终重跑 RVC-R16。 |
| RVC-R23 | code-landed / deployed-live pending | capability-state prerequisite、Git read-only snapshot、owner-private durable/idempotent query receipt、C3 projection、required sequence/seal binding、authority state hash、typed stale receipt → atomic prerequisite refresh、独立 refresh budget 与 refresh-exhausted terminal routing 已落地；acceptance/read-model projection 只暴露 snapshot metadata、required sequence 和 sealed operation capability-state hash，不暴露 owner-private content，并供 live verifier 验证。query receipt 采用临时文件 fsync 后的 no-replace 原子发布，不会暴露半写 JSON；同一 query acquisition key 只有 exact snapshot payload 可幂等重放，不同 payload typed conflict，双 store 并发竞争也恰有一个 durable winner；Git stale preflight 要求每个前序 requirement 携带同一 sealed dispatch 的 exact accepted/durable/changed receipt binding。file-backed matrix 覆盖 absent/unstaged/staged sequence、query receipt 与 snapshot commit 的 restart、init receipt 后仅继续 add/commit、stale refresh CAS restart，以及 exact duplicate、late stale、mismatched stale receipt 的幂等/冲突路径；stale detail 已进入 receipt identity hash，refresh 不污染 candidate 或 focus repair budget。 |
| RVC-R24 | code-landed / deployed-live pending | Git/Patch discriminated descriptor、candidate seal、typed accepted variant refs 全量迁移、raw authority 删除及 Git/Patch owner enumeration 已落地；provider schema 按 sealed `AcceptedOperationVariants` 与 required sequence 的并集收窄 variant，且用 descriptor-derived stable rule ID 与 final seal 诊断对齐。descriptor 现在也拒绝 Git/Patch owner 会忽略的跨变体参数，避免在 schema/seal 后留下无语义输入；新 Run bootstrap 封存 `OperationContractRevision`，旧非终态 persisted Run 以 typed revision mismatch fail-fast，revision 纳入 C2 stable prefix/authority identity。file-backed matrix 覆盖 Git absent/unstaged/staged 的 `init/add/commit` sequence、Patch `file_write/text_replace/unified_diff`，以及三种 Patch variant 在 owner receipt 已提交、evidence 尚未提交时崩溃重启：复用 execution key/fence/receipt，不重放 durable apply。 |
| RVC-R25 | code-landed | RVC-R16 terminal rerun 发现 profile capability-state `OwnerModuleID` 使用 `git` EventHub alias 而非 descriptor canonical `blocks/git`，导致 sealed prerequisite 无 observer；已收口为 profile contract owner/accepted-kind validation、内置模板和 bounded fixture canonical asset、fixture 创建前 provider target preflight。不得为旧 owner 注册别名或迁移旧 aggregate。 |
| RVC-R26 | code-landed | RVC-R25 后 fresh Git fixture 证明 provider schema、sealed operation 和 Git command 均有 `requiredOperationRef`，但 strict candidate DTO 漏字段导致合法 candidate 被 `DisallowUnknownFields` 拒绝。DTO 现已显式映射该 typed field，保持 fail-closed policy；focused/full tests 与 positive-convergence file verifier 已覆盖。 |

## 17. Implementation Batches

本记录保留已完成 foundations 和批次依赖，用于审计，不再作为 active implementation backlog。

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
    MaxProviderTransportAttempts      int
    MaxOperationRecoveryAttempts      int
    MaxCapabilityStateRefreshAttempts int
    MaxCASReloadAttempts              int
    MaxTerminalDeliveryAttempts       int
}

type RuntimeRecoveryLedger struct {
    ProviderTransportAttempts      int
    OperationRecoveryAttempts      int
    CapabilityStateRefreshAttempts int
    CASReloadAttempts              int
    TerminalDeliveryAttempts       int
}
```

- policy 在 bootstrap/activation 时封存；credential、base URL、client 不持久化。
- candidate contract attempt 固定 0/1，不进入 recovery policy。
- per-focus repair ledger 与 recovery ledger 分离，任何一个成功/重置不得清零另一个。
- CAS conflict 只消耗 CAS reload budget，不写业务 failure/evidence。
- terminal delivery failure 只消耗 delivery budget，不改变 reducer terminal。
- provider auth/configuration 直接进入 configuration governance，不消耗普通 repair。
- capability-state query transient/stale 只消耗 state refresh budget；该字段由 RVC-R23 扩展，
  不改变 RVC-R13 已落地的其他预算语义。
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

Implementation closure:

- `verify-runtime-v2-positive-convergence-live` 默认 `terminal` mode，启动前从 snapshot 校验每个 workspace 的 focus/coverage 上限；超限立即写出 `fixture-preflight-*.json` 和 `summary.json` 并拒绝调度。
- `observe-runtime-v2-positive-convergence-live` 使用同一 verifier 的 `liveness` mode；禁止 activation，只将 accepted evidence、focus advance 或合法 blocked/terminal lifecycle 转移视为有效进展，避免把纯 revision/candidate/dispatch 重试误报为正向推进；同时检查 owner receipt 的 executionKey/fence 唯一性、无 `outcome_unknown` 及 projection 对齐。
- terminal timeout 不再直接退出；会持久化 final snapshot、governance ticket/approval、run observability 中的 operation receipt、usage audit、readyz 和汇总错误。

Exit gate:

- bounded dual workspace terminal gate 连续通过；
- LSP/ai-proxy observation 无非法迁移、无 authority split、无 projection lag、无重复副作用；
- 最终重新执行 RVC-R16，两个指定 positive fixtures 均 accepted，真实 canary 另行记录 liveness 结论。

### RVC-R23 — Owner-confirmed capability-state prerequisite

Scope: Activation、SM-2/SM-3/SM-4、Run owner、Git owner（首批）、read model

目标：把“能力存在”与“能力在当前 subject state 下怎样合法执行”拆为两个 typed
prerequisite。capability inventory 继续证明 owner 支持某 kind；capability-state snapshot
只读确认该 kind 当前所需的 operation variant sequence。任何 side effect 仍只能在 sealed
dispatch 中发生。

#### RVC-R23.1 Contract and authority

- `PrerequisiteKindCapabilityState` 进入 `RuntimeRequirement` / `RuntimePrerequisite`，至少
  封存 owner module、capability kind、subject ref 与 stable requirement ID；
- profile/activation 增加显式 typed capability-state requirement；application compiler 只
  canonicalize/seal 该声明，不从 accepted variant、`commit` 文本、workspace 名称、历史
  失败或 owner implementation 猜测；
- RVC-R23 contract foundation 同时提供最小 `RuntimeOperationVariantRef{Kind, VariantID}`，
  供 owner required sequence 引用；profile/focus 的 accepted-operation authority 全量迁移仍
  属于 RVC-R24；
- owner 通过 concrete EventHub query 返回 canonical `RuntimeCapabilityStateSnapshot` 与
  durable query receipt。共享 snapshot value 可以位于 runtime contract 包，但 Git/File 等
  command/result 必须属于各 owner，禁止通用 `QueryCapabilityStateEnvelope`；
- Orchestrator aggregate 是“当前 Run 采用哪个 snapshot”的权威；owner query receipt 是
  “owner 实际观察了什么”的权威；read model 只投影二者；
- `RuntimeResourceFacts`、`AuthoritySnapshot`、`AuthorityTransaction`、state clone/hash/
  validation、Result closeout audit 必须完整携带 snapshot/sequence/state hash identity。
- snapshot validation 必须联合检查 disposition/sequence：`ready` 不得带 required sequence，
  `operation_sequence_required` 必须有非空 acyclic sequence，治理/不可用/未知 disposition
  不得进入 candidate。

Git 首批 state query 至少观察：deliverable scope、repository initialized、staged/
unstaged state、HEAD/unborn identity。Git block 自行把这些 owner conditions 转为
`repo-init` / `stage-changes` / `create-commit` required sequence；core 不读取这些 condition
名称，也不把 `git init` 当 prerequisite acquisition。

#### RVC-R23.2 Legal transitions

```text
capability-state requirement pending
  -> acquire_prerequisite
  -> owner query receipt persisted
  -> snapshot committed to Run aggregate
  -> reducer evaluates owner-neutral disposition
       ready / operation_sequence_required -> recompute RequiredOutput -> candidate allowed
       governance_required                 -> request_governance
       unavailable / unknown               -> refresh or typed bounded routing

sealed operation preflight reports capability_state_stale
  -> persist stale receipt/diagnostic
  -> invalidate exact active snapshot
  -> increment capability-state refresh ledger
  -> acquire_prerequisite with next refresh generation
  -> new snapshot/required sequence
  -> new authority transaction and candidate attempt 0
```

合法状态变化必须满足：

- initial acquisition 成功后才允许 candidate；query transient 可按独立 budget 重试；
- snapshot commit 必须匹配 workspace/run/requirement/subject/acquisition key 与 owner receipt；
- stale refresh 后 `CapabilityStateHash` 与 authority contract 必须更新，旧 candidate/dispatch
  不得 reseal；
- 同一 sealed dispatch 中，前序 accepted receipt 造成的预期 owner state 变化继续执行剩余
  dependency，不触发 refresh；
- dispatch 完成后，按 variant invalidation metadata 使 active snapshot 失效，下一次 reducer
  需要相关 state 时重新 acquisition；owner receipt 必须携带 exact owner/kind/subject ref，
  Orchestrator 只失效精确匹配项。

#### RVC-R23.3 Forbidden transitions

- capability-state pending/stale → candidate；
- prerequisite query → `git init`、file write、configuration mutation 或任何 effect receipt；
- execution failure 文本 `not_git_repository` → reducer 直接插入 Git operation；
- owner query result 未持久化/identity 不匹配 → aggregate snapshot commit；
- `ready` + non-empty sequence、`operation_sequence_required` + empty sequence 或 core 根据
  condition 文本重算 disposition → candidate；
- stale snapshot → 普通 focus repair、candidate attempt 1 或第三次 candidate；
- external state drift → 继续使用旧 `CapabilityStateHash` reseal；
- projection/TUI/Web 改写 snapshot freshness、required sequence 或 refresh decision；
- Resume 为旧 activation 补 capability-state requirement/default snapshot。

#### RVC-R23.4 Duplicate, restart and recovery

- `AcquisitionKey = workspace/run/requirement/refresh-generation`；same key duplicate query
  返回相同 query receipt/snapshot，different payload 必须 conflict；
- crash after owner query receipt / before aggregate commit：restart 查询 same key 并提交同一
  snapshot，不创建下一 generation；
- crash after snapshot commit / before candidate：restart 直接 reducer，不重复 query；
- crash after `init` receipt / before `add`：SM-3 exact receipt recovery 关闭已完成 node，只继续
  remaining dependency；不得因 snapshot 已被 init 改变而把预期推进误判为 stale；
- stale/duplicate query receipt 只进入 diagnostic/idempotent path，不得推进 evidence revision；
- old dispatch 返回 `capability_state_stale` 时必须同时匹配 exact transaction/state hash、
  snapshot ID、state ref 与 observed detail，否则只是 stale receipt diagnostic，不能失效当前新 snapshot。

#### RVC-R23.5 Budget and terminal routing

`RuntimeRecoveryPolicy/Ledger` 增加独立 capability-state refresh budget。initial successful
acquisition 不消耗预算；query transient retry、execution preflight 的 confirmed external stale
各消耗一次。成功 refresh 只重置当前 requirement 的 failure signature/no-progress window，
不清零历史 used count；focus 原子 advance 或 requirement 不再 active 时建立新 ledger scope。

- owner auth/configuration failure → configuration governance；
- capability unavailable/subject inaccessible → capability/external governance 或 typed bounded fail；
- owner disposition `governance_required` → typed dependency/ticket，不先请求 candidate；
- refresh budget exhausted → `capability_state_refresh_exhausted` governance/failure；
- 以上均不得消耗 candidate attempt、focus repair、provider transport 或 operation recovery budget。

#### RVC-R23.6 Implementation order and exit gate

实现顺序固定为：shared value/validation/hash → activation compiler → owner concrete query +
receipt → prerequisite adapter/mainline → reducer/state hash → stale routing/recovery budget →
read model → file-backed/restart tests。

Exit gate:

- repository absent 时 candidate 前已持久化 snapshot，并要求 sealed `init -> add -> commit`；
- initialized + unstaged 只要求 `add -> commit`；initialized + staged 只要求 `commit`；
- prerequisite acquisition 零副作用，`git init` 只出现在 sealed operation receipt；
- stale snapshot 能有界 refresh，且不污染 candidate/focus repair budgets；
- ready/sequence-required/governance/unavailable/unknown disposition 的合法与禁止迁移测试齐备；
- crash after query、snapshot commit、init receipt 均可自动恢复且无重复 side effect；
- core test/scan 不出现 repository、Git condition、commit message 等领域分支。

### RVC-R24 — Discriminated operation variant contract and schema

Scope: SM-2/SM-3、`workorchruntimecontract`、activation/profile、LLM provider boundary、
Git/File/Patch owners、read model

目标：把当前 kind-level 参数 catalog 升级为 variant-level executable authority，使
descriptor → provider schema → candidate validation → final seal → owner preflight 全部调用
同一 matcher，消除 `commit + pathspecs` 这类直到 final validation 才暴露的机械偏差。

#### RVC-R24.1 Type migration

- `RuntimeOperationContract.Variants` 定义 discriminator、required/one-of/forbidden
  parameters、effect、idempotency 与允许失效的 capability-state kinds；owner receipt 再给出
  exact owner/kind/subject invalidation refs；
- `RuntimeOperationVariantRef{Kind, VariantID}` 成为 profile evidence requirement、
  CoverageFocus、RequiredOutput 的 typed authority；
- `AcceptedOperations []string` 从当前 activation/profile/aggregate authority 删除，仓库内
  current fixtures/config 全量迁移为 `AcceptedOperationVariants`，不提供 `commit ->
  git/write:commit` 默认推断；
- `RuntimeOperation.RequiredOperationRef` 绑定 capability-state sequence requirement；variant
  ID 只能由 parameters + descriptor matcher 派生；
- normalized hash/same/clone 必须覆盖 variant catalog revision、accepted refs、required refs
  与 operation binding，且无业务顺序语义的 catalog stable sort，有依赖语义的 sequence 保序。

candidate prompt/schema ID 必须升级到 `workorch.runtime_candidate*.v3`，activation/definition
必须封存新的 `OperationContractRevision`。新代码遇到缺少该 revision、仍携带 raw
`AcceptedOperations`、或 required sequence 无 requirement identity 的非终态 Run，必须 typed
fail 并要求创建新 activation；禁止 compatibility/default projection。

#### RVC-R24.2 Single matcher and seal order

单一 executable matcher 位于 `workorchruntimecontract`，至少暴露：

```text
MatchRuntimeOperationVariant(operation, contract)
ValidateRuntimeOperationVariant(operation, variant)
ValidateRequiredOperationBindings(operations, requiredSequence)
```

正式 seal 顺序：

```text
operation kind canonicalization
  -> kind-level parameter name/type validation
  -> exactly-one variant match
  -> variant required/one-of/forbidden validation
  -> RequiredOutput accepted variant gate
  -> required-operation one-to-one binding
  -> dependency graph / requirement order / cycle validation
  -> expected effect + idempotency resolution
  -> CapabilityStateHash / authority transaction match
  -> persist SealedDispatch
```

provider schema 与 owner adapter 都调用或投影此 matcher 的数据；允许 owner 保留具体执行
switch，但必须有 enumeration regression 证明 owner supported operations 与 descriptor variants
完全相等，不能维护另一份参数合法性表。

#### RVC-R24.3 Provider schema and retry

- C2 只包含 stable static variant catalog、parameter types 与 declarative rules；
- C3 包含 current accepted variant refs、snapshot identity/hash、required operation refs/
  sequence；C4 包含 exact rejected variant failure 与 shape hash；
- schema 对每个 variant 生成 discriminator `const` 和 variant parameter constraints；required
  sequence 对 operations array 生成不同 requirement ref 的 `contains` 分支；
- attempt 0/1 使用相同 authority transaction 与 C0～C2 stable prefix；attempt 1 只更新 C4；
- attempt 1 failure 必须包含 operation ID、kind、derived/expected variant、requirement ref、
  missing/forbidden/conflicting parameter 与 descriptor rule ID；
- provider 不完整支持 `if/then/contains` 时仍可返回 candidate，但 final seal 必须一致拒绝，
  不允许放宽为 prompt-only 安全。

#### RVC-R24.4 Initial variant catalog

| Kind | Required variants and executable constraints |
| --- | --- |
| `git/write` | `init`：`operation=init`，forbid `pathspecs/message/ref/baseRef`；`add`：`operation=add`，require non-empty `pathspecs`，forbid `message/ref/baseRef`；`commit`：`operation=commit`，require non-empty `message`，forbid `pathspecs/ref/baseRef` |
| `git/read` | `status/diff/log/show/branch`；每个 variant 明确 `ref/baseRef/pathspecs/limit` 的 required/allowed/forbidden 集合 |
| `patch/apply` | `file_write` require `content`；`text_replace` require non-empty `oldText` + `newText`；`unified_diff` require non-empty `diff`；三者均复用 canonical target/baseHash gate |
| `patch/preview` | 与 apply 使用同一 discriminator/parameter variant，但 effect scope 为 none/ephemeral observation，禁止复用 apply durable effect |

表中未写出的细节必须由现有 owner contract 盘点后显式进入 descriptor；不得把当前 switch
中的隐含默认继续留在 owner 内。Patch `file_write` 与独立 `file/write` 仍是不同 kind/owner
合同，不得因 variant 同名重新合并。

#### RVC-R24.5 Legal/forbidden transitions and recovery

合法：attempt 0 variant contract failure → same transaction attempt 1；attempt 1 合法 → seal；
两次失败 → `candidate_retry_exhausted`；sealed execution state stale → RVC-R23 refresh/new
transaction，而不是复用 attempt 1。

禁止：

- zero/multiple variant match → seal；
- `git/write:commit` 携带 `pathspecs`；`git/write:add` 无 pathspecs；
- required operation ref 缺失、重复、绑定错误 kind/variant 或 dependency 不满足 → seal；
- LLM 显式 variant 字段覆盖 parameters 派生结果；
- provider schema、final validator、Git/Patch owner 各维护一套不一致规则；
- attempt 1 改变 accepted variants、required sequence、state hash 或 stable prefix；
- owner adapter 用 execution failure 修补/重写已 sealed operation parameters。

duplicate/restart 继续使用 sealed operation ID + ExecutionKey/fence；variant ID、requirement ref、
state hash 都是 sealed identity 的一部分。duplicate receipt 必须与这些字段相同才幂等；
不同 variant/binding 使用同 execution identity 必须 conflict。

#### RVC-R24.6 Implementation order and exit gate

实现顺序固定为：variant/ref types → current profile/fixture typed migration → matcher + canonical
tests → RequiredOutput/candidate/seal → provider schema/retry projection → Git owner → Patch owner →
read model → full file-backed matrix。

Exit gate:

- provider schema 在 attempt 0 即拒绝 `commit + pathspecs`，final seal 对同 shape 给出同一
  typed rule ID；
- attempt 0 非法、attempt 1 精确修正后可 seal；两次非法严格 bounded terminal；
- required `init/add/commit` 三项各绑定一次且 DAG 顺序正确，漏项/重复/错绑均拒绝；
- Git/Patch descriptor、provider schema、seal、owner preflight enumeration 一致；
- Patch 三 variants 与独立 `file/write` 全量回归通过；
- capability-state C3 变化、attempt 0/1、restart 均不造成 C0～C2 stable prefix drift。

### 17.1 Mandatory closure order

后续代码必须按以下依赖顺序逐项收口，前一项 exit gate 未通过时不得把后一项标记
completed：

1. RVC-R23 contract foundation：capability-state types、acquisition identity、snapshot/hash、
   typed owner query；
2. RVC-R23 mainline/recovery：activation compiler、prerequisite/reducer、stale routing、独立
   refresh budget、restart/read model；
3. RVC-R24 contract migration：typed accepted variant refs、descriptor variants、single matcher、
   current profile/fixture 全量迁移；
4. RVC-R24 boundary/owners：provider schema + exact attempt 1、final seal、Git/Patch owner
   preflight/enumeration；
5. deterministic file-backed matrix：absent/unstaged/staged、stale、crash、duplicate、Patch
   variants、stable prefix；
6. RVC-R22 clean bounded 双 fixture 创建与 terminal gate 预检；
7. RVC-R16 final rerun：先 bounded dual terminal gate，再真实 canary observation。

RVC-R11～R15、RVC-R17～R22 保持 code-landed foundation；后续不得回退 exact fence、typed
grant、independent budget、startup inventory、terminal delivery lane、schema v2-only、server-first
allowed action 或 terminal/liveness 分层。

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
- capability-state requirement missing/stale 时只能 acquire prerequisite，不能请求 candidate。
- owner snapshot required sequence 只按 requirement/variant identity解释，不按 Git condition 名称分支。
- owner-neutral ready/sequence-required/governance/unavailable/unknown disposition 迁移与 sequence cardinality 联合校验。
- capability-state refresh 与 focus repair、candidate/provider/operation budgets 互不污染。

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
- repository absent → snapshot → sealed `init/add/commit`，每项绑定唯一 requirement ref。
- initialized + unstaged → `add/commit`；initialized + staged → `commit`。
- `git/write:commit + pathspecs` 在 provider schema/final seal 使用同一 typed rule 拒绝。
- attempt 0 variant failure、attempt 1 exact correction；两次 variant failure bounded terminal。
- required variant 漏项、重复 ref、wrong kind/variant binding、dependency cycle/缺边均拒绝。
- stale state receipt 失效旧 snapshot 后必须新 transaction/candidate attempt 0，不得复用 attempt 1。
- crash after `init` receipt 只继续 `add/commit`，不重复 init。
- duplicate/stale query/operation receipt 不增加 accepted evidence 或重复副作用。
- accepted receipt 的 capability-state invalidation refs 必须是 descriptor allowed kinds 子集且 exact subject matching。

### 18.4 SM-4

- startup inventory active/no-dispatch。
- active/sealed receipt recovery。
- blocked wait 不自动继续。
- terminal delivery restart retry。
- CAS conflict reload，不生成伪 terminal。
- provider/operation/CAS/delivery budget 各自独立消耗与耗尽。
- projection high-watermark/readiness 不反向驱动 reducer。
- pending governance proposal startup reconciliation 不依赖 application Resume watcher。
- crash after capability-state query receipt / before aggregate commit 使用 same acquisition key 恢复。
- crash after snapshot commit / before candidate 不重复 owner query。
- capability-state refresh budget exhausted 进入 typed governance/failure，不形成 query/candidate 空转。

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
- capability-state snapshot/required sequence 只进入 C3/C4；C0～C2 hash 在 state refresh 前后不漂移。
- attempt 0/1 的 static variant catalog 与 stable prefix 完全一致。
- Git owner supported sub-operations 与 descriptor variants 双向 enumeration 一致。
- Patch apply/preview 的 `file_write/text_replace/unified_diff` variants 与独立 `file/write` 均回归。
- pre-R23/R24 nonterminal activation 缺 contract revision 时 typed fail；无 compatibility/default projection。

## 19. Verification Commands

每批先执行 focused tests，再逐步扩大：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationcore -count=1
go test ./internal/modules/blocks/git/... -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
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
make observe-runtime-v2-positive-convergence-live
```

`verify-runtime-v2-positive-convergence-file/live` 必须升级为本方案定义的真实行为验证，不得继续只聚合已有测试名称。

新增结构扫描必须检查：

```bash
rg -n 'ContextBudget map\[string\]any|CacheUsage map\[string\]any' internal/modules/blocks/llm/pkg/events
rg -n 'case "replan"|batchResolve\(.replan' internal/services/workorch/service/tui web/workorch-console/src
rg -n 'ActivateRuntimeV2Run|CanResumeRun' internal/modules/kernel/orchestrator
rg -n 'AcceptedOperations' internal/pkg/workorchruntimecontract internal/modules/application/workorchd internal/modules/kernel/orchestrator internal/modules/blocks/llm
rg -n 'not_git_repository|repository_initialized|case .*commit' internal/modules/kernel/orchestrator/biz/runtimev2
rg -n 'RuntimeOperationContractForKind|MatchRuntimeOperationVariant|ValidateRuntimeOperationVariant' internal/pkg/workorchruntimecontract internal/modules/blocks/{llm,git,file}
```

既有 LLM/治理/Resume 扫描命中不直接代表失败，但每处必须由 typed gate/test 证明不会绕过 authority。
RVC-R24 完成后，生产 authority 中的 raw `AcceptedOperations` 必须为零；variant matcher 扫描
必须证明 executable rules 只定义在 contract 包，LLM/owner 仅消费/投影，不复制规则表。

## 20. Deployed Live Acceptance

### 20.1 Service/provider preflight

- deployed binary hash 与 clean build 一致；
- `/readyz` 为 ready；
- source/projected revision 追平；
- ai-proxy catalog fresh；
- workspace target 为预期 `providerId/modelId`；
- credential/base URL 未进入 workspace/run persisted authority；
- candidate runtime contract revision 已包含 capability-state/operation-variant closure，fixture 不复用 pre-R23/R24 nonterminal Run。

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
10. projection revision 追平；
11. 双 fixture 至少一个覆盖 owner-confirmed capability-state prerequisite；snapshot 必须早于首个 candidate attempt；
12. 若 snapshot 声明 required sequence，sealed operation、receipt 与 requirement binding 数量/顺序完全一致。

fixture 额外限制：

- ≤2 focuses；
- 每 focus ≤4 coverage items；
- 使用全新 Run，不 Resume 历史 terminal/invalid governance Run；
- 180 秒 terminal SLA 只适用于该 bounded fixture；
- 至少一个 fixture 从 repository absent 或等价 owner-state 前置条件开始，证明 prerequisite 只读且 preparatory mutation 经过 sealed dispatch。

### 20.3 Real-workspace liveness proof

真实 LSP-Bridge/ai-proxy canary 至少证明：

1. source/projected revision 追平；
2. 一段 observation window 内 state revision、evidence、focus 或合法 governed/terminal 状态至少一项可解释变化；
3. blocked governance 的 ticket action 与 aggregate binding 一致；
4. resolved ticket 必须有 matching aggregate resolution，或明确处于 pending proposal reconciliation；
5. candidate contract failure、execution failure、provider transient 各自进入正确预算/终态；
6. 不把大型 DoD 未在 180 秒完成误判为状态机死锁；
7. 需要 owner state 的 focus 在 candidate 前已有 matching snapshot/required sequence；
8. state refresh 只生成新 authority transaction，不复用旧 candidate/dispatch。

### 20.4 Negative assertions

- 无 `stateChanging=false` operation 被记为 state-change；
- 无 session/read-only MCP 被记为 durable state-change；
- 无 reducer/candidate action contradiction；
- 无相同 ExecutionKey 重复副作用；
- 无 CAS/owner transient 被误记为 runtime contract defect；
- 无 terminal Run Resume；
- 无 stable prefix unexpected drift；
- 无 ticket resolved / aggregate unresolved split；
- 无 blocked Runtime V2 被普通 Resume 清 blocker；
- 无端侧提交不在 allowed actions 中的 resolution；
- 无 typed rejected terminal 被误报为 positive accepted pass；
- 无 capability-state prerequisite 产生 side effect；
- 无 stale snapshot 继续 seal，且无预期 predecessor state change 被误判为 external stale；
- 无 raw `AcceptedOperations` 默认推断 variant；
- 无 operation requirement ref 漏绑/重复/错绑后进入 owner；
- 无 `git/write:commit` 携带 `pathspecs` 到达 execution；
- 无 capability-state C3/C4 变化导致 C0～C2 `stablePrefixHash` 漂移。

## 21. Documentation Closure

代码收口期间必须同步：

- `docs/00-core/runtime-stage-contract.md`：四状态机正式语义、effect/receipt/recovery/lifecycle。
- `docs/00-core/runtime-request-cache-note.md`：static variant catalog=C2、dynamic state/sequence=C3/C4、candidate contract version 与 prefix stability。
- `internal/modules/kernel/orchestrator/readme.md`：capability-state prerequisite、state hash、variant seal、stale/restart 与 EventHub topic。
- capability owner `readme.md`：state query receipt、condition/sequence mapping、variant preflight、invalidation 与 stale contract。
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
- prerequisite acquisition 中自动执行 `git init`、file write 或其他 preparatory mutation。
- 在 reducer 中根据 `not_git_repository`、`commit`、`patchType` 或 workspace 名称选择 operation。
- 继续以 `AcceptedOperations=["commit"]` 脱离 kind 地匹配任意 parameters。
- 只在 provider JSON schema 增加 variant 规则，final seal/owner preflight 仍保留另一套手写判断。
- state stale 后继续 attempt 1、消耗 focus repair budget或复用旧 `CapabilityStateHash`。

## 23. Definition Of Done

RVC-1 只有同时满足以下条件才能关闭：

1. RVC-R5～R15 foundation 保持通过，RVC-R17～R26 全部完成，并最终重新通过 RVC-R16；
2. core contract、module readme、contract types 与代码一致；
3. effect、receipt、repair directive、governance、termination 和 recovery 均为 persisted typed authority；
4. 四状态机合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试齐备；
5. boundary verifier、full tests、file-backed E2E 通过；
6. 两个 bounded positive fixtures 均跨 focus并产生 accepted Result；LSP-Bridge/ai-proxy 真实 canary 均满足独立 liveness 断言且无非法迁移；
7. 无 credential/runtime client state 序列化回归；
8. active roadmap、current-state 和 unclosed matrix 不再保留虚假 code-closed 状态；
9. Runtime V2 governance 不存在 resolved-ticket/uncommitted-aggregate 或 aggregate-committed/unrecoverable-owner-projection 状态；
10. 普通 Resume、TUI/Web/CLI/HTTP/batch 均不能绕过 aggregate authority 与 allowed action contract；
11. bounded deployed terminal gate 与真实长任务 liveness observation 均有独立证据；
12. capability-state acquisition 在 candidate 前完成、只读、可审计、可失效、可有界 refresh，restart 不重复 owner query receipt 或 sealed operation side effect；
13. typed accepted variant refs 与 required operation refs 已替代 raw `AcceptedOperations` authority，descriptor/provider schema/candidate/seal/owner preflight 使用同一 matcher；
14. repository absent/unstaged/staged、variant attempt 0/1、stale refresh、crash after init receipt、duplicate/stale receipt、Patch variants 与 stable prefix file-backed matrix 全部通过。

上述条件均已满足；本项已完成，应迁入 completed archive，后续实现不得再将本计划作为 active 约束。
