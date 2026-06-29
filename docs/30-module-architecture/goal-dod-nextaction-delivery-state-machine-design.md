# Goal/DoD NextAction Delivery State Machine Design

Status: active

Type: module-architecture

Last Updated: 2026-06-26

Related:

- [Run Resume And Gap Confirmation Design](run-resume-gap-confirmation-design.md)
- [Contract Violation Recovery Loop Design](contract-violation-recovery-loop-design.md)
- [EventHub Driven Workorchd State Loop Design](eventhub-driven-workorchd-state-loop-design.md)
- [Runtime Execution Event Queue State Loop Design](runtime-execution-event-queue-state-loop-design.md)
- [MVVP Workspace ViewModel Architecture](mvvp-workspace-viewmodel-architecture.md)
- [LLM Cache Improvement Design](llm-cache-improvement-design.md)
- [Projection First ReadModel IO Closure Design](projection-first-readmodel-io-closure-design.md)
- [Runtime Contract Retry Convergence Closure Plan](../70-roadmap/active/runtime-contract-retry-convergence-closure-plan.md)
- [Runtime Coverage Convergence State Machine Closure Plan](../70-roadmap/active/runtime-coverage-convergence-state-machine-closure-plan.md)
- [Workspace Goal Convergence Acceleration Design](../70-roadmap/active/workspace-goal-convergence-acceleration-design.md)
- [Convergence Authority Simplification Design](../70-roadmap/active/convergence-authority-simplification-design.md)
- [Convergence Authority Dispatch Closure Checklist](../70-roadmap/active/convergence-authority-dispatch-closure-checklist.md)
- [Convergence Authority Dispatch Live Verification](../60-verification/convergence-authority-dispatch-live-verification.md)
- [Runtime Validation NextAction Convergence Improvement Plan](../70-roadmap/active/runtime-validation-nextaction-convergence-improvement-plan.md)

## Purpose

本文重新完整固化 WorkOrch 的长期收敛设计：用户只需要定义 `Goal` 与 `DoD`，WorkOrch 应通过业务中立的状态机、coverage ledger、evidence ledger、NextAction 推导、workspaceTemplate/profile 阶段策略和严格的 EventHub 主路径，持续推进 workspace 交付，直到完成、进入治理阻塞或明确失败。

本文用于避免继续围绕单个 live workspace、某次 LLM 输出、某个提示词片段或某个 TUI 展示现象做单点修补。后续代码收口必须先引用本文对应条目，说明当前偏差位于 `Goal/DoD normalization`、`Coverage/Evidence ledger`、`NextAction`、`Stage Packet`、`LLM contract`、`EventHub command`、`Projection/ViewModel` 还是 `template policy`。

当前阶段不考虑兼容性。若现有实现、测试、文档或端侧行为与本文冲突，应按本文逐项收口。

## Problem Statement

近期核对暴露出多类现象：

- workspace 能进入 running、repair、waiting_decision、waiting_approval 等状态，但长时间没有符合 Goal 的有效产物。
- LLM 多次返回合同违例内容，runtime 在严格合同、自动重试、decision/replan、repair/follow-up 之间缺少统一收敛路径。
- 已有 evidence、DoD coverage、当前 gap、accepted/rejected diagnostics、目标快照和当前 requiredOutput 没有稳定进入每次 LLM 请求。
- 首次执行、resume、replan、restart、服务重启恢复等入口语义不统一，导致有时没有重新触发有效调度。
- TUI/Web 曾经出现状态展示、prompt、workspace 列表和主视图不同步，说明端侧和 workorchd projection 边界需要继续固化。
- 多 workspace 并发、长会话软件开发和多轮外部调用场景下，周期扫描、补偿恢复、watchdog、history 读取容易造成 CPU/IO 压力或状态噪声。

核心判断是：问题不能简单归因于 LLM 不稳定，也不能通过继续叠加更严苛的 prompt 或更多 core 特判解决。WorkOrch 需要把“目标定义 -> 缺口确认 -> 下一动作 -> 候选输出 -> 合同校验 -> 证据入账 -> 再次推导”的闭环做成确定性的基础能力。

## Design Goals

1. 用户只定义 Goal 与 DoD，系统能自动拆解为可追踪、可验证、可恢复的交付流程。
2. WorkOrch core 保持业务中立，只提供状态机、调度、证据、治理、上下文和投影能力。
3. 软件开发等长会话策略由 workspaceTemplate、profile、Soul、policy 和 skill 承载，core 不写模板、语言、命令、文件名或 provider 特判。
4. Runtime 负责 deterministic state transition；LLM 只负责当前窄 action 的 candidate content generation。
5. 所有状态变化都以 EventHub command/event 进入 owner，TUI/Web/CLI 只提交命令和展示 ViewModel。
6. LLM 违例可以被接收、记录、诊断和反馈重试，但不能执行、不能接受为结果、不能污染 artifact/result/memory。
7. 长会话必须通过结构化 Stage Packet 衔接阶段，避免前后阶段靠自然语言历史和压缩摘要传递关键状态。
8. 历史记录读取采用“近期完整数据 + 历史摘要 + ledger 权威”的模式，避免每轮全量扫描和全量注入上下文。
9. 多 workspace 并发下，每个 workspace/run/ticket/approval/step/event 都必须具备 scope、幂等 key 和可投影状态。

## Non-Goals

1. 不把 WorkOrch 改成软件开发专用 agent。软件开发只是一个 workspaceTemplate/profile 组合。
2. 不要求 LLM 每次输出都完美符合合同。系统必须允许违例并能自动分类、反馈和重试。
3. 不用 TUI/Web 本地状态推断替代 workorchd 权威投影。
4. 不用 watchdog 或补偿恢复掩盖 EventHub 主路径丢事件、owner 状态不一致或 command 未投递的问题。
5. 不通过扩大 prompt、无限读取历史、无限 repair、无限 replan 或放宽 completion gate 追求表面推进。
6. 不将未验证、被截断、被 redacted 或无法追溯的内容作为可完成 DoD 的 accepted evidence。

## Target Flow

目标流程固定为：

```text
User Goal/DoD
  -> normalize workspace definition
  -> build or refresh coverage ledger
  -> derive deterministic NextAction
  -> render narrow Stage Packet and LLM contract
  -> LLM generates candidate content
  -> validate candidate against current contract
  -> execute admitted side effect or validation
  -> record accepted/rejected evidence
  -> recompute ledger and projection
  -> repeat until complete / governed blocked / failed
```

该流程有三个关键约束：

- `coverage ledger` 与 `evidence ledger` 是收敛权威，不是 LLM 的自然语言总结。
- `NextAction` 由 runtime 根据 ledger 和策略确定，不由 LLM 自行选择状态转移。
- LLM 每轮只处理当前 narrow action，不能同时承担全局规划、状态判断、证据验收和完成判定。

## Ownership Boundary

### WorkOrch Core

Core 可以包含：

- Goal、DoD、Workspace、Run、Step、Ticket、Approval、Artifact、Result、Memory、Coverage、Evidence 等通用模型。
- EventHub command/event、owner state、projection、read model、runtime scheduler、idempotency、scope、lane policy。
- Goal/DoD normalization schema、coverage ledger、evidence ledger、NextAction interpreter、completion gate。
- LLM request 分区、context budget、contract violation recovery、rejected diagnostic evidence、provider retry budget。
- approval/decision/governance 的通用状态机。
- block/runtime adapter 的中立能力描述、执行准入、结构化输出和错误分类。

Core 不应包含：

- `software-dev`、`weather-query`、`code-review` 等模板 ID 或业务判断。
- Go、Python、Node、React、LSP bridge、某测试命令、某文件名、某 provider 品牌等场景特判。
- 根据自然语言业务文本、workspace title、Soul ID、日志 marker 或 live workspace ID 推断流程。
- 为某个 e2e 或当前 workspace 临时加入的兼容分支。

### WorkspaceTemplate / Profile / Soul / Policy / Skill

模板与策略层承载：

- 阶段目录、阶段进入/退出条件、默认 Soul、默认 DoD、产物类型、验证偏好、工具偏好。
- 软件开发工作流的任务粒度、文件粒度、验证节点偏好、review/approval 策略。
- 可选能力和工具绑定，例如 MCP、LSP、shell/file/web/LLM 能力的使用方式。
- template-bound evidence policy 与质量评分规则。
- 业务 prompt、技能规范、示例 fixture、e2e 数据。

模板策略必须以结构化数据进入 workspace 绑定或 run execution context。只修改 markdown 文档但没有绑定路径，不算完成收口。

### LLM

LLM 的职责是：

- 针对当前 Stage Packet 和 requiredOutput 生成候选内容。
- 给出候选计划、patch、artifact、validation graph、assessment 或 blocker。
- 在收到 rejected diagnostic evidence 后，按同一合同或收窄合同重试。

LLM 不拥有：

- Goal/DoD 的权威状态。
- coverage 是否完成的最终判定。
- workspace/run/ticket/approval 状态转移。
- accepted evidence、artifact/result acceptance 或 memory 写入准入。

### TUI / Web / CLI / Agent

端侧只负责：

- 展示 workorchd 输出的 ViewModel/projection。
- 提交 scoped command，例如 resume、restart、decision resolve、approval approve/reject。
- 展示命令提交状态、projection revision、当前等待项和输入提示。

端侧不得：

- 本地维护 run/decision/approval/coverage 状态机。
- 根据旧 prompt 或旧本地缓存推断 workspace 当前状态。
- 直接绕过 workorchd owner state 修改 runtime 状态。

## Goal/DoD Normalization

### Workspace Definition Revision

每次 Goal、DoD、template binding、profile policy 或 workspace 关键约束变化，都必须形成新的 `WorkspaceDefinitionRevision`。

```text
WorkspaceDefinitionRevision
  id
  workspaceId
  templateId
  profileId
  goalItems[]
  dodItems[]
  policyRefs[]
  capabilityRefs[]
  createdAt
  sourceRef
  cumulativeFrom
```

约束：

- 最新 revision 是权威定义。
- 新 definition 默认 cumulative。除非存在显式冲突和用户确认，不得弱化旧目标或删除仍有效 DoD。
- revision hash 进入 LLM C1，不应污染 C0 stable prefix。
- coverage ledger 必须绑定 definition revision。

### Goal Item

Goal 应被规范为可引用项，而不是只作为大段自然语言：

```text
GoalItem
  id
  title
  intent
  scope
  constraints[]
  priority
  sourceRef
  ownerPolicyRef
```

Core 只理解通用字段。具体优先级解释可以来自 profile policy。

### DoD Item

DoD 应被规范为可验证项：

```text
DoDItem
  id
  title
  acceptanceCriteria[]
  requiredEvidenceKinds[]
  targetRefs[]
  policyRef
  sourceRef
```

`requiredEvidenceKinds` 必须结构化，例如 `state_change`、`validation`、`assessment`、`artifact`、`approval`、`external_confirmation`。Core 不通过自然语言判断“这条是不是测试通过”。

## Coverage Ledger

Coverage ledger 是“哪些 Goal/DoD 已完成、还缺什么证据、下一步应补什么”的权威账本。

```text
CoverageLedger
  workspaceId
  runId
  definitionRevision
  items[]
  currentFocus
  unresolvedGaps[]
  acceptedEvidenceRefs[]
  rejectedEvidenceRefs[]
  updatedAt
```

```text
CoverageItem
  id
  goalItemId
  dodItemId
  status              # open | in_progress | satisfied | rejected | blocked
  requiredEvidenceKinds[]
  acceptedEvidenceRefs[]
  rejectedEvidenceRefs[]
  missingEvidenceKinds[]
  lastActionRef
  blockerRef
```

规则：

- `satisfied` 只能由 accepted evidence 触发，不能由 LLM 自评自然语言直接触发。
- `state_change` 必须有明确 state-changing evidence，例如 patch/file write/artifact update/result update 等被接受记录。
- `validation` 必须有具体 validation evidence ref，不接受泛化“已验证”文本。
- `assessment` 只能说明判定过程，不能替代缺失的 state_change 或 validation evidence。
- rejected diagnostic evidence 可以帮助后续 retry，但不能满足 coverage。

## Evidence Ledger

Evidence ledger 是所有候选输出、执行结果、验证结果、拒绝原因和治理结论的审计权威。

```text
EvidenceRecord
  id
  workspaceId
  runId
  stepId
  kind                # state_change | validation | assessment | artifact | decision | approval | diagnostic
  status              # accepted | rejected | superseded
  source              # llm | runtime | block | user | provider | projection
  sourceRef
  targetRefs[]
  summary
  payloadRef
  quality
  rejectionReason
  createdAt
```

证据规则：

- 任何 state-changing side effect 之前必须通过合同准入。
- 未通过合同的 LLM output 只能写为 `diagnostic/rejected`。
- 被截断、脱敏、摘要化、不可追溯或未完整加载的内容不能作为 exact patch 或 complete file evidence。
- Memory 不是 evidence ledger。Memory 只能召回参考信息，不作为 completion authority。
- 历史 compact 不是 evidence ledger。Compact 只能辅助阅读，不能覆盖 accepted/rejected evidence。

## Deterministic NextAction

Runtime 每轮根据 ledger、policy 和当前状态确定唯一或可排序的 NextAction。

```text
NextAction
  id
  workspaceId
  runId
  type                # assess_gap | plan | state_change | validate | repair | request_decision | request_approval | complete | fail
  focusCoverageItemIds[]
  requiredOutput
  allowedActions[]
  forbiddenActions[]
  requiredEvidenceKinds[]
  stage
  stageMode
  idempotencyKey
  reason
```

推导顺序：

1. 如果有 pending approval，NextAction 是 `request_approval` 或等待 approval resolution。
2. 如果有 open decision 且缺少外部信息、权限、用户选择或治理确认，NextAction 是 `request_decision`。
3. 如果存在可自动恢复的 rejected diagnostic，NextAction 是 `repair` 或 retry same contract。
4. 如果 ledger 缺少 gap baseline，NextAction 是 `assess_gap`。
5. 如果存在 open coverage 且缺少 state_change evidence，NextAction 是 `state_change`。
6. 如果存在 state_change 但缺 validation evidence，NextAction 是 `validate`。
7. 如果 evidence 足够但缺 assessment，NextAction 是 `assessment`。
8. 如果所有 required evidence 满足，NextAction 是 `complete`。
9. 如果外部依赖、预算或不可恢复合同违例满足失败条件，NextAction 是 `fail` 或 governed blocked。

关键约束：

- LLM 可以建议 action，但不能覆盖 deterministic NextAction。
- NextAction 必须可以从 ledger 重建，不能只保存在 prompt 或日志里。
- 多 workspace 下 NextAction 必须带 workspace/run scope 与 idempotency key。

## Stage Model

Core 只定义通用 stage 类型，模板/profile 可以定义具体 stage catalog 和优先级。

通用 stage：

- `intake`: 解析 Goal/DoD/template/profile，建立 workspace definition。
- `gap_analysis`: 根据 definition 与已有 evidence 生成 GapBaseline。
- `planning`: 为当前 open coverage 生成可执行策略或拆分。
- `state_change`: 产生候选状态变更，例如 patch、artifact update、result update。
- `validation`: 产生或执行验证证据。
- `assessment`: 根据 accepted evidence 判断 coverage。
- `repair`: 针对 rejected diagnostic 或 failed validation 进行修正。
- `governance`: 处理 decision/approval/ticket。
- `artifact`: 整理交付物或结果投影。
- `complete`: 结束 run 并冻结完成证据。

软件开发 template 可以把这些 stage 组合为“需求理解 -> gap baseline -> implementation slice -> validation slice -> review/repair -> acceptance”，但 core 仍只消费通用 stage 和结构化策略。

## Stage Entry And Exit Criteria

每个 stage 必须有结构化进入和退出条件。

```text
StageRule
  stage
  entryCriteria[]
  exitCriteria[]
  requiredEvidenceKinds[]
  allowedNextStages[]
  policyRef
```

示例规则：

- `gap_analysis`
  - Entry: 最新 definition revision 已存在。
  - Exit: CoverageLedger 已构建或刷新，open/satisfied/blocked 项可投影。
- `state_change`
  - Entry: 当前 focus 至少缺少一个 state_change evidence，且没有 pending approval/decision 阻塞。
  - Exit: 产生 admitted state-changing candidate，并执行或创建 approval。
- `validation`
  - Entry: 当前 focus 已有 state_change evidence，但缺 validation evidence。
  - Exit: 产生 accepted validation evidence 或 rejected diagnostic。
- `repair`
  - Entry: 存在可恢复 rejected diagnostic，预算未超限。
  - Exit: 新 candidate 被 accepted，或重复违例升级治理。

退出条件必须基于 ledger/evidence，不基于 LLM 自称“已完成”。

## Stage Packet

Stage Packet 是长会话阶段衔接的唯一结构化载体。每次 LLM 请求、runtime step、decision/replan/resume 都必须能重建当前 Stage Packet。

Stage Packet 也是 provider dispatch 前的完整性边界。任何 LLM/runtime
step 在进入 provider、block adapter 或其他外部执行前，都必须已经具备非空
Stage Packet，并且 Stage Packet、current focus、NextAction、requiredOutput、
convergenceContract 和 gapBaseline 必须来自同一次 convergence authority
snapshot。若 dispatch 前发现这些派生字段不一致，runtime 必须刷新 authority
并重建派生字段；若刷新后仍不一致，应按 runtime internal contract
misalignment fail-fast，不得把互相矛盾的合同发送给 LLM。

```text
StagePacket
  workspaceId
  runId
  stepId
  definitionRevision
  templatePolicyRefs[]
  activeStage
  activeFocus
  nextAction
  requiredOutput
  allowedActions[]
  forbiddenActions[]
  requiredEvidenceKinds[]
  acceptedEvidenceRefs[]
  rejectedEvidenceRefs[]
  currentFailure
  currentGap
  targetSnapshots[]
  processEvidenceTimeline[]
  compactSummaryRef
  memoryRefs[]
  idempotencyKey
```

必须包含：

- 最新 Goal/DoD definition revision。
- 当前 focus 与 open coverage item。
- requiredOutput 和 allowed/forbidden actions。
- accepted/rejected evidence refs。
- 当前 failure 或 contract violation 诊断。
- 目标快照引用和完整性状态，例如 full、partial、truncated、redacted。
- 历史摘要引用，但摘要不能替代 ledger。

禁止：

- 只把旧 execution graph、旧 memory、旧 decision resolution 当作当前合同。
- 在 Stage Packet 中混入 provider 私有缓存字段。
- 缺少 workspace/run scope。
- 使用被截断快照要求 LLM 输出 exact complete file。
- 从旧 failed-step input、旧 repairActionPackage、旧 replanContinuation、旧
  contract violation recovery 或旧 budget scope 继承当前动作权威。旧 lineage
  只能作为 rejected diagnostic evidence、trace refs 或 retry hint。

## LLM Request Partition

LLM 上下文按稳定性分区：

- `C0`: 稳定 system 合同、输出 schema、中立 runtime 规则。不得包含 workspace/run/goal/failure/current action。
- `C1`: 当前 WorkspaceDefinitionRevision，包括 Goal/DoD、definition hash。
- `C2`: 声明式 capability、governance、template/profile policy contract。
- `C3`: Stage Packet 中的 stage contract、requiredOutput、allowed/forbidden actions。
- `C4`: 当前 progress、failure、rejected diagnostics、current requested action、retry hint。
- `C5`: 大对象引用、target snapshots、tool outputs、process evidence timeline。

C3 Stage Packet 是 LLM 可见上下文里的唯一当前 action/focus contract。若
Stage Packet 已存在 `activeFocus`，C4 不得再渲染并列的 `acceptance-focus`
当前权威信号；acceptance、历史 focus、旧 slice 或旧 repair lineage 如需保留，
只能作为 diagnostic/historical context，或必须在 dispatch 前由当前
convergence authority 重新派生。

缓存目标：

- 同一 run 内仅因 repair、provider retry、current failure、current progress、requested action 变化，不应改变 C0。
- Goal/DoD definition revision 变化影响 C1，不应污染 C0。
- provider 私有 cache-control 由 adapter 根据中立 cachePlan 转换，core 不直接写 provider 私有字段。

## Contract Violation Recovery

LLM 输出一定可能不稳定，因此合同处理必须分为“严格准入”和“容错恢复”。

```text
LLM candidate
  -> parse/schema gate
  -> contract gate
  -> if valid: admit side effect or evidence acceptance
  -> if invalid and recoverable: rejected diagnostic evidence + bounded retry
  -> if invalid and not recoverable: decision / failed / blocked
```

规则：

- 合同门禁不放松。
- 违例内容不能执行，不能 accepted，不能写 artifact/result，不能写长期高价值 memory。
- 可恢复违例必须记录 `ContractViolation`，包含 violatedRule、expectedContract、actualOutputSummary、rejectedOutputRef、recoverability、retryHint。
- retry 必须继承原 runtimeStage、requiredOutput、repairObjective、Goal/DoD definition 和 evidence timeline。
- retry 只在 C4 增加违例诊断，不改变 C0/C1/C2。
- 相同 signature 超预算、不可解析输出持续出现、重复不产生 delta 时，升级 decision 或 failed。

## Resume / Replan / Restart Semantics

### Resume

`resume` 是唯一常规推进语义。它覆盖首次启动、服务重启后重新激活、decision/approval 释放后继续和 recoverable run 继续。

Resume 必须：

- 通过 EventHub command 进入 workorchd owner。
- 解析当前 active run；无 run 时创建首个 run。
- 刷新 GapBaseline 和 Stage Packet。
- 基于 NextAction 调度后续 step。

### Replan

`replan` 是 decision resolution 后对当前 active run 的重新规划，不是 replacement run。

Replan 必须：

- 携带 workspaceId、runId、decisionId、resolutionRevision、requestId。
- 继承 accepted/rejected evidence、Goal/DoD definition、当前 gap 和失败阶段语义。
- 通过同一 scheduler 主路径刷新 NextAction。
- 不清空旧 run，不把 resolution 文本当作新 Goal/DoD 权威定义。

### Restart

`restart` 是显式 replacement run。

Restart 必须：

- supersede 或关闭旧 active run。
- 创建 replacement run。
- 基于最新 workspace definition 重新进入 gap_analysis。
- 在 TUI/Web 中以 destructive 语义展示。

### Continue

`continue` 不再作为 canonical 语义。若入口临时接受该文本，只能立即归一化为 `resume`，不得在 port、REST、EventHub、projection 中形成第二套状态。

## EventHub Driven State Loop

所有端侧命令都必须转换为 scoped EventHub command：

```text
TUI/Web/CLI
  -> workorchport/workorchclient command DTO
  -> REST or embedded backend
  -> EventHub command
  -> workorchd owner state
  -> scheduler queue
  -> orchestrator/runtime step
  -> owner state update
  -> projection/ViewModel event
```

要求：

- TUI/Web 不直接调用 kernel owner 或 repository。
- command 必须包含 workspace scope 与 request id。
- owner 接收 command 后先记录 accepted/rejected command projection，再进入调度。
- 调度必须具备幂等 key，重复 command 不创建多个等价 run/step。
- EventHub lane 仅用于需要状态串行化的 owner；无状态 block/module 不应被不必要串行化。
- `Send` 同步调用不得造成同 lane 递归等待；需要异步滚动时使用队列或 Post 语义。
- 如果运行中出现 event 丢失，应修复投递/消费/持久化路径，不用 watchdog 长期掩盖。

## Projection And MVVP

workorchd 是 ViewModel owner。

```text
owner state
  -> projection builder
  -> WorkspaceViewModel revision
  -> SSE / REST / TUI stream
  -> TUI/Web render
```

ViewModel 至少包含：

- workspace list 状态、active workspace、run state、decision/approval summary。
- current prompt mode、allowed commands、input hint。
- coverage summary、current focus、latest stage、latest diagnostic。
- deterministic NextAction、authority diagnostics、blocking ticket/approval。
- projection revision、source revision、projected source revision、last event id、refresh state。

TUI/Web 规则：

- 主视图、workspace 列表、状态栏、prompt 必须来自同一 projection revision。
- 切换 workspace 只切换展示目标，不触发本地业务状态推断。
- 端侧命令提交后先显示 command accepted/pending，再等待 projection revision 更新。
- SSE 中断需要重连和 last event id，不得造成永久白屏或状态停滞。

Projection health 语义：

- `ready` projection 必须保证 projected source revision 与 source revision 对齐。
- `stale` / `degraded` projection 可以暴露落后的 projected source revision，但这
  是可显示的 projection health 状态，不等价于 display contract 失败。
- Projection 可以暴露 `authorityDiagnostics`，例如 persisted derived field
  stale 或 source/projected revision lag，但不得修复 owner state，也不得成为第二套
  runtime 状态机。
- TUI/Web prompt、workspace list、main view 和 status bar 必须优先消费
  server-projected ViewModel；没有 ViewModel 时只能显示 degraded/loading 状态，
  不得用本地 blocker cache 推断业务状态。

## Long Session Handoff

软件开发等长会话需要阶段衔接，但衔接必须通过 Stage Packet 和 ledger，而不是靠历史对话全量重读。

长会话阶段衔接规则：

- 每个阶段结束时写入 accepted/rejected evidence。
- 下一阶段从 ledger 推导 current gap，而不是从上一轮自然语言总结猜测。
- compact summary 只总结历史背景；必要 evidence refs、target snapshot refs、failure refs 必须保持结构化可追溯。
- 近期完整数据保留最近 N 个关键事件、当前失败、当前 focus、最近 accepted/rejected evidence。
- 历史完整事件不应每轮全量注入 LLM；需要时通过 sourceRef 按需加载。
- 如果需要完整文件或精确 patch，必须确认 target snapshot 为 full 且未截断。

## External Interaction And Watchdog

Watchdog 的必要性仅限于运行中健康检查和幂等补偿，主要覆盖外部调用未按预期返回的场景。

允许的健康检查：

- provider stream 长时间无 token/无 finish。
- MCP/tool/shell/web/file adapter 调用超时。
- approval/decision 外部输入已提交但 owner 未进入下一步。
- scheduler 已接受 command 但对应 step 未在合理时间进入 terminal。

限制：

- watchdog 不用于周期性扫描所有 workspace 重新推断状态。
- watchdog 不替代 EventHub command 主路径。
- watchdog 补偿必须幂等，必须有 scope 和 idempotency key。
- 非外部交互导致的状态异常，应定位 owner/event/projection 缺陷并修复。

服务重启后：

- 旧 running 但无本进程执行上下文的 run 不应继续显示为 active running。
- 应刷新为 idle/recoverable/pending_activation 等可重新激活状态。
- 是否恢复执行由用户 `resume` 或明确策略触发，不靠隐式重启后自动跑偏。

## IO And History Strategy

为避免 CPU/IO 持续高负载：

- projection 优先读 read model，不在每次 view refresh 全量扫描 workspace 文件。
- history 访问采用近期完整事件 + 历史 compact + ledger refs。
- 大对象通过 payloadRef/snapshotRef 按需读取。
- 每个 run/step 更新只重算受影响 workspace 的 projection，不全局 rebuild。
- memory recall 只召回当前 action 有用的高价值条目，diagnostic-only、stale、superseded、低置信内容不得注入正文。
- 文件扫描、证据抽取、snapshot 构建必须有预算和缓存。
- 高频 `run.llm_stream_progress` 应作为进度心跳或观测事件处理，不应强制触发完整
  `workspace_projection_changed` fan-out、列表 refetch 或 detail hydrate。
- 普通 projection/detail snapshot 可以排除高频 stream progress timeline 记录；
  observability/debug 查询必须保留完整事件时间线。LLM usage/cache 等读模型必需数据
  仍应按结构化查询保留。
- 普通 run/workspace projection 的 event timeline 默认使用 metadata-only 查询；
  只有显式 observability/debug 入口允许 hydrate 完整 payload。任何新增 detail
  接口若需要完整 payload，必须通过显式参数或独立 observability API 表达。

## Template Strategy For Software Development

软件开发 template/profile 应定义以下策略，但这些策略不进入 core hardcode：

- 阶段目录：gap_analysis、implementation_slice、validation_slice、repair_slice、acceptance。
- 任务粒度：避免一次生成过大文件，优先小切片、可验证、可回滚的变更。
- 文件粒度 guidance：单文件保持合适职责和大小，过大时要求拆分或分阶段提交。
- 验证偏好：优先使用 bounded foreground harness、项目测试、fixture、stub/fake、MCP/tool evidence。
- LSP/MCP 能力：作为软件开发 workspace 可用能力，但是否启用、如何排序由 template/profile 绑定。
- DoD coverage 策略：每个 DoD 至少需要 state_change 与 validation 或 assessment 组合，具体 evidencePolicy 由模板声明。

Core 只解释这些结构化策略，不根据“软件开发”文本执行特殊逻辑。

## Completion Gate

Workspace 不能仅因 LLM 声称完成而 complete。Completion gate 必须检查：

- 最新 definition revision 的所有 required coverage item 均 satisfied 或被显式 governed waived。
- 每个 satisfied item 都有 accepted evidence refs。
- 必需的 state_change、validation、assessment、artifact、approval evidence 均满足 policy。
- 没有 open decision/approval/blocker。
- 没有未处理的 rejected diagnostic 仍影响当前 focus。
- final artifact/result projection 已刷新并可追溯。

如果 coverage 大但已充分证据覆盖，不应被简单归类为 large DoD 阻塞。大 DoD 只影响切片策略，不影响 completion gate 的确定性判定。

## Migration Scope

后续代码收口应按以下范围逐项推进。

### Phase 1: Contract Audit And Model Alignment

- 梳理 Goal/DoD definition revision、coverage item、evidence record、NextAction、Stage Packet 当前模型差距。
- 明确每个字段 owner、repository、projection 和 EventHub event。
- 搜索并移除 core 中模板、语言、命令、文件名、provider、live workspace 特判。

### Phase 2: Coverage/Evidence Ledger Authority

- 确保 accepted/rejected evidence 是 coverage 变化唯一依据。
- 修正 state_change、validation、assessment 互相误满足的问题。
- 为 rejected diagnostic evidence 建立不污染 accepted ledger 的路径。

### Phase 3: Deterministic NextAction

- 将 resume/replan/restart/provider retry/contract retry/repair/follow-up 统一映射到 NextAction。
- 保证每个 NextAction 可从 ledger 重建并具备 idempotency key。
- 避免 finalize/projection/TUI 同步递归触发 runtime step。

### Phase 4: Stage Packet And LLM Context

- 每个 LLM step input 使用 Stage Packet 渲染。
- 修正 C0/C1/C2/C3/C4/C5 分区，避免动态字段污染稳定前缀。
- 针对快照截断、历史 compact、memory recall 建立准入和引用规则。

### Phase 5: EventHub Command Main Path

- TUI/Web/REST/embedded backend 全部只投递 scoped command。
- workorchd owner 记录 command accepted/rejected projection。
- scheduler queue 统一处理 resume/replan/restart/retry。
- 排查并删除依赖补偿恢复掩盖主路径缺陷的逻辑。

### Phase 6: Projection And TUI/Web

- workorchd 输出统一 WorkspaceViewModel revision。
- TUI prompt、主视图、workspace 列表、状态栏来自同一 projection。
- SSE 断线恢复、last event id、白屏错误和 incomplete chunked encoding 进入 projection/transport 测试。

### Phase 7: Watchdog And IO Budget

- 限定 watchdog 为外部调用健康检查和幂等补偿。
- 删除或降频非必要周期扫描。
- 将 history、snapshot、projection rebuild 改为增量和按需。

### Phase 8: Template/Profile/Soul Strategy

- 把软件开发阶段策略、验证偏好、文件粒度、MCP/LSP 使用说明放入 template/profile/Soul/skill。
- 确认策略在 workspace 创建期绑定，并进入 run executionContext。
- Core 只通过结构化 policy 解释策略。

## Verification Plan

代码收口完成后，至少需要以下验证：

1. 单测覆盖 Goal/DoD revision、coverage ledger、evidence accepted/rejected、completion gate。
2. 单测覆盖 NextAction 推导：首次 resume、replan、restart、contract violation retry、provider retry、approval resolve、decision resolve。
3. 单测覆盖 Stage Packet 渲染：关键 Goal/DoD、requiredOutput、accepted/rejected evidence、current failure、target snapshot 不丢失。
4. LLM context/cache 测试证明动态 repair/retry/current failure 不改变 C0 stable prefix。
5. EventHub 测试证明 TUI/Web command 通过 owner state 和 scheduler queue 推进，不同步等待同 lane。
6. Projection 测试证明 prompt、workspace list、main view、status bar 来自同一 ViewModel revision。
7. IO 回归证明 view refresh、history access、workspace execution 不持续全量扫描。
8. e2e 覆盖双 workspace 并发、decision replan、approval approve、service restart recoverable、SSE reconnect。
9. live verification 记录 workspaceId、runId、command time、projection revision、LLM request hash、accepted/rejected evidence refs、最终状态。
10. dispatch authority live verification 至少观察两个 workspace，并区分
    `workspaceProgress` 与 `goalConvergence`：in-flight 观测只要求 accepted
    evidence、正确治理阻塞或 active progress heartbeat；最终 Goal/DoD completion
    需要显式开启更严格 gate。

## Acceptance Criteria

本文对应收口完成的判定标准：

- 任何 workspace 执行都能从 Goal/DoD definition revision 推导 coverage ledger。
- resume/replan/restart 都进入 EventHub command 主路径，并能在 projection 中观察 accepted command 与后续 NextAction。
- 每个 LLM step input 都包含当前 Stage Packet，且不依赖旧历史自然语言来恢复关键状态。
- 每个 LLM/runtime dispatch 前都有 provider-before-dispatch integrity gate，证明
  current focus、NextAction、Stage Packet、requiredOutput 和 convergenceContract 对齐。
- LLM 违例输出只成为 rejected diagnostic evidence，可恢复时 bounded retry，不可恢复时治理升级。
- accepted evidence 是 coverage satisfied 的唯一依据。
- TUI/Web 不再出现本地状态和 workorchd 投影不一致的 prompt/main view/workspace list。
- 多 workspace 并发不会因同 lane 同步等待、全局锁、全量扫描或无 scope command 互相阻塞。
- Core 目录中没有新增模板、业务、语言、命令、provider 或 live workspace 特判。
- 软件开发相关推进策略能追溯到 workspaceTemplate/profile/Soul/policy/skill 绑定。

## Anti-Patterns

后续收口禁止以下做法：

- 在 core 中根据模板名、文件名、命令输出、provider 名称或 workspace title 分支。
- 用更多自然语言 prompt 替代 coverage/evidence/NextAction 结构化状态。
- 把 LLM 违例输出作为 accepted evidence 或长期 memory。
- 把 old graph、old decision、old memory 当作当前权威合同。
- 用 watchdog 反复补偿主路径未投递的 command。
- TUI/Web 自行推断 decision/running/approval 状态。
- 每轮 LLM 请求全量读取所有历史事件、文件和 artifact。
- 使用被截断快照要求生成完整文件或精确 patch。
- 对同一阻塞条件连续局部打补丁，而不回到完整合同审计。

## Open Items

后续实现前仍需逐项确认：

- CoverageLedger 与 EvidenceLedger 的最终存储 owner 和 repository 边界。
- StagePacket 是独立持久化对象，还是由 run checkpoint + ledger + projection 动态重建。
- NextAction 是否需要独立 event 类型，或作为 scheduler command payload。
- `resume` canonical 化后，REST/TUI/CLI 中旧 `continue` 的清理范围。
- service restart 后 recoverable state 的 projection 命名和用户触发语义。
- watchdog 健康检查预算、外部调用分类和幂等补偿 key。
- software-dev template/profile 当前策略字段与现有 souls/templates 的映射方式。

## Contract Audit Template For Future Changes

任何后续代码修改前，应在对应任务文档或提交说明中填写：

```markdown
## Contract Audit

- Scope:
- Observed state:
- Expected state:
- Deviation:
- Owner:
- Event path:
- Ledger impact:
- Stage Packet impact:
- Contract gap:
- Proposed change:
- Boundary check:
- Verification:
```

如果不能说明 `Owner`、`Event path`、`Ledger impact`、`Contract gap` 和 `Verification`，不得继续修改代码。
