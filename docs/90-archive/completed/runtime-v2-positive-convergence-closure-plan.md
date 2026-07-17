# Runtime V2 Positive Convergence Closure Plan

Status: completed

Type: roadmap

Last Updated: 2026-07-17

## Purpose

本计划用于收口 Runtime V2 在真实 workspace 长流程执行中暴露出的“能够启动和产生局部证据，但无法持续正向收敛”问题。

目标不是围绕某个 workspace、某个模型或某条错误信息增加特殊分支，而是建立以下统一闭环：

```text
typed activation
  -> feasible focus contract
  -> capability-backed candidate
  -> sealed operation
  -> evidence quality gate
  -> assessment
       -> next focus
       -> same-focus repair
       -> governance wait
       -> bounded terminal rejection
  -> structured result / continuation
  -> trustworthy projection
```

本计划是 active 工作分解，不是 `docs/00-core/runtime-stage-contract.md` 之外的并列核心合同。凡涉及 Runtime V2 语义调整，必须先更新核心合同，再同步 flow、模块 readme、类型、实现和验证。

## Authority And Constraints

实现与验收必须遵守：

- [Runtime Stage Contract](../../00-core/runtime-stage-contract.md)
- [Runtime Request Cache Note](../../00-core/runtime-request-cache-note.md)
- [Orchestrator Implementation SoT](../../../internal/modules/kernel/orchestrator/readme.md)
- [Current State](../../01-overview/current-state.md)
- [Verification Overview](../../60-verification/overview.md)

不可突破的边界：

- 生产路径保持 Runtime V2 only：`DriveRuntimeV2` + reducer + sealed dispatch。
- 不恢复 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、V1 checkpoint 或自动 replan successor。
- core 只理解中立的 focus、evidence、assessment、capability、side effect、repair、governance 和 terminal，不理解软件开发、LSP、HTTP、Go、测试框架或具体业务语义。
- application 只负责编译 typed activation/bootstrap；kernel/orchestrator 是唯一 convergence authority。
- capability blocks 只执行 operation 并返回 typed result/evidence，不裁决 coverage、assessment 或 terminal。
- Result owner 只持久化 Orchestrator 已裁决的 outcome，不参与 acceptance 决策。
- Run owner 持久化 aggregate 和 lineage，不选择 RuntimeAction。
- projection、TUI、Web、HTTP 只展示和触发正式用例，不成为第二条 runtime authority。
- 跨 owner 交互继续使用 EventHub + concrete typed command/result。
- provider-facing cache prefix 继续保持 C0/C1/C2 稳定；current focus、failure、repair gap 位于 C3/C4。

## Normative State Machine Binding

本计划涉及的完整状态机已经固化到 [Runtime V2 四状态机固化规约](../../00-core/runtime-stage-contract.md#runtime-v2-四状态机固化规约)：

1. SM-1 Run Lifecycle State Machine；
2. SM-2 Authority And Focus Convergence State Machine；
3. SM-3 Candidate And Sealed Dispatch Transaction State Machine；
4. SM-4 Drive, Recovery And Projection Delivery State Machine。

本计划中的流程图、INV-01～INV-10 和 RVC-01～18 只用于工作分解与证据跟踪，不得修改、简化或替代核心规约。后续每个 RVC 实现项必须标注影响的 SM 编号，并补齐核心规约要求的合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试。

## Confirmed Live Evidence

2026-07-16 对重新部署实例的只读核对确认：

| Workspace | Run | Provider Target | 运行结果 | 直接终止原因 |
| --- | --- | --- | --- | --- |
| `ws-bea4fee67e459bcc` / ai-proxy | `run-003` | `ai-proxy/deepseek-v4-flash` | failed | `op-validate-healthz` 连续两次被判定为带 workspace side effect，触发 `candidate_retry_exhausted` |
| `ws-1138d6b3bfddb4fd` / LSP-Bridge | `run-003` | `ai-proxy/deepseek-v4-flash` | failed | 只生成 `docs/tools.md` 并以 `file/read` 验证，assessment 判定十个工具未实现，触发 `assessment_rejected` |

同时确认：

- 两个 Run 都成功调用了 workspace 封存的 provider/model；不是 provider 选择、credential、context ceiling 或请求超限导致。
- ai-proxy Run 实际 focus 总量为 397（392 个 DoD item + profile focuses），首个 focus 后仍有 396 个。
- LSP-Bridge Run 实际 focus 总量为 30（25 个 DoD item + profile focuses），首个 focus 后仍有 29 个。
- ai-proxy 的 run-002 与 run-003 重复出现 validation side-effect failure，属于可复现结构问题。
- `validation/managed_session` 存在于 activation capability contract，但未进入实际 `RuntimeCapabilities.ValidationKinds`。
- 普通 DoD focus 未继承 `scope=dodItem, id=*` evidence requirement，导致 `file/read` 可作为 validation evidence 被 reducer 接受。
- rejected result continuation 只有 summary，`UnsatisfiedDoDItems`、`CoverageGaps`、`RequiredEvidence` 为空。
- RuntimeBootstrap continuation 携带 predecessor result，但 Run lineage 的 `PredecessorRunID` 为空。
- workspace projection 的 source/projected revision 对齐，但全局状态持续为 `degraded`；历史 rebuild failure counter 污染当前健康判断。
- LLM usage event 已产生，但部分 Run snapshot 的 `llmUsageSummary` 仍为零值。

## Root Cause Matrix

| ID | 根因 | 结构影响 | 不能只做的局部处理 |
| --- | --- | --- | --- |
| GAP-01 | operation kind 的 owner、side-effect、authority compatibility 分散定义 | capability contract 与实际 owner capability 漂移 | 只给 `validation/managed_session` 增加一个 switch case |
| GAP-02 | focus 编译没有统一解析 exact/wildcard/default evidence policy | DoD focus 接受弱证据，profile 要求未进入 reducer | 只在 LSP prompt 中要求执行测试 |
| GAP-03 | Run 级 `must_produce_state_change` 被机械应用到每个 DoD focus | 每个 claim 都要求修改 workspace，放大长任务成本并制造无意义改动 | 只提高 transition guard 或 candidate 次数 |
| GAP-04 | `Assessment.Satisfied=false` 直接终态化 | 已识别到明确缺口后仍不能在同 Run repair | 只针对 `assessment_rejected` 自动 Resume |
| GAP-05 | candidate contract retry 与业务 repair budget 混用 | 同一非法候选两次后整个 Run 失败，无法转成 capability gap 或定向修复 | 只把 attempt 从 2 调大 |
| GAP-06 | 每个 DoD item 机械生成独立 focus，未落实 acceptance slice policy | 30/397 个串行 focus，粒度过细且无法部分验收 | 只提高 LLM token/output budget |
| GAP-07 | rejected Result 缺少结构化 gap，Run lineage 未完整写入 | 新 activation 重新从第一 focus 开始，重复旧失败 | 只把旧 summary 拼进 prompt |
| GAP-08 | projection degraded 状态不能按当前 generation 恢复 | live verifier 失败，UI 健康状态不可信 | 只清空 metrics counter 或隐藏 degraded |
| GAP-09 | event/read-model projection 对 LLM usage、terminal release 存在不一致 | 运行事实与展示事实偏离，干扰推进判断 | 只在 TUI 中读取另一条 API |

## Target Invariants

### INV-01 Feasible Authority

Orchestrator 在请求 LLM candidate 前必须证明当前 `RequiredOutputContract` 至少包含一个：

- 已由 owner capability preflight 确认；
- 与当前 RuntimeAction 兼容；
- side-effect class 与 required output 兼容；
- 可通过 typed EventHub operation command 执行；
- 满足 focus accepted/rejected kind 与 accepted operation 约束；

的 concrete operation kind。

若不存在，不得请求 LLM 猜测操作，必须生成 typed capability gap 并进入 deterministic prerequisite、governance 或 bounded terminal。

### INV-02 Single Operation Descriptor Authority

共享 runtime contract 必须提供唯一的中立 operation descriptor：

```text
RuntimeOperationDescriptor
  Kind
  OwnerModuleID
  EffectClass           // non_mutating | workspace_state | remote_state | unknown
  AllowedActions        // produce_state_change | produce_validation
  CapabilityClass
```

descriptor 只描述技术合同，不注册 executor，不恢复通用 RuntimeAdapter registry。

以下逻辑必须从同一 descriptor 派生：

- operation kind -> owner module；
- profile accepted kind -> runtime prerequisite；
- validation feasibility；
- side-effect gate；
- operation dispatch destination；
- contract verifier inventory。

### INV-03 Compiled Focus Policy

Activation 必须通过单一 `CoverageFocusCompiler` 将 Goal/DoD/profile policy 编译为 typed focus queue。

policy precedence：

```text
exact(scope, id)
  > wildcard(scope, *)
  > scope default
  > neutral runtime default
```

编译结果至少包含：

- coverage work item IDs；
- required evidence kinds；
- accepted/rejected step kinds；
- accepted operations；
- evidence role；
- direct/claim/executable evidence requirements；
- assessment requirement；
- slice identity 和 item membership。

### INV-04 Run-Level Vs Focus-Level State Change

`must_produce_state_change` 应表达 Run 至少产生一次与 active coverage 对齐的 primary state-change evidence，不表示每个 DoD item 都必须修改 workspace。

- deliverable/source-change focus 可以显式要求 state_change；
- 行为、错误路径、性能、运行时等 claim 通常要求 validation + assessment；
- 当 assessment 或 validation 识别实现缺口时，reducer 可以在同 focus 补充 state_change requirement；
- 已有 state-change evidence 不能跨 coverage 无条件满足其他 focus。

### INV-05 Bounded Acceptance Slices

Activation 应按 profile/template 声明的 acceptance slice policy，将 work item 编译为有界 slice；core 不解析业务标题、语言或技术栈。

要求：

- slice 大小有配置上限；
- item 原始顺序稳定；
- 每个 item 保留独立 coverage identity；
- assessment 支持 slice 内部分 satisfied/unsatisfied；
- satisfied item 不因同 slice 其他 item 失败而丢失 evidence lineage；
- focus queue 只推进仍未关闭的 work item。

### INV-06 Repairable Assessment

Assessment candidate 不得只使用一个布尔值决定 Run terminal。需要增加中立 disposition 与 gap：

```text
AssessmentDisposition
  satisfied
  repairable_gap
  governance_required
  terminal_rejection

AssessmentGap
  CoverageWorkItemIDs
  MissingEvidenceKinds
  TargetRefs
  EvidenceRefs
  Reason
```

reducer 行为：

- `satisfied`：关闭对应 items；有 remaining focus 时推进下一 focus；
- `repairable_gap`：写 assessment/gap fact，按缺口回到 state-change 或 validation；
- `governance_required`：打开 typed decision/approval；
- `terminal_rejection`：仅在明确不可恢复事实成立时终态；
- repair budget 耗尽：由 Orchestrator 将当前结构化 gap 收口为 rejected Result。

### INV-07 Separate Retry And Repair Budgets

- candidate attempt 0/1：只修正同 authority transaction 的 schema/contract candidate；保持 stable prefix 不变。
- operation/evidence/assessment repair：按 workspace/run/focus/failure signature 独立计数；每次有效 evidence 或 focus change 重置对应 no-progress 条件。
- capability 不可行不得消耗 candidate attempt。
- attempt 1 再次失败后必须保留结构化 failure；不得只生成 summary string。
- 不通过扩大 candidate attempt 数量掩盖能力或合同缺失。

### INV-08 Structured Result And Continuation

Rejected Result 必须持久化：

- satisfied/unsatisfied coverage work item IDs；
- missing evidence kinds；
- target refs；
- evidence/artifact refs；
- repair disposition；
- terminal failure class/signature；
- definition revision；
- predecessor run/result lineage。

新的显式 activation：

- 只从 unresolved structured gap 编译 focus；
- accepted predecessor evidence 只作为 historical lineage，不直接伪装为 current-run evidence；
- definition 未变化时保留 unresolved coverage identity；
- definition 变化时先做 baseline diff，不直接复用旧 actionable gap；
- Run owner 同步持久化 `PredecessorRunID`，必要时回写 predecessor 的 successor ref；
- 继续保持“无自动 successor lifecycle”。

### INV-09 Recoverable Projection Health

projection state 必须由当前 stream generation 和当前 workspace entry 计算：

- 累计 failure counter 只用于 metrics，不决定当前 ready/degraded；
- deleted/retired workspace 的历史失败不得污染当前 generation；
- successful full rebuild 或所有 current workspace rebuild success 后可以恢复 ready；
- degraded 响应必须暴露 current failed workspace IDs、error、generation、since；
- projection failure 不得改变 Runtime V2 aggregate、Run terminal 或 Result authority。

### INV-10 Observable Positive Progress

正向推进不能只用 `Run.state=active` 判断，至少需要可投影：

- state revision 单调增长；
- current/remaining/closed coverage item count；
- accepted/rejected evidence count；
- repeated failure signature count；
- current action 与 assessment disposition；
- repair attempt/budget；
- last progress timestamp；
- next action / next activation requirement；
- sealed provider target 与 configuration revision。

## Module Landing Matrix

本计划不新增 framework module。只在现有 owner 或 focused package 内扩展：

| 职责 | 正式落点 | 不允许的落点 |
| --- | --- | --- |
| operation descriptor、focus/assessment/gap shared types | `internal/pkg/workorchruntimecontract` | application 私有 DTO、LLM block 私有 map |
| Goal/DoD/profile -> focus queue 编译 | `internal/modules/application/workorchd/biz/appservice/activationcore` 或其 focused package | kernel reducer 解析 software-dev/profile ID |
| continuation/result -> activation context | `internal/modules/application/workorchd/biz/appservice/activationports` | TUI/HTTP 自行拼 continuation |
| RuntimeFacts/reducer/repair/terminal | `internal/modules/kernel/orchestrator/biz/runtimev2` | application queue、projection watcher |
| drive/prerequisite/candidate/operation dispatch | `internal/modules/kernel/orchestrator/runtime_v2_*` | 通用 adapter registry |
| capability execution | 现有 `internal/modules/blocks/{file,git,shell,mcp,...}` | Orchestrator 直接执行 shell/file |
| formal aggregate/lineage persistence | `internal/modules/kernel/run` | Orchestrator repository 直写 |
| formal outcome persistence | `internal/modules/blocks/result` | Result block 自行 assessment |
| provider-facing candidate schema/rendering | `internal/modules/blocks/llm` | kernel 内 provider/model 特化 |
| workspace/read-model projection | `internal/modules/application/workorchd/biz/appservice/{readmodelprojection,workspaceviewcore}` | TUI 临时补算正式状态 |
| TUI/Web presentation | `internal/services/workorch/service/tui`、`web/workorch-console`，经 port/client | 直接 import kernel owner service/repository |

## Implementation Contract

本节是 RVC-02～18 的执行级约束，用于消除实现过程中的临场设计。它从属于核心四状态机规约，不新增并列 authority。以下 public type、字段、默认值、兼容行为和 owner 落点在实现前不得自行改义；如果实现证据证明需要调整，必须先更新本节和核心规约，再修改代码。

### Persisted Schema And Compatibility

Runtime V2 保持 `RuntimeContractVersionV2 = "v2"`，该值继续只表示 Runtime V2 审计版本，不作为 V1/V2 路由开关。持久化结构另外增加：

```text
RuntimeStateRecord.SchemaVersion uint32

RuntimeStateSchemaVersionLegacy = 0
RuntimeStateSchemaVersionCurrent = 1
```

本计划新增的 `RuntimeBootstrap.SchemaVersion` 和 `RuntimeResultCloseout.SchemaVersion` 同样写 `RuntimeStateSchemaVersionCurrent`；schema 版本表示本组 Runtime V2 persisted contract，不各自创建无协调的版本号。

兼容规则：

| 持久化对象 | legacy 读取规则 | current 写入规则 | 禁止行为 |
| --- | --- | --- | --- |
| active Run `RuntimeStateRecord` | `SchemaVersion=0` 时在 `runtimeAggregateFromRecord` 入口执行纯内存 normalization；成功后继续当前 Run | 下一次成功 Run owner CAS 时写 `SchemaVersion=1` 和完整新字段 | 启动时批量改写全部 Run；因缺新字段从第一个 focus 重跑 |
| terminal Run | 保持可读；缺失 closeout/ledger 字段按 `unknown_legacy` 展示 | 不为迁移目的改写 terminal Run | 重新打开、重算或改变 accepted/rejected authority |
| `RuntimeBootstrap` | 缺 slices 时按原始 `Focuses` 稳定构造一 focus 一 slice；缺 convergence policy 时使用 contract neutral defaults `10/1/1`，不得读取当前系统配置 | 新 activation 必须写完整 slices、coverage items、policy 和 state-change requirement | 从当前系统配置覆盖已存在 Run 的封存 policy/provider target |
| legacy assessment | `Satisfied=true` 映射 `satisfied`；active Run 中 `false` 映射 `repairable_gap` + `legacy_unsatisfied_assessment`；terminal Run 只读 | 新 assessment 必须写 disposition、item facts 和 gap | 把 active legacy `Satisfied=false` 直接解释为不可恢复 terminal |
| state-change ledger | 从与当前 definition/coverage 绑定的 accepted `state_change` evidence 保守重建；无直接证据则为 unsatisfied | 后续由 reducer 随 accepted evidence 原子更新 | 根据文件存在、artifact summary 或历史 predecessor evidence 推断已满足 |
| repair ledger | legacy active Run 从 0 开始，保留已有 Failure 作为首个 signature diagnostic，但不回算历史次数 | 每个 repair cycle 通过 Run CAS 只计数一次 | 因 restart、duplicate event 或 projection retry 重复计数 |
| Result runtime closeout | 缺失时保留现有 Result 内容并标记 `unknown_legacy` | 新 rejected Runtime V2 Result 必须写 typed closeout | 从 summary 文本反向猜测 satisfied/unsatisfied coverage |
| Run lineage | legacy 空值保持空；新的显式 successor activation 从 continuation 写入 | 创建 successor Run 时原子写 predecessor IDs，再以 typed Run command 回写 predecessor successor | application/repository 直写；自动创建 successor |

Normalization 必须满足：

- 幂等：同一 record 重复 normalization 得到相同结果；
- 保守：无法证明 satisfied 的 coverage 一律保持 open；
- 无副作用：load/inspect 不写 repository；
- 可诊断：失败返回 `runtime_internal_contract_misalignment`，包含 workspace/run/schemaVersion；
- 可恢复：active record 可成功重建时继续当前 focus，不改变 sealed provider target、definition revision 或 stable prefix contract；
- file backend JSON 对新增字段采用 additive/omitempty 兼容；旧文件无需离线迁移；
- schema 高于当前支持版本时 fail-fast，禁止降级读取后覆盖原记录。

升级、restart 和 rollback 规则：

1. 部署前备份 repository 和 `.workorch-data`/file backend 数据；备份范围必须覆盖 Run、Result、event records 和 workspace projection source。
2. 新 binary 必须先通过 legacy schema load tests，随后才允许在 active Run 的下一次 CAS 中写 schema v1；不得以启动任务批量迁移。
3. 服务 restart 后由 Runtime V2 recovery scanner 读取同一 Run；active dispatch、candidate attempt、repair cycle 和 governance wait 分别按 SM-3/SM-4 恢复，禁止创建替代 Run。
4. schema v1 尚未写入任何 active Run 时，可以直接回滚 binary；一旦产生 schema v1 state mutation，旧 binary 不再被视为安全 reader/writer。
5. schema v1 写入后的 rollback 只能选择恢复部署前 repository 备份，或部署支持 schema v1 的 forward-fix binary；禁止用旧 binary 忽略新字段后继续写入。
6. Result/Run 的 additive HTTP 字段不要求客户端锁步升级，但 server、embedded backend 和 remote backend 必须同版本发布。
7. projection store 是可重建数据，不参与 repository rollback；binary rollback/restart 后必须从 owner/event source 完整 rebuild 并重新计算 current generation health。

### Typed Contract Delta

以下名称和字段为实现基线。为保持 shared contract 中立，所有类型放在 `internal/pkg/workorchruntimecontract`；Result/Run model 只引用这些类型或保存 owner-specific lineage，不复制同义 DTO。

#### Operation Descriptor

```text
RuntimeOperationEffectClass
  non_mutating | workspace_state | remote_state | unknown

RuntimeOperationDescriptor
  Kind             string
  OwnerModuleID    string
  EffectClass      RuntimeOperationEffectClass
  AllowedActions   []RuntimeAction
  CapabilityClass  RuntimeCapabilityClass
```

必须提供唯一查询 API：

```text
RuntimeOperationDescriptors() []RuntimeOperationDescriptor
RuntimeOperationDescriptorForKind(kind string) (RuntimeOperationDescriptor, bool)
FeasibleOperationKinds(required RequiredOutputContract,
                       capabilities RuntimeCapabilities) []string
```

descriptor 返回值必须 normalized、稳定排序且不可由调用方修改全局状态。未知 kind 返回 `ok=false`，effect 按 `unknown` 保守拒绝。

#### Bootstrap, Slice And Coverage

`RuntimeExecutionContract` 增加 typed policy，activation compiler 不再把这两类规则只留在 raw context map：

```text
AcceptanceSlicePolicy *RuntimeAcceptanceSlicePolicy
ValidationPolicies    []RuntimeValidationPolicy

RuntimeAcceptanceSlicePolicy
  SchemaVersion       string
  MaxWorkItems        int
  CompletionPolicy    string
  ResultPolicy        string
  Groups              []RuntimeAcceptanceSliceGroupPolicy

RuntimeAcceptanceSliceGroupPolicy
  ID                  string
  Title               string
  Scope               string
  Priority            int
  MaxWorkItems        int
  Match               RuntimeAcceptanceSliceMatch

RuntimeAcceptanceSliceMatch
  EvidenceRoles        []string
  MissingEvidenceKinds []string
  SourceRequirementIDs []string
  SourceScopes         []string
  All                  bool

RuntimeValidationPolicy
  ID                               string
  Scope                            string
  AppliesToEvidenceKinds           []string
  ValidationFailureImpliesStateChange bool
  ImplementationGapDiagnosticClasses []string
  MaxValidationOnlyRetriesPerFocus int
```

当前 `workorch.acceptance_slice_policy.v1` raw template 在 application boundary 转换为以上 typed policy；raw map 只可作为输入兼容层，不能进入 kernel 或作为 restart authority。

本次 closure 支持的 policy 值固定为：`SchemaVersion=workorch.acceptance_slice_policy.v1`、`CompletionPolicy=all_required_items_accepted`、`ResultPolicy=slice_result_allowed`。后两者缺值分别使用上述值；schema version 缺失只允许 legacy normalization，新的 activation 必须显式为 v1。出现其他非空值必须 activation fail-fast，不能在 kernel 中猜测语义。

`RuntimeProfileEvidenceRequirement` 增加 `RequiredEvidenceKinds []string`。其 `ID` 只允许具体 coverage source ID 或 `"*"`；空 ID 非法。compiler 的 scope default 是代码中立默认，不通过伪造另一条 profile requirement 表达：Goal/DoD/acceptance coverage 默认 `RequireAssessment=true`，不凭空增加业务 evidence kind；完全未知 scope 使用同一 neutral assessment default。显式 exact/wildcard policy 可以增加 required evidence、accepted/rejected kinds、operation 和 direct/claim/executable 约束。

`RuntimeBootstrap` 增加：

```text
SchemaVersion          uint32
Slices                 []RuntimeAcceptanceSlice
CoverageItems          []RuntimeCoverageItem
ConvergencePolicy      RuntimeConvergencePolicy
StateChangeRequirement RuntimeRunStateChangeRequirement
```

新增类型：

```text
RuntimeAcceptanceSlice
  ID                    string
  Title                 string
  Scope                 string
  Priority              int
  CoverageWorkItemIDs   []common.CoverageWorkItemID
  RequiredEvidenceKinds []string
  CompletionPolicy      string
  ResultPolicy          string

RuntimeCoverageItem
  ID                    common.CoverageWorkItemID
  SliceID               string
  SourceScope           string
  SourceID              string
  Order                 int

RuntimeCoverageItemFact
  ID                    common.CoverageWorkItemID
  SliceID               string
  State                 RuntimeCoverageItemState
  EvidenceRefs          []string
  LastAssessmentID      string

RuntimeCoverageItemState
  open | satisfied | unsatisfied

RuntimeRunStateChangeRequirement
  Required              bool
  CoverageWorkItemIDs   []common.CoverageWorkItemID
```

`CoverageFocus` 增加 `SliceID string`。一个 current focus 对应一个 slice，可包含多个 coverage item；不得再次假设 `len(CoverageWorkItemIDs)==1`。

#### Facts, Assessment And Repair

`RuntimeFacts` 增加：

```text
Slices       []RuntimeAcceptanceSlice
Coverage     []RuntimeCoverageItemFact
StateChange  RuntimeStateChangeLedger
Repair       RuntimeRepairLedger
Feasibility *RuntimeFeasibilityFact
```

新增或扩展类型：

```text
RuntimeStateChangeLedger
  Required              bool
  Satisfied             bool
  CoverageWorkItemIDs   []common.CoverageWorkItemID
  EvidenceRefs          []string
  LastEvidenceRevision  string

AssessmentDisposition
  satisfied | repairable_gap | governance_required | terminal_rejection

RuntimeAssessmentItemFact
  CoverageWorkItemID    common.CoverageWorkItemID
  Satisfied             bool
  Reason                string
  EvidenceRefs          []string
  ArtifactRefs          []common.ArtifactID

RuntimeAssessmentGap
  CoverageWorkItemIDs   []common.CoverageWorkItemID
  MissingEvidenceKinds  []string
  TargetRefs            []string
  EvidenceRefs          []string
  Reason                string
  FailureClass          FailureClass
  FailureCode           string
  FailureSignature      string

RuntimeAssessmentFact
  ID                    string
  FocusID               string
  SliceID               string
  Disposition           AssessmentDisposition
  Items                 []RuntimeAssessmentItemFact
  Gap                   *RuntimeAssessmentGap
  Summary               string
  EvidenceRefs          []string
  ArtifactRefs          []common.ArtifactID

RuntimeRepairLedger
  FocusID               string
  SliceID               string
  ActiveGap              *RuntimeAssessmentGap
  RepairAttempts        int
  ValidationOnlyRetries int
  NoProgressCount       int
  SignatureCounts       []RuntimeFailureSignatureCount
  LastCycleID           string
  LastProgressRevision  string
  LastProgressAt        string
  Exhausted             bool

RuntimeFailureSignatureCount
  Signature             string
  Count                 int

RuntimeFeasibilityFact
  FocusID               string
  Action                RuntimeAction
  RequiredOperationKinds []string
  FeasibleOperationKinds []string
  MissingCapabilityKinds []string
  Reason                string
```

`RuntimeAssessmentFact.Satisfied` 在 schema v1 中停止作为 authority 字段：为兼容旧 reader 可以暂时保留并由 `Disposition==satisfied` 派生，但 reducer、Result 和 projection 不得再读取它作裁决。完成 RVC-15 后再决定后续 schema 是否移除。

#### Convergence Policy And Defaults

```text
RuntimeConvergencePolicy
  MaxRepairAttemptsPerFocus       int
  MaxValidationOnlyRetriesPerFocus int
  MaxAcceptanceSliceWorkItems     int
```

正式来源和默认值：

| 字段 | 配置来源 | neutral 默认 | 限制 |
| --- | --- | --- | --- |
| `MaxRepairAttemptsPerFocus` | activation 时读取 `runtime.maxRuntimeRepairAttempts` 并封存 | `10` | `1..100` |
| `MaxValidationOnlyRetriesPerFocus` | matched typed validation policy 的 `maxValidationOnlyRetriesPerFocus` | `1`；software-dev 当前声明 `2` | `0..10` |
| `MaxAcceptanceSliceWorkItems` | `acceptanceSlicePolicy.maxWorkItems` | `1`；software-dev 当前声明 `4` | `1..32` |

group-level `maxWorkItems` 可以缩小全局 slice 上限，不得超过 `MaxAcceptanceSliceWorkItems`。非法值必须在 activation compiler 阶段返回 typed invalid-contract error，不得静默截断。policy 一旦进入 `RuntimeBootstrap`，restart/resume 必须继续使用封存值；系统配置变化只影响新的显式 activation。

candidate attempt 固定为 `0/1`，不进入 `RuntimeConvergencePolicy`。transition guard 只防 reducer/controller 内部失控，不是 repair budget，也不得通过提高 guard 绕过 budget。

#### Result Closeout And Lineage

新增 shared closeout：

```text
RuntimeResultCloseout
  SchemaVersion              uint32
  DefinitionRevision         string
  SatisfiedCoverageWorkItemIDs   []common.CoverageWorkItemID
  UnsatisfiedCoverageWorkItemIDs []common.CoverageWorkItemID
  Gaps                       []RuntimeAssessmentGap
  MissingEvidenceKinds       []string
  TargetRefs                 []string
  EvidenceRefs               []string
  ArtifactRefs               []common.ArtifactID
  Disposition                AssessmentDisposition
  FailureClass               FailureClass
  FailureCode                string
  FailureSignature           string
  PredecessorRunID           common.RunID
  PredecessorResultID        common.ResultID
```

`blocks/result/pkg/models.Result` 增加 `RuntimeCloseout *RuntimeResultCloseout`。`RunLineage` 增加 `PredecessorResultID`；已有 `PredecessorRunID`、`SuccessorRunID` 继续保留。

现有 `RuntimeAssessmentFact` 的 `Scope`、`RequirementID`、`RequirementText` 和 `CoverageWorkItemIDs` 在 schema v1 中继续保留用于审计/兼容；新增字段不得删除这些信息。新 assessment 的 `ID` 使用 sealed assessment dispatch ID，legacy normalization 使用稳定的 `legacy-assessment:<stateRevision>:<ordinal>`。

continuation additive delta：

```text
RuntimeRejectedResultContinuation
  DefinitionRevision string
  Disposition        AssessmentDisposition
  Failure            *RuntimeContinuationFailure

RuntimeContinuationItem
  CoverageWorkItemID common.CoverageWorkItemID

RuntimeContinuationCoverageGap
  CoverageWorkItemIDs  []common.CoverageWorkItemID
  MissingEvidenceKinds []string
  TargetRefs           []string
  FailureClass         FailureClass
  FailureCode          string
  FailureSignature     string

RuntimeContinuationFailure
  Code                 string
```

Result continuation 一一映射：

| Runtime closeout | `RuntimeRejectedResultContinuation` |
| --- | --- |
| satisfied IDs | `SatisfiedDoDItems`，每项保留 coverage ID/state/evidence refs |
| unsatisfied IDs | `UnsatisfiedDoDItems` |
| `Gaps` | `CoverageGaps`，不得降级为 summary-only |
| `MissingEvidenceKinds` | `RequiredEvidence` |
| evidence/artifact refs | 同名字段 |
| failure class/code/signature | 扩展 continuation 的 typed failure 字段 |
| predecessor run/result | 现有 `PredecessorRunID`/`ResultID` |

### Coverage Focus Compiler Contract

统一入口定义为：

```text
CompileCoverageFocuses(input CoverageFocusCompilerInput) (CompiledCoverage, error)

CoverageFocusCompilerInput
  Definition            RuntimeDefinitionContext
  ExecutionContract     RuntimeExecutionContract
  AcceptanceSlicePolicy typed policy
  Continuation          RuntimeContinuationContext
  ConvergencePolicy     RuntimeConvergencePolicy

CompiledCoverage
  Focuses               []CoverageFocus
  Slices                []RuntimeAcceptanceSlice
  CoverageItems         []RuntimeCoverageItem
  Requirements          []RuntimeRequirement
  StateChangeRequirement RuntimeRunStateChangeRequirement
```

编译算法必须确定性执行：

1. 按 definition 中 Goal/DoD 原始顺序生成 coverage items；ID 不依赖 slice 大小。
2. continuation 存在且 definition revision 未变化时，只保留 unresolved coverage IDs；已满足项仅作为 historical lineage。
3. definition revision 变化时先执行 baseline diff；无法安全映射时生成一个 `continuation_gap_analysis` slice，而不是回到第一 DoD。
4. evidence policy 依次按 exact `(scope,id)`、wildcard `(scope,*)`、scope default、neutral default 合并。`RequiredEvidenceKinds`、`RejectedStepKinds` 取稳定去重并集；direct/claim/executable/coverage-target 布尔约束取 OR，不能被较高优先级放松；`EvidenceRole`、`AcceptedStepKinds`、`AcceptedOperations` 取最高优先级的第一个非空值；description 仅用于审计，不进入 reducer authority。
5. slice group 按 priority 升序、group declaration order、coverage item order稳定匹配；match 中同一数组内为 OR，不同非空字段之间为 AND；`All=true` 只能作为单独兜底条件。每个 item 只进入第一个 matched group；最终 `all=true` group 接收剩余项。
6. 每个 slice 不超过 effective `maxWorkItems`；超出时按原始顺序分片，ID 固定使用 `fmt.Sprintf("%s-%03d", groupID, ordinal)`，ordinal 从 1 开始且即使只有一片也保留后缀；不得使用随机数或内容摘要。
7. slice 内 required evidence 是未关闭 item requirements 的稳定并集；accepted/rejected kinds 和 operations 继续保留 item-level policy，用于 assessment 与 evidence gate。
8. `must_produce_state_change` 只生成 Run-level requirement；只有显式 evidence role/deliverable policy 命中的 slice 才直接要求 focus-level state change。
9. 输出重复 ID、空 coverage、未知 evidence/operation kind 或无兜底 slice 时 activation 失败。

`continuation_gap_analysis` 使用现有 `RuntimeActionProduceAssessment`，其 focus 设置 `RequireAssessment=true`，不新增 RuntimeAction。assessment 必须输出可映射的 structured gap；仍无法映射时才允许 `governance_required` 或 typed terminal。

### Feasibility Gate And Dispatch Contract

feasibility gate 位于 `DriveRuntimeV2` 中：在 reducer 已产生 decision、prerequisite 已满足之后，在 `BeginCandidateAttempt` 和 provider request rendering 之前执行。

```text
load aggregate
  -> reduce decision
  -> satisfy prerequisite
  -> resolve feasible operation kinds
       -> none: RecordFeasibilityFact(CAS) -> reduce again
       -> non-empty: BeginCandidateAttempt -> render/request candidate
```

`RecordFeasibilityFact` 必须是 `RuntimeController` 的 typed mutation，通过同一 Run owner CAS 持久化到 `RuntimeFacts.Feasibility`。无 feasible kind：

- 不调用 provider；
- 不创建 candidate attempt；
- 不消耗 repair attempt；
- reducer 根据 missing capability/authority 进入 prerequisite、governance 或 bounded terminal；
- unknown descriptor 始终按不可执行处理。

`validation/managed_session` descriptor 固定为 Shell owner、`non_mutating`、`produce_validation`、validation capability class。operation dispatch destination、side-effect gate、profile requirement compilation 和 verifier inventory 必须全部读取同一 descriptor API。

### Reducer Transition And Repair Rules

当前 focus 的 canonical 决策顺序：

| 条件 | action | 状态更新 |
| --- | --- | --- |
| prerequisite/feasibility facts 不完整 | `acquire_prerequisite` | 保持 current focus |
| Run/focus 明确缺 state-change | `produce_state_change` | 打开或继续同 focus repair |
| state-change 已满足但缺 validation | `produce_validation` | 保持 current focus |
| 当前 open items 的 required evidence 齐备但缺 assessment | `produce_assessment` | 保持 current focus |
| `satisfied` 且 slice 全部 item satisfied | advance/`complete` | 原子关闭 slice；切换下一 focus 或完成 Run |
| partial assessment：部分 satisfied、部分有 repairable gap | `produce_state_change` 或 `produce_validation` | 先永久关闭 satisfied items；同 slice 只保留 unresolved items |
| `repairable_gap` implementation gap | `produce_state_change` | 写 active gap 和 repair cycle |
| `repairable_gap` evidence gap | `produce_validation` | 写 active gap 和 repair cycle |
| `governance_required` | `request_governance` | 打开 typed decision/approval |
| `terminal_rejection` | `fail` | 生成 structured rejected Result |
| repair budget exhausted | `fail` | 当前 active gap 转为 `no_progress_exhausted` closeout |

约束：

- assessment candidate 只提交 item facts、disposition 和 gap；Orchestrator 验证其 coverage IDs 必须属于 current slice，并独立决定 terminal；
- `satisfied` 必须覆盖 current slice 全部 remaining item，否则 normalization 为 partial `repairable_gap`；
- `repairable_gap` 必须至少包含 coverage ID，以及 missing evidence、target 或明确 reason 之一；
- implementation gap 与 evidence gap 的分类只依赖 typed failure/evidence policy，不解析业务标题；
- 新 state-change evidence 只重新打开与其 coverage IDs 相交的 validation；
- predecessor evidence、unbound evidence、相同 content hash 的重复弱 evidence 不得关闭 current gap；
- focus advance、slice close、coverage close、state-change ledger、repair ledger 和 authority decision 必须在一次 RuntimeStateRecord CAS 中提交；
- duplicate/stale operation result、assessment result 或 governance resolution必须幂等忽略，不得重复关闭 item 或增加 budget；
- restart 从 persisted current focus、gap、cycle 和 decision 继续，不重新请求已 sealed/accepted 的相同 transaction。

不可恢复 terminal 仅包括：明确 governance rejection、不可读取的 future schema、无法满足且不允许治理的 authority/capability contract、不可修复的 internal contract corruption，以及正式 budget exhaustion。普通 validation failure、负向 assessment、缺文件、缺测试目标或 candidate feasibility gap 默认不属于 irrecoverable。

### Repair Budget And Failure Signature

repair cycle ID 和 failure signature 必须稳定：

```text
failureSignature = sha256(
  focusID + disposition + failureClass + failureCode +
  sortedUnique(missingEvidenceKinds) + sortedUnique(targetRefs)
)

repairCycleID = sha256(
  runID + focusID + failureSignature + sourceFactID
)
```

`sourceFactID` 优先使用 assessment dispatch ID；operation result 或 governance fact 触发 repair 时使用其 processed event ID。message、时间戳、provider request ID、evidence record ID 不进入 failure signature。相同 assessment/event replay 得到相同 cycle ID。

计数规则：

- reducer 首次接受新的 repair cycle 时 `RepairAttempts++`；duplicate cycle 不增加；
- `ValidationOnlyRetries` 只在没有新 accepted state-change 且再次进入 validation repair 时增加；
- `NoProgressCount` 在 repair cycle 未关闭 item、未增加满足 required kind 的 accepted evidence、未改变 active gap signature 时增加；
- state revision 增长、重新渲染 prompt、重复 file/read、projection 更新不算 progress；
- 有效 evidence、关闭 coverage item、active gap signature 改善或 focus advance时重置 `NoProgressCount`；
- `RepairAttempts` 在同 focus 内不重置，focus/slice 关闭后新 focus 从 0 开始；
- 接受新 cycle 前若 `RepairAttempts >= MaxRepairAttemptsPerFocus`，不得再进入 repair，直接 budget exhausted；因此默认值 `10` 允许最多十个 repair cycle；
- 再次选择 validation-only repair 前若 `ValidationOnlyRetries >= MaxValidationOnlyRetriesPerFocus`，reducer 必须转 state-change repair、governance 或 terminal；
- candidate attempt 失败只更新 `CandidateAttempts`，不得增加 repair ledger；可行性失败两者都不增加。

### Run Lineage Transaction

`CreateRunCommand` 增加 `Lineage RunLineage`，successor Run 创建时必须在首次写入中保存 `PredecessorRunID` 和 `PredecessorResultID`。

Run owner 增加 typed command：

```text
LinkRunSuccessorCommand
  WorkspaceID           WorkspaceID
  PredecessorRunID      RunID
  SuccessorRunID        RunID
  ExpectedSuccessorRunID RunID

Topic: /run/command/link-successor
```

语义：空 expected 表示仅允许从空 successor 建链；重复提交相同 successor 幂等成功；不同 successor 返回 conflict。application 只能在显式 activation 成功创建 successor Run 后调用该 command。失败时 successor Run 保留自身 predecessor lineage并返回可诊断错误，不删除或自动重建 Run。

所有 Result/Run HTTP、client、port 读取必须携带 workspace/run scope；现有只按 result ID/run ID 的 owner command 在本次收口中补足 scope，或只允许能证明全局唯一的内部调用。

### API, Port And Projection Delta

本次不新增第二套进度 endpoint。正式读取继续通过现有 Run status、Workspace RuntimeView 和 Result API，字段做 additive 扩展：

| surface | required delta | JSON 字段 |
| --- | --- | --- |
| `workorchport.RunInfo` / client Run | lineage | `predecessorRunId`, `predecessorResultId`, `successorRunId` |
| `ConvergenceStatusInfo` | coverage/slice/assessment/repair | `currentSliceId`, `closedCoverageItemCount`, `remainingCoverageItemCount`, `assessmentDisposition`, `repairAttempts`, `maxRepairAttempts`, `repeatedFailureSignatureCount`, `lastProgressAt`, `nextActivationRequired` |
| `AcceptanceProjectionInfo` | 使用 RuntimeStateRecord typed facts 投影，不再从旧 step diagnostics 推断 | 保持现有 `activeSliceId`, `slices`, `currentFocus`, `budget`, `summary` |
| `ProjectionInfo` | generation health | `streamGeneration`, `failedWorkspaceIds`, `degradedSince`；保留现有 revision/error 字段 |
| Result DTO | typed runtime closeout | `runtimeCloseout` |
| Workspace RuntimeView | 同一 snapshot 内一致 | 复用 `projection`, `revision`, `convergenceStatus`, `acceptance`, `outcome` |

正式 endpoint 落点：

- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/status`：轻量 convergence/repair 状态；
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/snapshot`：完整 Run snapshot、lineage、usage 和 runtime closeout引用；
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/results`：workspace/run scoped Result closeout；
- `GET /api/v1/workorch/workspaces/:workspaceId/runtime-view`：TUI/Web 当前 projection authority；
- `GET /readyz`：全局 projection current-generation readiness，不返回历史 metrics 结论。

兼容要求：

- 新字段均 `omitempty`；旧 client 可以忽略，新 client 对字段缺失按 legacy unknown 展示；
- embedded 和 remote backend 必须映射相同 port DTO；
- TUI/Web 不根据 summary、step kind 或 event topic重新推导 action/disposition；
- HTTP server、`workorchclient`、`workorchport`、remote backend 和 Web TypeScript type 必须在同一 RVC 提交中同步；
- provider target 继续显示 activation sealed value及 configuration revision，不从当前系统默认值回填 active Run。

### Projection Health Algorithm

`workspaceviewcore.Store` 在当前 `StreamGeneration` 内维护 current failure set，metrics counter 不参与状态计算：

```text
globalFailure *ProjectionFailure
workspaceFailures map[WorkspaceID]ProjectionFailure

ProjectionFailure
  Generation string
  Error      string
  Since      time
```

状态优先级：

```text
stopped/failed/building lifecycle
  > degraded: current generation global/workspace failure set 非空
  > stale: current source revision > projected revision 或 current stale set 非空
  > ready: initialized 且 current failure/stale set 均为空并 revision aligned
```

提交规则：

- successful workspace rebuild 删除该 workspace 的 current failure；
- successful full rebuild 原子替换 workspace entries，并清空 current global/workspace failure set；
- workspace list 已删除/retired 的 ID 同步从 failure/stale set 删除；
- failed rebuild 只写 current generation failure 和 metrics，不能修改 Runtime aggregate；
- full rebuild 的 global loader failure 写 `globalFailure`；单 workspace failure只写对应 workspace ID；
- rebuild 开始时捕获 entry/source revision；commit 时若发现更高 revision，丢弃 stale result并重新调度，不覆盖新 snapshot；
- `ReplaceAll` 与 workspace rebuild commit 在 Store lock 内原子更新 health、entries 和 projected revision；
- degraded API 返回当前 failed workspace IDs、最新 error、最早 since 和 generation；历史累计 failure total 仅出现在 metrics；
- process restart 创建新 generation，旧 generation failure 不复制到新 Store。

### Verification Fixture Specification

新增正式命令和脚本：

```text
make verify-runtime-v2-positive-convergence-file
  -> scripts/verify-runtime-v2-positive-convergence-file.sh

make verify-runtime-v2-positive-convergence-live
  -> scripts/verify-runtime-v2-positive-convergence-live.sh
```

file-backed verifier 使用 `internal/modules/application/workorchd/biz/testkit` 的 deterministic provider，不依赖公网模型。fixture 必须按 `(workspace fixture, run, focus, action, attempt)` 返回确定响应，至少包含：

| sequence | response |
| --- | --- |
| candidate attempt 0 | schema/operation contract failure |
| candidate attempt 1 | corrected sealed operation |
| first assessment | partial satisfied + repairable implementation gap |
| repair state-change | accepted coverage-bound state-change evidence |
| validation | managed executable evidence |
| second assessment | remaining items satisfied |
| exhaustion fixture | 相同 gap 的不同 cycle，直到 repair budget exhausted |

两条 verifier 通用要求：

- poll interval `1s`，单 Run 默认 timeout `180s`；分别由 `WORKORCH_E2E_POLL_INTERVAL_SECONDS`、`WORKORCH_E2E_TIMEOUT_SECONDS` 覆盖；
- 每次轮询记录 workspace ID、run ID、state revision、focus/slice、action、disposition、repair count 和 projection generation；
- 成功必须观察到至少一次 state revision 增长、一个 coverage item close、一次同 focus repair，以及最终 focus advance或 structured terminal fixture；
- restart case 在 persisted gap 后重启服务，恢复后 run ID、focus ID、repair count、provider target 保持不变；
- 失败产物写到 `.workorch-e2e/runtime-v2-positive-convergence/<timestamp>/`，可由 `WORKORCH_E2E_ARTIFACT_DIR` 覆盖；该目录不提交；包含响应快照、event trace、projection status 和服务日志尾部；
- verifier 创建带唯一前缀的 workspace，结束后删除仅由本次脚本创建的资源；失败时默认保留 workspace 并打印 cleanup 命令；
- `WORKORCH_E2E_KEEP_WORKSPACE=1` 时成功后也保留 fixture；
- live verifier 从 `WORKORCH_BASE_URL` 读取实例地址，只读系统 provider catalog并显式选择目标；可用 `WORKORCH_E2E_PROVIDER_ID`、`WORKORCH_E2E_MODEL_ID` 固定目标，不修改系统默认 provider/model；
- stable prefix assertion 比较同 authority transaction attempt 0/1 的 `stablePrefixHash`，不同 transaction 不要求相等。

## Work Breakdown

### Phase A — Contract And Inventory

#### RVC-01 Core Contract Update

Status: completed

更新：

- `docs/00-core/runtime-stage-contract.md`
- `docs/20-runtime-flow/activation-definition-flow.md`
- `docs/20-runtime-flow/planning-execution-flow.md`
- `docs/20-runtime-flow/verification-acceptance-flow.md`
- `docs/20-runtime-flow/repair-replan-flow.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `docs/60-verification/core-runtime-verifier.md`

必须明确：

- repairable assessment 不直接 terminal；
- Run 级与 focus 级 state-change requirement 分离；
- operation feasibility gate；
- acceptance slice 和 item-level partial assessment；
- structured rejected result/continuation；
- 无自动 successor 的边界保持不变。

完成证据：`runtime-stage-contract.md` 已固化 SM-1～SM-4、协调协议与变更门禁；`AGENTS.md` 已把四状态机检查设为所有 Runtime V2 变更的强制 agent 约束；runtime flow、Orchestrator readme、current-state 与 core verifier 已同步引用同一规约。

#### RVC-02 Runtime Operation Descriptor

Status: completed

在 `internal/pkg/workorchruntimecontract` 建立唯一 descriptor authority，并迁移：

- `RuntimeModuleForOperationKind`；
- `OperationHasWorkspaceSideEffectsWithCapabilities`；
- profile capability requirement compilation；
- Runtime V2 operation dispatch destination；
- operation kind contract tests。

`validation/managed_session` 必须正式映射到 Shell owner，并声明 non-mutating managed validation effect。

完成证据：所有 canonical operation kind 都能解析 descriptor；未知 kind 保守拒绝。

#### RVC-03 Capability Inventory Conformance

Status: completed

建立测试和启动期/activation 期校验：

- profile accepted kind 必须存在 descriptor；
- descriptor owner 必须能通过 EventHub 返回对应 capability；
- `CapabilityContract`、`RuntimeRequirement`、`RuntimeCapabilities` 三者一致；
- managed validation 出现在 software-dev Run 的 `ValidationKinds`；
- owner capability 合并不得互相覆盖。

### Phase B — Focus Compilation

#### RVC-04 Evidence Policy Compiler

Status: completed

实现统一 policy precedence：exact > wildcard > scope default > runtime default。

重点回归：

- `scope=dodItem,id=*` 作用到普通 DoD focus；
- accepted/rejected step kinds 保真；
- accepted operations 保真；
- executable/direct/claim evidence 规则进入 typed focus；
- profile policy 只作为中立 typed 输入，kernel 不识别 profile ID。

#### RVC-05 Run-Level State Change Ledger

Status: completed

调整 `RuntimeBootstrap` / `RuntimeFacts`：

- Run 级记录是否已产生符合 coverage 的 primary state-change；
- DoD focus 不再因全局 policy 自动要求 state_change；
- assessment/validation 识别实现缺口后，可以将当前 focus 提升为 state-change repair；
- deliverable/source-change focus 继续显式要求 state_change；
- 最终 completion gate 校验 Run 级 state-change policy。

#### RVC-06 Acceptance Slice Compiler

Status: completed

落实现有 acceptance slice policy：

- bounded `maxWorkItems`；
- stable ordering；
- item-level coverage identity；
- slice-level current focus；
- partial assessment；
- accepted item lineage 保留；
- remaining queue 只包含未关闭 item。

大 DoD fixture 必须证明 focus 数量按 policy 收敛，而不是 1 item = 1 LLM assessment round。

### Phase C — Reducer And Repair

#### RVC-07 Structured Assessment Gap

Status: completed

扩展 shared contract、LLM schema、candidate parser 和 assessment ledger：

- disposition；
- unsatisfied coverage work item IDs；
- missing evidence kinds；
- target refs；
- reason/evidence refs；
- partial item assessment。

LLM 只描述 assessment facts；terminal 仍由 Orchestrator 决定。

#### RVC-08 Same-Focus Repair Loop

Status: completed

reducer 必须实现：

```text
repairable implementation gap -> produce_state_change
repairable evidence gap       -> produce_validation
missing capability/authority  -> request_governance or typed terminal
satisfied slice               -> advance focus
budget exhausted              -> rejected result
```

要求：

- 新 state-change evidence 可以重新打开 validation；
- 新 validation evidence 可以重新进入 assessment；
- 相同弱证据不能重复关闭 gap；
- accepted progress 重置 no-progress 计数；
- restart 后从 persisted facts 继续同一 repair loop。

#### RVC-09 Retry And Repair Budget Separation

Status: completed

- candidate attempt 保持 0/1；
- feasibility failure不消耗 attempt；
- repair budget 按 focus/failure signature；
- repeated identical candidate failure 保留 structured diagnostic；
- repair budget、candidate attempts、no-progress count 分别投影；
- 不增加第三次 same-contract candidate request。

### Phase D — Result, Continuation And Lineage

#### RVC-10 Structured Rejected Result

Status: completed

Result owner 持久化 Orchestrator 已裁决的结构化 closeout：

- satisfied/unsatisfied items；
- coverage gaps；
- missing evidence；
- target refs；
- evidence/artifact refs；
- disposition；
- failure class/signature；
- definition revision。

禁止 Result owner 根据字段自行改变 accepted/rejected。

#### RVC-11 Successor Focus Rehydration

Status: completed

新的显式 activation：

- 从 latest rejected result 读取 structured gaps；
- 只构建 unresolved focus/slice；
- definition change 时先做 baseline diff；
- predecessor evidence 只作为 historical/ref-only context；
- continuation 缺少结构化 gap 时进入 neutral gap-analysis focus，不静默从第一 DoD 重跑。

#### RVC-12 Run Lineage Persistence

Status: completed

- 创建 Run 时写入 predecessor Run/result lineage；
- predecessor/successor 更新遵守 Run owner CAS；
- HTTP/client/read model 保留 workspace/run scope；
- 同 workspace 可重复 `run-001` 的读取仍必须 workspace-scoped；
- 不引入自动 successor lifecycle。

### Phase E — Projection And Presentation

#### RVC-13 Projection Generation Recovery

Status: completed

- 当前 generation 重建成功后恢复 ready；
- historical/deleted workspace failure 不影响当前 state；
- degraded 响应包含失败 workspace/error/since；
- full/workspace rebuild metrics 与 current health 分离；
- live verifier 能区分 aligned-but-degraded、stale、ready。

#### RVC-14 Runtime Usage And Terminal Projection

Status: completed

- `/run/llm_usage/recorded` 正确进入 Run snapshot summary；
- terminal Run 可靠释放 workspace activeRun；
- projection retry 不重写 terminal authority；
- snapshot、observability、workspace list 对 current focus/evidence/repair state 一致；
- projection source/projected revision 与 entry revision 可解释。

#### RVC-15 TUI/Web Positive Progress Presentation

Status: completed

通过现有 port/client 暴露并展示：

- closed/remaining work item count；
- current slice/focus；
- current action；
- assessment disposition；
- repair attempt/budget；
- latest accepted evidence；
- repeated failure；
- `nextActivationRequired` 与 structured gap summary。

TUI/Web 不自行推导 reducer action，不直接读取 owner repository。

### Phase F — Verification And Deployment

#### RVC-16 Contract And Unit Regression

Status: completed

RVC-02～15 的直接单元/合同测试必须随对应工作项提交。RVC-16 不承担“最后统一补测试”，只负责跨项 regression inventory audit、遗漏补齐和全量 gate 稳定化。

audit 至少确认：

- descriptor completeness/unknown kind tests；
- managed validation owner/capability tests；
- profile wildcard policy compiler tests；
- Run-level state-change tests；
- bounded slice/partial assessment tests；
- repairable assessment -> same-focus repair reducer tests；
- budget exhaustion -> rejected result tests；
- structured continuation/lineage tests；
- projection recovery/usage summary tests；
- restart recovery and duplicate event idempotency tests。

#### RVC-17 Deterministic E2E Fixtures

Status: completed

新增或扩展 deterministic provider fixtures，覆盖：

1. candidate contract 第一次错误、第二次修正；
2. assessment 返回 repairable implementation gap；
3. reducer 进入 state-change repair；
4. managed validation 产生 executable evidence；
5. partial slice assessment；
6. focus advance；
7. restart 后继续；
8. budget exhaustion 时 structured rejected result；
9. 新 activation 只承接 unresolved items；
10. projection 最终 ready。

#### RVC-18 Deployed Dual-Workspace Acceptance

Status: completed

必须使用最新部署实例完成两个真实 workspace 验收。

ai-proxy：

- `validation/managed_session` 出现在 sealed validation capability；
- `/healthz` executable validation 被接受；
- `focus-001`/slice 关闭并推进下一 focus；
- 不再出现 generic shell side-effect candidate loop；
- 重新启动后仍能继续 current focus。

LSP-Bridge：

- “只写工具文档 + file/read”不能关闭工具实现 claim；
- assessment gap 进入同 Run state-change repair；
- 实现和 executable validation evidence 产生后才能关闭 slice；
- 不因首次负向 assessment 直接 terminal；
- 后续 focus 按 unresolved queue 推进。

全局：

- projection state = ready；
- source/projected revision 对齐；
- llmUsageSummary 非零且与 usage events 一致；
- workspace activeRun 与 Run terminal 一致；
- provider target 仍为 activation 封存值；
- stable prefix 在 same-contract attempt 中不漂移。

**Deployed evidence (2026-07-17)**：最新 `./bin/workorch --server -config-dir config/file` + deterministic verify mock（`127.0.0.1:18188`，LLM config provider=`verify`/`verify-model`）。双 workspace resume activation HTTP 202：

| workspace | run | path | outcome |
| --- | --- | --- | --- |
| ai-proxy `ws-bea4fee67e459bcc` | `run-012` | validation(managed_session) → assessment(repairable_gap) → same-run state_change(patch/apply) → assessment(satisfied, evidenceRefs=current-run) → complete | completed / accepted；ActiveRun cleared；usage entries=4 tokens=15653；acceptedEvidence=2；completedDispatches=4 |
| LSP-Bridge `ws-1138d6b3bfddb4fd` | `run-011` | 同上正向收敛路径 | completed / accepted；ActiveRun cleared；usage entries=4 tokens=10307；acceptedEvidence=2；completedDispatches=4 |

关键：projection `ready`，source/projected revision 对齐（269/269）。`make verify-runtime-v2-positive-convergence-live`（`WORKORCH_E2E_WORKSPACE_ID_A/B` 指向上述 workspace）通过。产物目录：`.workorch-e2e/runtime-v2-positive-convergence-live/20260716T174224Z/`（含 `final10-*.json`、`rvc18-acceptance-summary.json`、`live-verify/`）。说明：当前两 workspace bootstrap 仅编译出 `focus-001` 单 focus；跨 focus advance 由 RVC-17 fixture 覆盖，live 证明同 Run repairable_gap 不直接终态与 managed_session/side-effect gate。

## Dependency Order

| RVC | merge 前必须完成 |
| --- | --- |
| RVC-02 | RVC-01 |
| RVC-03 | RVC-02 |
| RVC-04 | RVC-01 |
| RVC-05 | RVC-04 |
| RVC-06 | RVC-04；与 RVC-05 的 shared bootstrap type 已稳定 |
| RVC-07 | RVC-04、RVC-06 |
| RVC-08 | RVC-03、RVC-05、RVC-07 |
| RVC-09 | RVC-08 |
| RVC-10 | RVC-07、RVC-09 |
| RVC-11 | RVC-06、RVC-10 |
| RVC-12 | RVC-11 |
| RVC-13 | RVC-01，可独立于 reducer 主线 |
| RVC-14 | RVC-09、RVC-10、RVC-13 |
| RVC-15 | RVC-12、RVC-14 |
| RVC-16 | RVC-02～15 |
| RVC-17 | RVC-16 |
| RVC-18 | RVC-17 |

允许并行的范围：

- RVC-02 与 RVC-04 可在 RVC-01 后并行；RVC-03 必须在 descriptor API 稳定后合入；
- RVC-05 与 RVC-06 都依赖 RVC-04，因共同修改 bootstrap/focus type，若并行开发必须先约定 shared type commit，最终按 RVC-05 再 RVC-06 合入；
- RVC-13 可与 reducer/result 主线并行；RVC-14 必须等待 RVC-09/RVC-10 的 typed projection fields 和 RVC-13 health algorithm 稳定；
- 单元测试必须随每个工作项同步提交，不集中到最后补写。

## Per-RVC Delivery Matrix

每个 RVC 是独立可审查、可测试、可提交的交付切片。除 RVC-01 已完成外，只有同时满足“实现文件、直接测试、命令、局部 DoD、Progress Evidence”才能标记 completed。文件名为正式落点；若已有同职责文件，可在不改变 owner 边界的前提下落入现有文件，但必须在 Progress Evidence 记录实际文件。

| RVC | SM | mandatory implementation/test landing | targeted verification and local DoD |
| --- | --- | --- | --- |
| RVC-02 | SM-2、SM-3 | `workorchruntimecontract/runtime_operation_descriptor.go`；迁移 `block_kinds.go`、`runtime_candidate.go`、`orchestrator/runtime_v2_operation_executor.go`；新增 descriptor tests | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/... -count=1`；所有 canonical kinds 唯一解析，managed session descriptor 正确，unknown kind 保守拒绝，旧 switch 不再成为 authority |
| RVC-03 | SM-2、SM-4 | `activationcore/runtime_v2_bootstrap.go`、`activationports/runtime_v2_activation.go`、`runtimev2/prerequisite.go`、capability event adapter/tests | `go test ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports ./internal/modules/kernel/orchestrator/... -count=1`；profile/requirement/capability inventory 相等，owner merge 无覆盖，feasibility failure 不调用 LLM |
| RVC-04 | SM-2 | 新增 `activationcore/runtime_v2_focus_compiler.go` 与 tests；扩展 shared profile/evidence policy normalization | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/application/workorchd/biz/appservice/activationcore -count=1`；exact/wildcard/default precedence、ordinary DoD wildcard、stable ordering 和 invalid policy fail-fast 全覆盖 |
| RVC-05 | SM-2、SM-4 | `runtime_bootstrap.go`、`canonical_runtime.go`、`runtime_state.go`、`runtimev2/reducer.go`、state repository normalization/tests | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/... -count=1`；Run-level ledger 只由 coverage-bound state-change 满足，普通 DoD 不被机械要求修改，legacy active restart 可继续 |
| RVC-06 | SM-2 | focus compiler 增加 typed slice policy；`CoverageFocus.SliceID`、bootstrap slice/coverage types；compiler/large fixture tests | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/application/workorchd/biz/appservice/activationcore -count=1`；max `1..32`、group cap、稳定分片、partial item identity 和大 DoD focus 数收敛 |
| RVC-07 | SM-2、SM-3 | shared assessment/gap types；`blocks/llm/biz/runtimev2/candidate_boundary.go` schema/parser；reducer/controller tests | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/blocks/llm/biz/runtimev2 ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1`；boolean 不再裁决 terminal，invalid/out-of-slice item 被拒绝，partial assessment 保留 satisfied lineage |
| RVC-08 | SM-2、SM-3、SM-4 | `runtimev2/reducer.go`、`controller.go`、dispatch commit/state repository；same-focus repair/restart/duplicate tests | `go test ./internal/modules/kernel/orchestrator/... -count=1`；implementation/evidence/governance 四分支准确，同 focus 可 state-change→validation→assessment，CAS 原子推进，restart 不重放 sealed dispatch |
| RVC-09 | SM-2、SM-3、SM-4 | shared convergence policy/repair ledger/signature helper；`workorchruntimepolicy` snapshot mapping；reducer/controller/projection tests | `go test ./internal/pkg/workorchruntimepolicy ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/... -count=1`；attempt 0/1 固定，cycle 幂等计数，默认 budget=10，validation-only cap 生效，budget exhaustion 产生 typed terminal |
| RVC-10 | SM-1、SM-2、SM-4 | shared `RuntimeResultCloseout`；`blocks/result/pkg/models/result.go`；`orchestrator/runtime_v2_result_projection.go`、result adapter/module tests | `go test ./internal/modules/blocks/result/... ./internal/modules/kernel/orchestrator/... -count=1`；accepted/rejected state 由 Orchestrator 决定，rejected closeout 字段完整，重复 terminal projection 幂等 |
| RVC-11 | SM-1、SM-2 | `runtime_continuation.go`；`activationcore` result selection/continuation/baseline diff/focus compiler；activation tests | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports -count=1`；同 revision 只编 unresolved items，changed revision 先 diff，legacy summary-only 进入 assessment gap-analysis focus |
| RVC-12 | SM-1、SM-4 | Run model/contracts/events/module 增加 lineage create/link command；activation/run ports；server/client/read model scope mapping/tests | `go test ./internal/modules/kernel/run/... ./internal/modules/application/workorchd/biz/appservice/... ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1`；create 原子写 predecessor，link CAS 幂等/conflict 正确，workspace scope 不丢失，无自动 successor |
| RVC-13 | SM-4 | `workspaceviewcore/types.go`、`store.go`、`manager.go`；generation failure set、concurrent rebuild tests | `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`；current failure 精确驱动 health，成功 rebuild 可恢复 ready，deleted workspace 清除污染，stale commit 不覆盖新 revision |
| RVC-14 | SM-1、SM-4 | `readmodelprojection/llm_usage*`、runtime progress、terminal activeRun release、workspace snapshot tests | `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice -count=1`；usage event/snapshot parity，terminal/retry 幂等，activeRun 与 Run state 一致，revision 可解释 |
| RVC-15 | SM-4 | `workorchport`、`workorchclient`、readmodel port mapper、embedded/remote backend、TUI state/view、Web TS view/types | `go test ./internal/pkg/workorchport ./internal/pkg/workorchclient ./internal/services/workorch/... ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1 && npm --prefix web/workorch-console run build`；local/remote DTO parity，TUI/Web 仅展示正式字段，legacy missing fields 可用 |
| RVC-16 | SM-1～4 | verifier inventory、边界检查和遗漏 regression tests；不引入新 runtime behavior | 执行本计划 Required Commands 中除 deployed live 外的全部命令；每条 INV 和四状态机 mandatory matrix 都有直接 test ID，无测试后补债务 |
| RVC-17 | SM-1～4 | deterministic provider fixture；`scripts/verify-runtime-v2-positive-convergence-file.sh`；Make target | `make verify-runtime-v2-positive-convergence-file`；覆盖 attempt correction、partial gap、same-focus repair、restart、budget exhaustion、unresolved continuation、projection ready，失败产物完整 |
| RVC-18 | SM-1～4 | `scripts/verify-runtime-v2-positive-convergence-live.sh`；Make target；deployed evidence record | `make verify-runtime-v2-positive-convergence-live`；两个真实 workspace 均跨 focus 正向推进，全局 projection/usage/activeRun/provider/stable-prefix 断言通过 |

提交边界：

- RVC-02～15 每项至少一个 scoped commit；共享类型与第一使用方必须同 commit，禁止只提交无法被生产路径消费的孤立 type；
- schema additive change 与 legacy normalization/test 必须同 commit；
- port/client/server/embedded/remote 的同一字段变更必须同 commit；Web 只在字段已由正式 API 提供后提交；
- 任一 RVC 若需要改变本表类型、默认值、状态迁移或 owner，先更新文档再编码；
- RVC-17/18 verifier 发现结构缺陷时回退对应 RVC 修复，不在脚本中绕过。

## Verification Matrix

| 规则 | Unit/Contract | File-backed E2E | Deployed Live |
| --- | --- | --- | --- |
| operation descriptor 唯一权威 | required | required | capability snapshot |
| managed validation 可执行 | required | required | ai-proxy `/healthz` |
| wildcard evidence policy | required | required | LSP weak-evidence rejection |
| Run/focus state-change 分离 | required | required | multi-focus progress |
| acceptance slice/partial assessment | required | required | remaining item projection |
| repairable assessment | required | required | LSP same-run repair |
| retry/repair budget 分离 | required | required | repeated failure audit |
| structured rejected result | required | required | terminal fixture |
| successor unresolved rehydration | required | required | explicit next activation |
| lineage persistence | required | required | scoped API readback |
| projection recovery | required | required | `/readyz` ready |
| LLM usage projection | required | required | snapshot/event parity |

## Required Commands

每个实现切片先跑 targeted tests，最终至少执行：

```bash
make fmt-check
make vet
make test
make verify-boundary-contract
./scripts/verify-workorch-runtime-core-flow.sh
make verify-ci-mvp-file
make verify-runtime-v2-positive-convergence-file
make verify-authority-live-e2e-closure
make verify-runtime-v2-positive-convergence-live
```

`verify-runtime-v2-positive-convergence-live` 只在最新部署实例上执行；其余命令必须先在代码提交前通过。live verifier 的产物路径、timeout、cleanup 和断言遵守本计划 Verification Fixture Specification。

## Guardrails Against Partial Fixes

以下状态不得标记本计划完成：

- 只补 `validation/managed_session` owner mapping；
- 只增加 candidate retry/transition guard/LLM output token；
- 只调整 LLM prompt，不把 evidence policy 编译进 typed focus；
- 只把 `assessment_rejected` 改成 Resume 或无限循环；
- 只在 application/TUI 自动创建 successor；
- 只把旧 result summary 拼入新 prompt；
- 只隐藏 projection degraded 或清零 counter；
- 只在两个现有 workspace 上做定制判断；
- 在 kernel 中写入 software-dev、LSP、HTTP、Go 或具体测试命令逻辑；
- 重新引入 V1 graph/NextAction/checkpoint authority。

## Completion Criteria

只有同时满足以下条件，计划才能 `completed` 并迁入 `docs/90-archive/completed/`：

- RVC-01～RVC-18 全部完成并有直接证据；
- Runtime core contract、flow、module readme、shared types、实现和 verifier 一致；
- 两个真实 workspace 均证明跨 focus 正向推进；
- repairable assessment 不直接终态；
- managed validation capability 与 side-effect gate 同时正确；
- successor 只承接 unresolved structured gaps；
- projection 恢复 ready 且 usage/read model 一致；
- 全量 test/vet/build/boundary/file-backed/deployed gates 通过；
- 工作树无未解释变更；
- active roadmap、current state、unclosed matrix 与 unfinished index 同步移除本项。

## Progress Evidence

| ID | Status | Direct Evidence |
| --- | --- | --- |
| RVC-01 | completed | `docs/00-core/runtime-stage-contract.md` 四状态机固化规约；`AGENTS.md` 强制变更约束；`internal/modules/kernel/orchestrator/readme.md` 实现映射；`docs/20-runtime-flow/*` state-machine binding；`docs/60-verification/core-runtime-verifier.md` mandatory matrix |
| RVC-02 | completed | `workorchruntimecontract/runtime_operation_descriptor.go` 唯一 descriptor authority；`RuntimeModuleForOperationKind`/`OperationHasWorkspaceSideEffects*` 派生自 descriptor；`validation/managed_session` → Shell + non_mutating + produce_validation；executor/shell_operation 分发；descriptor/side-effect/feasibility 与 shell managed-session 测试通过 |
| RVC-03 | completed | Shell 支持 `validation/managed_session`；bootstrap 默认 validation 改为 managed_session；profile accepted kind 无 descriptor 时 fail-fast；capability merge 加法去重；candidate 前 feasibility gate 阻止无可行 kind 的 provider 请求；activationcore/shell/prerequisite/feasibility 测试通过 |
| RVC-04 | completed | `activationcore/runtime_v2_focus_compiler.go` 统一 exact>wildcard>scope default>neutral default；DoD focus 继承 `scope=dodItem,id=*` accepted/rejected/direct/claim/executable；profile/evidencePolicies 作为中立输入；CoverageFocus 扩展 Require* 字段；compiler/bootstrap 测试覆盖 precedence、unknown kind fail-fast 与 evidence policy coverageRequirements |
| RVC-05 | completed | `RuntimeRunStateChangeRequirement`/`RuntimeStateChangeLedger` 入 shared contract；bootstrap 封存 Run 级 requirement；DoD focus 不再机械要求 state_change；deliverable/source-change 仍显式要求；CommitEvidence 更新 ledger；reducer 完成门禁校验 Run ledger；ledger/compiler/reducer 测试通过 |
| RVC-06 | completed | typed `RuntimeAcceptanceSlicePolicy`/`RuntimeAcceptanceSlice`/`RuntimeCoverageItem`；`CoverageFocus.SliceID`；activationports 映射 raw acceptanceSlicePolicy；compiler 按 group priority/match 分桶并以 maxWorkItems 稳定分片（`groupID-%03d`）；item identity 独立于 slice 大小；大 DoD focus 数收敛与 invalid max fail-fast 测试通过 |
| RVC-07 | completed | `AssessmentDisposition`/`RuntimeAssessmentGap`/`RuntimeAssessmentItemFact`；AssessmentCandidate+Fact 扩展 disposition/items/gap；LLM schema/parser 支持；CommitAssessment 持久化结构化 facts；reducer `repairable_gap` 同 focus 重开 state-change/validation，不再因 `Satisfied=false` 直接 terminal；out-of-slice item 拒绝与 disposition 测试通过 |
| RVC-08 | completed | repairable assessment 四分支（state-change/validation/governance/terminal）；accepted evidence 使非 satisfied assessment 失效并清 failure，支持同 focus state-change→validation→assessment；assessmentForCurrentFocus 取最新；reducer same-focus repair/reassess 与 invalidate 测试通过；既有 rejected-validation→state-change 与 accepted-repair→validation 回归保持 |
| RVC-09 | completed | `RuntimeConvergencePolicy`/`RuntimeRepairLedger` 与 stable failureSignature/cycleID；bootstrap 封存 policy（默认 repair=10）；CommitAssessment 对 repairable_gap 幂等计 repair cycle；accepted evidence NoteProgress；reducer 在 RepairAttempts/ValidationOnlyRetries 耗尽时 `no_progress_exhausted` 或 escalate state-change；candidate attempt 仍独立 0/1；ledger/budget 测试通过 |
| RVC-10 | completed | shared `RuntimeResultCloseout`；`Result.RuntimeCloseout`；`runtimeV2ResultFromAggregate` 写入 satisfied/unsatisfied coverage、gaps、missing evidence、target refs、disposition、failure code/signature、definition revision；accepted/rejected 仍由 Orchestrator terminal 决定；projection 测试覆盖 structured rejected closeout |
| RVC-11 | completed | continuation 扩展 closeout 字段；`runtimeV2RejectedResultContinuation` 优先映射 RuntimeCloseout；focus compiler 在 definition 未变时只保留 unresolved cwi IDs（不重编号）；缺 structured gap 或 definition supersede 时生成 `continuation_gap_analysis` assessment focus；rehydrate/gap-analysis 测试通过 |
| RVC-12 | completed | `RunLineage.PredecessorResultID`；CreateRun 写入 predecessor lineage；`LinkRunSuccessor` CAS 幂等/conflict；activation 从 continuation 填 lineage 并回写 predecessor successor；port/client/remote/MapRun 暴露 lineage；workspace-scoped create/get 保持；无自动 successor lifecycle；lineage create/link 测试通过 |
| RVC-13 | completed | `workspaceviewcore` current-generation failure set（`globalFailure`/`workspaceFailures`）驱动 ready/degraded；`Status`/`WorkspaceViewModel`/`ProjectionInfo`/`ProjectionStatus` 暴露 `streamGeneration`/`failedWorkspaceIds`/`degradedSince`；成功 UpsertSnapshot/ReplaceAll 恢复 ready；DeleteWorkspace 清除污染；metrics 累计失败不决定 health；generation recovery 测试通过 |
| RVC-14 | completed | Run snapshot 全模式 hydrate topic-scoped `/run/llm_usage/recorded` 到 `LLMUsageSummary`/`Traces`（timeline 仍 metadata-only）；`releaseWorkspaceForTerminalRun` 幂等：已清空/他活 run 不冲突不抢占，stale terminal slot 可清；`ProjectRuntimeV2Terminal`/`closeLoadedRun` retry 不 rewrite completed authority；compact usage + terminal no-steal/stale-clear 测试通过 |
| RVC-15 | completed | `ConvergenceStatus` additive 字段贯通 readmodelcore/port/client/remote：`currentSliceId`、coverage closed/remaining、`assessmentDisposition`、repair attempts/max、`repeatedFailureSignatureCount`、`latestAcceptedEvidenceRef`、`nextActivationRequired`、`gapSummary`/`lastProgressAt`；`MapConvergenceStatus` 从 Acceptance+RuntimeV2+AssessmentOutcome 投影；TUI compact status 与 Web helpers 仅展示正式字段；projection/port/tui/backends 测试与 console build 通过 |
| RVC-16 | completed | inventory 覆盖 RVC-02～15 直接测试（descriptor/feasibility、shell managed_session、focus compiler/slice、state-change ledger、assessment disposition、same-focus repair、budget exhaustion、continuation/lineage、projection generation recovery、usage hydrate、terminal release）；`make fmt-check`/`make vet`/`make verify-boundary-contract`/`make test` 通过；修复 server 包并行 teardown 在 `close()` 中对已取消 ctx 调用 session.Close 导致的 10m hang（各阶段独立 timeout）；deployed/live 命令与 `verify-runtime-v2-positive-convergence-file` 归 RVC-17/18 |
| RVC-17 | completed | `runtimev2/positive_convergence_fixture_test.go`：attempt0 schema fail→attempt1 state-change→validation→repairable_gap→same-focus repair→satisfied complete；budget exhaustion；focus advance；`scripts/verify-runtime-v2-positive-convergence-file.sh` + Make target 串联 drive/reducer/continuation/projection/usage/terminal 回归；`make verify-runtime-v2-positive-convergence-file` 通过；live 脚本脚手架见 RVC-18 |
| RVC-18 | completed | 最新部署 `./bin/workorch --server -config-dir config/file` + verify mock `127.0.0.1:18188`；ai-proxy `run-012` / LSP-Bridge `run-011` 均 completed+accepted：`produce_validation(managed_session)`→`repairable_gap`→同 Run `produce_state_change`→`satisfied(evidenceRefs=current-run)`→`complete`；ActiveRun 清空；projection ready 且 source/projected revision 对齐；llmUsage 非零；`scripts/verify-runtime-v2-positive-convergence-live.sh` + Make target 通过；产物 `.workorch-e2e/runtime-v2-positive-convergence-live/20260716T174224Z/` |
