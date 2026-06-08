# Context Assembly Flow

Status: active

Type: runtime-flow

Last Updated: 2026-06-08

## Purpose

本文件定义 WorkOrch 如何为每次 LLM 调用组装上下文，而不是定义 provider API 本身。

## Flow

```text
collect contract + continuity + progress + evidence + memory + capabilities
-> apply protection / normalization / budget rules
-> build prompt sections / messages
-> record request audit
-> submit provider request
```

## Main Stages

### 1. Context Collection

当前 LLM 输入需要覆盖：

- 当前 Goal / DoD
- 当前 stage / contract
- 当前 failure / required output
- 当前 progress / recent evidence
- 当前 selectedContext / memory recall
- 当前 capability summary

memory recall 在 flow 上至少分三类来源：

- activation recall：Run 创建后从 activation context 投影出的 recall
- step input recall：当前 step 实际注入 selected context 后形成的 recall
- recovery recall：resume / compressed context / inherited selected context 进入下一轮前形成的 recall

这些来源都应形成正式 trace，而不是只保留在 raw executionContext。

### 2. Protection And Budgeting

不同 section 应具备不同保护等级：

- raw required
- normalize allowed
- summarize allowed
- reference allowed

Goal / DoD、关键 contract、当前 failure surface 不应被静默裁剪。

Memory 相关保护规则：

- recall trace 摘要可压缩，但当前 step 实际注入的 `memoryIds`、`memoryRecallRefs` 和高价值 recall reason 不应被静默删除。
- provider retry / repair / replan 应继承必要的 recall evidence，但不应把完整历史 Memory 默认重新注入。
- recovery 若只是继承已有 selected context，也应保留 `policy=inherit_compressed_context` 或等价事实。

### 3. Cache-Aware Organization

request 应分为：

- stable runtime prefix
- workspace definition
- capability contract
- current stage bundle
- volatile execution tail
- large refs / trace

Codex 对标核对补充了以下中立收口要求：

- LLM request 应是 layered context，而不是 full runtime dump
- 当前 Goal / DoD、当前 failure packet 和当前 required output 必须始终在历史上下文之前
- tool/command/validation 失败应优先以 bounded summary + ref 进入上下文
- raw stream、full provider payload、large artifact 默认只保留 ref，不直接注入 prompt
- superseded run、旧 rejected decision 和历史治理/repair 链路只能以摘要进入，不得覆盖当前 failure surface

### 4. Request Audit

每次 LLM request 应保留：

- contextBudget
- included / omitted / redacted sections
- protected fields
- integrity warnings
- source refs

## Context Bundle Projection

进入 provider request 前，flow 层至少要把输入归一化为以下 bundle section。
物理 JSON 可以分散，但逻辑上必须等价：

| Section | 必要内容 | 说明 |
| --- | --- | --- |
| `contract` | `workspaceId`、`runId`、`stepId`、`runtimeStage`、`runtimeStageMode`、Goal / DoD、`allowedOutputs` 或 `requiredOutput` | provider/model/base URL、raw stream、raw rejected output 不得进入 |
| `continuity` | 当前 unit / 恢复 root、predecessor lineage、active blocker / run 状态 | 只保留当前恢复链必要事实，不注入完整旧 executionContext |
| `failure` | 当前失败 step、`failureClass`、`failureSignature`、stage、reason / error | raw malformed JSON、raw failed patch 不得直接进入 |
| `progress` | `currentExecutionProgress`、`processEvidenceTimeline`、`currentProgressDigest`、latest evidence refs | 自然语言进度必须可回链到 source refs |
| `coverage` | 当前 Goal / DoD coverage baseline；repair 时必须含 unresolved coverage 或明确说明无 gap | 不得靠关键词临时推断 coverage 缺口 |
| `snapshots` | 当前 repair 所需 target refs、exists、`contentHash`、completeness flags | 禁止伪完整快照或无 hash 的完整 file-write 基线 |
| `capabilities` | 当前 stage 可用 capability kind 与边界 | 不得把未声明能力静默注入 |
| `memory` | accepted input / necessary output / decision memory 的 refs 和摘要 | memory 是参考知识，不是运行流水 |
| `budget` | context budget、repair budget、provider retry budget、stage budget | 只表达预算，不表达业务结论 |

若某个 section 对当前 stage 不适用，必须显式标记 `notApplicable` 或等价省略原因，
不得留空后让 LLM 猜测。

## Context Gate

在真正提交 provider request 前，context assembly 必须先执行中立 context gate。

当前稳定处置只有以下几类：

| Gate | 触发条件 | Flow 处置 |
| --- | --- | --- |
| `pass` | P0-P4 必要字段完整，stage contract 可执行 | 允许提交 provider |
| `read_required` | repair / execution 需要当前 target snapshot，但快照缺失、不完整或不可安全写入 | 先进入 read/resource fetch，再重建下一轮 LLM 输入 |
| `degrade_to_assessment` | 缺口只剩 assessment / evidence 结构问题，已有候选证据 | 切到 assessment-only correction 或 concrete validation evidence |
| `blocker_required` | runtime 无法通过现有 refs / capability 自动取得必要信息 | 创建结构化 decision blocker |
| `fail_fast` | Goal / DoD、required output、current failure、coverage contract 等保护字段缺失 | 不提交 provider，写 runtime/context integrity failure |
| `replan_required` | 原 repair / continuation path 已被证据证明不可继续 | 创建 successor lineage，并继承 predecessor failure context |

补充约束：

- context gate failure 是 provider 前置问题，不得直接降级成普通 state-changing repair
- `stateChangingOutputRequired` 只约束业务产物或验证收口，不得覆盖 provider 前上下文准入失败
- `repairActionPackage`、`currentFailure`、`currentExecutionProgress`、`coverageTargetSet`、
  target snapshot 等保护字段必须按字段级 presence 进行保留，不得互相隐式替代

### Canonical Failure Hydration

进入 `repair` / `replan` / `runtimeRepairFollowUp` 前，flow 层必须先 hydrate
canonical failure context，而不是因为单一字段缺失就创建新的人工 ticket。

可接受的当前失败来源至少包括：

- `lastFailureDiagnostics`
- `failedStepOutput`
- 顶层 `currentFailure`
- `repairActionPackage.currentFailure`
- `repairActionPackage.lastFailureDiagnostics`
- `repairActionPackage.lastObservedFailure`
- `currentProgressDigest.lastObservedFailure`

只要其中任一来源提供了 `failureClass`、`failureSignature`、`reason`、`error`
中的有效信号，就不得误判为“当前失败缺失”。

### Coverage Target Auto-Rebuild

若 `missingDoDItems`、`missingGoalCoverage`、`missingDeliverableCoverage` 已经表明存在
coverage repair，但 `coverageTargetSet` 在预算整形或输入整理中暂时缺失，runtime 应先从
现有缺口事实自动重建中立 `coverageTargetSet`，再决定是否继续 fail-fast / blocker。

只有当前输入里确实没有任何可辨认的 coverage 缺口事实时，才允许把
`coverage_repair_required_without_coverage_target_set` 作为 gate failure。

### Gate Failure Specific Rules

- `runtime_repair_process_evidence_missing`
  - 仅在当前已经进入 `repair` / `runtimeRepairFollowUp` / `replanContinuation`，
    但 `processEvidenceTimeline` 与 `currentProgressDigest` 都缺失时触发
- `protected_repair_action_target_snapshot_missing_after_context_budget`
  - 仅在原始输入中已有可安全写入的 `repairActionPackage.targetSnapshots`，
    但预算处理后丢失时触发
- `llm_context_integrity_failed`
  - 属于 provider 前 context gate failure，不得转成普通 state-changing repair

## Semantic Admission

发送 LLM 前，除了结构 gate，还必须执行语义准入，避免把不具备可行动性的资源、
记忆或历史噪声误装配为当前 repair 合同。

### Repair Target Admission

`repairTargetSet`、`failureTargetRepairContext`、`repairActionPackage.targetSnapshots`
只允许包含 workspace deliverable 下的 workspace-relative 资源引用。

稳定规则：

- 禁止绝对路径、路径穿越、用户机器路径、`.workorch-data`、`.workorch-workspaces`、
  runtime raw payload ref、本地 runtime 存储路径进入默认 repair target
- 禁止目录、二进制产物、运行态决策/计划文件、provider request/response 日志、
  checkpoint、run/step 元数据作为默认 state-changing repair target
- shell stdout/stderr 中出现的路径默认只是诊断信号；只有能解析成真实可行动资源，
  或由上游 step 显式声明为 `targetRef/failureTargets/conflictRef`，才可升级为 repair target
- 若没有通过准入的 concrete target snapshot，state-changing repair 必须降级为
  `read_required` 或 `acquire_concrete_failure_evidence`

### Context Semantic Layers

LLM-facing context 至少要区分以下层级：

- `Goal/DoD`
  - 最终验收合同，始终是收敛判断的权威来源
- `currentProgressDigest`
  - 当前 run 的最新真实进展和下一步缺口
- `processEvidenceTimeline`
  - 最近有效证据链；wrapper/schema/provider 失败只能作摘要
- `failureEvidence` / `structuredFailureSignals`
  - 机械诊断证据；未准入 target 只能作 diagnostic-only；repair-of-repair 中必须优先取 primary actionable failure，不能让 schema/mode/contract wrapper 覆盖当前失败面
- `repairActionPackage`
  - 本轮唯一可行动 repair 合同，必须包含 required output、target admission 结果和必要 snapshots
- `historicalSignals` / `governanceHistory` / `memory`
  - 参考信息，不得覆盖当前 Goal/DoD、当前 target snapshot 或当前失败证据

若 `replanContinuation` 与 `rejectedResultContinuation` 同时存在，context assembly 必须先从 `replanContinuation.currentProgressDigest.lastObservedFailure` 和 `processEvidenceTimeline` 中恢复未关闭的 concrete actionable failure。只有没有这类 failure，或 failure 已被后续成功 evidence 明确关闭时，`rejectedResultContinuation` 的 coverage / deliverable 缺口才可作为当前 `currentFailure`。这保证验收层缺口不会覆盖执行层仍未修复的真实失败。

### Memory Recall Admission

- `selectedMemoryIds` 为空时，LLM-facing `selectedContext.memories` 必须为空或不存在
- `excludedMemoryIds` 标记的 memory 不得通过兼容字段再次注入
- strict repair / strict convergence 默认不得注入 failed/superseded run 的过程失败 memory，
  除非其适用边界和当前 failure / Goal / DoD 存在结构化匹配
- provider retry、schema invalid、stream incomplete、patch syntax failure、重复 repair loop
  等瞬态过程只能作为 process evidence 或 repair diagnostics，不得提升为可复用 memory
- wrapper failure 的 rejected raw preview、schema 字段名、runtime contract 字段名和 mode violation 文本不得进入 current failure target admission；它们只能作为 `recentRejectedAttempt` / diagnostics 辅助 LLM 修正输出形态

### Semantic Gate Triggers

以下情况必须 fail-fast、read-required 或转入 evidence acquisition，
而不是继续调用 LLM：

- Goal / DoD 缺失或与当前 activation 不匹配
- state-changing repair 要求 `patch/apply`，但没有通过准入的 concrete target snapshot、
  target refs、line refs 或明确 root-cause target
- `repairTargetSet` 包含绝对路径、目录、二进制、runtime 内部文件或未解析 workspace-relative 目标
- `selectedMemoryIds` 与实际 `selectedContext.memories` 不一致
- `processEvidenceTimeline` 只有 rejected/invalid wrapper 失败，没有当前可行动失败证据或下一步缺口
- 上下文同时要求“必须 patch 当前 target”和“当前 target 不存在/不可读/未通过准入”，
  但没有证据采集出口

## Stage-Specific Required Context

不同 runtime stage 在 flow 上至少需要以下上下文基线：

| Stage | 必要上下文 | 缺失时的 flow 方向 |
| --- | --- | --- |
| `exploration` | Goal / DoD、known resources、capability summary、探索预算 | Goal / DoD 缺失 fail-fast；manifest 可先 list/read |
| `planning` | Goal / DoD、profile contract、known resources、已有 evidence / rejected continuation | Goal / DoD 缺失 fail-fast；known resources 缺失先读取 |
| `execution` | 当前 unit、state-change policy、allowed capability、必要 target refs / snapshots | target snapshot 缺失进入 `read_required` |
| `verification` | 已变更 artifact refs、验证目标、latest action evidence | evidence 不足进入 concrete validation evidence |
| `acceptance` | effective Goal / DoD、coverage matrix、artifact refs、result policy | coverage 缺失进入 assessment correction |
| `repair` | current failure、repairActionPackage、target snapshots / manifest、recent attempts | 缺目标快照进入 `read_required`；缺 failure surface fail-fast |
| `replan` | predecessor result / failure context、failed paths、仍有效 contract | predecessor failure context 缺失 fail-fast |

这些规则在本 flow 文档中只表达“上下文何时足够继续推进”，
不表达任何 profile-specific 业务判断。

## Prompt Turn Organization

在 provider 仍以 chat-completions/messages 为主时，请求应采用多 message 组织，而不是把所有上下文压成一个大 user JSON。

推荐稳定顺序：

- stable runtime contract
- workspace definition
- capability contract
- current stage bundle
- current volatile execution tail
- large refs / trace
- current requested action

约束：

- Goal / DoD 原文必须完整进入 workspace definition block
- recent execution / failure / validation / memory recall 应进入 volatile tail
- large artifact、raw stream、旧 evidence 默认只通过 refs 进入
- prompt normalization 只允许做无语义损失清理，不能改写 Goal / DoD、ticket、approval、patch、diff 或代码块
- provider-facing request 必须把连续 `C0` / `C1` / `C2` 合并为第一段稳定 prefix，并把 `C3` / `C4` / `C5` 作为后续动态 tail
- 每个 provider-facing section 都应保留 section id heading，便于 request-log 与 cache plan 对齐

## Memory Recall Flow

```text
activation selects memory
-> record activation recall trace
-> build selectedContext for step input
-> record step_input recall trace
-> compressed/recovery context inherits or refreshes recall
-> record recovery recall trace
-> project memoryRecallRefs/memoryIds into execution sections and context bundle
```

### Step Input Refresh

进入每个新 step 前，context assembly 不应机械复用上一轮输入，而应先执行 step input refresh。

稳定方向：

- 重新读取当前 stage contract
- 重新投影当前 failure / progress / required output
- 重新判断 selectedContext 是否仍与当前 step 相关
- 若 definition / active blocker / target snapshot 已变化，应重建最小 action 面

约束：

- step input refresh 只重建“当前需要看到什么”，不要求重新注入完整历史上下文
- 若刷新后发现关键上下文缺失，应先进入 context gate，而不是继续沿用旧 step input

### SelectedContext Ordering

`selectedContext` 的当前排序至少应稳定遵守：

1. 当前 step / 当前 failure / 当前 coverage gap 直接相关 context
2. 当前 effective Goal / DoD / required output 的必要 context
3. activation 选中的高价值 Memory / capability / project context
4. recovery 继承的仍有效 selectedContext
5. superseded / historical 的低价值摘要作为最低优先级参考

约束：

- 当前失败面直接相关 context 不得被历史 summary 挤出主上下文
- 旧 run 的 context 只能作为补充引用，不得覆盖当前 active contract
- selectedContext 的排序是中立上下文选择规则，不表达业务结论

### Repair / Retry / Replan Memory Inheritance

repair、provider retry、schema repair、replan successor 进入下一轮时，Memory 继承应遵守：

- provider retry / schema repair
  - 继承当前有效 selectedContext 和 recall evidence
  - 不重新注入完整历史 Memory
- repair
  - 优先继承当前 failure target、当前 coverage gap、必要 project/decision/dod feedback memory
  - 旧失败 run 的低价值 `run_summary` 默认降级
- replan successor
  - 继承 predecessor 中仍有效的 Goal / DoD / context lineage
  - 不机械复制 superseded failure focus 或失效 target snapshot

这些继承都必须通过 `MemoryRecallTrace` 或等价运行态证据留下可追踪事实。

### Guarantees

- activation recall trace 应在 RunID 已确定后写入，避免仅存在未落盘的临时选择结果。
- step input trace 必须对应当前 `RunID + StepID + Stage`，即使 selected context 复用稳定 package 也要可追溯到当前 step。
- recovery trace 必须明确是“重新检索”还是“继承已有 recall evidence”。
- Web/TUI/API 应消费正式 `MemoryRecallTrace` 投影，而不是依赖 raw `executionContext` JSON 展示 recall。
- selectedContext 排序必须体现当前 step/failure/coverage gap 的优先级，而不是单纯按时间或来源堆叠
- repair / retry / replan 的 memory 继承必须留下“为何继承/为何排除”的 trace

## Key Flow Guarantees

- 上下文完整性优先于缓存复用
- 稳定信息靠前，易变信息靠后
- 当前失败、当前进展和当前动作要求不得被历史噪声覆盖
- Memory recall 应作为高价值决策信息进入上下文，而不是完整运行流水
- memory recall 需要具备持久化、事件通知、读模型投影和执行段引用，才能成为正式过程证据
- repair stage 输入应收敛为 deterministic repair packet，而不是拼接完整 run 历史
- context budget 应按层裁剪，P0-P2 关键合同不足时必须 fail-fast 或 blocker，而不是静默压缩
- context turn 组织必须是 cache-aware 且 stage-aware：稳定前缀靠前，易变 execution tail 靠后

## Source Documents

- `../60-verification/audits/codex-request-context-comparison.md`
- `../00-core/runtime-request-cache-note.md`
- `../00-core/runtime-stage-contract.md`
