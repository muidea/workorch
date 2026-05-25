# WorkOrch Unified Runtime Design Contract

本文定义 WorkOrch 面向长流程任务的唯一核心运行设计合约。目标是解决反复单点修复后仍无法稳定推进的问题，同时明确核心框架必须保持功能中立：runtime 只能治理阶段、约束、证据、结果、上下文和恢复策略，不得内置软件开发、代码 patch、具体技术栈或具体业务领域规则。

## 权威合约声明

本文是后续 WorkOrch 功能开发、runtime 完善、能力扩展和缺陷收口的唯一核心设计合约。任何涉及 WorkOrch runtime、orchestrator、LLM 执行、上下文组织、memory / recall、soul / skill / tool / mcp 能力选择、repair / replan、verification / acceptance、profile 映射的变更，都必须先核对本文。

从本文生效后，`docs/` 中不再保留第二份 runtime core / LLM context / repair / replan 补充合约。历史专项文档中的合理内容必须先抽象为本文中的中立规则，才能作为后续实现依据。若任何旧计划、历史收口文档、专题文档或代码注释与本文冲突，以本文为准。

后续实现必须满足：

- 不得突破本文定义的核心中立原则。
- 不得在 core runtime 中引入软件开发、代码 patch、测试框架、具体业务领域等特化逻辑。
- 新增或调整 runtime 流程时，必须能映射到本文定义的中立 stage、artifact、evidence、result、repair、replan 合同。
- 旧文档、历史计划、局部收口方案不得作为本文之外的并列约束；需要保留的合理内容应先抽象为中立规则并入本文，或作为 profile / capability 文档的非核心实现说明。
- 后续所有针对 WorkOrch 功能的开发完善，都必须在设计、实现和验证三个环节显式参照本文。
- Runtime core flow、LLM context integrity、neutral repair grounding、provider retry、readable ID / lineage、Goal / DoD change impact、result evidence strictness、rejected result continuation 等规则均已并入本文。后续不得再新增分散的 runtime 补充合约；如确需新增专题文档，只能作为 profile / capability / implementation note，并且不得改变本文的核心约束。

## 背景

近期多个 workspace 的执行核对显示，runtime 已经能够治理 provider retry、runtime repair、result policy、readable ID、state-changing repair 等底层问题，但在软件开发这类长流程任务中仍会出现持续循环：

- LLM 在尚未完成上下文理解时被过早要求输出 patch。
- 只读探索 graph 被统一视为实现阶段违规，导致 repair / replan 反复触发。
- runtime 对“应该开放判断”与“必须严格收敛”的阶段边界不够清晰。
- memory / recall / skill / tool / mcp / soul / prompt 已被接入，但缺少按阶段组织的输入合同，无法稳定为 LLM 提供决策所需信息。
- 修复容易滑向“为软件开发写死特殊逻辑”，破坏 WorkOrch 核心中立性。

因此后续不应继续围绕单个失败点打补丁，而应把 runtime 收口为“中立阶段状态机 + 阶段内 LLM 决策”的模式。

## 设计原则

### 核心中立

WorkOrch core runtime 只理解以下中立概念：

- workspace、run、step、stage、loop、attempt。
- Goal、DoD、input、artifact、evidence、result。
- stage contract、allowed output、forbidden output。
- state-changing、read-only、assessment-only、approval、decision。
- provider retry、runtime repair、replan、terminal result。
- memory recall / write 的价值等级与引用关系。

WorkOrch core runtime 不得理解以下领域概念：

- 什么是软件开发、代码实现、测试框架、语言栈、仓库结构。
- 什么文件必须创建或修改。
- 什么 patch 内容代表正确业务实现。
- 某类业务任务应使用何种技术方案。
- 某个领域的最佳实践或专用流程。

### 约束分层

WorkOrch 的阶段约束分三层：

| 层 | 职责 | 是否属于 core |
| --- | --- | --- |
| Runtime core | 阶段状态机、输出合约、恢复治理、证据/result、ID/lineage、memory 生命周期 | 是 |
| Capability / profile | 某类任务的 artifact 类型、tool/skill/mcp/soul/prompt 组合、阶段映射 | 否，属于可插拔能力层 |
| LLM decision | 在当前 stage contract 内选择方案、生成内容、解释缺口 | 否，由 LLM 执行 |

软件开发只能作为一个 profile 映射到中立阶段，不能成为 core runtime 的内置逻辑。

## 阶段模型

Runtime 统一使用中立阶段，不使用业务阶段名称作为核心状态。

| Stage | 目的 | 约束模式 | 允许输出 | 禁止输出 |
| --- | --- | --- | --- | --- |
| `exploration` | 收集上下文、识别问题、发现约束 | 开放判断 | read-only graph、tool read、context summary、candidate findings、blocker | 写 result 完成、无依据 state-changing、无限探索 |
| `planning` | 将 Goal / DoD 拆成可执行单元 | 半开放 | plan、implementation units、acceptance criteria、risk/blocker、stage transition proposal | 直接完成 run、绕过 DoD、空计划 |
| `execution` | 产生状态变更或明确无法变更 | 严格收敛 | state-changing artifact、tool action、patch-capable output、failed assessment、decision blocker | 只读 graph 循环、下钻 graph、placeholder output |
| `verification` | 用证据验证执行结果 | 严格证据 | verification tool result、evidence refs、failed/skipped/inconclusive assessment | 无证据 accepted、继续实现、弱证据通过 |
| `acceptance` | 对照 Goal / DoD 判定完成度 | 严格 result | accepted/rejected result、DoD coverage、evidence mapping | 新增实现、模糊完成、缺 result 终态 |
| `repair` | 针对失败证据做定向修复 | 继承原约束并收紧 | targeted correction、state-changing repair、failed assessment、blocker | 广泛重规划、降级为探索、重复同一失败 |
| `replan` | 原计划不成立时重新拆解 | 受控开放 | successor run proposal、new units、discard/keep rationale | 在旧失败路径继续循环、丢失 lineage |

### Acceptance Assessment 归一化

Acceptance 阶段允许 runtime 做领域中立的结构归一化：当 LLM 已声明 `dodAssessment` satisfied，但遗漏子项 `state` 或 `evidenceRefs` 时，runtime 可以从当前 run 已完成、非弱证据、可追溯的 shell/tool/patch/validation evidence 中补齐引用。该能力只修正结构缺口，不判断具体业务是否正确；若没有可用证据，或证据属于 read-only、zero-work、inconclusive、invalid，则必须保持失败并进入 repair / rejected result / decision 收口。

聚合命令的 evidence quality 判定必须支持正反两类 profile 配置：`zeroWorkMarkers` 表示弱证据信号，`workEvidenceMarkers` 表示同一输出中已经存在真实工作证据。当两类标记同时出现时，runtime 只能按通用优先级判定为非 zero-work，不能因为某个子目标或子包输出局部 zero-work 标记而否定整条聚合命令的有效工作证据。具体 marker 内容仍属于 profile/template 数据，core 不内置语言、测试框架或业务领域规则。

### Claim-To-Evidence Alignment

当 profile 声明 `requireClaimEvidence` / `requireClaimEvidenceAlignment` 时，runtime 必须对 satisfied 的 DoD item、Goal coverage item、acceptance dimension 或 deliverable coverage item 执行中立的 claim-to-evidence alignment 门禁。

该门禁只比较“被声明 satisfied 的条目文本”与“引用 evidence step 的中立证据文本”，不得理解业务领域含义。可接受的对齐信号包括：

- evidence step 的 `coverageTargets` / `coverageTarget` / `targetCoverage` 直接包含该 claim 或与其规范化文本匹配。
- evidence step 的输入、输出、step id、step kind 或 validation 摘要中包含足够的 claim 关键词。
- profile 明确声明的其他中立 evidence 字段。

该门禁用于阻止“泛化 build/test 通过”替代细粒度行为证据。例如某个 DoD 声称流式行为、错误路径、配置路径、持久化副作用或并发行为 satisfied，但 evidence 只引用一个没有对应测试名、输出、coverage target 或断言说明的总体验证命令时，runtime 必须拒绝该 satisfied assessment，并生成可 repair 的 evidence gap。是否把这条规则启用、应用到哪些 scope、接受哪些 step kind，必须由 profile/template 配置；core 只执行结构化对齐，不内置软件开发、测试框架、协议或具体业务规则。

若 claim 只是短标题、章节名或缺少足够可匹配信号，runtime 可以跳过 alignment 门禁，但仍必须执行 evidenceRefs、step kind、evidence quality 和 current-run evidence 等基础门禁。该例外只能防止标题型文本误伤，不能让具体行为 claim 绕过直接证据要求。

## 阶段转换合同

### 标准路径

```text
created
  -> exploration
  -> planning
  -> execution
  -> verification
  -> acceptance
  -> completed | failed | blocked_on_decision
```

### 恢复路径

```text
execution | verification | acceptance
  -> repair
  -> execution | verification | acceptance

repair exhausted | plan invalid | repeated failure
  -> replan
  -> successor run
```

### 转换规则

| From | To | 必要条件 |
| --- | --- | --- |
| `exploration` | `planning` | 已形成足够上下文摘要，或达到探索预算 |
| `planning` | `execution` | 已产生至少一个可执行 unit，且 unit 绑定 Goal / DoD 覆盖点 |
| `execution` | `verification` | 已产生 state-changing artifact，或明确 failed/skipped 并可验证 |
| `verification` | `acceptance` | 已有可引用 evidence，且覆盖目标可判定 |
| `acceptance` | `completed` | accepted result，Goal / DoD coverage 完整 |
| `acceptance` | `failed` | rejected result，缺口不可自动修复或预算耗尽 |
| 任意阶段 | `blocked_on_decision` | 需要人工信息、审批、provider/config 修复或外部依赖 |
| `repair` | 原阶段 | 修复输出满足原阶段 contract |
| `repair` | `replan` | 同一 failure scope 重复失败或 repair budget exhausted |

## 全局收敛合同

Runtime 必须把跨阶段、跨恢复路径的收敛约束实现为一条统一的中立 `ConvergenceContract`，而不是在 planning、graph follow-up、decision recovery、repair-of-repair、validation 和 replan 中分别拼接局部字段。该合同的目标是：只要当前 run 存在未关闭失败，且 runtime 已掌握 concrete target evidence，就持续维持可行动的 strict repair 合同，直到状态变更和验证成功、证据证明无法自动修复，或确实缺少外部决策/审批/配置。

统一合同构造应等价于以下中立接口：

```text
BuildConvergenceContract(run, currentStep, failedStep, lineage, evidence) -> ConvergenceContract
ApplyConvergenceContract(input, contract) -> input
ShouldCreateDecisionTicket(input, llmOutput, contract) -> bool
```

`BuildConvergenceContract` 只判断中立 runtime 事实，不判断语言、框架、业务领域或具体 Goal 语义。它必须输出以下状态之一：

| State | 触发条件 | LLM 合同 |
| --- | --- | --- |
| `need_evidence` | 未关闭失败存在，但缺少 concrete target evidence | `runtimeStage=repair`，`repairObjective=acquire_concrete_failure_evidence`，`concreteFailureEvidenceRequired=true`，只允许有界 `file/list` / `file/read` / `shell/command` / `mcp/tool_call` 或等价取证能力 |
| `need_state_change` | 未关闭失败存在，且已有 target refs / line refs / snapshots / patch repair context / coverage target 等 concrete evidence | `runtimeStage=repair`，`runtimeStageMode=strict_convergence`，`repairObjective=produce_state_changing_output`，`stateChangingOutputRequired=true`，`repairActionPackage.requiredOutput.acceptedKinds=["patch/apply"]` |
| `need_validation` | 当前 state-changing artifact 已应用，下一步需要证明结果 | `runtimeStage=verification`，只允许 bounded validation evidence graph；不得返回新的 LLM 嵌套或只读自检 graph |
| `need_assessment` | 验证证据足以进入 DoD / Goal 覆盖判断，或 assessment-only contract correction | `runtimeStage=validation` 或 assessment-only correction，只允许 `dodAssessment` / coverage correction |
| `need_governance` | 真实缺少外部信息、权限、凭证、配置、审批、破坏性变更授权或无法自动获取的业务决策 | 创建 decision / approval ticket，并写明 missing field、source refs 与无法自动获取原因 |
| `blocked` | 证据表明不可自动修复、取证失败且无可继续路径、预算耗尽或必须失败收口 | failed assessment / rejected result / blocker |

合同来源必须按以下优先级合并，低优先级只能补充背景，不得覆盖高优先级：

1. 当前 failed step 的 `runtimeDiagnostics.targetRef`、`failureSignature`、`failedStepOutput`、stdout/stderr/error、line refs。
2. 当前 failed state-changing artifact 的 `patches[]`、`targetRef`、`baseHash`、`patchRepairContext`、`patchRepairSuggestions`、`currentTargetAlreadySatisfiesFailedPatch`。
3. 当前 graph evidence 中的 target refs、line refs、file/list manifest entries、file/read snapshots、capability output。
4. 当前 run 的最新非 superseded failure / closure evidence。
5. predecessor run 中仍未关闭的 original failure target、target snapshot、coverage target 或 repairActionPackage。
6. 已解决 decision 的 resolution 语义。该语义只能作为继续/拒绝/重规划等治理信号，不能替代当前 target evidence，也不能把已具备机械证据的场景重新降级为人工决策。
7. memory、governance history、旧 run、旧 rejected result。它们只能作为 historicalSignals，不能覆盖当前 `currentProgressDigest`、`repairActionPackage` 或 target snapshot。

以下路径必须全部调用同一 ConvergenceContract 构造与应用逻辑，禁止各自维护互相不一致的 repair 字段：

- 初始 planning 后进入 execution。
- graph node failed 后进入 repair。
- evidence acquisition graph completed 后进入 follow-up。
- `blocked_on_decision` / `llm_decision_required` resolved 后恢复执行。
- LLM schema invalid / decision invalid / provider-normalized output invalid 后进入 repair。
- `patch/apply`、`file/edit` 或等价 state-changing artifact 失败后进入 repair-of-repair。
- `shell/command`、`mcp/tool_call` 或等价 validation/action evidence 失败后进入 repair。
- replan successor 继承 predecessor unresolved failure。
- validation / DoD / Goal coverage 发现缺口后进入 evidence repair、assessment correction 或 state-changing repair。
- duplicate failure、retry exhausted、budget exhausted 后进入 governance、failed assessment 或 bounded replan。

`ApplyConvergenceContract` 必须保证发送给 LLM 的上下文自洽：

- 若 `currentProgressDigest.currentGap` 是 patch/apply、file/edit、state-changing artifact 或 validation failure，则 `currentRuntimeStage` 不得被错误标记为普通 `verification` 或 `decision`，除非合同状态明确是 `need_validation` / `need_governance`。
- 若合同状态是 `need_state_change`，输入必须包含 `repairActionPackage`，并至少包含一个通过准入的 concrete target evidence：`repairTargetSet`、`structuredFailureSignals.targetRefs` / `lineRefs`、`targetSnapshots`、`patchRepairContext`、`failureTargetRepairContext`、`coverageTargetSet` 或等价 capability evidence。
- 若合同状态是 `need_evidence`，输入必须移除顶层 `stateChangingOutputRequired`，并把 `repairActionPackage.requiredOutput.acceptedKinds` 限定为 evidence acquisition 能力。
- 若合同状态是 `need_validation`，输入必须说明最近 state-changing artifact、目标 scope、bounded verification 条件和禁止的 read-only/self-inspection-only 输出。
- `currentProgressDigest`、`processEvidenceTimeline`、`repairActionPackage`、`structuredFailureSignals`、`latestClosureEvidence` 的当前事实必须一致；历史 wrapper/schema/provider 失败只能作为历史诊断摘要。
- 当最新 concrete failure 被包裹在 follow-up / wrapper LLM failure 后面时，runtime 必须从 `currentExecutionProgress.latestActionEvidence` / `completedGraphEvidence` 的 stdout、stderr、error、targetRef、line refs 或 failure signature 中提取中立 target evidence。若可机械提取出 target refs / line refs / failure target scope，则该 evidence 必须提升为当前 `currentGap` 与 `repairActionPackage.structuredFailureSignals`，并恢复 `need_state_change`；不得因为 wrapper failure 本身是 read-only graph、schema invalid 或 dodAssessment failed，就退回 `need_evidence`。
- `latest_successful_action_evidence` 只能作为已完成动作或候选证据引用，不得被提升为 `currentFailure`、`lastObservedFailure` 或 failure target scope。只有 step 自身失败，或 completed action evidence 被明确标记为 `evidenceValidity=actionable_failure` / 输出中存在可机械识别的失败标记时，才允许覆盖旧失败成为当前失败。
- `repairActionPackage.currentFailure` 与 `repairActionPackage.currentExecutionProgress` 是 canonical repair context 的组成部分。若顶层 `currentFailure` / `currentExecutionProgress` 缺失但 package 内存在有效字段，runtime 在 provider context gate 前必须提升或等价识别这些字段；不得误报 `runtime_repair_current_failure_missing`。
- `shell/command` 的可行动失败不能只依赖进程退出码。若命令因管道、截断、wrapper 或 shell 组合返回 `exitCode=0`，但 stdout / stderr 中存在明确失败标记、断言失败、文件行号或测试失败摘要，`processEvidenceTimeline` 与 `currentProgressDigest` 必须把它记录为 actionable validation failure，并作为下一轮 repair 的当前 gap。
- `repairActionPackage` 是唯一 primary actionable contract；不得让 `governanceHistory`、memory、旧 `executionGraph`、旧 rejected patch 或旧 decision question 与其竞争。

`ShouldCreateDecisionTicket` 必须作为所有 decision ticket 写入前的统一门禁。若当前合同状态是 `need_state_change`，并且已经具备 concrete target evidence、required output 允许 state-changing artifact、缺口不涉及外部权限/凭证/部署/审批/破坏性变更/无法读取的业务信息，则 LLM 返回的 `blocked_on_decision` 必须被转换为 continuable repair contract violation，而不是直接创建人工 ticket。Runtime 应记录 converted decision 摘要，并继续派发 repair 或 bounded validation。只有合同状态为 `need_governance`，或证据门禁明确缺少 runtime 无法自行获得的信息时，才允许创建 ticket。

若 validation / completion follow-up 已输出 failed `dodAssessment`，并且 assessment 中存在可机械映射的 DoD / Goal / deliverable coverage gap、nextActions、evidence candidates 或 coverage target set，则该输出属于 `need_validation` / `need_state_change` 的可继续收口合同。即使 LLM 同时返回 `llm_decision_required` / `blocked_on_decision`，runtime 也必须先转换为 runtime repair follow-up；只有缺口明确需要外部信息、权限、凭证、配置、审批或无法自动读取的业务事实时，才允许创建 decision ticket。

`patch/apply`、`file/edit` 或等价 state-changing artifact 失败的 repair-of-repair 必须遵守以下收敛规则：

- 若失败输出包含 `runtimeDiagnostics.targetRef`、失败 artifact 的 `targetRef`、`baseHash`、`patchRepairContext` 或当前目标快照，下一轮必须保持 `need_state_change` 或在 `currentTargetAlreadySatisfiesFailedPatch=true` 时进入 `need_validation`，不得退化为普通 `verification` 或泛化 decision。
- `unified_diff_hunk_context_mismatch`、oldText conflict、baseHash mismatch、target snapshot drift 等只说明 state-changing artifact 与当前目标不同步；runtime 应要求基于当前 snapshot / contentHash 重建最小 artifact，或在已满足失败 patch 意图时要求 bounded validation。
- 失败 patch 原文、stale diff、redacted partial output、malformed JSON 只能作为 rejected output summary；不得作为可复制 repair 输入。
- 如果失败 artifact 已经部分生效，后续合同必须体现 `currentTargetAlreadySatisfiesFailedPatch`、`latestClosureEvidence` 或新的 validation failure，避免重复同一 stale patch。

Resolved decision 恢复必须遵守以下规则：

- `continue`、`继续处理`、`approve_execution` 等治理恢复信号只表示允许运行继续，不表示需要重新询问，也不表示可以清空 repair contract。
- 恢复后若原阻塞已具备 concrete target evidence，应立即回到 `need_state_change` 或 `need_validation`，并继承原始 failure target、repairActionPackage、graph evidence、当前 snapshots 与 rejected output summary。
- 若 decision resolution 提供了真实外部信息，该信息应作为 `decisionResolutionEvidence` 合并进合同；但仍必须经过 target/evidence 准入，不能直接写入业务特定修复策略。
- 若 decision resolution 只是“继续处理”且没有提供新增事实，runtime 不得把缺失字段继续视为 missing human field；应根据已有 evidence 判断是否可机械推进。

该合同的回归测试必须覆盖以下矩阵：

- evidence acquisition graph 通过 file/read snapshot 完成后恢复 `need_state_change`。
- evidence acquisition graph 通过 file/list + shell stdout/stderr line refs 完成后恢复 `need_state_change`。
- evidence acquisition 无 target evidence 时保持 `need_evidence`，不得伪造 patch 合同。
- decision resolved 后，在已有 target evidence 的情况下恢复 state-changing repair 合同。
- patch/apply hunk mismatch / oldText conflict 后，基于 targetRef 和 patchRepairContext 恢复 repairActionPackage。
- failed patch 已经由当前 target 机械满足时，转入 bounded `need_validation`，不得重复 stale patch。
- validation shell failure 带 targetRef / lineRef 时不得创建 decision，而应进入 `need_state_change`。
- LLM 在 state-changing repair 中返回 blocked_on_decision，但 context 未声明外部依赖时，转换为 repair contract violation。
- replan successor 能继承 predecessor unresolved repair contract；若只剩 wrapper failure，则进入 `need_evidence`。
- assessment-only coverage correction 与 state-changing repair 不互相污染。
- memory / governance history / old rejected result 不得覆盖当前 repairActionPackage。

### Failure Scope 与收敛预算

Runtime repair / duplicate / replan 治理必须同时识别两类中立 scope：

- 完整失败签名：用于精确区分同一目标下的不同失败形态。
- 目标级失败签名：从 `failureTargets`、`targetRef`、`conflictRef`、`serverId`、`tool`、`uri` 等中立键提取，用于识别同一失败目标在不同 step、graph node、loop、attempt 中反复出现。

当失败同时包含 `serverId` 与 `tool` 时，目标级 scope 应组合为同一外部能力目标，例如 `serverId=<id>|tool=<name>`；只有缺少其中一项时才退化为单独的 `serverId` 或 `tool`。这仍然是能力目录层的中立引用，不得解释为某个具体业务服务。

当失败签名包含目标级 scope 时，node 名称、step ID、loop 编号或 graph 编号变化不得重置 repair budget。若同一 failure class + target scope 已达到 repair budget，runtime 必须进入 repair exhausted / duplicate / replan 治理，而不是继续派发普通 repair。

目标级 scope 不得跨越当前有效 artifact / graph root 窗口无限合并。若新的 state-changing graph、repair graph 或 successor root 已经重新写入同一 target，后续 build/test/verification 失败应优先按当前 graph root / recovery root 计算 repair budget；历史同 target 失败只能作为上下文证据和风险信号，不得直接耗尽新 artifact 版本的修复机会。

若较晚的 completed step 在同一中立 scope 内已经给出最新成功证据，较早的 failed graph step 必须视为 superseded，不得继续作为 repair target 或重复触发 ticket；runtime 应优先使用最新成功证据校正 completion/repair 视图。

外部能力依赖失败是例外中的中立子类：当 `mcp/tool_call` 或等价外部 capability 对同一 `failure class + target scope` 反复返回不可用、未找到、权限拒绝、配置缺失等确定性失败时，新的 graph root 或 loop 不应把该外部依赖预算重置为 0。runtime 应收敛到 rejected result、blocked_on_decision 或 replan，而不是继续要求 LLM 生成同一能力调用。是否换用其他能力、补充配置或继续重试，必须通过 capability/profile 或治理决策表达，core 不得内置具体服务名、工具名或业务 fallback。

当失败签名不包含目标级 scope 时，runtime 不得臆造业务目标，也不得把无目标失败合并为同一预算；此时继续按完整失败签名、root step 或显式 lineage scope 收敛。

当当前失败 step 同时携带历史 `repairOfFailureSignature` 和自身 `failureSignature` 时，scope 判定必须优先使用当前失败 step 输出中的 `failureSignature` / `failureClass`。历史 repair lineage 只能作为上下文与来源说明，不得覆盖当前失败证据。真正的 provider/runtime 调度外壳若只记录了 provider retry 状态且没有新的有效失败签名，可以继续按原始 repair scope 计数；除此以外，repair-follow-up 自身产生的 runtime contract violation、patch contract violation、验证失败或能力失败都必须作为当前失败证据进入下一轮治理。该规则适用于所有中立失败类型，尤其用于避免“上一轮失败签名”串线到新的执行超时、验证失败或能力 contract violation。

若当前失败签名包含短目标引用，runtime 可以用当前 workspace manifest 做唯一后缀解析，以便把失败证据定位到实际资源；若存在多个候选，必须保留原始短引用并标记为未确定目标，不得猜测具体资源。该解析只基于 workspace 中已知资源路径这一中立事实，不得依赖文件类型、语言、测试框架或业务约定。

### Replan Successor Lineage

当 repair exhausted、duplicate failure 或人工/自动治理决策要求 `replan` 时，successor run 必须继承 predecessor 的 Goal / DoD / execution contract，并在 `replanContinuation` 中记录中立 lineage：

- predecessor run ID。
- 触发 replan 的 ticket / resolution 引用。
- ticket reason 与 resolution payload。
- 触发 replan 的 failure context，包括 failure class、failure signature、failure target scope、failed step id/kind、failed step output summary、最近 repair attempts 或可用摘要。
- predecessor run 的中立过程证据摘要，包括 `processEvidenceTimeline` 与 `currentProgressDigest`。该摘要必须保留最近有效动作、最近失败、当前缺口和下一步要求，且每条 evidence 必须带 `sourceRef` / step ref。

若本次 activation 同时检测到 `definitionChangeImpact.changed=true`，说明 effective Goal / DoD 已发生新增、补充、删除或同 ID 修订。此时旧 `governanceHistory` 仍可作为 summary/ref-only history 进入上下文，但旧 `replanContinuation` 与旧 `rejectedResultContinuation` 不得再作为 actionable continuation 注入新 run。runtime 必须把本轮视为基于最新 definition 的新规划 / 新验收基准，先重新做 gap analysis、目标覆盖判断与必要的执行计划选择；旧 repair target、旧 strategy reassessment、旧 rejected result required evidence 只能作为候选历史证据，不能驱动 initial step 进入 repair continuation。

当定义变更压过旧 continuation 时，activation context 应显式记录中立策略标记，例如 `definitionChangeContinuationPolicy=definition_change_supersedes_prior_actionable_continuation`。该标记只表达运行态优先级，不解释任何业务语义；它用于审计为什么旧治理决议没有被提升为当前可执行恢复合同。

已 resolved 的 `replan` 决策是 run 级恢复信号。若 run 仍处于 `running` 且没有 active blocker，runtime resume 不得只检查当前 terminal step 是否直接绑定该 ticket；当 resolved replan ticket 绑定的 step 仍是当前 run 内的 failed step 时，即使后续存在同 lineage 的并行失败分支、provider retry 分支或辅助失败步骤，也必须创建 successor run 或显式终态化，不得保持 `running` 空转。

Successor 的首个 LLM step 不应只知道“需要重规划”。它必须获得足够的失败证据来判断哪些产物可以保留、哪些路径已失败、哪些 contract 仍然必须继承。若 successor 被设为 state-changing repair continuation，则其初始输入必须继续携带 `stateChangingOutputRequired`、`repairObjective` 与对应 guidance；同时不得把历史 provider/model 等运行期动态绑定写入 successor 的恢复上下文。

若 successor run 刚创建、当前 run 尚无 completed / failed step，`executionContextPackage` 必须从 `replanContinuation` 继承 predecessor 的 `processEvidenceTimeline` / `currentProgressDigest`。不得因为 successor run 本地历史为空，就把首个 repair LLM step 降级为无过程证据的普通重新规划。

若同一 successor run 同时存在 `rejectedResultContinuation` 与旧 `replanContinuation`，`rejectedResultContinuation` 表示最新未接受结果，必须优先生成当前 `processEvidenceTimeline` / `currentProgressDigest`。旧 `replanContinuation` 只能作为 historical-only lineage 保留，不得继续驱动 `currentProgressDigest.currentGap`、`repairActionPackage.currentFailure`、evidence follow-up 的 `currentExecutionProgress` 或下一步 target 选择；否则会让旧 patch/runtime failure 覆盖最新 rejected result required evidence。

`rejectedResultContinuation.requiredEvidence` 生成的 `currentProgressDigest.lastObservedFailure` 应标记为中立 `evidenceValidity=actionable_gap`。该标记与 `actionable_failure` 一样可以提升为当前 repair/follow-up 的 primary actionable contract，但它表达的是“验收缺口仍需补证/补改”，不是某个具体业务失败。runtime 在后续 repair、context gate retry、concrete evidence acquisition follow-up 中必须优先使用该 actionable gap；任何 `historicalOnly=true` 或 `useAsCurrentProgress=false` 的 replan continuation 不得被重新提升为当前失败或当前进展。

Initial step 生成也必须遵守同一优先级。若新 run 的 execution context 同时携带 `rejectedResultContinuation` 与 `replanContinuation`，即使 execution contract 要求 state-changing output，也不得先由旧 replan 构造 `replanStateChangingContinuation`、`repairActionPackage.targetSnapshots` 或 `strategyReassessment`。runtime 应先基于 rejected result 的 required evidence 进入 evidence acquisition / actionable gap 收口；旧 replan 的 target、patch mismatch、strategy reassessment 只能作为 historical lineage，除非当前 rejected result 已被关闭或显式移除。

若旧版本或恢复路径已经基于 historical replan 派发了 repair step，并且该 step 后续失败，runtime repair follow-up 必须识别其输入中的 `historicalOnly` replan 与仍有效的 `rejectedResultContinuation`。这类失败不得继续把 historical replan target 提升为当前 `repairActionPackage`；应把该失败记录为 `historicalRepairAttempt`，并把当前合同重置为 rejected result 的 `actionable_gap` / evidence acquisition，直到 rejected result required evidence 被新的当前证据关闭。

`governanceHistory`、`replanContinuation.failureContext`、`replanContinuation.ticketContext`、`replanContinuation.processEvidenceTimeline` 必须是 summary/ref-only lineage。允许保留 failure class、signature、target scope、failed step id/kind、attempts、recovery policy、failed step output summary、target snapshot metadata、ticket/resolution refs、人工决议摘要与过程证据摘要；禁止携带 `failedStepInput`、完整 `executionContext`、完整 `executionGraph.nodes[].input.diff/content`、raw LLM stream、raw preview、provider/model 动态绑定、target snapshot `content`、历史 step output 的 `content` / `patches[].content` / `patches[].diff`。若需要完整证据，必须通过 step/tool/LLM raw ref 按需读取。该规则是核心中立约束：它只约束运行态数据形态，不得引入具体应用领域判断。

### Rejected Result Continuation

当 workspace 已无 active run，且同一 workspace / effective Goal / effective DoD 的最新终态 result 为 `rejected` 时，新的 activation 必须把该 result 转换为中立 `executionContext.rejectedResultContinuation`。该 continuation 只表达 predecessor run/result、DoD satisfied/unsatisfied items、coverage gaps、required evidence、evidenceRefs/artifactRefs 等结构化事实，用于下一轮继续补齐缺口。

若 workspace 已处于终态 failed 且没有 active run，新的 activation 不得尝试恢复旧终态 run，也不得只返回旧 terminal run；必须创建新的 successor run，并把最新 rejected result 作为 `rejectedResultContinuation` 注入 successor 的 execution context。旧 run 保持终态不可变，successor 通过 `predecessorRunId/resultId` 建立 lineage。

`rejectedResultContinuation` 必须遵守 summary/ref-only 规则：允许携带 `predecessorRunId`、`resultId`、`sourceType`、`sourceRef`、result summary、`goalOutcome`、`satisfiedDoDItems`、`unsatisfiedDoDItems`、`coverageGaps.missingGoalCoverage`、`coverageGaps.missingAcceptanceDimensions`、`coverageGaps.missingDeliverableCoverage`、`requiredEvidence`、`resultEvidenceRefs`、`artifactRefs`；禁止携带 provider/model 动态绑定、raw LLM stream、完整 execution graph、完整 failed step input、完整 workspace 文件内容或领域特化修复指令。

Coverage gap 只能从 `dodAssessment` 的结构化 `goalCoverage`、`acceptanceDimensions`、`deliverableCoverage` 与 result 的 `nextActions` / `residualRisks` / `knownLimitations` 抽取。Core 不得根据自然语言 summary 或某个业务 profile 的文字关键词推断缺口类型；软件开发等领域如何解释这些缺口，只能由 workspaceTemplate、profile、soul、skill 或用户 Goal / DoD 决定。

### Active Run 一致性

Workspace 的 `ActiveRunID` 只能指向同一 workspace 下的非终态 run。`completed`、`failed`、`cancelled`、`superseded` 均为终态，任何 workspace 都不得继续以这些 run 作为 active run。

Runtime 在以下边界必须执行 active-run 一致性核对：

- run 启动、恢复、完成、失败、取消、supersede 后。
- replan 创建 successor run 前后。
- 服务启动或运行恢复扫描时。
- runtime 验证脚本核对 workspace/run 执行过程时。

若发现 workspace 指向终态 run，runtime 必须清空 `ActiveRunID`，并按终态 run 状态收敛 workspace 状态：`completed` 映射为 workspace `completed`，`failed` / `cancelled` / `superseded` 映射为 workspace `failed`。若该 run 存在已解决 replan 决策，服务层恢复扫描可以在 workspace 回到无 active run 的终态后继续调度 successor，但 core runtime 不得通过业务领域逻辑推断 successor 内容。

Workspace detail、workspace list、snapshot、taskProgress 等 read model 投影必须从当前 active run 派生 blocker refs。若 active run 为 `blocked_on_decision` 或 `blocked_on_approval`，workspace 级响应必须暴露对应 `blockingTicketId` / `blockingApprovalId`，并与 active run 的 blocker 保持一致。终态 run 不得继续污染 workspace blocker 投影。

Workspace outcome 与 taskProgress 投影必须优先反映最新终态证据：当最新终态 result 的时间不早于最新 run 事件时，outcome / taskProgress 应共同指向该 result 对应 run；当存在更新的终态 run 事件但该 run 尚无 result 时，outcome / taskProgress 应指向该最新 run 及其失败/终态原因。历史 accepted / rejected result 必须继续保留在 results 列表中，但不得覆盖更新 run 的终态状态。若 snapshot / read model 中的 event records 因分页、保留窗口或恢复重放而滞后，更新的 terminal result 必须可以覆盖旧 run event，避免 workspace 已由最新 run 失败/完成但 outcome 或 taskProgress 仍投影到历史 run。

Result 事件与持久化 marker 必须表达 result 的真实状态。`accepted` result 才能生成 `event/accepted` 标识；`rejected` result 必须生成 `event/rejected` 标识，即使当前事件 topic 仍用于通知 result materialized / refresh，也不得让 rejected result 在文件系统或 read model 中留下 accepted marker。

Result summary / artifact summary 必须以 runtime 已记录事实为准进行最终校正。若同一 run 已存在已应用 patch、tool call、validation、artifact 等 state-changing 或 evidence 记录，accepted result、summaryDetail、result-summary artifact、deliverable-manifest 不得保留与这些事实矛盾的 LLM 自述，例如“仅 assessment-only”“无新增可执行操作”“不包含新的执行性修复”。Core 只允许基于中立 runtime evidence 类型做事实校正，不得推断该变更属于代码、测试、配置或任何具体业务领域；领域解释继续由 profile / template / soul 提供。

Run observability / snapshot 的 outcome 也必须遵守同一语义：`superseded` run 的 outcome 首先表达该 run 已被后续 lineage 取代，而不是继续被其历史 `rejected` result 投影为 `failed`。换言之，`rejected -> superseded` 只保留为历史证据，不再是当前 run observability 的有效终态结论。

## 阶段内约束模式

### 开放判断模式

适用于 `exploration` 和部分 `planning`。

Runtime 允许 LLM：

- 主动选择读取哪些上下文。
- 输出多个候选原因或候选方案。
- 暂不产生 state-changing artifact。
- 请求必要 tool / skill / mcp。
- 记录高价值 memory。

Runtime 必须限制：

- 探索预算。
- 重复读取同一信息。
- 无信息增量的循环。
- 在缺少 evidence 时写 accepted result。

### 严格收敛模式

适用于 `execution`、`verification`、`acceptance` 和大多数 `repair`。

Runtime 要求 LLM：

- 对当前 unit 给出可执行输出、失败 assessment 或 blocker。
- 输出必须能被 runtime 派发、验证或终态记录。
- state-changing 需求不得降级为只读 graph。
- verification 必须绑定 evidence。
- acceptance 必须绑定 Goal / DoD coverage。

Runtime 必须拒绝：

- shell-only / read-only graph 冒充实现进展。
- 下层 `executionGraph` 继续依赖 `llm/workspace_execution` 决策节点套娃。
- placeholder artifact。
- weak evidence 直接 accepted。
- assessment-only 阶段输出可执行字段。

### Goal / DoD Coverage Assessment 合同

当 acceptance / verification 需要对 Goal 与 DoD 做完成度判断时，runtime 必须把覆盖判断建模为中立 coverage 合同：

- Goal / DoD definition 的“最新”必须按同一 workspace 内 definition event 的最后一次出现确定。同一 definition ID 被多次定义或更新时，累计 definition lineage 中只能保留该 ID 的最后一次事件位置，不能把重复事件展开成 `[old,new,old]` 这类误导性顺序。若同一 ID 发生更新，`definitionChangeImpact` 必须通过 `changedInPlace=true` 与 `definitionRevisions[]` 标记该修订；不能只按 ID 数量判断是否发生定义变化。
- Goal / DoD definition 在新增或补充时必须保持结构化、可分辨。Goal 的自由文本补充必须被整理成独立段落或列表项；不得直接拼接到前一个 leaf 文本末尾。DoD 必须保持结构化 item 列表并去重。该整理只处理文本结构，不解释业务语义。
- Goal definition 输入可以是 Markdown 或普通文本：已有 Markdown 标题、列表或段落结构时，runtime 应保留结构并清理空行、行尾空白；普通文本输入应按换行、英文分号和中文分号拆成独立 `- item` 列表项。作为新增补充追加到已有 Goal 时，即使只有一句普通文本，也必须写成独立列表项。
- 当 `definitionChangeImpact.changed=true` 时，后续 planning / verification / acceptance 必须把最新 effective Goal / DoD 作为当前验收基准，并重新评估新增、删除、改写或同 ID 更新后的 requirement。已有历史 evidence 只能作为候选或 lineage；accepted result 必须明确映射最新 effective Goal / DoD，不能仅复用变更前 assessment。
- 当 `definitionChangeImpact.changed=true` 时，accepted result 不能只由历史 run 的 evidence refs 支撑。每个 satisfied 的 DoD item、Goal coverage item、required acceptance dimension 和 required deliverable coverage item，必须至少引用一条当前 run 内的可追踪 runtime evidence；历史 evidence 可以作为 lineage 保留，但不能单独关闭新增或修订后的定义。若当前 run 只完成了 assessment-only schema correction，且没有新的 state-changing、verification、tool、patch、validation 或等价 runtime evidence 证明最新定义已被重新处理，completion gate 必须拒绝该 assessment，并把缺口归类为中立 coverage/evidence gap，而不是 accepted。
- Runtime core 不得从 Goal / DoD 文本中识别或内置任何具体业务对象、供应商、协议名称、接口名称、测试框架或字段语义。Core 只能要求 coverage item、acceptance dimension、deliverable expectation 和 evidence refs 的结构完整性；具体对象如何被解释、实现或验证，必须由当前 workspace 的 Goal / DoD、profile / template / soul / skill 和 LLM assessment 动态派生。
- Goal 中被拆出的 leaf requirement 必须获得确定性、可复现的 `goalRequirementId`，ID 来源只能是 requirement 顺序与规范化文本，不得使用随机值或领域语义。
- DoD 与 Goal coverage 必须分别表达；DoD item 通过不代表 Goal leaf 自动覆盖，除非 `dodAssessment.goalCoverage.items[]` 明确给出对应 Goal requirement、状态和 evidence refs。
- profile 声明的 required `acceptanceDimensions` 与 required `deliverableExpectations` 必须分别通过 `dodAssessment.acceptanceDimensions.items[]` 与 `dodAssessment.deliverableCoverage.items[]` 表达；core 只校验声明的 `id`、状态和 evidenceRefs / artifactRefs，不得根据 id 名称推断业务含义。
- coverage item 的中立标识允许来自稳定字段别名集合，而不能强绑定单一字段名：Goal coverage 可使用 `goal` / `goalItem` / `item` / `requirement`；acceptance dimension 可使用 `id` / `dimension` / `key` / `name`；deliverable coverage 可使用 `id` / `item` / `deliverable` / `name` / `path`。runtime 只能把这些字段视为“coverage 条目标识”的结构别名，不得据此推断业务语义。
- `missingGoalCoverage` 中的每个条目都必须在 assessment-only correction 中成为独立的 `dodAssessment.goalCoverage.items[]` 条目；不得把多个缺口合并成一个宽泛描述后宣称满足。completion gate 校验 Goal coverage 时，稳定 `goalRequirementId` 可以直接匹配；没有 ID 时只允许精确或保守等价文本匹配，不允许用包含关系把多个 leaf 合并为一个满足项。
- `missingDeliverableCoverage` 中的每个条目都必须在 assessment-only correction 中成为独立的 `dodAssessment.deliverableCoverage.items[]` 条目；满足项必须复制 required deliverable id，并引用具体 `evidenceRefs` 或 `artifactRefs`。
- runtime 可以为 `missingGoalCoverage` 附加中立 `evidenceCandidates`，候选来源只能是当前 run 已完成或失败 step 的 step id、kind、state、output 文本与 requirement 文本的通用匹配。候选证据不是自动通过依据，LLM 必须判断其 output 是否真实支持该 Goal leaf，并在满足时引用具体 `evidenceRefs`。
- assessment-only correction 输出缺少可由 `missingGoalCoverage` 唯一确定的 requirement 文本或 `goalRequirementId` 时，runtime 可以在 completion gate 前进行中立 schema normalization：按条目顺序或稳定 ID / 文本匹配补齐 requirement 元数据，并剔除非可追踪 `evidenceRefs`；若满足项仅给出不可追踪引用，可回填该缺口的中立候选 step id。该 normalization 只修复合同结构和证据引用形态，不代表 runtime 自动接受候选证据的业务含义。
- 当 `dodAssessment.goalCoverage.satisfied=true` 且子项已提供可追踪 evidence refs，但遗漏子项 `state` 时，runtime 可以在 completion gate 前将该子项归一化为 `state=satisfied`。该容错只适用于 assessment-only coverage correction，且必须同时具备父级 satisfied 与可追踪 evidence refs / evidenceCandidates；不得为缺少证据的子项推断 satisfied。
- Coverage correction 的 repair 分流必须先基于当前 run evidence 补齐中立候选证据，再判断是否仍缺少可用于判断的证据。若缺口可以通过稳定 `goalRequirementId`、精确规范化文本、通用 token overlap 或 CJK n-gram overlap 与已有 satisfied DoD/Goal evidence 机械匹配，仍属于 assessment-only correction；若缺口没有候选证据，也不能从已有 satisfied evidence 中机械映射，runtime 才能转入 concrete validation evidence repair，并允许 LLM 返回受限 verification `executionGraph`。
- 该机械匹配只用于决定“是否已有可评估证据可供 LLM 做 coverage assessment”，不得直接判定业务满足。最终是否 accepted 仍必须由 `dodAssessment.goalCoverage.items[]` 明确列出 requirement / goalRequirementId、`state=satisfied` 与可追踪 evidence refs，并通过 completion gate 校验。
- completion gate 在校验 `dodAssessment` 前必须基于 runtime evidence catalog 规范化 `evidenceRefs`：已完成 step、通过的 validation result、已应用 patch、tool call 等 runtime 已知证据可以作为 canonical evidence；LLM 输出的 graph node 别名、`runId/nodeId`、旧 `block/nodeId/patch/N`、`runId/<assessment-follow-up-step>-<node-id>` 等结构别名必须映射到 canonical step / validation / patch ref；别名匹配应支持 node id 的 `_` / `-` 等价与 graph step 顺序前缀剥离，但只能映射到同 run 内可追踪且可用的 runtime evidence。已知失败、无效、不可追踪或无法映射的引用必须从满足项中剔除。若剔除后某个 satisfied item 不再有有效 evidence，completion gate 必须拒绝该 assessment，而不能让历史失败 evidence 或 LLM 自造 ref 支撑 accepted result。
- 若 LLM 将 `dodAssessment.goalCoverage`、`dodAssessment.acceptanceDimensions` 或 `dodAssessment.deliverableCoverage` 误输出为 bare array，runtime 可以在解析前中立规范化为 `{ "satisfied": <由 items 状态推断>, "items": [...] }`。该容错只修复 schema 形态，不得改变 item 文本、证据引用、artifact 引用或业务判断。
- completion gate 因 `goalCoverage`、`acceptanceDimensions` 或 `deliverableCoverage` 缺失、未满足或结构不完整失败时，runtime 必须保留对应 coverage 诊断，并在后续 repair 中进入 assessment-only coverage correction；不得把纯 assessment 合同缺口误升级为普通 execution repair。
- assessment-only correction 不得返回 executable work；只能返回 completed / failed `dodAssessment` 或真实治理 blocker。
- assessment-only contract correction 的输入必须是 narrow assessment package：不得携带 `executionGraph`、patch/shell/tool/memory 可执行字段，也不得携带 `governanceHistory`、`replanContinuation`、历史 `convertedDecision/llmDecision`、state-changing repair action package、patch repair context 或 provider retry lineage。correction 只用于修正 assessment schema / coverage / evidence refs，不得被旧 replan 或 decision 语义诱导成新的执行规划。
- 若 assessment-only correction 给出 failed assessment，但同一缺口仍存在候选证据或满足项矛盾，runtime 应在 repair budget 内再次派发 assessment-only correction，而不是立即写 rejected result。预算耗尽后才进入 rejected / blocked / replan 治理。
- rejected result 的 `SourceRef` 应指向最终产生 rejection 的终态 assessment / repair step；root step、failed step、候选证据 step 应保留在 `EvidenceRefs` 中，确保终态来源与 lineage 都可追踪。

### 抽象覆盖矩阵合同

当 Goal / DoD 或 profile contract 显式要求多个目标对象、多个接口契约、多个格式/协议、配置化行为、流式/长流程行为、错误行为、副作用或并发行为时，runtime 必须要求 LLM 在 planning / verification / acceptance 输出中形成中立覆盖矩阵。矩阵只是结构化验收表达，不是 core 领域知识。

允许的中立矩阵包括：

| Matrix | 触发条件 | 必须表达的中立内容 |
| --- | --- | --- |
| `targetCompatibilityMatrix` | Goal / DoD 声明多个目标对象、目标类别、运行目标、外部系统或适配对象 | 每个目标的名称/标识、来源、预期行为、实现状态、验证证据 |
| `interfaceContractMatrix` | Goal / DoD 声明接口、协议、格式、输入输出兼容或转换 | 输入契约、输出契约、转换/适配规则、响应/结果映射、验证证据 |
| `configurationBehaviorMatrix` | Goal / DoD 声明配置文件、环境覆盖、启用/禁用、默认值或错误配置 | 配置来源、默认值、覆盖顺序、启用/禁用行为、缺失/非法配置行为、验证证据 |
| `streamingBehaviorMatrix` | Goal / DoD 声明流式、增量、长流程、取消或完成行为 | 触发输入、增量输出、完成条件、超时/取消、结果汇总或等价度量、验证证据 |
| `errorBehaviorMatrix` | Goal / DoD 声明错误透传、异常处理、依赖失败或状态映射 | 错误输入、错误来源、输出状态/消息、是否透传或转换、验证证据 |
| `sideEffectEvidenceMatrix` | Goal / DoD 声明写入、追加、审计、记录、状态变化或外部副作用 | 写入时机、字段/内容、追加/覆盖语义、并发/幂等约束、验证证据 |

矩阵行的具体名称只能来自当前 Goal / DoD、workspace 文件、配置、工具返回、用户输入或 LLM 对当前上下文的解释；core 和通用 template 不得预置具体业务对象名称。若某行缺少实现或验证证据，assessment 必须标记 failed / skipped / inconclusive，并给出需要补齐的中立 evidence 类型；不得用一个兼容路径、一个 mock、一个总体通过命令或一段 summary 概括所有矩阵行。

Claim-to-evidence 一致性也必须保持中立：summary、goalOutcome、implementationSummary、verificationSummary、knownLimitations 中的每个能力声明，只要可被拆成矩阵行或 coverage item，就必须能追溯到对应 `evidenceRefs` / `artifactRefs`。Core 只能检查“声明是否有结构化覆盖项和证据引用”，不得判断某个具体协议、工具、模型、文件或业务规则是否正确。

若 `execution/strict_convergence` 阶段的 LLM graph 节点返回的 `executionGraph` 已经收敛为具体 runtime capability，且图中至少包含一个有效 state-changing 节点、没有继续包含 `llm/workspace_execution` 决策节点、state-changing 节点不依赖新的 LLM 决策祖先，则 runtime 可以把该图作为新的 graph scope 派发执行。该判断只基于 capability kind、依赖关系和 capability input contract，不得引入业务领域语义。

当 run 级 `stateChangePolicy=must_produce_state_change` 时，`planning` 阶段可以先输出只读 graph 做上下文探索；但一旦 graph 中的 `llm/workspace_execution` 节点被调度到 `execution` 阶段，该 LLM 节点必须继承 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output` 与 state-changing guidance。普通 `file/read`、`file/list`、只读 tool 节点不因此被强制变更。

LLM 输出 `executionGraph` 时，runtime 必须按当前 Soul 绑定阶段解析有效 Soul 绑定，并用中立 capability contract 校验 graph 节点：若 `allowedToolKinds` 非空且不含 `*`，所有节点 kind 必须在允许列表内；若 `allowedMCPServerRefs` 非空且不含 `*`，MCP 节点引用的 server 必须在允许列表内。该校验只依据 Soul 的能力白名单，不得写入具体业务领域或特定工具 fallback 规则。

当某类 workspace 需要版本控制证据时，必须通过 WorkspaceTemplate 的中立
`contextHints.versionControl.git.initializeOnCreate=true` 声明；workspace 创建层只按该
hint 初始化本地 Git 仓库，不得在 orchestrator/runtime 核心中根据模板名、业务领域或
Goal 文本硬编码 Git 行为。Soul 的 `allowedToolKinds` 必须与提示词中的工具要求保持一致：
若提示词要求使用 `git/read` 或 `git/write`，结构化白名单也必须显式包含对应 tool kind，
否则该提示词视为无效约束并会造成 LLM 上下文冲突。
运行期也可以通过中立 `git/write` 的 `operation=init` 初始化尚未建仓的 workspace
交付目录；该操作只表达“为当前交付目录建立本地版本控制状态”，不得推导任何具体业务
模板、Goal 语义、远程仓库或自动提交行为。已有同根仓库时必须返回幂等零工作结果。

当 `execution/strict_convergence` 阶段的非只读步骤失败，且 run 级 contract 要求 `must_produce_state_change` 时，后续 runtime repair 必须继续携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。这包括 shell/test/build、patch/apply、file/edit、MCP tool 等执行或验证 state change 的步骤失败；普通 read observation 失败只有在缺失目标文件、DoD 缺口或其他明确 contract failure 时才提升为 state-changing repair。

若非 LLM 执行步骤失败原因是中立的瞬时执行超时（例如 runtime deadline / timeout，且尚未形成确定性失败证据），runtime repair 不应强制要求 state-changing 输出；应允许一次受预算约束的重试、缩小验证范围或返回 failed assessment / blocker。该规则只基于“执行超时”这一中立失败形态，不得写入特定命令、语言、测试框架或业务领域判断。

当 provider/config 决策解除后需要重试原失败 LLM step 时，decision resolution 只能补充“已授权重试、已提供信息、恢复动作”等恢复上下文，不得覆盖原 step 已存在的 `runtimeStage`、`runtimeStageMode`、Soul 绑定阶段、`stateChangingOutputRequired`、`repairObjective` 等阶段合同。换言之，`decision` 是恢复入口，不是被重试任务的执行阶段；provider retry 必须继续继承失败 step 的原阶段语义。

当 execution graph 以并行 batch 方式调度时，batch 完成后的续跑必须以本批次所属 graph root / lineage 为锚点计算 dependency-satisfied 节点，不得用 run 内全局“最新 completed graph step”推断当前 graph。全局最新节点可能属于其他 repair、replan 或历史 graph scope；runtime 必须按当前批次 root 继续推进，直到该 graph scope 内没有可执行节点，再进入 follow-up / verification / acceptance。

当 state-changing 输出依赖具体 runtime capability 时，prompt / tool schema / repair guidance 必须明确该 capability 的中立能力边界。若某个 state-changing capability 只支持有限操作，LLM 不得用未声明语义伪造操作。例如某能力只支持“写入完整内容”和“应用 diff”时，不得把“空内容写入”解释为删除。Runtime 必须拒绝不满足 capability contract 的输出，并把拒绝原因作为下一轮 repair evidence；prompt 层也必须把该约束暴露给 LLM，减少重复输出必然失败的 artifact。

`unified_diff` 是通用 patch capability，不属于软件开发专用能力。其 hunk 必须包含显式行号范围，例如 `@@ -12,3 +12,4 @@`；裸 `@@` hunk 属于 capability contract violation。若 LLM 无法确定行号，必须选择安全的替代状态变更表达：基于完整安全快照的 `file_write + baseHash`、精确 `oldText/newText` 编辑、先读取目标，或返回 failed assessment / blocker。Runtime prompt、repair guidance 和 dispatch 校验必须共同表达该规则。

当 state-changing repair 输入已经携带当前 target snapshot，例如 `failureTargetRepairContext`、`patchRepairContext` 或 `repairActionPackage.targetSnapshots` 时，`unified_diff` 还必须包含实质性非空变更。只替换为完全相同内容、只新增/删除空白行、没有 changed lines、或其他无实质状态变化的 diff 必须被 runtime 拒绝，并作为 repair contract evidence 回流。该规则只校验通用 diff 形态与 state-changing contract，不判断具体业务内容是否正确。

能力边界属于 runtime capability contract，不属于业务领域规则。Core 可以声明“某 capability 不支持 delete、content 不能为空、diff 必须完整”等中立能力事实；但不能据此推断某个业务文件是否应该删除、保留或重命名。

### 已知可读资源不得升级为人工缺失信息

在任意阶段中，如果 LLM 返回 `blocked_on_decision/provide_missing_info`，但其 `missingFields` / `requiresHumanFields` 指向的内容已经存在于 runtime 可确认的 workspace manifest、known resource list、read-only evidence 或当前工作目录中，则该 decision 不属于真实人工阻塞。

Runtime 必须按中立规则处理：

- 不得要求用户粘贴已知可读资源的内容。
- 当 LLM 请求 `blocked_on_decision/provide_missing_info`，但 `missingFields` 与 `requiresHumanFields` 均为空，且 decision context 已提供可执行目标、候选动作、目标资源、计划摘要或等价 `knownValues` 时，该请求不是人工信息缺失。若上下文没有声明外部依赖、WorkOrch runtime 配置依赖、provider 配置依赖、审批风险或 rejected result policy，runtime 必须把它转换为继续执行 / repair / replan 的中立恢复路径，不得创建人工 ticket。类似“是否批准切换阶段继续自动实现/验证”的请求属于阶段合同恢复，不属于用户决策。
- 不得把“已知资源内容未进入当前 prompt”误判为人工缺失信息。
- 应将该 decision 转换为 runtime repair / continuation，并把对应资源的当前快照或可读取引用放入 repair 输入。
- `workspaceFiles`、`existingFiles`、`readableResources`、`knownResources`、`workspaceFileManifest` 等字段都只能作为“资源是否已知”的中立证据；若这些字段不完整，runtime 仍应以当前 workspace 工作目录中真实可读资源为准做兜底核对。
- 若资源读取失败，应按普通 read observation failure 或 workspace path failure 收敛，而不是继续追问用户。

该规则只识别“资源是否已知且可读”这一中立事实，不得写入 `main_test.go`、测试文件、代码文件、业务配置文件等领域特判。Profile 可以决定哪些资源更重要，但 core 只负责避免把可自动读取的信息升级为人工决策。

## Artifact 中立类型

Core runtime 不定义“代码 patch”这类领域概念，只定义中立 artifact 能力。

| Artifact type | 中立含义 | 示例 profile 映射 |
| --- | --- | --- |
| `state_change` | 对 workspace 状态产生可追踪变更 | 软件开发 profile 可映射为 patch/file write |
| `read_observation` | 读取或观察得到的上下文 | file read、web fetch、mcp query |
| `plan` | 可执行单元与验收点拆解 | implementation plan、migration plan |
| `evidence` | 验证结果或外部证明 | test result、build result、manual approval |
| `assessment` | 对 Goal / DoD 的判定 | accepted/rejected/inconclusive DoD assessment |
| `decision_request` | 需要人工或外部系统补充 | provider config、approval、missing requirement |
| `memory_write` | 可复用的高价值信息 | accepted input、necessary output、tool capability |

Capability / profile 可以把 `state_change` 绑定到具体 step kind，例如 `patch/apply`、`file/edit`、`mcp/update`，但 core 只按 state-changing contract 治理。

`patch/apply` 在 core runtime 中只表示一种中立 state-changing capability kind：它接收 targetRef、baseHash、diff/content/edit 等能力输入，并尝试改变 workspace 交付物。Core 可以校验 target 准入、baseHash、diff 形态、placeholder/no-op、同图依赖、bounded verification 和 evidence refs；但不得把 `patch/apply` 解释为“软件代码修改”，也不得根据语言、测试框架、文件名或业务字段判断 patch 内容是否业务正确。软件开发如何使用 patch、如何解释代码语义、如何设计测试，必须由 profile / Soul / skill / Goal / DoD 与 LLM 在当前上下文中动态决定。

## Profile 映射规则

Profile 是 core 外层能力定义，用于把某类任务映射到中立 stage contract。

Profile 可以定义：

- 每个 stage 的推荐 prompt。
- 可用 skill / tool / mcp / soul。
- 允许 artifact type。
- state-changing artifact 的具体 step kind。
- evidence 的有效性标准。
- evidenceRequirements：按 profile 命名空间声明某类 coverage / dimension / deliverable 需要或拒绝的 step kind。
- memory recall 的筛选优先级。

Profile 不可以改变：

- runtime 状态机。
- terminal result 要求。
- provider retry / repair / replan 的治理规则。
- ID / lineage / budget 规则。
- core 对 stage contract violation 的判定。

Core 可以按中立结构执行 profile 声明的 evidenceRequirements，但只能做机械校验：

- 读取 `profileContracts.<profile>.evidenceRequirements[]`。
- 按 `scope` 选择 `acceptanceDimensions`、`deliverableCoverage`、`goalCoverage` 或 `dodAssessment.items`。
- 当对应 item 为 `satisfied` 时，检查其 `evidenceRefs` 指向的 step kind 是否命中 `acceptedStepKinds` 或 `rejectedStepKinds`。
- 当 `acceptedStepKinds` 已声明时，`rejectedStepKinds` 表示“不能单独支撑该声明”的证据类型；若同一 item 同时引用 accepted 证据，只读/辅助证据可以保留为上下文。
- 当 profile 声明 `requireDirectEvidence=true` 时，对应 satisfied item 必须至少有一条可追踪 evidenceRef；若声明了 `acceptedStepKinds`，该直接证据还必须命中 accepted step kind。不可追踪 ref、只读 ref 或被 profile 拒绝的 ref 不能单独支撑 satisfied。
- 失败时生成中立 `dod_evidence_quality_gap` diagnostic，驱动后续 repair / rejected assessment。

Core 不得根据 `id` 名称推断业务含义。例如 `persistence_or_side_effect`、`concurrency` 只是 profile 声明的字符串；core 不知道它们对应软件开发、数据写入、测试或任何具体领域。

Run activation 必须把当前有效的 profile/template contract 组装进 `executionContract`。若 workspace 持久化了 `TemplateID`，activation 必须优先按该 ID 读取当前 template 的 `ContextHints` 作为 contract 基线，再保留 workspace 中 template 未声明的扩展字段；template 已声明的同名 contract 字段以当前 template 为准，避免旧 workspace 长期携带过期的 `profileContracts`、`deliverableExpectations` 或 `evidenceRequirements`。仅当旧 workspace 缺少 `TemplateID` 时，activation 才允许根据 default soul / stage soul bindings 兜底匹配 template。该刷新只作用于中立 execution contract，不得在 activation 阶段重新写入或覆盖 workspace 的 soul bindings、skill、mcp 或其他业务绑定。

`executionContextPackage` 交给 LLM 的 `executionContract` 必须保留完整结构，尤其是 profile 下的 `acceptanceDimensions`、`deliverableExpectations`、`evidenceRequirements` 及其 `required`、`acceptedStepKinds`、`rejectedStepKinds` 等字段。Goal / DoD / 历史上下文可以按预算压缩，但 contract 不能被压缩成数组数量或摘要，否则 LLM 无法按 profile 约束做决策，runtime 也会在后续 verification / repair 阶段反复纠偏。

### 软件开发 profile 示例

该示例只作为 profile 映射，不得写入 core runtime 特判。

| 中立 stage | profile 行为 |
| --- | --- |
| `exploration` | 读取仓库结构、相关文件、已有测试和文档，形成问题摘要 |
| `planning` | 拆分实现 unit、DoD 覆盖点、验证命令 |
| `execution` | 生成 state-changing artifact，可映射为 patch/file edit |
| `verification` | 运行测试、检查 diff、收集 runtime evidence |
| `acceptance` | 对照 Goal / DoD 写 accepted/rejected result |
| `repair` | 针对失败 evidence 生成定向修复 |
| `replan` | 原实现路径不成立时创建 successor plan |

## Memory / Recall 合同

Memory 需要按阶段服务 LLM 决策，而不是堆叠 runtime 流水。Memory 是带适用边界的参考知识；过程证据、失败流水、provider retry、schema invalid、单次 patch 失败和普通 step 日志必须优先进入 `processEvidenceTimeline`、`currentProgressDigest`、`repairActionPackage.recentRejectedAttempts`、run evidence 或 result artifact，而不是默认写入 memory。

### Memory 写入准入

Memory 写入准入必须前置于 recall 降噪。若一条信息无法作为后续 LLM 的参考信息使用，runtime 不应先写入 memory 再依赖 recall 阶段过滤。

允许写入 memory 的信息必须满足以下条件：

- `referenceValue=true` 或等价可推导条件成立：该信息是稳定事实、已接纳输入、必要输出、可迁移经验、人工决策结论或 Goal / DoD 覆盖反馈。
- 具备适用边界：至少包含 `scope`、`precondition`、`appliesTo`、`validUntil`、`evidenceRef`、`sourceStage` 或等价上下文字段之一。
- 非瞬态过程：不得只是 provider retry 次数、schema parse error、单次 runtime failure、重复 step id、旧 graph payload、raw stdout/stderr 或已被拒绝的 LLM 计划。
- 非重复记录：可从当前 workspace/template/config/soul/skill/tool 绑定直接重建且未发生变化的信息，不应每轮重复写入 memory。
- 不含敏感信息、provider 动态绑定、运行期 API key、临时 endpoint 或不可持久化凭据。

Core 准入只判断 memory 的稳定性、边界、来源和可复用性，不判断具体业务领域含义。

`llm_dod_assessment` 只能在产生真实 DoD 覆盖反馈时隐式写入 `dod_feedback` memory。若 assessment 表达的是 provider timeout、provider retry exhausted、schema invalid、stream incomplete、context gate failure、`coverageTargets` 合同违规、`invalid_unified_diff`、non-substantive patch、重复 patch 失败等瞬态运行问题，或 DoD item 全部为无 evidence 的 `skipped`，runtime 必须把它作为当前 run 的过程证据、失败证据或 repair package 信息保存，禁止写入长期 memory。显式 `memoryWrites` 同样必须通过该准入，不允许用 `dod_feedback` 包装 provider/runtime 流水。

### 高价值 memory

高价值 memory 包括：

- 被 LLM 实际接纳的输入。
- 根据 Goal / DoD 产生的必要输出。
- 当前阶段的有效 plan / unit / acceptance criteria。
- 已验证 evidence 与其覆盖的 DoD。
- 已确认不可行的方案及原因。
- 当前可用 skill / tool / mcp / soul / prompt 的能力摘要。
- 人工 decision / approval 的结论。

被 LLM 实际接纳的 skill / tool / mcp / soul / prompt 信息只有在初始化、绑定变化、用户确认、或已经产生有效输出证据时才应写入 memory。普通 LLM step 不能因为再次携带同一份能力上下文就重复生成 accepted input memory。

## 抽象矩阵收口跟踪

本节记录针对“核心保持业务中立，template 避免绑定具体 Goal / DoD 内容”的收口项。后续同类修复必须先落到本文的中立合同，再决定是否需要在 profile / template / soul 中表达通用工程约束。

| ID | 内容 | 落点 | 状态 | 验收口径 |
| --- | --- | --- | --- | --- |
| SNC-01 | Core 明确不得内置具体业务对象、供应商、协议、接口、字段或测试框架语义 | 本文 Goal / DoD Coverage Assessment 合同与阶段输出合同 | completed | 文档只要求 coverage / matrix / evidence 结构完整，不包含具体业务行 |
| SNC-02 | 定义中立覆盖矩阵，替代具体目标/接口类型规则 | 本文“抽象覆盖矩阵合同” | completed | 使用 `targetCompatibilityMatrix`、`interfaceContractMatrix`、`configurationBehaviorMatrix`、`streamingBehaviorMatrix`、`errorBehaviorMatrix`、`sideEffectEvidenceMatrix` |
| SNC-03 | software-dev planning Soul 从具体目标类型矩阵改为目标/接口契约矩阵 | 内置 `soul-software-dev-planning` | completed | 默认值测试断言包含 `target/interface contract matrix`，并禁止旧特化表达 |
| SNC-04 | software-dev execution / validation Soul 去除具体目标类别、计量对象和外部适配对象表达 | 内置 `soul-software-dev-execution`、`soul-software-dev-validation` | completed | 默认值测试断言不再出现旧特化表达，保留多目标、多接口契约、计量/副作用等通用工程约束 |
| SNC-05 | 保留 workspace 动态派生业务矩阵能力 | profile / soul 约束 | completed | 具体矩阵行只能来自当前 Goal / DoD、workspace 上下文、工具结果或 LLM assessment，不由 core/template 预置 |

### 低价值或噪音 memory

默认不应进入 recall 主上下文：

- provider retry 流水。
- schema invalid / JSON parse error / stream partial 的单次失败。
- state-changing repair 被 runtime 拒绝的无效 graph、无效 patch、无效 rootCauseTarget 或无效验证计划。
- coverageTargets 缺失/不匹配、invalid unified diff、non-substantive patch、schema repair 重试失败等 runtime 合同失败摘要。
- 重复 step ID 列表。
- 已被 supersede 的旧 repair 过程。
- 只读扫描的原始日志。
- 没有被 LLM 接纳或没有影响后续决策的中间文本。
- 已过期的 blocker。

这些信息若对当前 run 仍有价值，应作为当前失败证据或 rejected attempt 摘要进入 `repairActionPackage` / process evidence，而不是进入长期 memory。

### 阶段化 recall

| Stage | recall 优先级 |
| --- | --- |
| `exploration` | Goal、DoD、workspace manifest、相关历史结论、可用能力 |
| `planning` | 探索摘要、约束、DoD、当前 workspace manifest / known resources、已有方案、不可行路径 |
| `execution` | 当前 unit、必要上下文、accepted plan、可用 state-changing capability |
| `verification` | 变更摘要、验证要求、evidence 标准、历史失败原因 |
| `acceptance` | Goal / DoD、evidence refs、coverage map、remaining gaps |
| `repair` | failure scope、失败 evidence、最近修复尝试、必须继承的 contract |
| `replan` | 原计划失败原因、当前 workspace manifest / known resources、可保留产物、需要重新拆解的目标 |

Strict repair / strict convergence 阶段的 recall 必须进一步收窄：只有与当前 `failureSignature`、`failureTargetScope`、`repairTargetSet`、Goal / DoD gap、coverage target 或当前 stage contract 存在结构化匹配的 memory 才能进入 LLM-facing context。来自 failed / superseded run 的 memory 默认不召回，除非其内容满足写入准入中的可复用经验和适用边界要求。缺少适用边界的 memory 不得作为参考注入 strict repair prompt。

### LLM Prompt Section 优先级

Provider prompt 必须按阶段把上下文拆分为可审计 section，而不是把所有信息混入一个大 JSON。默认优先级如下：

1. System / schema / output format contract。
2. 原始 Goal 与原始 DoD，保持 raw required。
3. Effective Goal / DoD view。
4. Stage Contract：当前 `runtimeStage`、`runtimeStageMode`、`stateChangingOutputRequired`、`repairObjective`、`requiredOutput`、当前阶段可接受输出与禁止输出。
5. Current Failure Package：`repairActionPackage`、当前失败、目标快照、当前执行进展、最近被拒绝尝试和退出条件。
6. Background Context：workspace manifest、已接纳 result、deliverables、governance history、memory references。
7. Current Requested Action：必须与 Stage Contract / `repairActionPackage.requiredOutput` 完全一致。

当存在 `repairActionPackage` 时，它必须独立成为 LLM-facing prompt section，且优先级高于历史 memory、governance history、manifest 和 generic runtime capabilities。`repairActionPackage.requiredOutput` 不得只出现在大段 `Execution Context and Evidence Signals` 内。

Strict repair 的 `Current Requested Action` 不得继续使用泛化 `plan_next_step`。当 `stateChangingOutputRequired=true` 或 `repairObjective=produce_state_changing_output`，且 `requiredOutput.acceptedKinds` 包含 state-changing capability kind 时，Current Action 必须表达为 `produce_state_changing_repair_or_failed_assessment` 或等价中立动作，并显式带上 accepted / forbidden output kinds。若只能先读取资源，runtime 应把该轮建模为读取闭环而不是 state-changing repair 闭环；同一轮不得一边要求 `patch/apply`，一边把 `plan_next_step` 作为当前动作。

### Context Conflict Gate

提交 LLM provider 前必须执行中立上下文冲突门禁：

- `stateChangingOutputRequired=true` 或 `repairObjective=produce_state_changing_output`，但 Current Action 仍是 `plan_next_step`，必须 fail-fast 为 `context_contract_conflict`。
- `repairActionPackage.requiredOutput.acceptedKinds` 存在，但 Current Action 未声明同一 accepted kinds，必须 fail-fast。
- Strict repair 同时鼓励 read-only exploration / context rehydration / self inspection，且没有显式读取闭环合同，必须 fail-fast。
- Goal / DoD、Stage Contract、`repairActionPackage.requiredOutput`、当前 failure package、必要 target snapshots 或 current progress 被预算裁剪到不可用时，不得提交 provider。

该 gate 只检查阶段合同、输出 kind、上下文存在性和提示词一致性，不引入任何具体业务领域语义。

### STG-10 Prompt / Memory 收口跟踪

| ID | 内容 | 落点 | 状态 | 验收口径 |
| --- | --- | --- | --- | --- |
| STG-10-01 | Strict state-changing repair 的 Current Action 不再使用 `plan_next_step` | LLM prompt builder | completed | request prompt 中 action type 与 `requiredOutput` 一致 |
| STG-10-02 | `repairActionPackage` 独立成高优先级 prompt section | LLM prompt builder / request evidence | completed | request prompt 有独立 repair package section，且保留 requiredOutput |
| STG-10-03 | Provider 前上下文冲突 gate | LLM provider request budget / gate | completed | 冲突 action / requiredOutput 在 provider 调用前 fail-fast |
| STG-10-04 | Memory 写入准入前置，禁止 transient failure / provider retry 默认写入 | runtime memory / recall selection | completed | transient repair/provider failure 不产生长期 memory；历史低价值 transient memory 不进入 recall 主上下文 |
| STG-10-05 | Strict repair recall 只接纳有适用边界且相关的 memory | execution context memory selection | completed | 无边界历史 memory 不进入 strict repair context |

## Repair 合同

Repair 不是重新探索。Repair 必须继承来源阶段的约束，并根据失败类型收紧。

| 来源阶段 | repair 要求 |
| --- | --- |
| `exploration` | 只修复探索输出无效、上下文缺失或工具失败；不得直接进入 implementation 特判 |
| `planning` | 修复 plan 缺口，必要时要求明确 executable unit |
| `execution` | 必须回到 state-changing output、failed assessment 或 blocker |
| `verification` | 必须补 evidence、解释 weak evidence 或转 rejected |
| `acceptance` | 必须补 DoD coverage / result，不得新增实现 |

Runtime repair 预算必须来自 runtime 配置，默认 `maxRuntimeRepairAttempts=10`；不得在 core runtime 分支中写死固定次数。repair 输入、repair exhausted ticket、auto-resume / auto-replan 判断、duplicate failure 判断和 `repairActionPackage.maxRuntimeRepairAttempts` 必须使用同一份归一化预算。Provider retry 预算与 runtime repair 预算互相独立，provider retry 不得消费 runtime repair 次数。

State-changing repair 必须满足目标相关性约束。若 repair 输入已经包含 `failureTargetScope`、`failureTargetRepairContext`、`patchRepairContext`、`repairActionPackage.targetSnapshots` 或等价的显式 `targetRef`，则 `patch/apply` 只有在目标属于这些失败目标、当前目标快照或明确 repair 目标时，才算有效 state-changing artifact。写入无关 marker、空转文件、报告文件或与失败目标无关的 no-op patch，不得满足 state-changing repair 合同；runtime 必须将其作为 contract violation 重新进入 repair / blocker / rejected 治理。

Scoped repair 允许在 state-changing artifact 后追加验证，但验证必须能被 runtime 识别为 bounded。满足任一条件即可视为 bounded verification：`shell/command` 输入显式声明 `targetRef`、`relatedFailureTargets`、`verificationScope` 或 `boundedReason`；或该验证节点依赖一个直接命中当前 failure target / repair target 的 `patch/apply` 节点。若验证命令宽泛且既无显式 scope，也不依赖命中失败目标的 patch，runtime 必须拒绝并要求补 scope 或返回 failed assessment / blocker。该规则只约束“验证与当前失败目标的可追踪关系”，不得硬编码命令、语言、测试框架或业务类型。

该约束属于 core runtime 的中立合同，只比较结构化目标引用、target snapshot 和 capability input，不解释文件类型、测试框架、编程语言或业务正确性。Core 可以判断“patch target 与失败目标不相关”，但不能判断“某个业务文件应如何修改”。当没有任何显式目标引用时，runtime 不得臆造目标；此时只能依赖 capability contract、LLM 输出和后续 evidence 判断。

若失败证据本身明确指向依赖、manifest、metadata、checksum、lock 等配套资源缺失，state-changing repair 可以写入与失败目标同目录的中立配套元数据目标。该放宽只基于失败文本中的通用前置资源信号、目标路径关系和目标文件名的通用 metadata/manifest/lock/checksum 形态，不得硬编码语言、框架、包管理器或具体业务文件。若配套目标不在同一目录、失败证据没有前置资源信号，或目标只是无关 marker/report/no-op 文件，仍必须按无关 patch 拒绝。

Shell / tool 调用不得被用来隐藏实际目标变更，再通过一个无关 `patch/apply` marker 来满足 state-changing 合同。若某个 capability 本身是声明过的 state-changing artifact，必须由该 capability 的输入、输出和 evidence 直接表达目标关系；否则修复应使用目标相关的 `patch/apply`、返回 failed assessment，或进入 blocker。

当 run 级 `stateChangePolicy=must_produce_state_change` 且失败输出包含 `missingDoDItems` 时，runtime repair 必须先区分中立缺口类型：

- `implementation_gap`：已有 evidence 表明实现缺失、实现错误、artifact 无法应用或非只读执行失败，repair 必须显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。后续 provider retry 也必须继承该约束，不能把实现缺口修复降级为只读 exploration graph。
- `evidence_gap`：verification 阶段只证明“缺少 concrete evidence / 未执行验证 / evidence 不足”，但尚未形成实现错误诊断时，repair 应携带 `repairObjective=produce_concrete_validation_evidence`，允许受限 tool / shell / mcp 验证图生成证据；若该验证图后续产生确定性失败，再由新的失败证据触发 state-changing repair。
- `hybrid_gap`：validation / completion 阶段已经具备明确 coverage gap，并且当前代码/graph evidence 已提示至少一部分缺口需要 state-changing implementation 或 state-changing test updates 才能关闭。此时 runtime 不应把 failed `dodAssessment` 仅视为纯 assessment 结果；应显式把 repair 目标提升为“优先输出 bounded state-changing repair graph，必要时附带对应 verification nodes”，避免先输出 failed assessment 再由 runtime 被动二次归约。

当 state-changing repair 输入已经携带 `missingDoDItems`、`missingGoalCoverage` 或 `missingDeliverableCoverage` 时，返回的 `patch/apply` 至少必须有一个节点声明中立 `coverageTargets`，并且目标必须匹配对应缺口的原始 DoD item 文本、`goalRequirementId`、Goal coverage item 文本或 deliverable coverage 文本。该字段只表达“这个 state-changing artifact 试图关闭哪个 coverage gap”，不表示 runtime 认可其业务正确性；最终是否满足仍由后续 validation / acceptance evidence 判断。若 patch 不能绑定任何具体缺口，应返回 failed `dodAssessment` 或 blocker，而不是提交泛化骨架、marker、报告或只具备形式变更的 patch。

`coverageTargets` 可以是字符串数组，也可以是结构化对象数组。结构化对象允许使用中立字段表达目标，例如 `coverageTargetToken`、`goalRequirementId`、`targetText`、`item`、`raw`、`requirement`、`goal`、`goalItem`、`id` 或 `expectationId`；runtime 只做规范化后的精确 coverage target 匹配，不解释业务含义、不基于具体领域关键词推断是否满足。

Coverage contract 必须贯穿 planning -> execution -> follow-up -> repair -> validation，而不能只在 repair 阶段生效。但 `coverageTargets` 只在当前输入明确携带 `missingDoDItems` / `missingGoalCoverage` / `missingDeliverableCoverage` / `coverageTargetSet`，或 `repairActionPackage.coverageTargetSet` 时强制要求。普通 state-changing repair，例如编译错误、目标文件语法错误、patch context mismatch、schema recovery 后的目标文件最小修复，只需满足 target relevance、当前快照、state-changing graph 和 bounded verification 合同，不得因为全局 `stateChangePolicy=must_produce_state_change` 或完整 Goal/DoD 文本存在而强制 LLM 猜测 `coverageTargets`。真正 coverage-bound 的 `patch/apply` state-changing artifact 必须显式声明 `coverageTargets` 并匹配当前 coverage target set；任何后续验证节点必须显式声明 `targetRef`、`relatedFailureTargets`、`verificationScope`、`boundedReason` 或依赖一个已绑定 coverage target 的 state-changing artifact。该规则避免 coverage 缺口修复退化为形式变更，同时避免普通诊断修复被无关 coverage 元数据阻塞。

对于 `graphFollowUp=true` 且 `runtimeStage=verification|acceptance` 的 strict convergence follow-up，如果当前证据已经明确指出 missing DoD / Goal / deliverable coverage，并且这些缺口可由后续自动 repair 关闭，LLM 不应优先返回“只有 failed dodAssessment、没有 actionable continuation”的结果。更符合合同的输出顺序应为：

1. 直接返回 bounded state-changing repair graph，并绑定相应 `coverageTargets`；
2. 若无法立即 state-change，但可以先补 concrete verification evidence，则返回 bounded verification evidence graph；
3. 只有在确实无法自动推进时，才返回 failed `dodAssessment`，并明确哪些 gap 需要 runtime repair / replan。

该规则仍是中立的：core 不判断具体业务实现方案，只要求“当缺口和修复目标已经结构化存在时，follow-up 输出优先给出可执行 continuation，而不是停在纯描述性失败”。

Runtime 在处理 `completionState=failed|continue|incomplete` 且输出包含 failed `dodAssessment` / `goalCoverage` / `deliverableCoverage` 时，必须先把该 assessment 视为可行动 coverage gap 候选，而不是直接报 `completionState <state> has no actionable continuation`。若 assessment 中存在 failed item、reason、nextActions、evidenceCandidates、coverage target、可解析资源引用或仍未满足的 Goal / DoD / deliverable 维度，runtime 必须构造 continuation：

- 若缺口只有证据不足，进入 `need_validation` 或 concrete validation evidence repair。
- 若缺口需要状态变更但尚无 target snapshot，进入 `need_evidence`，要求有界读取/取证。
- 若已存在 `coverageTargetSet`、target refs、target snapshots 或等价当前证据，进入 `need_state_change`。
- 只有 assessment 明确声明无法自动推进，或缺少 runtime 无法获取的人工信息/权限/外部配置，才允许 `need_governance` / blocker。

该转换只依据中立 assessment 结构、coverage gap、evidence refs/candidates、manifest、target refs、nextActions 和当前 evidence，不得按软件开发、具体文件名、供应商、语言或测试框架做 core 特判。连续多次相同 coverage target 的 failed assessment 必须消耗同一 coverage repair budget；step ID 或 follow-up 包装变化不得重置预算。

所有 state-changing artifact 的合同校验必须发生在副作用执行之前。`patchRequests`、`patch/apply` graph node、`nextStep=patch/apply` 和 runtime patch output 在写入 workspace、创建 applied patch evidence、记录 completed step evidence 之前，都必须先通过适用的 coverage target、target relevance、state-changing graph、bounded verification 等中立合同校验。只有 coverage-bound 输入缺少或不匹配 `coverageTargets` 时，runtime 才应 fail-fast 并进入 repair/provider retry/schema recovery；普通诊断修复不得被提升为 coverage target violation。`coverageTargets` 属于 patch 的中立治理元数据，必须从 LLM decision schema、top-level `patchRequests`、`executionGraph.nodes[].input`、patch evidence、runtime validator 与后续 `repairActionPackage` 全链路保留；任何 schema normalization、decision mapping、evidence compaction 或 provider retry 都不得静默丢弃该字段。

Runtime 必须维护中立 coverage ledger 或等价的 coverage 状态视图。每个 coverage target 至少包含 `kind`、`targetText`、可选 `goalRequirementId` / `coverageTargetToken`、`state`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs` 与 `reason`。Core 只记录结构关系和证据质量，不判断业务正确性。证据质量至少分为 `direct`、`indirect`、`smoke`、`unrelated`、`invalid`：只有 `direct` / 合理 `indirect` evidence 可以进入相应 coverage target 的 `evidenceCandidates` 或关闭 evidence 链；`smoke` evidence 只能证明基础可执行或环境可用，不得进入功能 Goal / DoD / deliverable coverage candidates；`unrelated` / `invalid` 必须记录 rejected reason 并从后续候选中过滤。

Follow-up、repair、provider retry、replan successor 和 validation 输入必须从 coverage ledger / 等价状态视图重建上下文，至少包含未关闭 coverage targets、最近 accepted evidence、最近 rejected evidence 及原因、当前 failure surface、`repairObjective`、`stateChangingOutputRequired`、`requiredOutput`、`currentExecutionProgress` 与 `processEvidenceTimeline`。若 follow-up 输入只包含最近 step 摘要而缺少 Goal / DoD coverage contract，runtime 必须先补齐上下文或 fail-fast，不得让 LLM 基于低价值 smoke evidence 继续生成 state-changing 输出或 failed assessment。

当上述 coverage repair 因 repair 输出合同违规而再次进入 repair-of-repair，例如 patch/apply 缺少 `coverageTargets`、返回只读 graph、返回 shell-only graph 或 LLM decision JSON 无效，runtime 必须继承上一轮 `missingDoDItems`、`missingGoalCoverage`、`missingDeliverableCoverage`、候选 evidence、已完成 action evidence、当前失败信息和最近 repair 尝试摘要。`repairActionPackage` 必须携带中立 `coverageRepairRequired=true`、`coverageTargetSet`、`currentExecutionProgress` 与 `requiredOutput.mustDeclareCoverageTargets=true`。这里的 `currentExecutionProgress` 只保留 step id、kind、state、命令/patch 摘要、stdout/stderr 预览、覆盖缺口计数和失败摘要，不得复制大段 patch content、历史 raw LLM 输出或被拒绝的失败正文。该规则保证 LLM 能看到“已有内容、当前进展、遇到的问题和仍未关闭的覆盖目标”，同时避免单 JSON 膨胀和失败产物复用。

当 patch / preview / apply 因 context mismatch、base mismatch 或 stale artifact 失败，或 `file/edit` 因 `oldText` 不存在、上下文冲突失败时，默认 repair 仍要求基于当前 target snapshot 重新生成可应用 state-changing artifact。runtime 必须把失败目标的当前快照放入 `patchRepairContext` / `repairActionPackage.targetSnapshots`，并明确失败 patch / edit 片段已经陈旧，不能重复使用同一 hunk、oldText 或未解析模板。完整 `file_write` 只允许建立在完整、未脱敏、未截断且标记为 `safeForFileWrite` 的当前快照上，且 patch 请求必须携带与当前快照 `contentHash` 完全一致的字面 `baseHash`；否则必须优先使用 `text_replace` 的精确 `oldText` / `newText` / `baseHash`、最小 `unified_diff`、`file/edit` 或先重新读取目标。若 repair context 中的内容包含 runtime redaction marker、被省略片段或截断标记，LLM 不得把该内容复制进 patch content / diff，而应先读取目标、使用 exact oldText 的最小替换，或基于可见上下文生成最小 `unified_diff`。`baseHash` 只能省略或使用当前 evidence 中的字面 `contentHash` 值；不得输出 `${...}`、`{{...}}`、`<<...>>` 或其他未解析占位符。`unified_diff` 的 hunk header 必须是可解析的真实行号范围，`@@ ... @@` 等伪 hunk 属于无效 artifact。

当前 repair target snapshot 是定向修复的高优先级输入。Provider context budget、prompt normalization 和 outbound redaction 不得把满足以下条件的 `failureTargetRepairContext[].content`、`patchRepairContext[].content` 或 `repairActionPackage.targetSnapshots[].content` 压缩、摘要化或替换为 `[REDACTED]`：`contentComplete=true`、`safeForFileWrite=true`、`redacted=false`、`truncated=false`、`isBinary=false` 或未声明、且存在字面 `contentHash`。该豁免只适用于当前 repair target snapshot 的 `content` 字段，且只表达“当前资源快照可作为修复基线”的中立事实；其他敏感字段、历史输出、失败正文、stream partial、provider 配置和不满足安全标记的 snapshot 仍必须按通用 redaction / budget 规则处理。redaction 审计必须按 JSON 解码后的等价结构比较快照数组，不得因为 `[]any` / `[]map` 运行时类型差异把已保护的安全快照误报为 redacted。若在极端预算下连安全当前快照也无法完整提供，runtime 必须显式标记快照不可用于完整 `file_write`，要求 LLM 重新读取目标、使用最小 diff / exact edit，或返回 blocker / failed assessment；不得向 LLM 发送已被 `[REDACTED]` 替换的“伪完整快照”。

当前失败目标快照的完整性优先级高于通用 4KB 文本摘要。runtime 在 target-scoped repair budget 内应尽量把安全、非二进制、未脱敏、未截断的当前目标作为完整 `content` 放入 `repairActionPackage.targetSnapshots`；只有超过 target-scoped repair budget 或包含不可安全写入内容时，才降级为 `contentPreview` 并标记 `contentComplete=false` / `safeForFileWrite=false`。该规则避免在目标文件略大于普通摘要窗口时，LLM 既缺少完整当前文件又被要求输出固定 patch，从而在 `file_write`、`file/read` 与 `patch/apply` 合同之间循环失败。若完整快照可用，`requiredOutput` 必须显式声明：对 existing target 使用 `file_write` 时必须携带等于当前 `contentHash` 的字面 `baseHash`；否则只能使用精确 `text_replace`、有界 `unified_diff` / `file/edit` 或先进入独立读取闭环。

`text_replace` 是 `patch/apply` 的中立精确替换表达，用于把当前文件中唯一匹配的 `oldText` 替换为 `newText`；当 `oldText` 多次出现时必须显式声明 1-based `occurrence`。它不表达任何编程语言或业务语义，只表达“基于当前文本快照的精确状态变更”。`text_replace` 必须遵守与其他 patch 相同的 targetRef、baseHash、coverageTargets、approval、reserved path、redaction、placeholder 和 no-op 准入；`baseHash` 存在时必须匹配当前目标文件。它的主要用途是避免在 hunk 行号漂移、模型难以稳定生成 unified diff、或完整 file_write 会导致 LLM 输出过大时反复失败。runtime prompt 和 repair guidance 在 patch context mismatch 后应优先推荐 `text_replace`，再考虑最小 `unified_diff`；只有完整、安全且确需整体替换时才使用 `file_write`。

若 state-changing artifact 依赖刚读取的目标内容，runtime 不得接受同一张 `executionGraph` 中“先 `file/read` 再生成已固定 `patch/apply`”的伪依赖。原因是 patch 内容在 LLM 本次响应中已经确定，无法真正消费运行时后续 `file/read` 的输出。此类场景必须拆成两个闭环：第一轮只允许读取目标并记录完整快照 / contentHash；第二轮 LLM 基于新的 `executionContextPackage` / `repairActionPackage` 生成具体 state-changing artifact。只有当 patch 仅依赖本轮输入中已存在的完整、安全、未脱敏当前快照时，才允许在同一 graph 中派发 patch。

`repairActionPackage` 是 state-changing repair 的主输入，而不是历史失败材料的附属摘要。它必须优先包含：`currentFailure`、`lastFailureDiagnostics`、`requiredOutput`、`structuredFailureSignals`、`targetSnapshots`、`workspaceFileManifest`、`currentExecutionProgress`、`coverageTargetSet`、`recentRepairAttempts`、`forbiddenActions`、`repairBudget` 与 `exitCriteria`。旧字段可以作为 runtime 内部兼容数据存在，但 LLM-facing context 必须以 `repairActionPackage` 为准；当其存在时，不得再把完整 `failedStepInput`、rejected raw output、raw patch body、raw stream 或历史 graph payload 复制给 LLM。

`lastFailureDiagnostics` 是当前可行动失败的必要诊断摘要，不是历史噪点。若最新失败来自 `shell/command`、`mcp/tool_call`、`patch/apply`、`file/edit` 或等价 action/validation step，runtime 必须把失败的 `error`、`reason`、`failureClass`、`failureSignature`、`command`、`args`、`exitCode` 以及必要的 `stdoutPreview` / `stderrPreview` 写入 `repairActionPackage.lastFailureDiagnostics`；若这些诊断中包含路径、line ref 或目标引用，也应同步进入 `currentFailure` 或 `structuredFailureSignals`。Provider context budget、prompt normalization 和 outbound redaction 必须把该字段作为 protected repair contract 字段保留。不得只把 `exit status` 这类抽象标签传给 LLM 后再要求 LLM 判断修复目标。

`currentProgressDigest.currentGap`、`processEvidenceTimeline.failureReason` 与 `currentFailure.reason` 必须遵守同一优先级：当 `stdout` / `stderr` / `stdoutPreview` / `stderrPreview` 中存在可机械识别的 target ref、line ref 或等价资源形状诊断时，优先使用该具体诊断文本；`exit status`、`command failed`、`runtime failed` 这类通用状态只能作为没有具体诊断时的兜底。该规则只依据中立的路径、行号、targetRef、identifier-shaped token 和 failure signature，不得根据语言、测试框架、业务对象或供应商语义推断修复方案。

`structuredFailureSignals` 只能通过中立机械方式抽取：target refs、line refs、identifier-shaped tokens、field-shaped tokens、bounded diagnostic lines、failure class / signature。这些信号只表达“失败文本中出现了可追踪形态”，不表达业务语义。LLM 在 repair 中引用这些 token 时，必须先与当前 target snapshot、workspace manifest 或同一 patch 中新增的资源对齐；不得仅因为失败文本出现某个 token 就凭空发明资源、字段、入口或目标结构。

当失败 evidence 已经包含中立的资源形状冲突诊断时，repair 需要遵守“首要阻塞优先”合同。这里的中立诊断包括但不限于：line-ref 执行/验证错误、unresolved identifier、invalid attribute、shape mismatch、missing method、schema mismatch，以及其他明确指向当前目标资源无法通过基本执行或验证的结构性错误。runtime core 只能表达以下中立约束，不得写入具体语言框架、业务模型或供应商细节：
- repairGuidance / provider prompt 必须要求 LLM 先修复这些已报告的首要阻塞，再处理格式整理、文案修饰或其他次要改进。
- 当 `structuredFailureSignals` 中存在可追踪 `lineRefs`、`targetRefs` 或 identifier-shaped tokens 且触发资源形状冲突时，runtime 必须生成中立 `primaryDiagnosticRepair.required=true` 合同，并同步写入 `repairActionPackage.requiredOutput.mustAddressPrimaryDiagnostics=true`。该合同只包含 targetRef、lineRef、identifier、bounded diagnostic message 与通用 match rule，不得包含语言、框架、测试库或业务领域专名之外的推断。
- LLM 返回 state-changing `patch/apply` 时，runtime 准入必须验证 patch 是否优先覆盖 `primaryDiagnosticRepair`：直接 patch 诊断 target 时，`unified_diff` 的 hunk 行区间必须覆盖诊断 lineRef 附近，或完整 `file_write` 必须基于当前 target snapshot 改变/移除诊断行；patch 相关目标时，应声明 `relatedFailureTargets`，并通过 `primaryDiagnosticResolution` / `addressesFailureSignals` 或 patch 内容中的诊断 identifier 表明正在解除哪个主诊断。若 LLM 未显式声明 related target，runtime 只能在中立结构证据充分时推断成立：patch target 与诊断 target 处于同一可关联资源范围，patch 的实际变更行而非注释/marker 命中诊断 identifier，并且补丁已通过非占位 state-change 准入。只修改同一文件的其他行、调整文案/断言/格式、补报告、添加注释或执行无关 marker，不得满足该合同。
- 若 `failureTargetRepairContext`、`patchRepairContext` 或 `repairActionPackage.targetSnapshots` 已提供当前目标快照，LLM 必须优先把目标与当前可见资源形状或契约形状对齐，而不是凭空发明新的名字、属性、入口或资源结构。
- 若具体任务属于软件开发等领域，workspaceTemplate / soul 可以在领域层进一步细化“对齐现有类型、构造函数、配置结构、入口函数和测试入口”的要求；这些领域规则不得下沉到 core runtime 里。
- 该规则只定义 repair 的排序、grounding 与准入，不判断某个业务符号该叫什么、某个框架入口该怎么写，也不允许 core runtime 根据命令名、语言名或业务关键字生成特定修复方案。

失败产物上下文必须遵守“不可复用失败正文”约束。对于被拒绝的 LLM decision、schema-invalid 输出、失败 patch / edit 正文、stream partial output 或任何包含 runtime redaction marker 的片段，runtime 不应把原文再次放入 repair input；只能暴露中立诊断摘要，例如失败 stage、failureClass、error/reason、streamDiagnostics、llmUsage、rejectedExecutionGraph 摘要，以及当前 target snapshot / workspace manifest / patchRepairSuggestions。若 stream partial output 或 rejected graph 摘要中包含 redaction marker，runtime 必须记录 omitted 标记与不可复用原因，而不是保留可复制片段。该约束用于防止 LLM 复制 malformed JSON、过期 diff、脱敏片段或重复 rejected artifact；它不判断具体业务内容，只限制 runtime repair 的输入形态。

Runtime 可以从失败 diff 与当前 target snapshot 中抽取中立的 `patchRepairSuggestions`，例如“某个 oldText 在当前目标中唯一匹配，可替换为 newText”。该建议只来源于失败 artifact 与当前资源内容的机械对齐，不包含业务语义；它用于帮助 LLM 生成更小、更精确、可审计的 repair artifact，避免重复输出行号漂移、上下文过期或依赖脱敏快照的 patch。

LLM 决策 schema 归一化只允许做中立、无语义扩展的形态修复。例如：`plan` 的字符串可归一化为单元素数组，`validationPlan` 可并入 advisory `plan`，顶层 `strategyReassessment` 可机械下沉到 `patch/apply` 节点输入，`nextAction` 的非空字符串可机械归一化为 `{"kind":"<string>"}`。这类归一化只能保留原有字面值，不得凭空补造业务参数、执行内容、领域字段或新的决策分支；若无法做无歧义形态修复，仍应按 invalid decision 处理。

Runtime 也可以从失败 diff 与当前 target snapshot 中抽取中立的 `currentTargetAlreadySatisfiesFailedPatch` 信号：当失败 artifact 的新增行已存在、删除行已不存在，且该判断只来自当前文件内容与 diff 行级状态的机械比对时，runtime 可以标记 `stateChangeAlreadySatisfiedByCurrentTarget=true`。该信号不代表业务已正确完成，只表示“重复同一失败 patch 不再合理”。后续 repairActionPackage 应禁止 `repeat_same_failed_patch`，并要求 LLM 返回受限 concrete validation evidence graph，或在无法验证时返回 failed assessment / blocker。

如果 repair 基于当前 `patchRepairContext` 判断目标状态已经满足、失败 patch 已不再需要，runtime 可以接受一个受限 concrete validation evidence graph，用于验证当前状态；该 graph 不得只包含 read-only inspection，也不得包含新的 LLM 嵌套或 patch 节点。若验证失败，新的验证失败 evidence 再驱动后续 state-changing repair。

若 `dodAssessment.satisfied=true` 但 runtime 校验发现 DoD / Goal / acceptance dimension / deliverable coverage 字段缺失或不完整，该失败首先属于 assessment contract correction，而不是新的 state-changing implementation 缺口。runtime 必须先尝试以 `assessmentOnly` / `verification` 模式要求 LLM 补齐 `dodAssessment.items`、`dodAssessment.goalCoverage`、`dodAssessment.acceptanceDimensions`、`dodAssessment.deliverableCoverage` 与 evidenceRefs / artifactRefs；不得在没有新的实现缺口证据时强制重新生成 patch、shell 或其他 state-changing 工作。若补齐后的 coverage 缺口仍没有任何中立候选证据，也不能从已有 satisfied evidence 机械映射，runtime 应切换为 concrete validation evidence repair，要求 LLM 产生受限 verification evidence 或明确 failed assessment / blocker。

如果 assessment-only coverage correction 仍返回 `dodAssessment.satisfied=true` 但继续缺失 DoD / Goal / acceptance dimension / deliverable coverage，runtime 不得立即把 run 终结为 rejected result；应在 runtime repair budget 内继续派发 assessment-only correction，并把最新 missingDoDItems / missingGoalCoverage / missingDeliverableCoverage 作为强约束输入。只有 coverage correction 预算耗尽、LLM 返回明确 unsatisfied assessment、或出现不可恢复 contract violation 时，才进入 rejected / decision / fail 路径。

Completion / assessment 必须遵守“最新非 superseded 证据优先”规则。对于同一中立 scope（例如同一 graph node label、同一 targetRef、同一 failure target scope 或同一可追踪步骤标签），如果较早失败 evidence 之后出现了成功完成的验证、patch 或执行证据，较早失败只能作为 lineage / 历史诊断保留，不能继续主导 accepted / rejected 判断。runtime 在 completion gate 中应把可机械匹配的旧失败 evidenceRef 归一化为后续成功 evidenceRef；在 assessment-only coverage correction 输入中应携带 `latestClosureEvidence` 与 `supersededFailureEvidence`，其中 `latestClosureEvidence` 至少应覆盖最新成功 step evidence、已应用 patch evidence 与已通过 validation evidence。LLM 必须对比最新成功证据后再判断；若最新成功证据仍不足，必须说明具体缺失的中立验收维度，而不是笼统复述旧失败。

Graph expansion limit assessment 是 assessment-only coverage correction 的一种来源，而不是特殊业务流程。当前 follow-up 图扩展达到 runtime 上限时，runtime 必须把 assessment 转为严格证据收口：输入应包含 `runEvidenceSummary`、`latestClosureEvidence`、`supersededFailureEvidence` 和具体 evidence guidance；LLM 只能用可追踪 step / patch / validation evidence 证明 satisfied coverage，不得用 run ID、memory-only ref 或旧失败证据替代。

为了防止长流程中 LLM 过程步骤淹没关键证据，runtime 在构造 `runEvidenceSummary` 时必须按中立证据价值裁剪：优先保留已完成/失败的 action evidence、state-changing artifact evidence、validation evidence 和可用于 closure 的步骤；普通 LLM 过程输出只能作为辅助上下文。该排序只基于 step kind、state、evidence quality 与可追踪性，不解析命令内容、语言、测试框架或业务语义。

当 graph expansion limit assessment 的 failed `dodAssessment` / `goalCoverage` / `deliverableCoverage` 暴露 coverage 缺口时，后续 repair 输入必须将缺口与可候选的最新 action evidence 绑定，例如 `missingDoDItems[].evidenceCandidates` 或 `missingGoalCoverage[].evidenceCandidates`。若候选证据存在，优先进行 assessment-only correction；若候选证据不存在或不足，才进入 concrete validation evidence repair 或治理终态。core 不能因为处于 graph expansion limit 路径就跳过 coverage correction，也不能因缺少领域判断而直接要求 state-changing patch。

如果 assessment-only coverage correction 自身返回 `satisfied=false` 的 failed assessment，并列出仍缺失的 DoD / Goal / deliverable coverage，这已经不是“结构补齐”问题，而是缺少可接受验证证据或真实未满足。后续 runtime repair 必须退出 assessment-only，保留缺口清单到 `verificationEvidenceGap`，并进入 concrete validation evidence repair、rejected result 或 decision blocker；不得继续要求 LLM 只做 assessment，也不得把失败 scope 漂移回早期 planning / scan 节点。

Goal coverage 校验只应把可判定的叶子需求作为覆盖对象。Markdown 标题、分组项、以冒号结尾的父级目标项只用于组织上下文，不得作为必须单独满足的叶子需求。若 Goal 叶子需求与 DoD 项文字不完全一致，LLM 必须通过 `dodAssessment.goalCoverage` 显式建立 Goal requirement -> evidenceRefs 的映射；runtime 不应依赖领域语义硬编码进行推断。

为避免格式差异造成误判，runtime 可以对 Goal / DoD 覆盖文本做中立归一化后再比较，例如去除 Markdown 装饰、标点、空白，并对长文本做保守近似匹配；但不得把完全不同的需求机械合并为同一项，也不得用这种近似匹配替代真实 evidenceRefs。DoD item coverage 可使用包含关系处理“完整原文 vs 归一化条目”的格式差异；Goal leaf coverage 不得使用包含关系接受宽泛合并条目。

Repair 输入必须尽量包含足以定向修复的当前证据：

- `failedStepOutput`：当前失败的精简输出，包括 failure class / signature、error、reason、status、关键诊断和必要的 stdout/stderr 摘要。
- `failureTargetRepairContext`：当失败证据指向具体 workspace 资源时，提供当前目标资源快照、存在性、content hash 和来源说明。
- `resourceShapeAlignmentContext`：当失败诊断包含 unresolved identifier、invalid attribute、shape mismatch、schema mismatch 等中立资源形状冲突时，提供同目录、非失败目标的当前资源快照，帮助 LLM 在修改前对齐现有资源形状。该上下文只能基于路径、manifest、当前快照和失败诊断机械生成，不得按语言、框架、测试约定或业务名称做 core 特判。
- `workspaceFileManifest`：当失败目标需要与其他已知资源核对、DoD 缺口涉及资源证据、或 repair 需要避免误替换无关资源时，提供当前 workspace 资源清单。
- `patchRepairContext` 或等价 capability repair context：当失败来自 state-changing artifact 应用失败时，提供当前目标快照，并要求 LLM 基于当前快照重新生成可应用 artifact，而不是重复失败 artifact。
- `patchRepairSuggestions` 或等价 capability repair suggestion：当 runtime 能从失败 artifact 与当前目标快照中机械抽取唯一 oldText / newText 替换时，提供可直接复用的最小修复证据；该 suggestion 不代表业务判断，只代表当前资源内容与失败 artifact 的中立匹配事实。
- 近期 repair attempt 摘要：用于避免重复同一失败输出。

State-changing repair 必须有 concrete repair target evidence 才能要求 LLM 输出 `patch/apply`。Runtime 在设置 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output`、`repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true` 或 `acceptedKinds=["patch/apply"]` 之前，必须确认至少存在一个可追踪目标证据：`failureTargetRepairContext`、`patchRepairContext`、`repairActionPackage.targetSnapshots`、可解析到 workspace manifest 的 `repairTargetSet` / `structuredFailureSignals.targetRefs` / `lineRefs`、明确 `coverageTargetSet`，或带当前快照的 `rootCauseCandidateContext`。如果这些证据都不存在，runtime 不得把“必须修复状态”合同发送给 LLM；否则会诱导无关 marker、报告、README 或其他形式性 patch。

当 replan、repair-of-repair、schema invalid recovery 或 runtime contract recovery 缺少 concrete repair target evidence 时，runtime 必须进入中立 `acquire_concrete_failure_evidence` 模式。该模式允许有界 `file/list`、`file/read`、`shell/command`、`mcp/tool_call` 或等价能力来获取当前 workspace 的真实失败面、target refs、line refs、stdout/stderr、exit code 和当前快照；不得要求 `patch/apply`，不得接受无边界探索、自检、run snapshot、context rehydration 或新的 LLM 嵌套。若 evidence acquisition 产出确定性失败，再由该失败证据触发 target-scoped state-changing repair。

`acquire_concrete_failure_evidence` 是 state-changing repair 的前置取证阶段，不是新的 verification 终态。取证 graph 完成后，若 graph evidence 中已经包含可追踪 target refs、line refs、manifest/file-list 资源引用、当前资源快照、结构化诊断或等价 concrete evidence，runtime 必须恢复原 repair lineage 与 state-changing repair 合同：`runtimeStage=repair`、`runtimeStageMode=strict_convergence`、`repairObjective=produce_state_changing_output`、`stateChangingOutputRequired=true`，并把原 `repairActionPackage` 中的 failure signature、target scope、structuredFailureSignals、previous rejected summary 与新获得的 target snapshots / target refs / line refs 合并为下一轮 primary actionable contract。不得因为取证 graph 本身成功完成，就把 follow-up 降级为普通 verification，也不得把 `requiredOutput` 置空。

取证 graph 的 follow-up 生成必须以 graph root 的 evidence-acquisition repair 输入作为合同源，而不是只读取最后一个 graph node 的输入。普通 `file/list`、`file/read`、`shell/command` 节点通常不携带原始 `repairActionPackage`；如果 follow-up 只继承最后节点，会丢失原始 repair lineage 并退化为 `verification`。runtime 应从 graph root 恢复原始取证合同，同时把各已完成 graph node 的输出作为 concrete evidence 汇总。可升级为 repair target 的证据仅限通过中立准入的 workspace-relative 路径、line-ref、manifest/file-list entry、当前快照或 capability output；不得根据语言、框架、测试文件名、业务字段名或领域语义做判断。

若 evidence acquisition 完成后仍然没有 concrete target evidence，runtime 不得伪造 state-changing contract；应继续要求有界取证、返回 failed assessment/blocker，或进入真实治理决策。该判断只基于路径/line-ref、manifest、当前快照、failure signature、输出合同和 capability evidence，不得基于语言、框架、测试文件名或具体业务语义。

当 LLM 在 state-changing repair 阶段返回 `blocked_on_decision` / `llm_decision_required`，runtime 必须先判断该阻塞是否可由已有中立证据机械推进。若当前输入已经同时具备：state-changing repair 合同、`repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true`、允许 `patch/apply`、concrete target evidence（failureTargetRepairContext / patchRepairContext / targetSnapshots / repairTargetSet / structured target refs）以及 bounded verification 约束，且 decision context 未声明外部依赖、runtime 配置、上下文门禁、审批或 rejected-result 治理策略，则不得直接创建人工 ticket；runtime 应把它作为 continuable repair contract violation，记录 converted decision 摘要并进入下一轮 repair/replan。只有缺少目标证据、缺少必要外部信息、涉及真实外部依赖/权限/审批、或输出合同不允许 runtime 自行继续时，才允许创建 decision ticket。

Replan continuation 不得只因为 predecessor run 的 execution contract 要求 state change 就直接派发 state-changing repair。它必须先从 predecessor lineage 恢复最近未解决的 concrete failure target、target snapshot、contentHash、coverage target 或 patch repair context；若只能恢复到 LLM schema/runtime contract wrapper failure，例如 `llm_schema_invalid`、`no json object found`、`executionGraph without patch/apply` 或 `coverageTargets missing`，则这些 wrapper failure 只能作为 rejected output / schema recovery evidence，不能替代原始未解决的 workspace 失败面。找不到原始目标证据时，replan successor 必须进入 `acquire_concrete_failure_evidence`。

本约束的实现收口必须覆盖以下检查点：

- Repair action package 统一由 required output contract 决定是否允许 state-changing output；当 `mustIncludeStateChangingNode=false`，任何后续证据门禁不得再把它提升回 patch，例如 `currentTargetAlreadySatisfiesFailedPatch` 必须保持 concrete validation evidence contract。
- 缺少 concrete repair target evidence 时，runtime 必须移除顶层 `stateChangingOutputRequired`，设置 `repairObjective=acquire_concrete_failure_evidence` 与 `concreteFailureEvidenceRequired=true`，并把 `repairActionPackage.requiredOutput.acceptedKinds` 限定为 evidence acquisition 能力。
- Replan continuation 必须把 predecessor failure context 中可机械解析的 `failureTargetScope`、`targetRef`、target snapshot 或 coverage target 写入 `repairActionPackage`；只有该 package 确认可追踪目标证据存在时，才能保持 state-changing repair。
- Runtime contract wrapper failure、schema invalid、`patch/apply targetRef is required`、`executionGraph without patch/apply` 等只描述输出合同的失败，不能单独作为 workspace repair target；若无法恢复原始 target context，必须先获取证据。
- 无论进入 state-changing repair 还是 evidence acquisition，`repairActionPackage` 都是 primary actionable contract；runtime 应压缩重复的 `failedStepOutput`、`previousStepOutput`、`failedExecutionGraph` 等历史 payload，只保留被 package 摘要化后的中立事实，避免 LLM 上下文再次膨胀和冲突。
- 测试必须同时覆盖“无目标证据进入 evidence acquisition”和“有 target refs / target snapshots / coverage target 时保持 patch contract”两类路径，避免后续修改重新回到按失败类别一刀切。

如果 repair-of-repair 是因为 LLM 输出违反 contract，例如缺少必需 artifact 字段、返回只读 graph、返回能力不支持的操作，后续 repair 必须继承上一轮可用的 target context、workspace manifest、state-changing contract、coverage gap context、current execution progress 和 rejection evidence。不得因为 contract violation 发生在 LLM repair step 外壳上，就丢失原始 failure target、Goal/DoD coverage target、已完成证据或当前失败信息，也不得降级为泛化探索。若原始失败来自 graph node，后续 repair 的 ID、lineage label 与 scope 必须继续指向该 graph failure label；即使当前外壳是 `repair-...` 或 schema-invalid repair，也不得退回 planning root。

当 repair-of-repair 的当前失败是 `llm_schema_invalid` / `llm_decision_invalid`，runtime 必须把该失败作为 LLM decision contract failure 处理，而不是 provider failure。repair input 应包含中立的 schema invalid recovery contract：只能基于 `repairActionPackage`、当前 target snapshot 和 schema 约束重建一个合法 JSON decision；禁止复用 rejected raw content / preview，禁止 markdown 包裹、注释、重复 JSON object、尾随多余括号和局部数组。该规则只治理输出形态和 lineage，不引入任何业务领域判断。

当 repair-of-repair 的失败来自 runtime 输出合同或准入合同，例如 `strategyReassessment.required=true`、`executionGraph patch/apply input invalid`、`scoped state-changing runtime repair requires bounded verification` 或等价中立合同错误，failure class 必须保持 `runtime_failed` 或 `llm_decision_invalid`，不得因为错误文本中包含 `forbiddenRepair`、`API Key`、`auth` 等业务/合同词而误归类为 `auth_or_config_error`。Provider auth/config 只适用于 `llm_provider_*` 阶段或明确 provider auth/config stage；runtime 合同失败后续 repair 必须继续继承原始 failure surface、repairActionPackage、target context 和 strategy reassessment。

Runtime contract correction failure 是一类独立的中立 repair-of-repair 信号。若失败文本命中 `strategyReassessment.required=true`、state-changing repair contract、patch/apply input contract、bounded verification contract、primary diagnostic contract 或等价输出准入合同，runtime 必须把它识别为 LLM/runtime contract loop，而不是新的业务失败面。该类失败的下一轮输入必须满足：

- `repairActionPackage` 继续作为 primary actionable contract，继承上一轮的 `currentFailure`、`failureTargetScope`、`targetSnapshots`、`repairTargetSet`、`coverageTargetSet`、`primaryDiagnosticRepair`、`rootCauseCandidateContext` 和 `strategyReassessment`。
- 若上一轮已有 concrete target evidence，下一轮不得降级成泛化 exploration / planning / verification，也不得把合同字段名当作 workspace repair target；应继续 `need_state_change`，并把 rejection reason 作为 `previousRejectedOutputSummary` / `contractCorrectionFailure`。
- 若上一轮没有 concrete target evidence，下一轮进入 `acquire_concrete_failure_evidence`，但仍保留原 contract violation 摘要，避免让 LLM 把合同字段名、schema 字段或错误类别当作业务资源。
- 同一 `failure class + target scope + contract marker` 的重复合同失败必须消耗同一 repair budget。step ID、run graph 编号、repair 编号或 follow-up wrapper 改变不得重置预算；达到预算后必须进入 bounded replan、failed assessment、rejected result 或真实治理阻塞。
- `currentProgressDigest` 与 `processEvidenceTimeline` 中的最新缺口必须仍指向原始未关闭 failure target 或 coverage target；runtime contract wrapper 只能作为 latest rejected output，不得覆盖当前目标。

上述规则只依赖中立合同 marker、target scope、lineage、evidence 和 budget，不得解释代码语言、测试框架、业务领域或具体 Goal 语义。

当 `strategyReassessment.required=true` 且 repair 输出需要 state-changing patch 时，`repairActionPackage.requiredOutput` 必须显式包含 `mustDeclareStrategyReassessmentFields=true` 与 `strategyReassessmentRequiredFields`，默认字段为 `failedSurface`、`rootCauseTarget`、`repairTargetDecision`、`repairMode`、`forbiddenRepair`、`verificationPlan`。这些字段是中立 runtime 合同，不表达任何具体业务含义；LLM patch/apply 节点必须在自身 `input` 对象中直接声明这些字段并与当前 failed surface 对齐。不得输出 top-level `strategyReassessment`，也不得把 `strategyReassessment` 放在 executionGraph node 的 `input` 兄弟字段上；否则属于 JSON / execution contract violation。为避免可机械修复的决策因字段位置偏差被直接拒绝，parser 可以在存在 `executionGraph.patch/apply.input` 时把 top-level `strategyReassessment` 的字段下沉到每个 patch/apply node input 并移除顶层字段；该归一化只修正 JSON 形态，不放宽 strategy reassessment 的字段完整性、failed surface 对齐、repairMode 和 bounded verification 校验。

重复 failed surface 的策略重评估不得被简化为“必须切换 root-cause target”。Runtime 必须要求 LLM 明确选择 `repairMode`：

- `continue_same_target_repair`：继续修复当前 failed surface / failure target。只有当 patch/apply 命中当前 `primaryDiagnosticRepair` 或等价结构化 failure signal，并且目标属于当前 `repairTargetSet` / `failureTargetRepairContext` / target snapshot，且后续 verification 绑定当前 failure scope 时才允许。该模式适用于上一轮 state-changing artifact 部分成功后留下新的同目标残留诊断，或当前证据表明 failed surface 本身仍是最小可修复目标。它不是预算重置；若同一 failure class + target scope + diagnostic 在 state-changing 后继续重复失败，仍必须进入 blocker / failed assessment / replan 治理。
- `shift_root_cause_target`：切换到不同 root-cause target。只有当目标来自 `rootCauseCandidateContext`、当前 manifest / failure refs 唯一解析出的中立候选，或与当前 failure target 存在可追踪的中立关系证据时才允许。patch/apply target 必须与声明的 `rootCauseTarget` 对齐，并通过 `relatedFailureTargets` 说明它解除的 failed surface。Core 不得根据语言、框架、文件名或业务语义推断候选。

当 `repairMode=continue_same_target_repair` 时，runtime 不得因为 repeated failed surface 直接拒绝同目标 patch；准入重点应转为：是否基于当前 target snapshot、是否命中当前主诊断、是否使用有界验证、是否仍受 repair budget 约束。当 `repairMode=shift_root_cause_target` 时，runtime 才要求 patch target 与声明的不同 root-cause target 对齐。缺少 `repairMode`、`repairMode` 非法、同目标继续修复未命中主诊断、切换目标缺少中立候选证据，均必须作为 strategy reassessment contract violation 拒绝或要求返回 blocker / failed assessment。

`primaryDiagnosticRepair` 只在结构化失败证据包含具体 workspace target refs 或 line refs 时启用。仅有 identifier、schema-invalid 字段名、LLM decision contract 字段名或错误类别文本时，不得升级为“必须 patch 行级主诊断”的资源修复合同；这类失败应按 LLM decision contract recovery 或 runtime contract recovery 处理，并继承上一轮可用的 target snapshot 与 repairActionPackage。该规则避免把输出 JSON 合同错误误判为 workspace 资源形状错误，同时保持资源修复约束完全基于中立路径/行号/manifest 证据。

当 failed surface 由错误输出中的裸文件名、相对路径、行号或 `failureTargets` 给出时，runtime 必须先用当前 workspace manifest 将其解析为真实 workspace-relative path，再生成 `failureTargetRepairContext`、`repairTargetSet` 和同目录候选。不得把 `main_test.go` 这类裸目标直接当成最终 scope，导致 `gateway/main_test.go` 的同目录资源被遗漏；也不得把运行时栈、标准库路径、临时测试入口或不存在的外部路径提升为唯一修复目标。该规则是中立路径解析规则，不依赖具体语言、测试框架或业务语义。

当 failed surface 已有当前快照，且 workspace manifest 中存在同目录、同 scope、结构化 failure refs、当前 target snapshot 或 resource-shape alignment 等机械证据指向的相邻资源时，runtime 可以向 repair 输入提供中立 `rootCauseCandidateContext`。候选只能来自这些当前 workspace 证据；不得根据文件名、语言、测试框架、业务语义或具体 goal 内容硬编码推断。对于重复 failed surface 或 repair-of-repair 的 schema / contract wrapper failure，runtime 还必须继承上一轮 `strategyReassessment.failedSurface` / `failureTargetScope` 来解析原始 failed surface，并据此生成候选；不得因为外壳失败签名缺少 target scope 就丢失候选上下文。候选生成必须跳过二进制、不可访问、runtime 内部文件和已知 failed surface 本身，并按同 scope、同目录、路径 stem 关系、结构化 refs 等中立关系排序，保留足够数量的当前快照，避免文件系统顺序或构建产物遮蔽真实可修复资源。LLM 可以 patch 不同于 failed surface 的 root-cause candidate，但 patch/apply 必须命中 `repairTargetSet` 中的候选，或声明 `relatedFailureTargets` 指向被其解除阻塞的 failed surface。runtime 准入只允许命中这些已给出当前快照且存在中立关系证据的候选；未出现在候选上下文或未能对齐 failed surface 的目标仍必须被拒绝或要求返回 blocker / failed assessment。

`rootCauseCandidateContext` 也是当前 repair context 的一部分。若候选快照完整、安全、未脱敏、未截断并带有 `contentHash`，它可以作为 `unified_diff` 和受限 `file_write + baseHash` 的当前基线；若候选快照不完整或不存在，runtime 不得让 LLM 直接生成固定 state-changing artifact，而应要求先读取、返回 blocker 或转入人工决策。该规则只治理“失败面与根因候选之间的中立可追踪关系”，不判断候选内容是否业务正确。

被 runtime 拒绝的 repair plan 不得把自身 `summary` / `thoughtSummary` 当成可复用 execution lesson 写入 memory。memory 只能记录 rejected fact、failure class、failure signature、当前失败和仍需遵守的 contract，不能把已拒绝的 rootCauseTarget、无关目标选择、占位变更理由或验证计划沉淀为下一轮决策依据。

State-changing patch 不能只是为了满足 runtime contract 或 repair convergence 而添加 marker/comment/no-op。若 patch 摘要或内容表明其目的只是“满足补丁节点/状态变更/契约/收敛/严格收敛/验证锚点”，或类似 `runtime-repair` marker / `state-change anchor` / `verification anchor`，runtime 必须作为 non-substantive state-change placeholder 拒绝。该规则只检查补丁意图与形态，不判断具体业务语义。

当 verification / strict convergence follow-up 已经识别出需要继续收敛时，可以返回受限 concrete validation evidence graph，例如明确的 shell/tool/mcp 验证节点；该 graph 不需要包含 patch，但不得只包含 read-only inspection、自检、run snapshot、context rehydration 或新的 LLM 嵌套。若再次返回只读 inspection graph 且没有 state-changing output、concrete validation evidence 或 rejected assessment，该失败属于 contract violation。后续 repair 必须携带对应的 rejection evidence；只有 failure evidence 已表明缺口是实现/状态变更缺口时，才提升为 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`，不能把纯验证证据缺口误升级为写文件 repair。

Runtime 可以把失败证据中的目标引用解析为当前 workspace 中的已知资源快照，但只能使用中立路径匹配和 manifest 事实；不能把某个 profile 的文件命名、测试约定或技术栈规则写入 core repair。

失败目标引用不仅来自结构化 `failureTargets` / `targetRef` / `failureSignature`。当非 LLM 执行步骤的 `stdout`、`stderr`、`error`、`reason`、`lastFailureDiagnostics` 或 `failedStepOutput` 中以中立路径/line-ref 形态出现 workspace 资源引用时，runtime 必须把这些引用纳入 `structuredFailureSignals.targetRefs`，再通过当前 workspace manifest 唯一解析为 `failureTargetRepairContext` 与 `repairActionPackage.targetSnapshots`。该抽取只能依赖路径形态、line-ref、manifest 和当前资源快照，不得依赖命令名、语言、测试框架或文件命名约定。若目标无法唯一解析，必须保留诊断摘要并要求读取/阻塞，而不能让 LLM 在缺少当前快照的情况下生成固定 state-changing artifact。

结构化 failure target 抽取必须过滤明显不是 workspace 资源的中立噪声，例如单独的运行产物标签、duration / numeric measurement、runtime contract 字段名、URL、空路径和无法通过 manifest 唯一解析的裸 token。过滤规则只能基于通用路径形态、扩展名形态、数值形态和 workspace manifest，不得基于具体命令、语言或测试框架。过滤后的 token 仍可作为 diagnostic message / identifier 保留，但不得进入 `targetRefs`、`failureTargetRepairContext` 或 `repairTargetSet`。

Repair exhausted 后必须二选一：

- 写 rejected result 并进入 failed / blocked_on_decision。
- 创建 replan successor run，并保留 predecessor lineage。

为支持 blocked_on_decision 后的继续处理，因 repair exhausted、duplicate failure 或其他可恢复治理点写入的 rejected result 只能代表当时失败窗口的临时结论。若后续已经出现针对该 rejected result source step 的 provider retry、runtime repair 或 decision-authorized retry 进展，该 rejected result 必须转为 `superseded`，不得继续阻断当前 run 的恢复、repair 或最终 acceptance。

## Runtime Capability Time Budget

非 LLM runtime capability 的默认执行预算必须来自中立配置或 capability 自身声明，不得在 core runtime 中按业务领域、语言、框架或命令名称写死。`shell/command` 这类通用执行能力应提供可配置默认超时，并允许单个 step 通过结构化字段声明更长但有上限的预算。

当 `shell/command` 通过 `sh -c` 执行的脚本显式依赖 `pipefail` 等 bash-only 语义时，runtime 可以在不改变步骤语义的前提下切换到兼容的 shell 执行器；该兼容必须是通用执行策略，不得以业务命令名或业务目录名做条件分支，并且应在 step output 中记录实际执行器以便追踪。

`shell/command` 的工作目录属于 runtime capability contract，而不是业务语义。若 capability 已声明其工作目录由 runtime 绑定到 workspace 交付目录，则 LLM 不得再硬编码 `cd /home/user/workspace`、`.workorch-workspaces/...` 或等价的绝对 workspace 路径；core 可以把这类输出视为 capability-boundary violation 并拒绝，因为它破坏的是通用执行能力合同，而不是某个具体业务规则。

`mcp/tool_call` 的 stdio server 启动目录同样属于 runtime capability contract。对 workspace-scoped stdio session，runtime 应以 workspace 交付目录作为子进程工作目录，并继续通过 env / args 提供必要的 workspace ref；LLM 向工具传入的相对 path/root 参数应按该能力合同解析，不应依赖 WorkOrch 进程自身 cwd。若 MCP tool result 按协议返回 `isError=true` 或等价结构化失败标记，runtime 必须把该 step 标记为 failed 并保留原始 result 作为诊断证据，不能把其作为 completed evidence 输入后续 DoD / Goal coverage 判断。

默认预算用于避免普通长流程任务被过短固定值误杀；单步覆盖用于让 LLM 在严格收敛阶段为明确长耗时操作申请有限执行窗口。无论默认值还是单步覆盖，都必须是有界的，并在 step output 中记录实际 `timeoutMillis`，作为后续 repair / verification 的中立证据。

当非 LLM 执行因为 timeout 失败时，runtime repair 只能基于“执行超时”这一中立失败形态选择重试、缩小动作、调整有界预算、返回 failed assessment 或 blocker，不得依赖具体命令名推断业务策略。

## Provider Retry 合同

Provider retry 始终只处理 provider transient failure，与 stage 语义无关。
Provider retry 预算必须来自 runtime 配置，默认 `maxProviderRetryAttempts=10`；不得在 core runtime 分支中写死固定业务场景次数。

## Approval Auto-Governance Boundary

Approval auto-governance 只能依赖中立风险信号、当前 Goal / DoD 边界和可追踪的 workspace scope，不得因为具体语言、包管理器、框架或业务场景写死审批规则。

当 approval 请求满足以下中立条件时，core 可以将其归类为 `L2 / workspace_low_risk_write` 并走自动治理，而不是强制人工审批：

- 动作是当前 workspace 交付目录内的最小状态变更。
- 变更目标属于 dependency / manifest / metadata / lock / checksum / requirements 或等价依赖声明配套资源。
- 目的仅是恢复当前 Goal / DoD 所需的本地构建、验证或交付一致性。
- 未出现 remote / ssh / external / network write / production / deploy / publish / destructive 等更高风险信号。

该边界只表达“工作区内低风险配套元数据修复”的中立风险分类，不允许 core 根据具体生态名字、依赖源、语言栈或业务模板推断更多领域语义。只要信号升级到远端写入、生产操作、供应链风险扩大、凭证访问或破坏性变更，runtime 仍必须进入人工审批。

Provider retry / config blocker 的边界必须基于中立失败形态：

- `timeout`、`rate limit`、`context too large`、`provider stream incomplete`、已完成 stream 但内容为空的 `provider stream error`、未带明确配置错误标记的 `provider unavailable / request failed / provider error` 等瞬时 provider 执行失败，应先消耗 provider retry 预算。
- `auth or config`、`schema drift`、明确不支持的 provider operation、明确 client/config unavailable、4xx、缺失 provider / model / credential 等配置型失败，可以直接进入 provider/config blocker。
- provider/config blocker 的 `attempts` 必须记录真实已失败 provider retry 次数，`maxAttempts` 才记录预算上限；不得在未实际重试时把 `attempts` 写成预算上限。
- provider retry 不得消费或重置业务 repair/replan 预算；它只表示 runtime LLM provider 调用基础设施暂时未产生可用 LLM 输出。
- LLM 已经返回可解析 decision，但 decision 因 runtime 输出合同或准入合同被拒绝时，不属于 provider transient failure。包括 `executionGraph patch/apply input invalid`、`patch describes a non-substantive state-change placeholder`、`scoped state-changing runtime repair requires bounded verification`、`strategyReassessment.required`、invalid decision/schema contract 等等价中立合同错误，必须继续进入 runtime repair / repair-of-repair / blocker 治理，并继承 `repairActionPackage`、target snapshot、current failure 与 stage contract。

### LLM 运行期动态绑定

LLM provider、model、base URL、context profile、failover 目标等信息属于运行期动态绑定，不属于 workspace/run/step/ticket 的可恢复业务状态。runtime 持久化记录可以保留中立观测指标，例如 token 数、请求/响应字节、耗时、failure class、stream diagnostics，但不得把 provider/model/base URL 作为后续恢复、retry、repair、memory 或 decision 的依据。

因此：

- run activation、CLI、HTTP、backend/eventhub 等入口不得把 `llmProviderId` 写入 run execution context。
- step input、provider retry input、decision ticket context 和 LLM context compaction 必须剥离 provider/model/base URL 等绑定字段。
- provider/config blocker 被解决并收到“继续处理 / retry same step”后，runtime 必须重新按当前系统配置动态解析 provider；不得因为历史 step 记录中的旧 provider 继续原地 reblock。
- provider unavailable / auth or config / schema drift 这类配置型失败可以先进入 config blocker；但 blocker 解决后的 retry 是一次新的运行期绑定，不继承旧 provider 选择。
- 显式 provider 选择如后续重新支持，必须设计为短生命周期调度参数，而不是持久化恢复状态。

允许：

- timeout。
- rate limit。
- context too large。
- provider stream incomplete。
- provider stream error with empty content。

禁止：

- 用 provider retry 修复 LLM contract violation。
- retry step 成为新的 retry root。
- retry 后丢失原 stage contract。
- retry 后把 state-changing 要求降级为 exploration。

### LLM Stream 完整性

Runtime LLM 的结构化输出必须以完整 provider 响应为准。

- HTTP SSE / compatible chat completion provider 只有收到 `[DONE]` 才能把 streamed content 视为完整输出。
- EOF 但未收到 `[DONE]` 必须归类为 `provider_stream_incomplete`，不得把已收到的 partial delta 交给 LLM decision parser。
- timeout 且已有 partial stream delta 时仍按 `provider_timeout` 治理，但 failure output 必须记录 `streamDiagnostics`，至少包含 `doneReceived`、`streamIncomplete`、`receivedDeltaCount` 和 `receivedBytes`。
- 收到 done 但 `receivedBytes=0` / `receivedDeltaCount=0` 时，不得当成有效 LLM 输出；应归类为 provider stream error 并进入 provider retry 预算，预算耗尽后再进入 provider/config blocker。
- 已收到 done 但 JSON schema 无效时，归类为 LLM decision/schema 问题，不得误判为 provider stream incomplete。

## 文档合并状态

本文已经吸收 runtime core flow、LLM context integrity、neutral repair grounding、provider retry、lineage / assessment、readable ID / recovery、Goal / DoD change impact、result evidence strictness、rejected result continuation、repair convergence 等核心约束。相关旧文档不得继续作为并列设计依据；后续实现、核对和评审只以本文为 WorkOrch core runtime 的长期核心约束。

### 已移出核心合约范围的内容

| 内容 | 原因 | 保留位置 |
| --- | --- | --- |
| 具体软件开发实现流程、测试命令、代码 patch 规则 | 属于 software-dev profile，不属于 core runtime | `software-dev-*` 文档或后续 profile 文档 |
| Web/TUI/API 交互细节 | 属于产品交互和服务接口，不是 runtime stage contract | 对应 Web/TUI/API 文档 |
| 数据库存储路径、file backend layout 细节 | 属于持久化实现，不是阶段合约 | `workspace-data-layout-plan.md` |
| 已完成任务流水和日期记录 | 只用于追溯，不应污染权威设计 | progress/status/audit 文档 |
| 某个具体 workspace 的排障样本 | 可作为验证 case，不应成为 core 规则 | 部署核对记录或 e2e fixture |

## LLM 上下文完整性合同

每次 LLM 调用必须优先回答四个问题：

1. 当前目标是什么：workspace、run、stage、step、Goal、DoD、deliverable expectation、profile contract、allowed output、forbidden output。
2. 已经完成什么：最新 completed action evidence、state-changing artifact、validation evidence、artifact refs、coverage refs、当前 deliverable 状态摘要。
3. 当前失败是什么：failedStepId、failureClass、failureSignature、failureTargetScope、stage、error、reason、diagnostics，以及当前失败是否只是外壳 contract failure。
4. 下一步必须做什么：repairObjective、requiredOutput、forbiddenActions、coverageTargetSet、exitCriteria、是否允许只读、是否必须 state-changing。

若上述任一问题无法由当前上下文回答，runtime 不得让 LLM 假装继续严格收敛；必须补充上下文、读取目标、缩小任务、进入 blocker / failed assessment，或按恢复策略 replan。

### 上下文优先级

| 层级 | 内容 | 约束 |
| --- | --- | --- |
| P0 Contract | runtime contract、输出 schema、workspace/run/stage/step identity、当前 Goal / DoD、当前动作要求 | 不得静默删除；超预算必须 fail-fast、换更大上下文配置或进入治理 |
| P1 Current Failure | failedStepId、failureClass、failureSignature、stage、error、runtimeDiagnostics、repairObjective、requiredOutput | 不得被 provider retry / repair-of-repair 外壳失败覆盖 |
| P2 Coverage Scope | missingDoDItems、missingGoalCoverage、missingDeliverableCoverage、coverageTargetSet、evidenceCandidates | 不得因 contract violation 被清空 |
| P3 Current Evidence | latest action evidence、patch/tool/validation evidence、artifact refs、currentExecutionProgress | 保留 step/ref/摘要；大 stdout/stderr 使用 head/tail/preview + ref |
| P4 Current Snapshots | failureTargetRepairContext、patchRepairContext、targetSnapshots、workspaceFileManifest | 小目标完整；大目标窗口化或 ref；安全完整快照不得误脱敏 |
| P5 Recent Attempts | 最近 repair attempts、invalid strategy、duplicate failure、forbiddenActions | top-k 摘要；保留失败原因和避免重复策略 |
| P6 Historical Context | superseded runs、早期 memory、旧 governance、旧 graph evidence | 默认摘要或引用；不得覆盖 P0-P5 |
| P7 Raw Archives | response.sse、full tool output、完整 payload、历史 raw artifacts | 默认 ref only |

P0-P4 是当前决策必要上下文。若 context budget、redaction 或 compaction 导致 P0-P4 的关键字段缺失，runtime 必须在 `contextBudget.integrityWarnings` 中记录并阻断不可靠 state-changing 输出；不得把缺失上下文伪装为完整上下文继续提交 provider。

### 顺序与追溯

LLM 输入中的 step / event / evidence 顺序必须稳定：

- 同一 run 内优先按 monotonic sequence / event sequence 排序。
- 无 sequence 时按 occurredAt / createdAt 排序。
- 时间缺失时按 step id 中的 graph / node / attempt 编号排序。
- 同一中立 scope 内，较新的成功 action / validation evidence 优先于历史失败；历史失败只能作为 lineage 或避免重复策略的背景。

任何被摘要、裁剪、脱敏或引用化的内容都必须保留 source refs：workspaceId、runId、stepId、graphNodeId / graphRootStepId、artifactRef / payloadRef、targetRef、contentHash、omitted reason、redaction reason 和 compaction rule。

### Request Audit

每次 LLM request 必须输出可观测的 `contextBudget` / request audit 摘要，至少包含：

- capability source、provider/model context profile、max tokens / max bytes、actual bytes / estimated tokens。
- included / omitted / redacted sections。
- protectedFields 与 integrityWarnings。
- trace policy 与 source refs。
- 是否 goal / DoD 被压缩、是否 P0-P4 被截断、是否应用 redaction。

`contextBudget` 是上下文可靠性证据，不是业务状态。provider/model/base URL 等动态绑定不得作为后续 repair / memory / recovery 的持久依据。

### Cache-Aware Request Organization

LLM request 的组织方式还必须兼顾 provider prompt / context cache 的复用机会。缓存友好组织的详细设计见 `workorch-llm-request-cache-aware-context-design.md`，但该优化只能作为上下文完整性合同之下的 request rendering 规则，不能改变 stage contract、context gate 或 Goal / DoD 完整性要求。

核心约束：

- request 必须按稳定性分区组织：stable runtime prefix、workspace definition、capability contract、current stage bundle、volatile execution tail、large refs / trace。
- stable runtime prefix 不得包含 workspaceId、runId、stepId、timestamp、provider/model/baseURL、memory recall、当前失败或当前执行进展。
- C0 stable runtime prefix 只能表达全局 schema / runtime 输出合同，不能随 assessment-only、repair、follow-up、validation、provider retry 等当前路径变化。当前路径的模式限制必须进入 C3 stage contract 或 C4 current-action。
- Goal / DoD 必须完整位于 workspace definition block，并通过 revision / hash 被后续 stage bundle 引用；不得为了缓存复用裁剪、摘要或移入易变 memory。
- workspace definition block 只在同一 definition revision 内稳定。Goal / DoD 允许在运行期补充、澄清和修订；每次变更都必须生成新的 definition revision / hash，并使后续 stage bundle、coverage baseline、repair / validation / acceptance request 显式引用最新 revision。
- Goal / DoD 运行期变更后，旧 revision 只能作为 lineage / historical evidence 使用，不能继续作为当前目标或验收基准。Runtime 必须在 request 中提供 definitionChangeImpact，说明已有 plan / output / evidence 是否仍覆盖最新 definition，以及下一步应 replan、repair、revalidate、acceptance 还是进入 decision。
- 当前失败、当前进展、timeline tail、target snapshot、latest validation、memory recall 等高频变化信息必须位于易变尾部，不能污染 stable prefix。
- C2 capability contract 必须表达声明式能力目录和通用能力使用规则，不能因为 assessment-only 或当前阶段临时禁用可执行输出而改写为另一套能力目录。当前步骤是否允许调用可执行 block 属于 C3/C4 当前动作约束；runtime 仍必须在执行层拒绝违反 assessment-only 的 executable output。
- section 顺序、字段顺序、source refs 顺序、空字段策略必须 canonical 化，避免语义相同但文本不同导致缓存失效。
- 每次 request audit 应记录 stablePrefixHash、workspaceDefinitionHash、capabilityContractHash、stageBundleHash、volatileTailHash 和 cacheBreakers；这些字段用于观测，不作为业务恢复状态。
- 同一 run 内 `stablePrefixHash` 不应变化；若变化，视为 request rendering 缺陷。`cacheKey` 可以因 Goal / DoD revision 或 capability revision 变化而变化，但若同一 run 内无定义变更却出现过度分裂，必须通过 request-log 校验提示 C1/C2/C4 churn。
- provider 私有 cache-control 只能由 adapter 基于中立 `cachePlan` 转换，core runtime 不直接绑定具体 provider cache API。
- `promptContext.sections` 必须表达 C0-C5 语义分区、cacheability、hash 和 source refs；`turn-01` 这类顺序 ID 只能作为 display order，不能替代 section identity。`promptContext.turns` 作为 provider-facing 渲染结果时必须引用 `sectionId`。
- 易变尾部不能继续膨胀为一个无语义标签的大 user turn。Current failure、latest validation、tool call/result、artifact event、definition change、context gate、memory recall、requested action 等必须按中立 typed event tape 组织，并按 sequence 保序、按 event 粒度摘要 / 引用 / 裁剪。
- Memory recall 只能作为当前决策参考进入易变尾部；未被当前 retrieval 选中、已被 excluded、stale / superseded / diagnostic-only、低置信或低重要度的 memory 不得默认进入 C4。Retrieval 审计摘要可以保留，但不能让无效 memory 正文影响 LLM。
- Provider cache hint 必须保持 provider-neutral：core 只能输出 `cachePlan.adapterHint` 这类可选映射建议，具体 provider 私有 cache-control 只能由 provider adapter 自行消费，且不得写入业务恢复状态。
- cache-aware canonical rendering 只能稳定 section 顺序、map key 顺序、无顺序语义 refs / capability ids 顺序；不得重排 Goal / DoD definition events、processEvidenceTimeline、typed event tape、tool calls、validations、steps、memory recall priority 等具备语义顺序的数据。
- 实现落点必须优先集中在 LLM block 的 request / prompt 构造层：`PromptContext` / `PromptSection` / `PromptTurn` / `ProviderContextBudget` 模型、`prompt_messages` section builder、provider budget audit、provider request redaction 和 runtime adapter request 落盘。Orchestrator 侧只复用已有 `executionContextPackage`、`processEvidenceTimeline`、`currentProgressDigest`、`definitionChangeImpact` 与 `runtime_context_order`，不得为缓存优化另造与运行态 evidence path 不一致的排序规则。
- `scripts/verify-llm-cache-aware-request-log.sh` 必须能离线发现结构缺失、同一 run 内 stable prefix 变化、C4 体积膨胀、non-cacheable 输入占比过高、cacheKey / capability hash 过度分裂等缓存质量问题；这些检查只作为 request 质量审计，不参与业务状态恢复。

### Stage Input Bundle 合同

为避免 memory、soul、skill、tool、mcp、prompt 分散注入，后续 runtime 应为每个 stage 形成中立的 input bundle。Core 只负责 bundle 的结构与引用，profile 负责选择具体能力。

| Bundle section | Core 含义 | 来源 |
| --- | --- | --- |
| `contract` | Goal、DoD、runtimeStage、allowed artifact、forbidden output、result policy | run / goal / dod / stage contract |
| `continuity` | 当前 unit、上一步有效输出、必要 lineage、active blockers | orchestrator / result / ticket |
| `progress` | 当前执行进展、最近动作、最近验证、当前缺口、下一步要求 | step output / tool output / validation / runtime governance |
| `memory` | 被筛选后的 accepted input、necessary output、evidence、decision memory | memory recall |
| `capabilities` | 当前 stage 可用的 skill/tool/mcp/soul/prompt 引用与能力摘要 | profile / capability registry |
| `evidence` | 当前可用于 verification / acceptance 的证据引用 | step output / artifact / result |
| `budget` | stage、repair、provider retry、context budget | runtime governance |

Stage input bundle 必须遵守：

- Goal / DoD 不可被压缩、改写或用摘要替代。
- memory recall 只进入高价值决策信息，不堆叠 runtime 流水。
- skill/tool/mcp/soul/prompt 进入 bundle 时只提供能力摘要和约束，不注入领域硬编码。
- exploration / planning / replan continuation 的 LLM input 必须包含当前 workspace 的有界 manifest / known resources。该清单只表达“当前已知资源事实”，不得编码语言、框架、文件类型或业务领域规则；其目的在于避免 LLM 在已有工作区中误生成初始化路径。
- repair bundle 必须包含 failure scope、来源阶段、失败 evidence、已尝试修复、必要目标快照、workspace manifest 和必须继承的 contract。
- replan bundle 必须包含 predecessor lineage、replan ticket / resolution、原失败 evidence、已失败 repair path、可保留产物线索和仍然有效的 Goal / DoD / execution contract。
- capability 摘要必须包含当前能力支持和不支持的中立操作，例如必需字段、禁止字段、是否支持删除、是否要求完整内容或完整 diff。
- profile-specific bundle 字段必须位于 profile namespace，不能提升为 core 字段。

### Codex 启发的中立过程证据合同

通过核对 `/home/rangh/aispace/ai-proxy/logs/codex` 中 Codex 长流程代码开发请求，可以借鉴的是上下文组织机制，而不是 Codex 的私有 request schema、隐藏 reasoning、具体工具名或软件开发专用提示词。WorkOrch core 只能吸纳其中的中立原则：

- LLM request 必须是分层上下文包，而不是单个无差别 runtime 快照。
- 稳定 runtime 合同、当前 Goal / DoD、当前 stage bundle、当前 failure / required output 必须排在历史信息之前。
- 最近过程证据必须按发生顺序保留，包含“已存在什么、刚尝试了什么、实际输出是什么、失败在哪里、下一步必须完成什么”。
- 工具输出、验证失败、provider 诊断、large artifact 与 raw stream 必须以摘要 + source ref 形式进入上下文，完整原文通过 ref 追溯。
- 当失败 stderr / stdout / validation output 是 LLM 判断下一步的必要依据时，必须提供足够的 head / tail /关键片段和完整 ref；不得只给抽象失败标签。
- 历史 superseded run、旧 rejected decision、provider/model 动态绑定、raw stream delta 不得覆盖当前 failure surface。
- 不依赖隐藏 reasoning 作为可恢复状态；可恢复状态必须来自显式 step output、artifact、evidence、memory、ticket、approval 或 request audit。

WorkOrch 因此必须在 stage input bundle 中形成中立 `processEvidenceTimeline`。该 timeline 不表达任何具体业务领域语义，只表达可追溯的运行事实：

```json
{
  "processEvidenceTimeline": [
    {
      "order": 1,
      "sourceRef": "step://...",
      "stage": "execution",
      "actionKind": "tool_call|llm_decision|artifact_write|validation|runtime_gate",
      "state": "completed|failed|skipped|blocked",
      "summary": "...",
      "inputSummary": "...",
      "outputSummary": "...",
      "failureClass": "...",
      "failureReason": "...",
      "evidenceRefs": ["..."],
      "payloadRef": "artifact://..."
    }
  ],
  "currentProgressDigest": {
    "existingOutputs": ["..."],
    "lastAttempt": "...",
    "lastObservedFailure": "...",
    "currentGap": "...",
    "nextRequiredAction": "..."
  }
}
```

`processEvidenceTimeline` 的收敛规则：

- `order` 必须稳定、可复现，并与 step / event 发生顺序一致。
- 每条记录必须有 `sourceRef`，没有 sourceRef 的自然语言进度不得作为高置信证据。
- 最近 state-changing action、最近 validation、最近 failure、最近 accepted / rejected assessment 必须优先保留。
- 超预算时可以摘要较早 timeline，但必须保留 source refs、state、failureClass、evidenceRefs 和 omitted reason。
- P0/P1 必要过程证据缺失时不得静默提交 provider；必须由 context gate 选择 `read_required`、`blocker_required`、`fail_fast` 或 `replan_required`。
- `repair`、state-changing continuation、带 `replanContinuation` 的 successor step 若缺少 `processEvidenceTimeline` 或 `currentProgressDigest`，属于上下文完整性失败。Runtime 必须在 provider 前 fail-fast 或先补齐读取/继承，不得让 LLM 在缺失最近过程证据时继续生成 state-changing 输出。
- timeline 不得包含完整旧 execution graph、完整旧 provider request、raw stream 或 provider/model/baseURL 动态绑定。

Codex 中的软件开发行为只能作为 profile / workspaceTemplate / soul / skill 的输入约束吸收，例如“先读已有资源再改”“优先沿用当前工作区风格”“验证失败后根据失败证据修正产物”“避免无关变更”。这些规则不得进入 core runtime；core 只校验它们是否被 profile 声明、是否进入当前能力边界、是否产生可追踪 evidence。

## Runtime Context Enforcement Contract

上下文完整性不能只依赖 prompt 文案。Runtime 在向 LLM provider 提交 request 前，必须先对 stage input bundle 执行中立准入校验，并把校验结果写入 request audit / step output。该合同只校验结构、引用、完整性和阶段约束，不判断任何具体业务语义。

### Context Bundle Schema

每个 LLM step 的输入必须能归一化为以下 bundle。字段可以在物理 JSON 中分散存在，但 request assembly 前必须形成等价结构：

| Section | 必填字段 | 可选字段 | 禁止字段 |
| --- | --- | --- | --- |
| `contract` | `workspaceId`、`runId`、`stepId`、`runtimeStage`、`runtimeStageMode`、`goal` / `goals`、`dod` / `dods`、`allowedOutputs` 或 `requiredOutput` | `profileContract`、`stateChangePolicy`、`resultPolicy`、`assessmentOnly`、`stateChangingOutputRequired`、`repairObjective` | provider/model/baseURL、raw stream、raw rejected output |
| `continuity` | 当前 unit 或当前恢复 root、predecessor lineage、active run / active blocker 状态 | `replanContinuation`、`rejectedResultContinuation`、`governanceSummary`、`definitionChangeImpact` | 完整历史 executionContext、完整旧 graph payload、provider 动态绑定 |
| `failure` | 当前失败 step id/kind/state、failureClass、failureSignature、stage、reason/error | `failureTargetScope`、`structuredFailureSignals`、`runtimeDiagnostics`、`streamDiagnostics` | raw malformed JSON、raw failed patch、redaction marker content |
| `progress` | `currentExecutionProgress`、`processEvidenceTimeline`、`currentProgressDigest`、latest action / validation / artifact evidence refs | `latestClosureEvidence`、`supersededFailureEvidence`、recent repair attempts | 无 sourceRef 的自然语言进度、不可追踪 evidence、完整旧 provider request / raw stream |
| `coverage` | 当前 Goal / DoD coverage 基准；repair 时必须含 unresolved coverage 或说明无 coverage gap | `missingDoDItems`、`missingGoalCoverage`、`missingDeliverableCoverage`、`coverageTargetSet`、`evidenceCandidates` | 由 core 根据业务关键词推断的缺口 |
| `snapshots` | 当修复需要目标资源时必须含 target ref、exists、contentHash、content completeness flags | `targetSnapshots`、`failureTargetRepairContext`、`patchRepairContext`、`workspaceFileManifest`、`resourceShapeAlignmentContext` | 伪完整快照、无 hash 的完整 file_write 基线、被误标安全的 redacted/truncated 内容 |
| `capabilities` | 当前 stage 可用 capability kind 与能力边界 | skill/tool/mcp/soul/prompt refs、allowlist、input contract、unsupported operations | 领域硬编码 fallback、未声明能力语义 |
| `memory` | accepted input / necessary output / decision memory 的 refs 和摘要 | profile memory、tool capability memory、recent lessons | 被拒 repair plan 的自述、provider retry 流水、低价值日志 |
| `budget` | context budget、repair budget、provider retry budget、stage budget | timeout budget、request audit refs | 代码分支写死的固定预算 |

所有 section 的摘要字段必须带 source refs 或 omitted reason。若某个 section 对当前 stage 不适用，应显式标记 `notApplicable` 或等价省略原因，而不是留空后让 LLM 猜测。

### Context Gate

Runtime 在 provider request 前必须执行 context gate，并得到唯一处置：

| Gate | 触发条件 | 处置 |
| --- | --- | --- |
| `pass` | P0-P4 必要字段完整，输出合同可执行 | 允许提交 provider |
| `read_required` | state-changing repair 需要当前目标内容，但快照缺失、不完整、被截断或被脱敏 | 只允许派发 read / resource fetch；读完后进入下一轮 LLM，不允许同图固定 patch |
| `degrade_to_assessment` | 缺口是 assessment / evidence 结构缺口，已有候选证据但无需状态变更 | 进入 assessment-only correction 或 concrete validation evidence |
| `blocker_required` | 必要信息不在 workspace / refs / capability 中，且 runtime 无法自行读取 | 进入 blocked_on_decision，明确 missing field 与无法自动获取原因 |
| `fail_fast` | P0/P1 缺失、Goal/DoD 被压缩、stage contract 缺失、context over budget 且无法保留必要字段 | 不提交 provider，写 runtime_failed / context_integrity failure |
| `replan_required` | 原计划 / repair path 已被证据证明不可继续，且需要 successor lineage | 创建 replan successor，携带 predecessor failure context |

`contextBudget.integrityWarnings` 不能只是观测日志。若 warning 涉及 P0-P4 必要字段，例如 `protected_repair_action_target_snapshot_missing_after_context_budget`、Goal / DoD 压缩、requiredOutput 缺失、current failure 缺失、coverageTargetSet 缺失，context gate 必须选择 `read_required`、`fail_fast`、`blocker_required` 或 `replan_required` 之一。

`runtime_repair_current_failure_missing` 只能在顶层输入和 `repairActionPackage.currentFailure`、`failedStepOutput`、`lastFailureDiagnostics`、replan/ticket context 中都无法找到当前失败时触发。若当前失败已存在于 package、process evidence 或 resolved context gate ticket 中，runtime 必须先 hydrate canonical repair context，而不是创建新的人工 decision ticket。

State-changing repair 的失败诊断合同必须接受等价的 canonical failure 来源：`lastFailureDiagnostics`、`failedStepOutput`、顶层 `currentFailure`、`repairActionPackage.currentFailure`、`repairActionPackage.lastFailureDiagnostics`、`repairActionPackage.lastObservedFailure` 或 `currentProgressDigest.lastObservedFailure`。这些来源只要包含 failure class / signature / reason / error 中任一有效信号，就不得被误判为缺失 `lastFailureDiagnostics`；否则会把可行动验证失败错误转成 runtime 合同失败，并触发无意义的大上下文 repair 循环。

若 `coverageRepairRequired` 已由 missingDoDItems / missingGoalCoverage / missingDeliverableCoverage 触发，但 `coverageTargetSet` 因预算整形或输入整理暂时缺失，runtime 应先自动从现有缺口事实补齐中立 `coverageTargetSet` 再重试；只有当前输入里确实没有任何可辨认的 coverage 缺口事实时，才允许 `coverage_repair_required_without_coverage_target_set` 进入 fail-fast 或 blocker 流程。

Repair / replan continuation 的过程证据缺失也属于 P0-P4 必要字段缺失。若 provider input 显示当前是 `runtimeRepairFollowUp`、`stateChangingOutputRequired`、`runtimeStage=repair` 或带有 `replanContinuation`，但 `processEvidenceTimeline` / `currentProgressDigest` 不存在，context gate 必须输出 `runtime_repair_process_evidence_missing` 并 fail-fast；该失败不能交给普通 state-changing repair 处理。

`protected_repair_action_target_snapshot_missing_after_context_budget` 只能在原始 provider input 已包含可安全写入的 `repairActionPackage.targetSnapshots`，但预算处理后丢失时触发。`repairActionPackage.requiredOutput`、`coverageTargetSet`、`currentExecutionProgress` 等受保护合同字段不能被误判为 target snapshot 要求；否则会把“无目标快照需求”的普通 repair 合同错误转成 `read_required`。

`llm_context_integrity_failed` 是 provider 前的 runtime context gate failure，不能交给 LLM 生成 state-changing repair 来修复。若 gate 为 `read_required`，runtime 只能先补充 read/resource fetch 或阻塞；若 gate 为 `fail_fast`，应终止本轮 provider 提交并给出结构化原因。该类失败不得继承 `stateChangingOutputRequired=true` 后继续要求 `patch/apply`，避免形成 context gate -> state-changing repair -> context gate 的循环。

Context gate 的恢复必须由 runtime 结构化分流，而不是由普通 repair prompt 间接推断：

- `pass`：提交 provider。
- `read_required`：若 runtime 能从结构化 target refs 解析出可读资源，先派发 read/resource fetch；若无法解析或能力不可用，创建 `runtime_context_required` 决策阻塞。
- `fail_fast`：不提交 provider，不进入 state-changing repair；创建 runtime context blocker 或直接终止当前 successor，错误中必须包含 `contextGateAction`、`contextGateReason` 和受影响 protected field。
- `blocker_required`：创建 `blocked_on_decision`，上下文必须说明 missing field、source refs、为什么 runtime 无法自动获取。
- `replan_required`：创建 successor lineage，携带 predecessor failure context 和仍有效的 Goal/DoD 合同。

`runtime_context_required` 决策被 resolved 后，恢复动作只表示 runtime 获得继续构造上下文的授权或补充信息，禁止直接重放已失败的 `llm_context_integrity_failed` step。Orchestrator 必须创建新的 context retry step，并从 ticket context、failed step input、replan continuation、repairActionPackage、current snapshots 和 process evidence 中重新 hydrate `currentFailure`、`currentExecutionProgress`、requiredOutput 与 target evidence。若这些 P0/P1 字段仍无法恢复，runtime 必须进入明确的 replan / terminal / blocker 处置，不得生成同一原因的重复 context ticket。

跨模块不变量：

- LLM budget 模块只负责计算 `contextGateAction/contextGateReason`，不得假设 orchestrator 会用 patch 修复上下文。
- Orchestrator 看到 `llm_context_integrity_failed` 后必须按 context gate 语义处理，禁止进入 provider retry、普通 runtime repair 或 state-changing repair。
- `stateChangingOutputRequired` 只约束业务目标产物或验证收口，不得覆盖 provider 前上下文准入失败。
- `repairActionPackage` 的字段必须按字段级 presence bit 保护：合同字段、进展字段、coverage 字段和 target snapshot 字段互不隐式代表。

### Stage-Specific Required Context

| Stage | 必须完整提供 | 缺失时处置 |
| --- | --- | --- |
| `exploration` | Goal / DoD、workspace manifest / known resources、可用能力摘要、探索预算 | 缺 Goal / DoD fail-fast；缺 manifest 可先 list/read |
| `planning` | Goal / DoD、profile contract、known resources、已有 evidence / rejected continuation、stage transition rules | 缺 Goal / DoD fail-fast；缺 known resources 先读取；计划不能假设空 workspace |
| `execution` | 当前 unit、state-change policy、allowed capability、必要 target refs / snapshots、contract | 缺 target snapshot 时 read_required；缺 capability contract fail-fast |
| `verification` | 已变更 artifact refs、验证目标、evidence requirements、latest action evidence | 缺 evidence refs 时 concrete validation evidence；不得直接 accepted |
| `acceptance` | effective Goal / DoD、coverage matrix、evidence catalog、artifact refs、result policy | 缺 coverage 进入 assessment correction；缺 result policy fail-fast |
| `repair` | current failure、original failure surface、repairActionPackage、target snapshots / manifest、recent attempts、forbidden actions | 缺目标快照 read_required；缺原始语义 fail-fast；重复失败按 budget 收敛 |
| `replan` | predecessor run/result、failure context、failed paths、可保留产物线索、仍有效 contract | 缺 predecessor failure context fail-fast；不得只携带 ticket id |

### Implementation Mapping

后续代码收口必须把本文约束映射到明确实现点。至少包含：

| Contract Area | 主要实现点 | 验收 |
| --- | --- | --- |
| Context bundle assembly | `execution_context_package.go`、`runtime_step_input.go`、activation / resume context 构造 | LLM input 可投影出完整 bundle section |
| Cross-stage convergence contract | `runtime_convergence.go`、`runtime_repair.go`、`runtime_graph.go`、decision resume / replan / validation recovery | 每条恢复路径都先得到唯一 `need_*` 状态，再应用统一 input contract |
| Decision ticket gate | governance ticket writer、LLM decision output handler、runtime repair dispatcher | 已有 concrete target evidence 且可机械推进时，不创建新 ticket，转为 repair contract violation |
| Patch / edit failure repair-of-repair | patch adapter、runtime repair input builder、target snapshot provider | hunk mismatch / oldText conflict 后不丢 `repairActionPackage`，基于当前 snapshot 继续 repair 或 bounded validation |
| Context gate | `runtime_llm_context.go`、LLM provider request assembly、context budget normalization | P0-P4 缺失时不提交不可靠 provider request |
| Process evidence timeline | step / event / tool / validation evidence projection、request audit | LLM input 可按顺序看到已有输出、最近动作、失败原因、当前缺口和下一步要求 |
| Repair action package | `runtime_repair.go`、failure classification、repair input compaction | repair-of-repair 不丢原始 failure surface、coverage、target snapshots |
| Graph output contract | `runtime_graph.go`、LLM output validator、patch adapter | 禁止 read-only 严格收敛、同图 read 后固定 patch、placeholder patch |
| Evidence / acceptance | completion gate、result recorder、runtime evidence catalog | accepted result 必须有可追踪 evidence / coverage |
| Provider retry | provider runtime、failure recovery decision、decision blocker | provider retry 只处理 transient provider failure，不覆盖 stage contract |
| Memory recall/write | `runtime_memory.go`、memory selection、run summary memory | 只写入 accepted input / necessary output / rejected fact，不写 rejected plan 自述 |
| Profile boundary | template/profile loading、Soul routing、capability registry | core 只消费 profile 声明，不把 profile 语义硬编码进 runtime |
| Storage / refs | file backend、raw payload refs、event payload summaries | 大内容 ref-only、路径可读、source refs 可追溯 |

任何实现点如果无法提供所需字段，必须在 step output / contextBudget 中记录缺失原因，并让 context gate 选择明确处置。

### 收口顺序

后续代码落地按以下顺序推进，避免继续单点修复：

1. 提取 `ConvergenceContract` 数据结构和状态枚举，只表达中立 `need_*` 状态、target evidence、required output、governance reason 和 source refs。
2. 接入所有恢复入口：graph follow-up、decision resolved、patch/edit failed、schema invalid、validation failed、replan successor、coverage gap。
3. 用 `ApplyConvergenceContract` 统一写入 LLM step input，替换分散的 `repairObjective`、`stateChangingOutputRequired`、`repairActionPackage` 局部拼接。
4. 用 `ShouldCreateDecisionTicket` 包住所有 ticket 创建路径，先判断可机械推进的 decision 是否应转换为 repair contract violation。
5. 刷新 context gate，使 `currentRuntimeStage`、`currentProgressDigest`、`repairActionPackage`、`structuredFailureSignals`、`processEvidenceTimeline` 自洽成为发送 LLM 的必要条件。
6. 补齐回归测试矩阵，再用实际 workspace 运行数据核对是否不再出现合同丢失、verification/repair 冲突、无效 ticket 和 stale patch 循环。

### Core / Profile Boundary

Core 可以做：

- 校验 stage、contract、capability kind、target ref、evidence ref、coverage item、budget 和 lineage。
- 从失败文本中机械抽取 path-like、line-like、identifier-like、field-like token。
- 基于 workspace manifest 做唯一后缀解析、target snapshot 选择、sourceRef 归一化。
- 根据 profile 声明的 evidenceRequirements 做机械证据类型校验。
- 拒绝 placeholder、redaction marker、raw rejected content、同图 read 后固定 patch、无关 state-changing artifact。

Core 不可以做：

- 根据语言、框架、测试命令、文件名、供应商、协议或业务词推断应该如何修复。
- 根据 Goal / DoD 自然语言中的领域关键词生成具体实现方案或证据要求。
- 把某个 workspaceTemplate 的默认流程写成全局 runtime 规则。
- 把 profile-specific matrix 行、测试命令、入口约定、代码结构约定提升为 core 字段。

WorkspaceTemplate / profile / soul / skill 可以做：

- 声明某类任务的 stage prompt、capability allowlist、evidenceRequirements、deliverableExpectations。
- 提供领域术语解释、建议验证方式、典型产物结构、profile-specific coverage matrix。
- 将中立 state-changing artifact 映射到具体能力组合。

WorkspaceTemplate / profile / soul / skill 不可以做：

- 写死某个具体 Goal / DoD 的答案、文件内容或验收结论。
- 降低本文定义的 core evidence、result、repair、context gate 要求。
- 要求 core runtime 理解 profile 内部业务语义。

## 验证脚本要求

后续部署核对脚本需要按 stage contract 验证，而不是只按固定软件开发路径验证。

必须覆盖：

- 每个 step 都有 runtime stage 或可从结构化 input 推导 stage。
- LLM request 可投影出 Context Bundle Schema 中适用于当前 stage 的必填 section。
- P0 Contract / P1 Current Failure / P2 Coverage Scope / P3 Current Evidence / P4 Current Snapshots 的缺失会触发 context gate，而不是静默提交 provider。
- `exploration` / `planning` 允许 read-only output，但受预算限制。
- `execution` 的 state-changing requirement 不允许 read-only graph。
- `verification` accepted 必须有 evidence refs。
- `acceptance` terminal 必须有 result。
- `repair` 必须继承来源 stage contract。
- state-changing repair 必须携带 `repairActionPackage`，并保留 current failure、target snapshots / manifest、coverage target 和 recent attempts。
- target snapshot 不完整、redacted 或 truncated 时，不允许生成完整 file_write 或同图固定 patch。
- 同一 execution graph 中 `patch/apply` 不得声称依赖同图前置 `file/read` 的运行时输出。
- `replan` 必须创建 successor run 并保持 lineage。
- replan successor 的初始输入必须能看到触发 replan 的 failure context，而不是只看到 ticket / resolution id。
- repair-of-repair 必须保留原始 failure target context、workspace manifest 和 state-changing contract。
- 当前失败 step 的 output failure signature 必须优先于继承的历史 repair signature，避免新失败被计入旧预算。
- capability contract violation 必须被拒绝并形成 repair evidence；LLM prompt 必须包含该 capability 的中立限制。
- `contextBudget` 必须包含 protectedFields、integrityWarnings、included / omitted / redacted sections 和 source refs。
- `processEvidenceTimeline` 必须按 step / event 顺序表达最近有效动作、验证结果、失败输出和当前缺口；缺 sourceRef 的进度文本不得作为 completion / repair / replan 的高置信依据。
- profile-specific 检查只能放在 profile verifier 中，不能写入 core verifier。

## 实施收口计划

| ID | 优先级 | 内容 | 验收 |
| --- | --- | --- | --- |
| STG-01 | P0 | 引入显式 `runtimeStage` / stage contract helper | step input 可稳定表达 stage，不再依赖业务文案或错误字符串 |
| STG-02 | P0 | 将现有 planning / execution / assessment-only contract 归一到 stage contract | core contract validator 只按 stage + artifact type 判断 |
| STG-03 | P0 | 区分开放判断与严格收敛模式 | exploration/planning 不被误判为实现违规；execution/verification/acceptance 不再无限只读循环 |
| STG-04 | P0 | Profile 层定义软件开发映射 | 软件开发能力不污染 core runtime |
| STG-05 | P1 | Memory recall 按 stage 组织 | recall 主上下文只保留高价值决策信息 |
| STG-06 | P1 | Repair 继承来源 stage contract | provider retry / repair / repair-of-repair 不丢失约束 |
| STG-07 | P1 | 更新验证脚本为 core + profile 两层 | core verifier 保持中立，profile verifier 检查软件开发映射 |
| STG-08 | P2 | 清理旧的字符串匹配与领域化提示 | contract violation 使用结构化字段表达 |
| STG-09 | P1 | 引入 Codex 启发的中立过程证据 timeline | LLM request 能看到按序排列的已有输出、最近动作、失败证据、当前缺口和下一步要求，且全部带 source refs |
| CVC-01 | P0 | 引入统一 `ConvergenceContract` 与 decision ticket 门禁 | repair / validation / assessment / evidence acquisition / decision 写入路径先得到唯一 `need_*` 合同状态，再应用到 LLM input 或 ticket gate |

## 附录：当前落地记录

- 2026-05-13：
  - 已完成 STG-01 第一批收口。
  - orchestrator 新增显式 `runtimeStage` 输入字段与统一 helper，作为 core runtime 的中立阶段标识。
  - 初始 LLM step、graph node、graph follow-up、completion follow-up、assessment-only follow-up、runtime repair、provider retry、decision resolution、approval resolution 已开始显式写入 `runtimeStage`。
  - `validateLLMExecutionOutputContract` 和 follow-up graph state-changing contract 已开始优先基于 `runtimeStage` 判断，不再依赖 Soul 绑定阶段。
  - 2026-05-17 已完成破坏性收口：runtime contract 不再从 `soulStage` 反推 `runtimeStage`；Soul 绑定阶段只用于选择 Soul / capability allowlist，不再作为 runtime stage fallback。
- 2026-05-13：
  - 已完成 STG-03 第一批收口。
  - core runtime 新增 `runtimeStageMode`，当前支持 `open_judgment`、`strict_convergence`、`structured_governance`。
  - `runtimeStageMode` 已随 `runtimeStage` 一起进入 step input、execution context package、memory trace 和 LLM provider input。
  - blocks/llm prompt 已显式接入 `runtimeStage` / `runtimeStageMode`，让 LLM 在 prompt 层知道当前是开放判断还是严格收敛。
  - 当时仍未完成 profile 层的独立阶段映射与专用 verifier；后续 STG-04 / STG-07 落地记录见 2026-05-17 条目。
- 2026-05-13：
  - 已完成 STG-02 / STG-06 / STG-08 追加收口。
  - core runtime 已移除通过 `graphNodeId` 文本关键词推断 state-changing graph node 的逻辑；graph node 是否必须产生 state-changing output 只读取显式 `stateChangingOutputRequired` contract。
  - execution graph node 的阶段判断已从 `soulStage=execution` 切换到中立 `runtimeStage=execution`。
  - planning / exploration 保持 `open_judgment` 语义，即使 run 级 `executionContract.stateChangePolicy=must_produce_state_change`，也不在 planning 阶段强制 patch；state-changing 要求在 execution / repair 等严格收敛阶段执行。
  - repair 新写入的 `repairObjective` 已改为 `produce_state_changing_output`；`produce_state_changing_implementation` 已从 runtime state-change 判定中移除。
  - state-changing repair guidance 已改为中立的 state-changing output 要求，profile 层仍可把该 artifact 映射为 patch/file write 等具体能力。
- 2026-05-13：
  - 已完成 execution 失败 repair 追加收口。
  - `must_produce_state_change` run 中，`execution/strict_convergence` 阶段的非只读步骤失败会触发 state-changing repair contract，避免测试/构建/执行失败后的 repair 退回只读分析。
  - runtime repair ID 对普通 graph-scoped 失败使用失败 graph node 标签，例如 `repair-001-of-graph-003-003-run-go-test`；provider retry / runtime repair 等 recovery 外壳仍按归一 root 生成，避免再次出现 provider retry 壳嵌套。
- 2026-05-14：
  - 已完成 repair failure scope 与上下文继承追加收口。
  - 非 repair-follow-up 当前失败优先使用自身 output failure signature；历史 `repairOfFailureSignature` 不再覆盖新的无 target scope 失败，避免新执行超时或验证失败串入旧 repair budget。
  - failure target repair 已补充当前目标快照、唯一后缀路径解析、workspace manifest 与 repair-of-repair 上下文继承；重复 contract violation 不再丢失原始 failure target。
  - state-changing repair guidance 与 LLM provider prompt 已补充 runtime capability 边界：能力不支持的操作不得用伪语义表达，例如不得用空 `file_write` 表示删除。
  - replan successor 的 `replanContinuation` 已要求携带 ticket failure context，使 successor 能基于 predecessor failure evidence 继续收敛，而不是只收到 replan 元信息。
  - planning / exploration / replan continuation 的 execution context package 已要求携带当前 workspace 的有界 manifest；state-changing repair follow-up 也必须在未显式继承 manifest 时从当前 workspace 补齐，避免已有资源不可见导致重复初始化或错误覆盖。
  - patch repair context 已补充 `contentComplete` / `safeForFileWrite` 标记与 redaction/truncation 禁用规则；LLM provider prompt 不再无条件鼓励基于当前快照输出完整 `file_write`。
  - patch repair 已增加中立 `patchRepairSuggestions`：从失败 diff 与当前目标快照机械抽取唯一 oldText/newText 替换，帮助生成最小 diff，避免反复输出 stale diff 或复制脱敏内容。
  - `unified_diff` 应用能力已支持在行号漂移但 old/context lines 于当前目标中唯一匹配时按唯一匹配位置应用，降低 LLM 生成 diff 行号不准导致的无效 repair 循环。
- 2026-05-15：
  - 已完成 execution contract 下发与 template contract 刷新收口。
  - patch 输入校验已覆盖 `baseHash` 未解析占位符与 `@@ ... @@` 伪 hunk，LLM provider prompt / repair guidance 已要求使用字面 `contentHash` 或省略 `baseHash`，并在修复前基于当前 target snapshot 避免重复添加等价内容。
  - activation 会基于仍匹配 workspace soul routing 的当前 template `ContextHints` 刷新 run 级 `executionContract`，并保留 workspace 扩展字段；该逻辑只更新中立 contract，不重新绑定 workspace template、soul、skill、mcp。
  - `executionContextPackage` 不再压缩 `executionContract`，确保 LLM 能看到完整 `profileContracts`、required deliverables 与 evidence requirements。
  - verification follow-up 返回只读 execution graph 且缺少有效收敛输出时，后续 repair 会继承 contract violation evidence；需要状态变更时会显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。
  - assessment-only coverage correction 若仍缺 DoD / Goal coverage，不再首轮 repair 后直接终止；runtime 会在 repair budget 内继续派发 assessment-only correction，并携带最新缺失 coverage 项。
  - 外部 capability 失败的 target scope 已支持 `serverId+tool` 组合并使用 run 级目标预算；同一外部能力反复确定性失败时会进入 duplicate / repair exhausted 治理，rejected result 会携带 terminal failed step 与 repair step 证据，run failure reason 优先使用 unsatisfied DoD assessment summary。
- 2026-05-17：
  - 已完成运行态上下文放大收口。
  - verification follow-up 已允许受限 concrete validation evidence graph；只有纯 read-only inspection / 自检 / context rehydration 且无 evidence、无 rejected assessment 时才视为 contract violation，避免把验证阶段测试/检查图误升级为必须 patch 的 repair。
  - state-changing repair target relevance 已补充中立配套元数据目标规则：当失败证据明确指向 dependency / manifest / metadata / checksum / lock 前置资源缺失时，可修复同目录配套目标，但不得引入语言、框架或业务文件硬编码。
  - repair scope 已要求当前 repair-follow-up 自身产生的 runtime contract violation 优先使用当前 `failureClass/failureSignature`，provider retry 等无新签名的运行外壳才继续继承原始 scope。
  - patch repair 中已有目标 `file_write` 已改为安全条件放行：必须命中当前 repair target snapshot，且 snapshot 完整、未脱敏、可 file write，并且请求 `baseHash` 与 `contentHash` 完全一致；否则继续拒绝并要求使用最小 diff/edit 或重新读取目标。
  - rejected / partial LLM 输出中的 redacted marker 不再通过 `partialOutputPreview` 回流到 repair input，只保留 omitted 标记和不可复用原因。
  - `governanceHistory` 与 `replanContinuation` 的失败 lineage 改为 summary/ref-only，禁止把旧的 raw failure input/output、target content、LLM raw preview、stream events 或 provider/model 动态绑定复制进 successor run、step input 或 execution context package。
  - assessment-only contract correction 输入已进一步隔离旧治理/重规划/决策/执行修复上下文，只保留评估补正必要字段和已清洗的 invalid assessment 摘要。
  - workspace 终态 read model 已要求 `taskProgress` 与 `outcome` 使用同一最新终态 result / run 选择规则，避免 outcome 指向当前 rejected run 但 taskProgress 回退到历史 run。
  - completion gate evidence ref 归一已补充 graph follow-up / assessment follow-up 派生别名映射：`run/<follow-up-step>-<node-id>`、`verify_full_test` / `verify-full-test`、`003-verify-full-test` 等中立结构别名可映射到同 run 内真实 graph step；不可追踪或失败证据继续剔除。
  - accepted result / result-summary / deliverable-manifest 已增加 runtime 事实校正：同一 run 存在已应用 workspace 变更时，不再保留“仅 assessment-only / 无新增可执行操作”等与事实矛盾的 LLM 自述。
  - Step state event payload 改为 raw ref + compact summary，不再复制完整 step input/output；ToolCall runtime evidence 改为记录 step input/output refs 与摘要，不再把完整 step input 重复写入 `tool-calls/*/arguments.json`。
  - 该变更用于避免长流程 workspace 在 repair/replan 中反复 JSON clone/marshal/read/write，导致 CPU、内存和 payload 文件成倍膨胀。
- 2026-05-17：
  - 已完成 DoD / Goal coverage 缺口提示中立化收口。
  - core runtime 不再根据 DoD 文本中的测试、配置、并发、语言栈或其他业务关键词生成领域化 evidence hint。
  - `missingDoDItems` / Goal coverage diagnostics 只提示按当前 `runtimeStage` 与 profile contract 提供具体 evidence refs；软件开发所需的 build/test/diff/config/concurrency 等证据要求继续由 `software-dev` workspaceTemplate / Soul / profile contract 承载。
  - Result manifest 与 patch evidence artifact 命名同步从 `Code Change` / `code-change-diff` 收敛为中立 `Workspace Change` / `workspace-change-diff`，避免 core 把通用 workspace 状态变更解释为代码变更。
  - workspace 交付文件在 core manifest 中统一标记为 `deliverable`，不再按文件名或扩展名推断 source / configuration / test_report；若某个 profile 需要更细的产物分类，必须由 profile verifier 或模板输出结构化声明。
  - blocks/llm 全局 provider prompt 已移除软件开发默认路径、测试命令示例、项目初始化和领域化 DoD repair 话术；全局 prompt 只表达 stage / capability / workspace target / evidence 的中立约束，软件开发流程继续由 `software-dev` Soul 和 profile contract 下发。
  - 已新增 `scripts/verify-workorch-runtime-profile-software-dev.sh` 与 `make verify-runtime-profile-software-dev`，把 software-dev 的 `goalCoverage`、`acceptanceDimensions`、`deliverableCoverage` 等 profile-specific 验证从 core verifier 拆出；`verify-workorch-runtime-core-flow.sh` 继续只检查中立 runtime flow。
  - `scripts/verify-boundary-contract.sh` 已补充软件开发偏置守卫，禁止 reusable runtime / 全局 LLM prompt 重新出现 `Code Change`、固定测试命令、`implementation path`、`affected files`、profile deliverable id 等专属表达。
  - 已完成不兼容收口：`runtimeStageFromStepInput` 不再从 `soulStage` 反推 runtime stage；blocks/llm provider Soul 推断也不再把旧 `soulStage` 当作 fallback。后续所有 runtime 合同判断必须依赖显式 `runtimeStage` 或结构化 follow-up flags。
  - run-summary memory 上下文压缩已移除旧 `run_summary` 兼容 trust marker；core 只保留中立 provenance 与当前版本标签，不再为历史数据生成特殊降权字段。
  - run-summary memory 的 core 文本已把 `Verification commands` / `Patch targets` / `code_agent` 收敛为中立 `Evidence executions` / `State-change targets` / `runtime_result`，软件开发具体命令和 patch 语义只作为 profile evidence 内容出现，不作为 core 标签或固定标题。
  - continuable decision -> runtime repair 的判断已移除 `new project`、`project path`、`工作目录` 等文本关键词启发式；core 只接受中立结构化标记 `runtimeCanContinue` / `continuableByRuntime` / `workspaceActionable` 或已知可读 workspace resource 证据。
- 2026-05-19：
  - graph expansion limit assessment 输入已补齐 `latestClosureEvidence` / `supersededFailureEvidence` 与 assessment evidence guidance，要求 satisfied coverage 只能引用具体可追踪证据。
  - `runEvidenceSummary` 在超出上下文预算时优先保留 action / patch / validation / closure evidence，避免关键证据被普通 LLM 过程步骤挤出。
  - graph expansion limit assessment 触发 coverage correction 时会为 `missingDoDItems` / `missingGoalCoverage` 补充中立 `evidenceCandidates`；候选证据存在时继续 assessment-only correction，候选不足时再进入 concrete validation evidence repair。
  - runtime repair 已补充 `resourceShapeAlignmentContext`：当结构化失败信号表明当前目标与现有资源形状不一致时，repair 输入会携带同目录当前资源快照，要求 LLM 先对齐现有资源形状再生成 state-changing artifact。
  - patch/apply 准入已扩展 non-substantive placeholder 识别，拒绝“严格收敛”“验证锚点”“state-change anchor”“verification anchor”等只为满足流程存在性的 marker/comment/no-op 变更。
- 2026-05-20：
  - provider request 的 `contextBudget` 已补充 `contextGateAction` / `contextGateReason`，并在提交 LLM provider 前执行硬准入；P0-P4 必要上下文缺失、coverage repair 缺少 target set、repair 缺少当前失败、受保护 target snapshot 在预算处理后丢失等情况不再作为普通观测 warning 静默提交 provider。
  - `llm_context_integrity_failed` 作为 runtime contract failure 进入失败分类，避免被误归为 provider transient / config 类问题。
  - execution graph 校验已拒绝 `patch/apply` 依赖同图 `file/read` 的固定 state-changing artifact；需要先读取当前资源时必须拆成前置读取轮次，再由后续 LLM 基于已进入上下文的快照生成状态变更。
  - provider budget 的 repair target 保护已区分 `repairActionPackage` 合同字段与真实 `targetSnapshots`：只有原始输入存在安全 target snapshot 时才触发 protected snapshot missing；provider 前 context gate failure 不再被升级为要求 `patch/apply` 的 state-changing repair，并会进入显式 `runtime_context_required` 阻塞处置。

- 2026-05-20：
  - 已将 Codex 长流程上下文管理中可中立借鉴的内容并入本文：核心只吸收分层上下文、过程证据 timeline、失败输出保留、raw payload ref、request audit 与 profile 边界，不吸收 Codex 私有 schema、隐藏 reasoning、工具名或软件开发专用流程。
  - 新增并完成 STG-09 首批代码收口：LLM step input / `executionContextPackage` 会在严格收敛、repair、verification、acceptance、replan 等需要过程证据的阶段注入中立 `processEvidenceTimeline` 与 `currentProgressDigest`；timeline 按 runtime step 顺序表达已有输出、最近动作、验证失败、当前缺口和下一步要求，并且每条记录具备 `sourceRef` / `payloadRef`。
  - Provider context budget 已将 `processEvidenceTimeline` 与 `currentProgressDigest` 纳入 P2 保留与 source refs 审计。初始 planning 与普通 decision / approval follow-up 不额外读取 step history，避免为上下文增强引入无意义启动开销。
  - Replan successor 已开始继承 predecessor 的 `processEvidenceTimeline` / `currentProgressDigest`，并在 successor 当前 run 尚无历史步骤时从 `replanContinuation` 回填到 `executionContextPackage` 和顶层 LLM input。
  - Provider context gate 已将 repair / state-changing continuation / replan continuation 的过程证据缺失升级为 `runtime_repair_process_evidence_missing` fail-fast，避免继续向 LLM 提交缺少最近过程证据的 state-changing repair 请求。
  - Scoped repair verification 已支持中立 bounded 关系：宽泛验证命令若依赖直接命中当前 failure target 的 patch，或显式声明 target/scope/reason，即可作为当前 repair 的有界验证；无 scope 关系的宽泛验证仍会被拒绝。
  - LLM provider prompt 与 state-changing repair guidance 已明确 `unified_diff` 必须使用显式 hunk line range，禁止裸 `@@` hunk；行号不确定时应使用安全 `file_write + baseHash`、精确 edit、先读取目标或 failed/blocker。
  - Runtime repair 已从失败输出的 stdout/stderr/error/reason/diagnostics 中抽取中立 target refs，并通过当前 workspace manifest 注入 `failureTargetRepairContext` 与 `repairActionPackage.targetSnapshots`，避免 LLM 在缺少当前目标快照时反复输出无效 state-changing artifact。
  - 带当前 target snapshot 的 state-changing repair 已拒绝无实质 `unified_diff`，包括完全相同替换、只有空白行变化和无 changed lines，确保 repair 必须产生真实状态变化、failed assessment 或 blocker。
  - 重复 failed surface 的 root-cause target shift 已支持中立 `rootCauseCandidateContext`：只有来自 manifest / same-scope resource / structured failure / current snapshot 等机械证据且存在当前快照的候选可作为不同 root cause target；同时 failure target 抽取已过滤裸运行产物标签和 duration / numeric measurement，避免非资源 token 污染 repair scope。
  - 软件开发行为约束继续由 software-dev workspaceTemplate / Soul / skill 承载；core 仅处理 step、stage、state、capability kind、evidence ref、failure class / signature 等中立运行事实。
  - software-dev 默认 Soul 已同步吸纳 Codex 启发的工程行为约束：规划阶段要求参考 `processEvidenceTimeline` / `currentProgressDigest` 做已有输出、最近失败、当前缺口和下一步要求分析；执行阶段要求先基于当前代码/当前快照/当前失败证据再改、沿用本地项目模式、保护 workspace 已有有效内容、避免无关重构和长期只调整测试；验证阶段要求先对齐最新过程证据和 closure evidence，避免旧失败覆盖后续成功证据。
- 2026-05-22：
  - 已启动 CVC-01 第一批代码收口。编号避开历史 `STG-10 Prompt / Memory 收口跟踪`，避免进度记录冲突。
  - 新增 `ConvergenceContract` 中立合同结构与 `BuildConvergenceContract` / `ApplyConvergenceContract` / `ShouldCreateDecisionTicket` 统一入口；合同状态限定为 `need_evidence`、`need_state_change`、`need_validation`、`need_assessment`、`need_governance`、`blocked`。
  - Runtime repair follow-up 与 concrete evidence acquisition follow-up 已接入 `ApplyConvergenceContract`，避免 `stateChangingOutputRequired`、`concreteFailureEvidenceRequired`、`repairObjective`、`repairActionPackage` 分散拼接后互相污染。
  - LLM 返回 decision block 时，先通过 `ShouldCreateDecisionTicket` 判断：若当前合同为 `need_state_change` 且已有 concrete target evidence、未声明外部依赖，则转换为 continuable repair contract violation，不创建人工 ticket。
  - `createDecisionTicket` 写入前也执行同一门禁；provider/config、context gate、repair exhausted 等真实治理阻塞继续通过结构化 context 声明 `scope` / `dependency` / `recoveryPolicy`。
  - 已修正 assessment-only coverage correction 与 concrete validation evidence repair 的合同优先级：当 assessment-only 失败已转为 evidence repair 时，合同状态为 `need_validation`，并清理 assessment-only 标记，避免评估补正与验证取证互相污染。
  - 已补回归测试覆盖 `need_state_change`、`need_evidence`、mechanically repairable decision 转 repair、外部治理 decision 保留 ticket；`go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过。
  - 边界核对：本批新增/修改的 convergence 代码未引入软件开发、语言、测试框架或具体业务特判；全仓边界搜索仍存在历史测试、配置与 MCP/server 中立字段命中，未作为本批新增规则依据。
- 2026-05-22：
  - 已完成 CVC-01 第二批恢复入口收口。
- 2026-05-24：
  - 已收口 verification failed assessment 的 continuable decision 偏差：当 failed `dodAssessment` 已给出 coverage gap / nextActions / evidence candidates / coverage target set 时，runtime 不再升级为 `blocked_on_decision` ticket，而是直接转为 runtime repair follow-up。
  - 已收口 `currentProgressDigest` / `currentFailure` 污染：`latest_successful_action_evidence` 不再被提升为当前失败；只有真实 failed step 或带明确失败标记的 completed evidence 才能覆盖旧失败。
  - 已补回归测试覆盖“successful action evidence 不污染 currentFailure”和“failed assessment decision 自动转 repair”；`go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过。
  - `decisionResolutionInput` 已接入 `ConvergenceContract`，resolved decision 只作为治理恢复信号；若 ticket context 仍携带 state-changing repairActionPackage / target evidence，恢复输入会回到 `need_state_change` 而不是停留在普通 `decision`。
  - Replan successor 的初始 step input 已通过统一合同应用；当 `replanContinuation` 明确携带 predecessor repairActionPackage / target snapshots 且 run contract 要求 state change 时，首个 LLM step 进入 `runtimeStage=repair` 与 `need_state_change`。
  - `replanContinuation.repairActionPackage` 已纳入 successor 首步合同来源；successor execution context 也记录 compact `convergenceContract`，用于后续审计当前唯一 `need_*` 状态。
  - schema invalid / LLM decision invalid repair-of-repair 已继承上一轮 `repairActionPackage`、target snapshots、repairTargetSet、coverageTargetSet、structuredFailureSignals 等中立 evidence，避免丢失 concrete target evidence 后误降级为 `need_evidence`。
  - 已补回归测试覆盖 decision resolved 后恢复 `need_state_change`、replan successor 首步继承 `need_state_change`、failed patch 已由当前 target 满足时进入 `need_validation`、schema invalid repair-of-repair 保持 state-changing contract。
- 2026-05-22：
  - 已完成 CVC-01 第三批收口。
  - validation shell failure 入口已通过统一合同显式区分 `need_evidence` 与 `need_state_change`：无 concrete target 的 shell/tool 失败保持 bounded evidence acquisition；带 `failureTargets` / line refs / target snapshots 的失败继续进入 state-changing repair。
  - provider-normalized invalid 输出已补显式合同回归：当上一轮 repair 已携带 target snapshots 与 state-changing contract 时，`llm_decision_invalid` / provider-normalized invalid 不再误降级为 provider retry 或普通 evidence acquisition，而是保持 `need_state_change`。
  - patch/apply hunk mismatch、patch preview stale diff、`file/edit oldText` conflict 与 failed patch already satisfied 的端到端 repair follow-up 用例已补 `convergenceContract` 状态断言，确认 repair-of-repair 路径分别稳定落在 `need_state_change` 或 `need_validation`。
  - 本批未新增需要人工决策的冲突；继续沿用 CVC-01 编号是为避免与既有 `STG-10` 记录冲突，该冲突已在第一批中记录并处理。
- 2026-05-22：
  - 已完成 CVC-01 第四批收口。
  - assessment-only contract correction 已统一接入 `ConvergenceContract`，避免评估补正只靠局部标记而不写入统一合同。
  - `rejectedResultContinuation` 已允许从 execution context 进入统一合同构造，并在缺少当前 concrete target evidence 时继续收口到 `need_evidence`，确保 predecessor rejected result 的结构化事实可被继承但不虚增状态变更。
  - 新增回归测试覆盖 assessment-only correction 进入 `need_assessment`，以及 rejected result continuation 参与 `need_evidence` 合同构造。
  - 本批未出现新的人工决策冲突，继续沿用 CVC-01 编号。
- 2026-05-22：
  - 已完成 CVC-01 第五批收口。
  - `ConvergenceContract` 状态集已补默认 `blocked` 与 `need_governance` 回归，确保统一合同枚举闭环，`need_governance` 只在 runtime 依赖缺失时打开 ticket 门禁。
  - 新增回归测试覆盖默认 blocked 合同、runtime dependency governance 合同，以及 decision ticket 门禁仍按外部依赖信号放行。
  - 本批未出现新的人工决策冲突，继续沿用 CVC-01 编号。

## 非兼容性说明

当前阶段以彻底收口 WorkOrch 能力为目标，不考虑旧行为兼容。允许做以下破坏性调整：

- 替换旧的 `soulStage` 推断方式。
- 要求所有新 step 显式携带 `runtimeStage`。
- 重构 output contract validator。
- 将软件开发相关 prompt / tool 选择迁移到 profile 层。
- 清理旧 memory recall 中的低价值 runtime 流水。
- 移除旧 memory trust marker、旧 run-summary 降权分支和历史数据特殊解释。
- 移除基于自然语言关键词的历史 decision 兼容推断；需要继续执行时必须通过结构化 runtime context 表达。
- 替换旧的 runtime file-backend 文件命名与 raw payload ref 形态。

但即使不考虑兼容，也不得破坏核心中立原则。

## Runtime Storage Naming Contract

运行态存储命名属于 core runtime 可观测性合同。WorkOrch 的长流程执行需要人、LLM、read-model 和恢复逻辑都能从目录结构中快速定位 workspace / run / stage / loop / step / artifact，因此 file backend 不得把逻辑 ID 的完整字符串默认编码成不可读文件名。

### 主命名规则

- runtime 逻辑 ID 必须优先保持结构化、可读、稳定，例如 `run-xxx/graph-002/007-run-go-test`。
- file backend 必须把逻辑 ID 按 `/` 展开为可读层级路径，而不是把整个 ID 做 base64、hash 或随机 token。
- 每个路径 segment 只允许保留文件系统安全字符，例如 `[A-Za-z0-9._-]`；其他字符应做中立清洗，例如替换为 `-`。
- 空 segment、`.`、`..`、路径穿越和绝对路径必须被拒绝或清洗为安全 segment。
- 结构化 lineage 仍然是因果关系和恢复预算的权威来源；文件路径只用于定位、审计和排查，不得反向推断业务语义。

推荐落盘形态：

```text
runs/<run-id>/
  steps/<run-id>/graph-002/007-run-go-test/metadata.json
  tool-calls/<run-id>/graph-002/007-run-go-test/runtime-execute/output.json
  llm-calls/<run-id>/001-step-1/request.json
  llm-calls/<run-id>/002-follow-up-graph-planning-001-list_workspace/request.json
  llm-calls/<run-id>/003-repair-001-of-graph-004-004-verify/request.json
  checkpoints/<run-id>/checkpoint/0007.json
  results/<run-id>/result/dod-assessment.json
```

All mutable runtime evidence collections that are stored as directories must use
a shared sequence-scoped path rule rather than ad hoc per-collection naming:
`<collection>/<run-id>/<sequence>-<readable-label>/`.

- `sequence` is a zero-padded monotonic number such as `001`, `002`, `003`.
- `readable-label` is a sanitized, business-neutral projection of the logical
  object ID after removing the run prefix.
- repeated writes for the same logical object ID must reuse the existing
  sequence directory instead of allocating a new one.
- the helper implementing this rule must be shared by `llm-calls`, `steps`,
  `tool-calls`, `validations`, and any future mutable directory-scoped runtime
  evidence collection.

This rule ensures directory lexical order reflects execution order while still
keeping names readable and causally associated.

### 禁止项

以下形态不得作为默认 runtime 文件名或 raw payload ref：

```text
steps/cnVuLTMyNDk1MjE0OTUyNGQ2NDgvZ3JhcGgtMDAy/metadata.json
checkpoints/cnVuLTMyNDk1MjE0OTUyNGQ2NDgvY2hlY2twb2ludC8x.json
llm-calls/3f8a9c1b/request.json
```

base64/hash 只允许作为极端 fallback，例如单个 segment 超过文件系统限制、清洗后发生冲突且无法通过结构化顺序消解。即便 fallback，也必须带可读前缀和短 hash 后缀，例如：

```text
graph-002-007-run-go-test--h-abc123
```

不得使用纯 base64 或纯 hash 作为主路径。

### Run ID Sequence Contract

Run ID 是 runtime 执行状态的主定位符，必须优先使用可读、递增、稳定的形式。自动创建的 workspace execution run 不得再使用随机 hex/hash 字符串，应使用全局数字自增格式：

```text
run-001
run-002
run-003
```

规则：

- 自动 activation 创建 Run 时，runtime 必须扫描当前 Run 仓库中已有的 `run-<positive-int>`，取最大值后加一。该扫描必须基于持久化 Run 仓库，服务重启后继续复用已存在最大值，不得从 1 重新开始。
- 自动 Run ID 至少 3 位数字宽度，1 到 999 使用零填充，1000 以后自然扩展为更多位，保证常规文件名和列表按字典序排序时仍符合执行顺序。
- 历史随机 Run ID、人工指定的非数字 Run ID、测试固定 Run ID 不参与序列计算。
- 若并发创建发生冲突，必须重新读取现有 Run 列表并再次分配下一个数字序列，不得回退到随机 ID。
- Run ID 序列只表达执行顺序，不表达业务领域、任务类型、Goal/DoD 语义或模板语义，保持核心框架中立。
- Step、ToolCall、Patch、Result、Artifact 等 run-scoped ID 可以继续以 Run ID 为可读前缀派生，但不得把随机 hash/base64 作为主路径。

### Raw Payload Ref

`workorch://` raw payload ref 必须使用同一可读层级规则：

```text
workorch://workspaces/<workspace-id>/runs/<run-id>/llm-calls/<run-id>/repair-001/response.json
workorch://workspaces/<workspace-id>/runs/<run-id>/steps/<run-id>/graph-002/007-run-go-test/output.json
```

raw ref 是稳定的逻辑定位，不是本地绝对路径。API 实现必须继续校验 workspace、run、collection、item path 和文件名白名单，防止路径穿越。

## Governance Queue And Resume Contract

决策工单和审批项是 runtime 治理入口，属于中立控制面能力，不承载具体业务领域语义。

- Ticket / Approval 必须保存 `createdAt` 与 `updatedAt`。新建时两个字段相同，状态变更时只刷新 `updatedAt`，不得改变 `createdAt`。
- 治理列表默认按 `createdAt` 倒序分页展示；`cursorAfterId` 必须基于排序后的列表继续翻页，避免新生成的阻塞项被历史 resolved 项挤出第一页。
- File backend 对历史无时间字段的 Ticket / Approval 可用文件 mtime 作为 `createdAt/updatedAt` 兜底；该兜底只表达生成顺序，不引入业务推断。
- blocked run 在 ticket 提交后若短暂恢复又再次阻塞，必须通过 checkpoint / step output / latest ticket 定位新的失败原因，不得只依据“已提交”推断恢复成功。
- replan continuation 若没有显式 `processEvidenceTimeline/currentProgressDigest`，runtime 必须从 `failureContext/ticketContext` 合成最小过程证据，确保 repair / strict convergence 阶段能够通过上下文门禁并看到当前失败面、失败步骤和下一步缺口。
- Provider context gate 仍保持中立：repair / state-changing / replan continuation 必须带过程证据；证据可以来自真实 step timeline，也可以来自结构化 replan failure context 的中立投影。
- replan continuation 的 step output 只能作为诊断摘要继承，保留 status / stage / error / reason / failure class / failure signature / stdout/stderr preview / patch metadata / stream diagnostics summary；禁止继承 raw `content`、完整 `executionGraph`、`llmDecision`、`patches[].content`、`patches[].diff`。否则 successor run 会把历史执行产物重复嵌入 prompt，造成 request JSON 膨胀、CPU/内存尖峰和上下文噪点。

## LLM Context Admission Contract

LLM 输入必须是当前阶段可行动、可验证、无冲突的任务合同。runtime 可以传递 Goal / DoD、当前进展、失败证据、修复目标、历史摘要和 memory，但必须在发送前完成中立准入，不得把运行态噪点、历史失败产物或本地绝对路径包装成业务修复目标。

### Repair Target Admission

`repairTargetSet`、`failureTargetRepairContext`、`repairActionPackage.targetSnapshots` 只允许包含 workspace deliverable 下的 workspace-relative 资源引用。准入规则如下：

- 禁止绝对路径、路径穿越、用户机器路径、`.workorch-data`、`.workorch-workspaces`、runtime raw payload ref、本地 runtime 存储路径进入 patch target 或 repair target。
- 禁止目录、二进制产物、运行态决策/计划文件、provider/request/response 日志、checkpoint、run/step 元数据作为默认 state-changing repair target。
- shell stderr/stdout 中出现的路径默认只是诊断信号。只有该路径能解析为 workspace deliverable 中的真实可行动资源，或由上游 step 显式声明为 `targetRef/failureTargets/conflictRef`，才可升级为 repair target。
- shell 失败如果只表明执行目录、命令上下文、working directory 或验证入口错误，应归类为 `verification_command_context_issue` 或继续进行 bounded evidence acquisition；不得直接要求 LLM patch 目录或创建无关根文件。
- 若没有通过准入的 concrete target snapshot，state-changing repair 必须降级为证据采集合同，而不是强制要求 `patch/apply`。

该规则只约束资源引用的形态和可行动性，不判断资源所属业务类型。

### Context Semantic Layers

LLM-facing context 必须区分以下层级：

- `Goal/DoD`：最终验收合同，始终是收敛判断的权威来源。
- `currentProgressDigest`：当前 run 的最新真实进展和下一步缺口。
- `processEvidenceTimeline`：最近有效证据链，必须带 `sourceRef`，历史 wrapper/schema/provider 失败只能作为摘要而不能覆盖当前事实。
- `failureEvidence` / `structuredFailureSignals`：机械诊断证据，必须标注是否可行动；未准入 target 只能作为 diagnostic-only。
- `repairActionPackage`：本轮唯一可行动 repair 合同，必须包含 required output、target admission 结果和必要的 target snapshots。
- `historicalSignals` / `governanceHistory` / `memory`：参考信息，不得覆盖当前 Goal/DoD、当前 target snapshot 或当前失败证据。

### Memory Recall Admission

Memory 是参考知识，不是运行流水。LLM 输入中的 memory 必须满足 selected/excluded 一致性：

- `selectedMemoryIds` 为空时，LLM-facing `selectedContext.memories` 必须为空或不存在。
- 被 `excludedMemoryIds` 标记的 memory 不得通过 stable selectedContext、历史 executionContext 或兼容字段再次注入。
- strict repair / strict convergence 阶段不得注入 failed/superseded run 的过程失败 memory，除非该 memory 具备清晰适用边界、可复用经验和当前 failure/Goal/DoD 结构化匹配。
- provider retry、schema invalid、stream incomplete、patch syntax failure、non-substantive patch、重复 repair loop 等瞬态过程只能作为 process evidence 或 repair diagnostics，不得作为可复用 memory 影响当前决策。

### Semantic Context Gate

发送 LLM 前必须执行语义门禁。以下情况应 fail-fast、read-required 或转入 evidence acquisition，而不是继续调用 LLM：

- Goal / DoD 缺失或与当前 activation 不匹配。
- state-changing repair 要求 `patch/apply`，但没有通过准入的 concrete target snapshot、coverage target 或明确 root-cause target。
- `repairTargetSet` 包含绝对路径、目录、二进制、runtime 内部文件、运行态计划文件或未解析 workspace-relative 的目标。
- `selectedMemoryIds` 与实际 `selectedContext.memories` 不一致。
- `processEvidenceTimeline` 只有 rejected/invalid LLM wrapper 失败，没有当前可行动失败证据或下一步缺口。
- 上下文同时要求“必须 patch 当前 target”和“当前 target 不存在/不可读/未通过准入”，但没有证据采集出口。

门禁失败的输出必须保持中立，说明缺失的上下文层级和建议的下一步 runtime 动作，不得引入具体业务领域修复建议。

## 核对清单

后续每次收口前先核对：

1. 新逻辑是否只使用中立 stage / artifact / evidence / result 概念。
2. 是否把软件开发、patch、测试框架等领域语义写进 core runtime。
3. 当前阶段是开放判断还是严格收敛。
4. LLM 是否获得了该阶段必要的 Goal / DoD / memory / skill / tool / mcp / soul / prompt。
5. 输出是否满足当前 stage contract。
6. repair 是否继承来源阶段约束。
7. provider retry 是否只处理 provider transient failure。
8. terminal run 是否有符合状态的 result。
9. profile-specific 检查是否位于 profile 层。
10. 验证脚本是否能区分 core contract 与 profile contract。
11. runtime 文件名和 raw payload ref 是否保持可读层级路径，且没有默认 base64/hash 主命名。
12. LLM context 是否通过语义准入，且没有把 runtime 内部文件、绝对路径、目录或二进制产物作为默认 repair target。
13. 当前 run 是否已经通过统一 `ConvergenceContract` 计算出 `need_evidence` / `need_state_change` / `need_validation` / `need_assessment` / `need_governance` / `blocked` 中的唯一状态。
14. 若存在未关闭失败且已有 concrete target evidence，是否持续保留 `runtimeStage=repair`、`runtimeStageMode=strict_convergence`、`repairObjective=produce_state_changing_output`、`stateChangingOutputRequired=true` 与 `repairActionPackage`。
15. decision resolved、patch/apply failed、schema invalid、graph follow-up、replan successor、validation gap 等恢复路径是否都复用同一收敛合同构造，而不是各自拼接局部字段。
16. 在创建 decision ticket 前是否执行 `ShouldCreateDecisionTicket` 门禁；已有 target evidence 且可机械推进时，是否把 LLM decision 转为 repair contract violation。
17. `currentProgressDigest.currentGap`、`currentRuntimeStage`、`repairActionPackage`、`structuredFailureSignals`、`processEvidenceTimeline` 是否自洽，且没有让历史失败覆盖最新成功/失败事实。
