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

Claim-to-evidence alignment 失败属于可恢复的 evidence quality gap，不是默认终态失败。只要 runtime repair budget 未耗尽，completion gate 必须把该失败归类为中立 `dod_evidence_quality_gap`，在 diagnostics 中保留 `claimAlignedEvidenceNeeded=true`、缺失 claim 文本、原 evidenceRefs 与 reason，并继续派发 concrete validation evidence repair 或等价的 profile 允许验证动作。只有预算耗尽、LLM 明确给出 unsatisfied assessment、或缺少权限/配置等真实治理 blocker 时，才允许进入 rejected / decision / failed 收口。

## LLM 输出模式与分片增量合同

近期运行核对显示，长正文状态变更如果要求 LLM 在单次结构化 JSON 中同时返回大段正文、`executionGraph`、coverage 绑定和 repair 合同字段，容易在执行前失败：一类失败是 `llm_schema_invalid`，另一类失败是 LLM 为降低输出复杂度退化为只读或验证 graph，导致 `stateChangingOutputRequired` 合同无法满足。该问题属于中立的输出载荷与合同稳定性问题，不是某个业务领域的特殊逻辑。

Runtime 必须支持两种中立输出模式：

| 模式 | 适用场景 | 输出形态 | 执行方式 |
| --- | --- | --- | --- |
| `executionGraph` | 小补丁、单点 repair、普通 tool/shell/mcp/git 编排、可稳定放入单次 JSON 的状态变更 | 单次 JSON，包含 graph / patch / tool / assessment 等当前 stage 允许的一种输出 | 现有 graph dispatch 与 state-changing gate |
| `incrementalArtifact` | 大正文、多 artifact 初始化、长文档、长配置文本、连续 schema invalid 后 raw preview 显示大段正文、预计 graph JSON 超阈值 | 小 JSON envelope + 单个 artifact/chunk | Runtime 持有游标、逐 chunk 收集、组装后统一落地和验证 |

默认必须使用 `executionGraph`。只有 runtime 依据中立信号判定单次 JSON 风险过高时，才切换到 `incrementalArtifact`。LLM 可以申请切换，但不能自行决定模式；最终模式由 runtime 下发的 `outputMode` 决定。

### 模式切换规则

`selectOutputMode(context)` 必须只使用中立信号，不得识别业务类型、语言、文件名、框架或具体实现内容。推荐规则如下：

```text
if repeatedSchemaInvalidWithLargeRawPreview:
    return incrementalArtifact
if estimatedPatchContentBytes > maxPatchContentBytes:
    return incrementalArtifact
if estimatedGraphBytes > maxGraphBytes:
    return incrementalArtifact
if expectedStateChangingArtifactCount > maxGraphFileWrites:
    return incrementalArtifact
if emptyDeliverableSurface and openCoverageRequiresStateChange and estimatedInitialArtifactBytesExceedThreshold:
    return incrementalArtifact
return executionGraph
```

建议初始阈值由配置控制：

- `maxPatchContentBytes`: 8 KiB 到 16 KiB。
- `maxGraphBytes`: 24 KiB 到 48 KiB。
- `maxGraphFileWrites`: 2 到 4。
- `schemaInvalidLargePreviewThreshold`: 8 KiB。
- `maxArtifactChunkBytes`: 4 KiB 到 12 KiB。

切换状态机：

```text
executionGraph
  -> large_output_detected
  -> artifact_plan
  -> artifact_chunking
  -> artifact_assemble
  -> artifact_apply
  -> validation_graph
  -> acceptance
```

回切规则：

- 所有 chunk 收齐且顺序、hash、targetRef、artifactId 校验通过后，runtime 组装完整 artifact。
- 组装后的 artifact 必须通过内部 `patch/apply`、等价 state-changing artifact writer 或 profile 声明的写入 capability 落地。
- 落地后切回普通 `executionGraph` / `verification`，运行后续验证和 evidence 绑定。
- chunk 阶段不得直接声明 DoD / Goal satisfied；只能作为 state-changing artifact 的待组装输入。
- `incrementalArtifact` 的任何 continuation 状态推进、chunk 持久化、assemble / apply 调度、step completed 事件和 evidence 记录，都必须发生在当前响应通过 `artifactOutputContract` / mode contract 校验之后。未通过 mode contract 的响应只能记录为 failed step / rejected chunk 诊断，不得先写入 completed step、validation passed、checkpoint completed 或 artifact cursor。
- 分片期间允许必要的信息补充 tool 调用，但必须作为独立 `artifact_context` 阶段表达。该阶段只允许返回有界、只读或取证用途的 `executionGraph`，用于 `file/list`、`file/read`、`shell/command`、`mcp/tool_call`、`git/*` 等 capability 获取当前上下文；不得与 `artifactPlan`、`artifactChunk` 或 `artifactChunkRepair` 同一响应混用，也不得推进 artifact continuation cursor。

### Artifact Plan 合同

进入 `incrementalArtifact` 后，首轮只允许 LLM 返回小 JSON 的 artifact plan，不得夹带大正文、完整源码、完整文档或可执行 graph：

```json
{
  "completionState": "continue",
  "artifactPlan": {
    "artifacts": [
      {
        "artifactId": "artifact-001",
        "targetRef": "relative/path.ext",
        "operation": "create|replace|append",
        "coverageTargets": ["cwi-001"],
        "estimatedChunks": 3,
        "estimatedBytes": 12000
      }
    ]
  }
}
```

Runtime 必须校验：

- `outputMode` 是 runtime 下发的 request / stage contract 字段，不是 LLM response 顶层字段；LLM response 顶层不得返回 `outputMode`，避免响应 schema 与调度合同互相污染。
- 每个 `artifactId` 在当前 run / lineage 内唯一、稳定、可排序。
- `targetRef` 必须位于 workspace deliverable boundary 内，不得指向 runtime internal 目录。
- `coverageTargets` 必须匹配当前 `coverageTargetSet` / `coverageWorkItemContext` / deliverable coverage。
- `estimatedChunks` / `estimatedBytes` 只作为调度提示，不作为完成证据。
- artifact plan 不能替代 state-changing evidence；只有 assemble + apply 后才可产生 evidenceRefs。

### Artifact Chunk 合同

chunk 阶段每轮只允许返回一个 chunk，禁止同一响应同时返回 `executionGraph`、`dodAssessment`、`memoryWrites`、多个 artifact 或多段可执行计划。

Runtime 下发：

```json
{
  "outputMode": "artifact_chunk",
  "expectedArtifactId": "artifact-001",
  "expectedTargetRef": "relative/path.ext",
  "expectedSequence": 3,
  "maxContentBytes": 8192,
  "previousChunkTailBytes": 300,
  "previousChunkTailHash": "sha256:...",
  "stopAfterThisChunk": true,
  "doNotRepeatPreviousContent": true
}
```

LLM 必须返回：

```json
{
  "completionState": "continue",
  "artifactChunk": {
    "artifactId": "artifact-001",
    "targetRef": "relative/path.ext",
    "sequence": 3,
    "content": "next content bytes",
    "isFinal": false,
    "chunkHash": "sha256:...",
    "startsAfter": "optional previous tail marker",
    "endsWith": "optional current tail marker"
  }
}
```

Runtime 必须严格校验并持久化：

- `artifactId`、`targetRef`、`sequence` 必须等于 runtime 下发的 expected 值。
- `content` 不得为空，不得超过 `maxContentBytes`，不得包含 runtime 明确禁止的路径或 internal 管理内容。
- `sequence` 不得跳号、重复、倒退；`isFinal=true` 后不得继续接收 chunk。
- `chunkHash` 必须与 content 一致；重试同一 sequence 时 hash 相同视为幂等成功，hash 不同必须拒绝并重试当前 sequence。
- internal 存储可保存 `previousChunkTail` / `currentChunkTail`，用于连续性校验和重复片段检测；但 LLM-facing `artifactContinuationContext`、event、snapshot、memory recall 和 compact previous output 只能暴露 tail bytes/hash，不能暴露 raw tail 正文。
- chunk 可建议在行尾、段落、函数、JSON/YAML 行等自然边界结束，但 runtime 不得依赖该语义判断业务正确性；最终以 assemble 后验证为准。

### Artifact Context Tool 合同

当 LLM 在 artifact plan 或 chunk 前无法可靠继续，且需要读取 workspace、调用 MCP / shell / git / file capability 以补充上下文时，runtime 可以下发：

```json
{
  "outputMode": "incrementalArtifact",
  "artifactOutputContract": {
    "phase": "artifact_context",
    "mustReturnArtifactContextGraph": true,
    "forbiddenTopLevelOutputs": ["artifactPlan", "artifactChunk", "artifactChunkRepair", "dodAssessment", "memoryWrites"]
  },
  "artifactContinuationContext": {
    "expectedSequence": 3,
    "artifactId": "artifact-001",
    "targetRef": "relative/path.ext"
  }
}
```

该阶段的规则：

- LLM 只能返回一个 bounded `executionGraph`，graph 节点必须是中立 capability 调用；不得包含 state-changing artifact 写入、patch 内容、大正文、DoD accepted assessment 或 memory write。
- graph 执行完成后，runtime 必须恢复同一个 `artifactContinuationContext` 和 expected sequence，继续派发 `artifact_plan` 或 `artifact_chunk`。tool step output 只能以 compact evidence summary / refs 进入后续 prompt。
- `artifact_context` 不是新的 RunState，也不是 completion follow-up；它只在 `running` 内部为当前 artifact continuation 补充信息。
- 如果 context graph 失败，失败按普通 runtime repair / governance 收口，但不得把 artifact cursor 推进到下一 sequence。

Artifact 游标状态必须由 runtime 持有：

```text
artifactId
targetRef
operation
expectedSequence
receivedSequences
receivedChunkHashes
finalReceived
bytesWritten
rollingHash
lastChunkHash
previousChunkTail
coverageTargets
state
```

### 连续性与稳定性处理

为确保分片内容连续且可恢复，runtime 必须执行以下中立处理：

- 每轮只请求一个 chunk，并明确 `stopAfterThisChunk=true`。
- 请求中只携带前一 confirmed chunk 的 tail bytes/hash 等 metadata-only 连续性信号；raw tail 正文仅可留在 runtime internal 存储用于校验，不进入 LLM-facing context 或 evidence。
- 拒绝完整重复 `previousChunkTail` 的 chunk；对小范围重复或断裂只记录 warning，最终由 assemble validation 判断。
- 对每个 chunk 保存 `chunkHash`、`rollingHash` 和持久化状态；服务重启后必须能从最后 confirmed sequence 恢复。
- schema invalid、provider timeout 或 parse failure 只重试当前 chunk，不重跑已确认 artifact plan，也不要求重新生成全部 artifact。
- 若同一 sequence 多次返回不同 hash，runtime 必须进入 chunk repair 或 artifact replan；不得把不一致内容静默拼接。
- 若 artifact 组装后验证失败，repair 应针对组装后的具体 targetRef / validation diagnostics，而不是重新要求 LLM 生成全量多文件 JSON。

### 与现有合同的关系

`incrementalArtifact` 不削弱现有 state-changing、coverage、evidence 和 acceptance 合同：

- artifact plan / chunk 只是“生成状态变更载荷”的中间协议，不是 DoD evidence。
- assemble + apply 后必须产生普通 state-changing evidenceRef，并绑定 coverageTargets。
- 后续 validation、git/write、result、artifact、memory 仍按本文其他合同执行。
- 若 artifact 内容较小或 repair 目标明确，runtime 必须优先使用 `executionGraph`，避免把简单补丁复杂化。
- core runtime 只治理 output size、sequence、hash、target boundary、coverage binding、retry/recovery，不理解 artifact 的业务语义。

### 与 Orchestrator 状态管理的结合

`incrementalArtifact` 不得作为新的 Workspace / Run 顶层状态。Workspace / Run 仍只使用既有治理状态，例如 `running`、`waiting_decision`、`waiting_approval`、`completed`、`failed`、`superseded`。分片增量输出只是 `running` 内部某个 Step / Continuation 的输出承载模式。

### 统一状态层次

为了稳定状态流转，runtime 必须把当前执行拆成四层中立状态，并保持“每层只负责一件事”：

| 层级 | 字段 / 对象 | 负责内容 | 禁止负责内容 |
| --- | --- | --- | --- |
| Workspace / Run 治理状态 | `WorkspaceState`、`RunState` | 工作空间是否有活跃 run、run 是否运行中 / 阻塞 / 完成 / 失败 | 不表达当前 LLM 该输出 graph、assessment 还是 artifact chunk |
| 收敛状态 | `ConvergenceContract.State` | 当前缺口属于 `need_evidence`、`need_state_change`、`need_validation`、`need_assessment`、`need_governance` 还是 `blocked` | 不直接表达 provider prompt 的具体 JSON schema |
| 阶段状态 | `runtimeStage`、`runtimeStageMode` | 当前轮次属于 `planning`、`execution`、`repair`、`verification`、`decision`、`approval` 等哪一类阶段，以及 `open_judgment` / `strict_convergence` / `structured_governance` 模式 | 不决定 artifact plan / chunk / graph 三选一 |
| 输出承载模式 | `outputMode`、`artifactOutputContract`、`artifactContinuationContext` | 当前 `need_state_change` 具体采用 `executionGraph` 还是 `incrementalArtifact`，以及 artifact plan / chunk / apply continuation | 不得改变 `ConvergenceContract.State`，也不得单独把 run 推入 decision / approval |

这四层的允许桥接方向只能是：

```text
RunState(running)
  -> BuildConvergenceContract(...) = need_*
  -> ApplyConvergenceContract(...) = runtimeStage + requiredOutput
  -> selectOutputMode(...) = executionGraph | incrementalArtifact
  -> LLM output / graph / artifact continuation
  -> Validate / governance / next Dispatch
  -> RunState(completed | blocked_on_decision | blocked_on_approval | failed)
```

禁止反向污染：

- `outputMode` 不能覆盖 `ConvergenceContract.State`。例如 `incrementalArtifact` 只是 `need_state_change` 的一种输出方式，不能单独把流程改写成 `need_validation` 或 `need_governance`。
- `runtimeStage` 不能替代 `RunState`。例如 `runtimeStage=decision` 只说明当前 step 输入是治理恢复上下文，不等于 run 已进入 `blocked_on_decision`；只有 ticket/approval 真正创建并写回 run 后，外层治理状态才改变。
- `repairActionPackage.requiredOutput` 不能与 `outputMode` 冲突。若 `outputMode=executionGraph`，则 `requiredOutput` 中不得残留 `mustReturnArtifactPlan`、`mustReturnArtifactChunk`、`artifactPlanSchema`、`artifactChunkSchema`、`forbiddenTopLevelOutputs=[executionGraph,...]` 等 incremental-only 字段；若 `outputMode=incrementalArtifact`，则不得继续保留 `mustIncludeStateChangingNode=true` 这类 graph-only 强约束。
- `artifactOutputContract`、`artifactContinuationContext`、`artifactContinuationId` 只允许在当前 `outputMode=incrementalArtifact` 时进入 LLM-facing stage contract。只要 selector 回落到 `executionGraph`，这些字段必须从当前 step input 清理，continuation 的真实恢复状态只保留在 runtime internal repository 中。

实现上必须遵守“单一生效合同”规则：

- 一个 step input 在同一时刻只能有一套生效的 state-changing 输出合同。
- 若 selector 最终选择 `executionGraph`，LLM-facing 输入中只能出现 graph contract。
- 若 selector 最终选择 `incrementalArtifact`，LLM-facing 输入中只能出现 artifact plan/chunk contract。
- 任意 follow-up、repair-of-repair、resume、decision resolution、approval resolution、artifact assemble/apply 后回切 graph 时，都必须先做一次合同归一化，再进入下一轮 dispatch。

Orchestrator 的主合同仍由 `ConvergenceContract` 决定：

```text
BuildConvergenceContract(...) -> need_state_change
ApplyConvergenceContract(...) -> repairObjective=produce_state_changing_output
selectOutputMode(...) -> executionGraph | incrementalArtifact
```

因此：

- `need_state_change` 是主状态。
- `incrementalArtifact` 是 `need_state_change` 的一种输出 mode。
- artifact plan / chunk / assemble / apply 是 step lineage 内的 continuation，不是新的业务流程。
- mode 切换不得绕过 decision / approval / evidence / result / repair budget / replan 等现有治理机制。

Step input 必须显式声明当前输出模式：

```json
{
  "runtimeStage": "repair",
  "runtimeStageMode": "strict_convergence",
  "repairObjective": "produce_state_changing_output",
  "stateChangingOutputRequired": true,
  "outputMode": "incrementalArtifact"
}
```

当 output mode 为 `incrementalArtifact` 时，LLM step 只允许返回当前 mode 对应的单一结构：

| Step 目的 | 允许输出 | 禁止输出 |
| --- | --- | --- |
| artifact plan | `artifactPlan` | 大正文、完整 artifact、`executionGraph`、`dodAssessment` |
| artifact chunk | `artifactChunk` | 多 chunk、多 artifact、`executionGraph`、`dodAssessment` |
| artifact repair | 当前 chunk correction 或 artifact replan request | 重跑全部历史 chunk、直接 accepted |
| assemble/apply 后 | 普通 `executionGraph` / validation / assessment | 继续 chunking 已 final 的 artifact |

Completion follow-up 必须作为 assessment-only verification 处理。它只能消费已完成 graph / artifact apply 后的 evidence summary 与 compact previous output，必须清理 `stateChangingOutputRequired`、`repairObjective`、`repairActionPackage`、`outputMode`、`requestedOutputMode`、`outputModeSelector`、`artifactOutputContract`、`artifactContinuationContext` 和 `artifactContinuationId`。该规则防止已 applied 的 artifact 在完成确认阶段再次继承 chunk/plan 合同，从而重新启动 artifact plan 或 state-changing repair。

Orchestrator 需要持久化中立的 `ArtifactContinuation`。它属于 runtime internal state，不属于 workspace 交付物：

```text
runId
rootStepId
currentStepId
artifactId
targetRef
operation
outputMode
expectedSequence
receivedSequences
finalReceived
bytesWritten
rollingHash
lastChunkHash
previousChunkTail
coverageTargets
state
createdAt
updatedAt
```

`ArtifactContinuation.state` 推荐使用：

| State | 含义 |
| --- | --- |
| `planning` | 等待 artifact plan 或校验 plan |
| `chunking` | 正在逐 sequence 收集 chunk |
| `assembling` | chunk 已收齐，正在组装完整 artifact |
| `applying` | 正在通过 state-changing writer / patch / capability 落地 |
| `applied` | 已形成普通 state-changing evidenceRef |
| `failed` | 当前 artifact 无法继续自动恢复，需要 repair / replan / blocker |

Orchestrator 调度链路：

```text
LLM state-changing step
  -> output mode selector
  -> artifact plan step
  -> artifact chunk step sequence N
  -> artifact assemble internal step
  -> artifact apply state-changing step
  -> validation graph
  -> acceptance assessment
```

该链路必须复用现有 step / tool-call / validation / checkpoint / event / evidence 记录能力：

- artifact plan / chunk step 写入普通 step lineage。
- chunk 接收、hash 校验、sequence 推进必须写 checkpoint。
- assemble / apply 结果必须产生可引用 evidenceRef。
- validation / acceptance 仍通过现有 completion gate 与 DoD / Goal coverage gate。
- run 恢复时必须先恢复 `ArtifactContinuation`，再决定继续 chunk、assemble、apply、validation 或 repair。

失败恢复规则：

- `llm_schema_invalid`、provider timeout、parse failure 发生在 chunk step 时，只重试当前 `expectedSequence`。
- 当前 sequence 多次 hash 不一致时，进入 chunk repair；不得静默拼接不一致内容。
- artifact apply 失败时，进入普通 state-changing repair，repair target 是组装后的 `targetRef` 和 apply / validation diagnostics。
- validation 失败时，不回到 chunking；应针对已落地 artifact 执行普通 repair / validation flow。
- 若 artifact plan 本身不满足 coverageTargets、target boundary 或 mode contract，拒绝该 plan 并重试 plan step；不得进入 chunking。

事件与可观测性应保持中立：

- `/artifact_continuation/created`
- `/artifact_continuation/chunk_recorded`
- `/artifact_continuation/chunk_rejected`
- `/artifact_continuation/assembled`
- `/artifact_continuation/applied`
- `/artifact_continuation/failed`

这些事件只描述 artifact id、sequence、targetRef、hash、state、evidenceRefs 和 failure reason，不记录业务判断，也不得暴露超出权限的正文内容。正文内容如需存储，应进入 runtime internal managed directory，并按现有 workspace data boundary 与脱敏规则处理。

### 配置项

分片增量输出必须由配置控制，禁止在 core runtime 中写死阈值。推荐配置项：

| 配置项 | 含义 | 默认建议 |
| --- | --- | --- |
| `runtime.llmOutput.maxPatchContentBytes` | 单个 patch / artifact content 允许内嵌到 `executionGraph` 的最大字节数 | `12288` |
| `runtime.llmOutput.maxGraphBytes` | 单次 LLM JSON graph 建议最大字节数 | `32768` |
| `runtime.llmOutput.maxGraphFileWrites` | 单次 graph 内允许的 file-write 类状态变更节点数量 | `3` |
| `runtime.llmOutput.schemaInvalidLargePreviewThreshold` | schema invalid raw preview 达到该阈值后优先切 incrementalArtifact | `8192` |
| `runtime.llmOutput.maxArtifactChunkBytes` | 单个 artifact chunk content 最大字节数 | `8192` |
| `runtime.llmOutput.previousChunkTailBytes` | 下一轮请求携带的前一 chunk tail 字节数 | `300` |
| `runtime.llmOutput.maxChunkRetryAttempts` | 同一 sequence 解析失败、provider timeout 或 hash mismatch 的最大重试次数 | `3` |
| `runtime.llmOutput.maxArtifactPlanAttempts` | artifact plan 合同失败最大重试次数 | `2` |
| `runtime.llmOutput.maxArtifactReplanAttempts` | 同一 artifact 重新规划最大次数 | `2` |

配置加载后必须进入 runtime snapshot / run input 中，确保 LLM 请求、event、checkpoint 和后续恢复都能追溯当时使用的阈值。产品面可以展示生效值，但不能让某个业务 profile 在 core 中硬编码覆盖。

部署配置要求：

- `config/file/application.toml` 与 `config/container-file/application.toml` 必须显式包含 `[runtime.llmOutput]` 默认值，避免不同机器使用隐式默认时难以排查。
- Web “系统配置”只展示和保存这些中立阈值，不允许按具体 workspaceTemplate、语言、框架或业务目标覆盖 core runtime 阈值。
- 修改阈值后，新 Run 必须在 run input / context budget / checkpoint 中可追溯；历史 Run 继续以当时记录的阈值解释执行过程。
- 配置文档和部署说明只描述阈值语义、默认值、失败模式和观测入口，不嵌入具体业务场景示例。

### 产品展示边界

`incrementalArtifact` 是运行中的内部 continuation，不是用户交付物本身。core 只要求产品面默认展示 continuation metadata，而不是正文内容。

具体展示规则统一转入：

- `../50-product-interface/tui/`
- `../50-product-interface/web-console/overview.md`

### 持久化落点

`ArtifactContinuation` 和 chunk 正文属于 runtime internal managed state，不能写入 workspace 交付目录。推荐路径：

```text
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/metadata.json
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/plan.json
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/chunks/{sequence}.json
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/chunks/{sequence}.content
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/assembled/content
.workorch-data/internal/workspaces/{workspaceId}/runs/{runId}/artifact-continuations/{continuationId}/assembled/metadata.json
```

存储规则：

- `metadata.json` 保存 continuation 状态、游标、hash、targetRef、coverageTargets、retry counters 和 evidenceRefs。
- `plan.json` 保存已接受 artifact plan，不保存无效或被拒绝的大段正文。
- `chunks/{sequence}.json` 保存 chunk 元数据、hash、长度、LLM call refs、step refs 和校验结果。
- `chunks/{sequence}.content` 保存 chunk 正文；正文不进入 memory recall，不进入通用 event payload。
- `assembled/content` 保存组装后的完整正文，仅在 apply 前作为 runtime internal 临时载荷。
- `assembled/metadata.json` 保存 total bytes、rolling hash、source chunk refs、apply target 和 apply evidence refs。

LLM-facing context 必须对分片状态使用专用投影：

- chunk 请求只投影当前 continuation 游标、`expectedSequence`、tail bytes/hash、artifact/chunk hash、bytes、coverageTargets 和输出约束。
- 历史 step output 中的 `artifactChunk` 只能保留 artifactId、targetRef、sequence、isFinal、chunkHash、startsAfter、endsWith、contentBytes、contentOmitted 等元数据。
- artifact plan / chunk repair 只能保留目标、sequence、coverage、估算、失败原因和下一步动作等合同字段。
- chunk 正文、assembled 正文、raw body、diff-like body 不得通过 `previousStepOutput`、process evidence、event payload、run summary 或 memory recall 进入后续 LLM prompt。
- artifact chunk 阶段禁止写入 memory；该阶段的正文只属于 runtime internal artifact continuation 存储，不能被显式 `memoryWrites` 包装成长期参考信息。

清理策略：

- run 完成后，默认保留 metadata、plan、chunk hash 与 assembled hash。
- chunk 正文是否保留应由 runtime retention 配置决定；默认可保留在 internal 目录用于审计和恢复，但不得作为 workspace 交付物。
- 若 retention 禁止保留正文，必须至少保留 hash、bytes、targetRef、sequence 和 apply evidenceRef，保证 evidence 链可解释。

### Output Mode Selector 优先级

`selectOutputMode` 必须在以下入口统一调用：

- 初始 state-changing execution / repair LLM step 构造前。
- `llm_schema_invalid` / `llm_decision_invalid` repair 输入构造前。
- state-changing repair 连续失败且 raw preview 包含长正文时。
- replan successor 继承 open coverage 且需要 state-changing output 时。

优先级：

| 优先级 | 条件 | 结果 |
| --- | --- | --- |
| P0 | 当前阶段不是 `need_state_change`，或 state-changing output 不允许 | 禁止 incrementalArtifact |
| P1 | 已存在未完成 `ArtifactContinuation` | 继续当前 continuation |
| P2 | 当前 chunk / plan / assemble / apply 正在恢复 | 按 continuation state 恢复 |
| P3 | 连续 schema invalid 且 raw preview 超过阈值，或 raw preview 中存在大量 content-like 字段 | 切 `incrementalArtifact` |
| P4 | 预计 patch content / graph bytes / file-write 节点数量超过阈值 | 切 `incrementalArtifact` |
| P5 | LLM 申请 `requestedOutputMode=incrementalArtifact` 且 runtime 估算满足阈值 | 切 `incrementalArtifact` |
| P6 | 其他情况 | 使用 `executionGraph` |

selector 只能设置 output mode 与对应 contract，不得改变 `ConvergenceContract.State`。如果 selector 判定不能使用 incrementalArtifact，必须回到原有 `executionGraph` 合同，而不是直接创建 decision ticket。

selector 输出必须继续投影到 LLM-facing `Stage Contract`，至少包含：

- `outputMode` / `requestedOutputMode`。
- `outputModeSelector.priority`、`reason`、`mode`。
- `llmOutputPolicy` 中影响本轮输出模式的阈值。
- `artifactOutputContract.mode`、`phase`、chunk bytes、tail bytes、apply evidence stage 等字段。
- `artifactContinuationId` 与 `artifactContinuationContext` 中的 metadata-only 游标字段。

运行态不得只在 step input 或内部 executionContext 中保存这些字段。如果 `Stage Contract` 不包含 output mode 与 artifact phase，LLM 会继续按普通 graph schema 输出，导致 runtime 进入 schema invalid / repair / replan 循环。该投影仍属于中立阶段合同，不得包含任何具体业务、语言、框架或文件名特化判断。

### LLM Schema

进入 incrementalArtifact 后必须使用独立 schema，避免与普通 graph schema 混用。

`artifactPlan` schema 关键字段：

```text
completionState: "continue"
artifactPlan.artifacts[].artifactId
artifactPlan.artifacts[].targetRef
artifactPlan.artifacts[].operation
artifactPlan.artifacts[].coverageTargets[]
artifactPlan.artifacts[].estimatedChunks
artifactPlan.artifacts[].estimatedBytes
```

`artifactChunk` schema 关键字段：

```text
completionState: "continue"
artifactChunk.artifactId
artifactChunk.targetRef
artifactChunk.sequence
artifactChunk.content
artifactChunk.isFinal
artifactChunk.chunkHash
artifactChunk.startsAfter
artifactChunk.endsWith
```

所有 incremental artifact response schema 都不得包含顶层 `outputMode`。`outputMode`、`artifactOutputContract.phase`、`artifactContinuationContext` 和 selector reason 属于 request-side stage contract；response 只返回当前阶段允许的单一 artifact plan、chunk、chunk repair、context graph、assessment 或 blocker。

`artifactChunkRepair` / artifact replan schema 关键字段：

```text
completionState: "continue" | "blocked"
artifactChunkRepair.artifactId
artifactChunkRepair.targetRef
artifactChunkRepair.sequence
artifactChunkRepair.failureReason
artifactChunkRepair.nextAction: "retry_current_sequence" | "replan_artifact" | "blocker"
artifactChunkRepair.constraints[]
```

`blocked` completionState 只允许在显式 `artifactChunkRepair` schema 中使用；普通 `workorch.llmExecutionDecision.v1` 输出仍使用既有 `blocked_on_decision` / `blocked_on_approval` 状态。

Schema 互斥规则：

- `artifactPlan` 响应不得包含 `artifactChunk`、`executionGraph`、`dodAssessment`。
- `artifactChunk` 响应不得包含 `artifactPlan`、`executionGraph`、`dodAssessment`。
- `artifactChunkRepair` 响应不得包含大正文，除非 `nextAction=retry_current_sequence` 且正文在新的 `artifactChunk` 响应中返回。
- `artifact_context` 响应只能包含 bounded `executionGraph`，不得包含 artifact plan/chunk/repair、DoD assessment 或 memory writes。
- 普通 `executionGraph` 响应不得同时声明 active `artifactChunk`。

### 服务重启恢复

服务重启或 orchestrator 恢复时，必须按 continuation state 决定恢复动作：

| 恢复状态 | 处理 |
| --- | --- |
| 无 continuation | 按普通 ConvergenceContract 构造 |
| `planning` 且无 accepted plan | 重派 artifact plan step |
| `planning` 且 plan 已接受 | 切到 `chunking`，请求第一个 artifact 的 sequence 1 |
| `chunking` | 从 `expectedSequence` 继续；已确认 chunk 不重取 |
| `assembling` | 重新从 confirmed chunks 组装，并复核 rolling hash |
| `applying` | 检查是否已有 apply evidenceRef；若有则转 `applied`，否则重试 apply |
| `applied` 但未验证 | 切回 validation graph |
| `failed` | 进入 artifact repair / replan / blocker，由 ConvergenceContract 决定是否继续 |

恢复不得要求 LLM 重新生成已确认 chunk，也不得因服务重启丢失 sequence、hash 或 coverageTargets。

### Evidence 映射

artifact plan / chunk 不是 final evidence。证据必须在以下节点产生：

```text
artifact_assembled -> internal payload ready
artifact_applied -> state-changing evidenceRef
validation_graph -> validation evidenceRef
acceptance -> result evidence mapping
```

coverageTargets 继承规则：

- plan 中的 `coverageTargets` 是 artifact 级目标集合。
- chunk 默认继承 artifact 级 `coverageTargets`，除非 runtime 明确允许 chunk 细分目标。
- apply evidence 必须引用 artifact 级 `coverageTargets`。
- validation evidence 必须引用 apply evidence 或相同 coverage target。
- acceptance 只能引用 apply / validation evidence，不能直接引用 chunk 作为 satisfied 证据。

### Web / TUI 展示边界

core 只要求增量 artifact 流程具备可投影的进度 metadata，不在本文展开产品展示规则。

产品面的展示要求统一转入：

- `../50-product-interface/tui/`
- `../50-product-interface/web-console/overview.md`

### 验证与历史边界

core 只保留边界：`incrementalArtifact` 必须具备中立验收覆盖，至少覆盖
mode selector、artifact plan/chunk/apply/validation、服务重启恢复、event metadata-only
和 coverage target 准入。

具体验证矩阵与历史收口记录统一转入：

- `../60-verification/core-runtime-verifier.md`
- `../90-archive/completed/core-runtime-closure-history.md`

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

### Direct NextStep 合同

`nextStep` / `nextKind` / `nextInput` 只是 step lineage 的 continuation 指针，不是新的状态机。它们必须遵守以下中立规则：

- direct `nextStep` 只能延续当前 run 的既有 `ConvergenceContract` / `runtimeStage` 语义，不能绕过统一合同直接创造一套新的隐式阶段。
- workspace 级 LLM continuation kind 必须 canonicalize 到单一 step kind：`llm/workspace_execution`。历史或测试别名（如 `llm/workspace-plan`）若仍进入 orchestrator，runtime 必须在 dispatch 前折叠为 canonical kind，避免绕过 LLM step context enrichment、memory capture、failure classification、output contract gate 和 read model 统计。
- 若 `nextKind` 是非 LLM capability（如 `shell/command`、`mcp/tool_call`、`patch/apply`、`git/*`、`file/*`），是否需要显式 `runtimeStage` 取决于后续 follow-up / repair 合同；该 capability 本身不是新的 LLM 阶段入口。
- 若 `nextKind` 是 `llm/*`，且 `nextInput` 没有显式 `runtimeStage` / `runtimeStageMode` / `soulStage`，runtime 可以只在“普通 direct continuation”场景下继承上一 step 已显式存在的阶段：例如 `repair -> shell -> llm`、`execution -> tool -> llm`、`verification -> shell -> llm`。
- 只有当上一 step 不是 decision / approval blocker 输出，且不是通过 governance wrapper 恢复出的 continuation 时，才允许做上述继承。blocker 之后的 LLM continuation 阶段必须由 decision / approval resolution、assessment-only builder、graph follow-up builder、runtime repair builder 或等价显式入口注入。
- direct continuation 不得从 `blocked_on_decision` / `blocked_on_approval` 的 wrapper 语义中推断出新的执行阶段。例如前一步因为 decision/approval 被阻塞并声明了 `nextStep`，其后续 `llm/*` continuation 在恢复前应保持“阶段未决”；resolution 恢复链只能补充 governance envelope，真正的执行阶段由恢复链根据失败/contract 重新决定。
- governance resolution merge 只允许携带 `decisionResolution`、`approvalResolution`、`ticketContext`、`recoveryAction` 等恢复封套字段。若目标 spec 已经显式拥有 `runtimeStage`、`runtimeStageMode`、`soulStage`、`convergenceContract`、`repairActionPackage.requiredOutput`、`outputMode` 或 `artifactOutputContract`，merge 不得覆盖这些字段。

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
| `need_state_change` | 未关闭失败存在，且已有 target refs / line refs / snapshots / patch repair context / failure target repair context / structured diagnostics 等 concrete implementation target evidence | `runtimeStage=repair`，`runtimeStageMode=strict_convergence`，`repairObjective=produce_state_changing_output`，`stateChangingOutputRequired=true`，`repairActionPackage.requiredOutput.acceptedKinds` 必须来自当前 capability contract。`patch/apply` 是首选 state-changing artifact；当状态变更只能由已声明 capability 表达时，可接受显式声明 state-changing intent 的 `shell/command` / `mcp/tool_call`，并必须带依赖其后的 bounded verification。 |
| `need_validation` | 当前 state-changing artifact 已应用，下一步需要证明结果；或只存在 Goal / DoD / deliverable coverage gap、尚无 concrete implementation target evidence，且尚未证明 bounded validation evidence 无法继续推进 | `runtimeStage=verification`，只允许 bounded validation evidence graph；不得返回新的 LLM 嵌套、只读自检 graph 或 `patch/apply` |
| `need_assessment` | 验证证据足以进入 DoD / Goal 覆盖判断，或 assessment-only contract correction | `runtimeStage=verification` 且 `assessmentOnly=true`，只允许 `dodAssessment` / coverage correction |
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
- 若合同状态是 `need_state_change`，输入必须包含 `repairActionPackage`，并至少包含一个通过准入的 concrete implementation target evidence：`repairTargetSet`、`structuredFailureSignals.targetRefs` / `lineRefs`、`targetSnapshots`、`patchRepairContext`、`failureTargetRepairContext` 或等价 capability evidence。`coverageTargetSet` 只定义待关闭的验收范围，不能单独作为允许 `patch/apply` 的实现目标证据。
- 若合同状态是 `need_evidence`，输入必须移除顶层 `stateChangingOutputRequired`，并把 `repairActionPackage.requiredOutput.acceptedKinds` 限定为 evidence acquisition 能力。
- 若合同状态是 `need_validation`，输入必须说明最近 state-changing artifact 或 coverage target set、目标 scope、bounded verification 条件和禁止的 read-only/self-inspection-only 输出；`repairActionPackage.requiredOutput` 必须设置 `mustProduceConcreteValidationEvidence=true`，且不得继承旧的 patch-only `acceptedKinds=["patch/apply"]`。
- 若上一轮已经处于 concrete validation evidence 合同，LLM / completion gate 返回 failed `dodAssessment` 或等价 coverage-missing 结构，且输入、失败步骤和 package 都没有 concrete implementation target evidence，则下一轮不得继续重复 `need_validation`。Runtime 必须转为 `need_evidence`，用有界 `file/list` / `file/read` / `shell/command` / `mcp/tool_call` 收集目标证据；只有获得 target refs、line refs、target snapshots、structured diagnostics 或等价 capability evidence 后，才能进入 `need_state_change`。
- 若重复 validation-only repair 已取得当前 workspace surface evidence，且该 evidence 明确表示根路径 `.` / 空路径的 `file/list` 未截断并且 entries 数量为 0，则该 evidence 可作为中立的“当前交付面为空”事实。对于不依赖不可见既有资源的 open coverage gap，Runtime 必须把后续 repair 升级为 `need_state_change`，要求产生受当前 Goal / DoD / coverageWorkItem 约束的 state-changing artifact；不得继续在同一空 surface 上重复 bounded validation 或泛化 evidence acquisition。该规则只识别 workspace surface 形态，不推断具体业务、文件类型、语言、框架或实现内容。
- 若 `need_validation` 来源于 `rejectedResultContinuation.coverageGaps` 或 `unsatisfiedDoDItems`，`BuildConvergenceContract` 必须把未关闭缺口显式 rehydrate 为 `coverageTargetSet` 并写入 `ConvergenceContract.RepairActionPackage`。`ApplyConvergenceContract` 不得依赖构造阶段对克隆 input 的副作用；即使原始 input 只携带 execution context，也必须从 contract package 恢复顶层 `coverageRepairRequired` / `coverageTargetSet` 和 concrete validation `requiredOutput`。
- 若同一 `coverageTargetSet` 同时包含“主交付物状态变更”与“补充型状态变更证据”目标（例如版本快照、审计快照、提交记录、登记写入等），profile-specific evidence requirement 只能作为补充要求，不能覆盖主状态变更合同。Runtime 必须继续要求 primary state-changing artifact，并把补充写入类 evidence requirement 作为后续或并行约束；不得把 `acceptedKinds` 收窄为仅快照/提交/登记类写操作，从而跳过实际交付物变更。
- Runtime schema / contract 字段路径永远不能成为 workspace target evidence。包括但不限于 `validationEvidence.*`、`dodCoverage.*`、`dodAssessment.*`、`executionGraph.*`、`repairActionPackage.*`、`convergenceContract.*`、`runtimeDiagnostics.*`、`currentExecutionProgress.*`、`contextBudget.*`、`llmUsage.*` 等字段。它们可以保留为 diagnostic identifier / message，但不得进入 `repairTargetSet`、`targetSnapshots`、`failureTargetRepairContext`、`patchRepairContext` 或任何允许 state-changing artifact 的 target scope。
- 当 state-changing repair 只持有 coverage gap、runtime schema 字段、历史 wrapper 错误或不存在的伪 target，而没有通过中立准入的 concrete implementation target evidence 时，`BuildConvergenceContract` 必须降级为 `need_evidence`，并清理无效 target evidence。此时只允许 bounded `file/list` / `file/read` / `shell/command` / `mcp/tool_call` 获取当前目标证据；不得伪造缺失文件快照，也不得要求 LLM 基于无效 target 输出 patch。
- LLM 返回“缺少当前文件内容快照、需要先读取目标、无法安全生成补丁”等语义时，只要没有真实 governance blocker，runtime 必须把它解释为可机械执行的 evidence acquisition gap。该类输出不得直接造成 run failed，也不得创建人工 decision；应继续请求有界读取、manifest grounding、诊断命令或等价 capability evidence，直到获得有效 target evidence、预算耗尽或证明确实无法自动获取。
- `currentProgressDigest`、`processEvidenceTimeline`、`repairActionPackage`、`structuredFailureSignals`、`latestClosureEvidence` 的当前事实必须一致；历史 wrapper/schema/provider 失败只能作为历史诊断摘要。
- 当最新 concrete failure 被包裹在 follow-up / wrapper LLM failure 后面时，runtime 必须从 `currentExecutionProgress.latestActionEvidence` / `completedGraphEvidence` 的 stdout、stderr、error、targetRef、line refs 或 failure signature 中提取中立 target evidence。若可机械提取出 target refs / line refs / failure target scope，则该 evidence 必须提升为当前 `currentGap` 与 `repairActionPackage.structuredFailureSignals`，并恢复 `need_state_change`；不得因为 wrapper failure 本身是 read-only graph、schema invalid 或 dodAssessment failed，就退回 `need_evidence`。
- target evidence 抽取必须保持中立准入：显式 `targetRef` / `failureTargets` / `conflictRef`、行号诊断、文件系统失败诊断或 workspace manifest grounding 可以成为 repair target；普通 stdout/stderr 中偶然出现的 path-like 文本只能作为诊断消息，不能单独升级为 `repairTargetSet`。Core 不得通过枚举业务文件名、系统库路径、语言运行时路径、测试框架路径等黑白名单决定 target 是否可修。
- `latest_successful_action_evidence` 只能作为已完成动作或候选证据引用，不得被提升为 `currentFailure`、`lastObservedFailure` 或 failure target scope。只有 step 自身失败，或 completed action evidence 被明确标记为 `evidenceValidity=actionable_failure` / 输出中存在可机械识别的失败标记时，才允许覆盖旧失败成为当前失败。
- `repairActionPackage.currentFailure` 与 `repairActionPackage.currentExecutionProgress` 是 canonical repair context 的组成部分。若顶层 `currentFailure` / `currentExecutionProgress` 缺失但 package 内存在有效字段，runtime 在 provider context gate 前必须提升或等价识别这些字段；不得误报 `runtime_repair_current_failure_missing`。
- `shell/command` 的可行动失败不能只依赖进程退出码。若命令因管道、截断、wrapper 或 shell 组合返回 `exitCode=0`，但 stdout / stderr 中存在明确失败标记、断言失败、文件行号或测试失败摘要，`processEvidenceTimeline` 与 `currentProgressDigest` 必须把它记录为 actionable validation failure，并作为下一轮 repair 的当前 gap。
- `repairActionPackage` 是唯一 primary actionable contract；不得让 `governanceHistory`、memory、旧 `executionGraph`、旧 rejected patch 或旧 decision question 与其竞争。
- resolved `decisionResolution` / `approvalResolution` 只允许作为 governance recovery envelope 合并进下一轮 step input，用于携带 resolution id、operator answer、ticket/approval context 和 recovery action。若恢复后目标 spec 已经显式拥有 `runtimeStage`、`runtimeStageMode`、`convergenceContract`、`repairActionPackage.requiredOutput`、`outputMode` 或 `artifactOutputContract`，resolution envelope 不得覆盖这些已重建的当前合同；否则会把旧 decision wrapper 错误回灌为新的执行合同。

`ShouldCreateDecisionTicket` 必须作为所有 decision ticket 写入前的统一门禁。若当前合同状态是 `need_state_change`，并且已经具备 concrete target evidence、required output 允许 state-changing artifact、缺口不涉及外部权限/凭证/部署/审批/破坏性变更/无法读取的业务信息，则 LLM 返回的 `blocked_on_decision` 必须被转换为 continuable repair contract violation，而不是直接创建人工 ticket。Runtime 应记录 converted decision 摘要，并继续派发 repair 或 bounded validation。只有合同状态为 `need_governance`，或证据门禁明确缺少 runtime 无法自行获得的信息时，才允许创建 ticket。

同一门禁也适用于 `need_evidence`。若当前合同要求 `concreteFailureEvidenceRequired=true`，并且 `repairActionPackage.requiredOutput` 仍允许有界 `file/list` / `file/read` / `shell/command` / `mcp/tool_call` 取证能力，LLM 返回的 stage-choice / replan / blocked decision 不得直接形成人工 ticket；除非 decision context 明确声明外部依赖、runtime dependency、不可自动获取的业务信息或等价治理 blocker。否则 runtime 必须把它转换为可继续的 evidence-acquisition contract violation，继续要求 bounded evidence acquisition。

若 validation / completion follow-up 已输出 failed `dodAssessment`，并且 assessment 中存在可机械映射的 DoD / Goal / deliverable coverage gap、nextActions、evidence candidates 或 coverage target set，则该输出属于 `need_validation` / `need_state_change` 的可继续收口合同。即使 LLM 同时返回 `llm_decision_required` / `blocked_on_decision`，runtime 也必须先转换为 runtime repair follow-up；只有缺口明确需要外部信息、权限、凭证、配置、审批或无法自动读取的业务事实时，才允许创建 decision ticket。

当 `need_validation` / concrete validation evidence repair 中的 LLM 请求用户选择，但当前合同已经要求 `mustProduceConcreteValidationEvidence=true`，且 `requiredOutput.acceptedKinds` 仍包含可执行验证能力时，该请求默认不属于真实人工决策。Runtime 必须只依据结构化字段判断：若 context 没有明确声明 `scope=external_dependency`、`requiresExternalEvidence`、`equivalentEvidenceNotApplicable` 或等价中立 blocker 字段，则将该 decision 转换为 continuable validation repair contract violation，继续要求 bounded `shell/command` / `mcp/tool_call` 生成具体验证证据。Core 不得通过识别 API、网络、凭证、mock、stub、fixture 等自然语言关键词决定是否阻塞。

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
- replan successor 若继承的是 predecessor `rejectedResultContinuation` 中的 coverage gap / unsatisfied DoD，则首步不得退回普通 evidence acquisition；必须保持 `need_validation` 与 concrete validation evidence contract，并携带 unresolved coverage target set。
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

Result 事件与持久化 marker 必须表达 result 的真实状态。`accepted` result 只能发布到 `/result/accepted` 并生成 `event/accepted` 标识；`rejected` result 必须发布到 `/result/rejected` 并生成 `event/rejected` 标识；`superseded` result 必须发布到 `/result/superseded` 并生成 `event/superseded` 标识。不得让 rejected / superseded result 在 event topic、文件系统 marker 或 read model 中留下 accepted 语义。

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

- 未声明 state-changing intent、缺少 bounded verification 的 shell/tool graph，或 read-only graph 冒充实现进展。
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
- Assessment 输出结构缺陷必须先归类为中立 `assessment_output_contract_repair`，不得误进入 state-changing repair gate。典型缺陷包括 `dodAssessment.goalCoverage.items[]` 缺少 `item` / `requirement` 文本但已有 `goalRequirementId` 与 `state`、缺少 `state`、coverage 字段形态错误、或夹带不允许的 assessment 顶层字段。runtime 可以在 completion gate 校验前用当前 effective Goal / DoD / profile contract 的结构化 requirement catalog 做机械归一化：当 `goalRequirementId` 唯一匹配当前 Goal leaf 时，只允许回填该 leaf 的原文到 `item` / `requirement`；当 `item` 唯一匹配但缺少稳定 ID 时，只允许回填对应 ID。该归一化只补齐合同标签，不推断 satisfied、不新增业务证据、不改变 evidenceRefs / artifactRefs 的质量判定。
- `assessment_output_contract_repair` 属于非状态变更收敛路径：runtime 必须优先进入 assessment-only correction 或直接完成安全结构归一化，不能要求 `repairActionPackage.requiredOutput.acceptedKinds`、不能触发 `state_changing_current_action_missing_accepted_kind`，也不能为可机械修复的 assessment schema 缺口创建 decision ticket。若多次 correction 后仍无法得到结构完整的 assessment，应写入结构化 failed / rejected result 或继续受限 assessment repair；只有确实缺少人工信息、权限、外部配置或治理选择时，才允许进入 decision / approval。
- completion gate 因 `goalCoverage`、`acceptanceDimensions` 或 `deliverableCoverage` 缺失、未满足或结构不完整失败时，runtime 必须保留对应 coverage 诊断，并在后续 repair 中进入 assessment-only coverage correction；不得把纯 assessment 合同缺口误升级为普通 execution repair。
- completion gate 因 evidence quality、claim-to-evidence alignment、current-run evidence 或 profile evidenceRequirements 失败时，runtime 必须保留 `dod_evidence_quality_gap` 诊断，并在后续 repair 中进入 concrete validation evidence repair 或 profile 允许的等价验证路径；不得因为该失败发生在 runtime repair follow-up / assessment correction 之后就立即写 rejected result。repair 输入必须继承缺失 item、scope、claim-aligned evidence 标记、候选 evidence、最新成功/失败 evidence 摘要与原失败 reason，避免 LLM 在下一轮只看到泛化“证据不足”。
- 若 profile evidenceRequirements 对某个未关闭 coverage / deliverable target 声明了 state-changing evidence（例如任意中立 state-changing capability kind 及其 operation），该声明优先于历史 `need_evidence` / `concreteFailureEvidenceRequired` 包装。即使上一轮因 schema invalid、provider repair 或 assessment correction 留下了 evidence-acquisition 标记，ConvergenceContract 也必须恢复为 `need_state_change`，并把 profile 声明的 `acceptedStepKinds` / `acceptedOperations` 写入 `repairActionPackage.requiredOutput`；不得继续要求 LLM 先获取泛化失败证据，也不得创建人工 ticket。该规则只读取 profile 的结构化 evidenceRequirements，不解释具体业务语义。
- Coverage target 与 profile evidenceRequirements 匹配必须支持中立 scope 标注归一化。当 failed assessment 将某个子 coverage 写成 `目标文本（scope）` 或 `目标文本(scope)`，runtime 可以把括号内 scope 规范化为 `goalCoverage`、`deliverableCoverage`、`acceptanceDimensions` 或 `dodAssessment.items` 对应的中立 scope，并把括号前文本作为 requirement id / item alias 参与匹配。该规则只处理结构标签错位或 wrapper 输出造成的 scope 丢失，不得通过业务关键词推断 scope，也不得让无 scope 标注的 DoD item 跨 scope 匹配 deliverable / goal requirement。
- 后续 repair 若出现新的 `missingDoDItems` / `missingGoalCoverage` / `missingDeliverableCoverage`，必须以当前缺口重建 `coverageTargetSet`，并覆盖旧 `repairActionPackage.coverageTargetSet`。旧 state-changing package 只有在其 profile evidenceRequirements 仍能匹配当前 coverage target 时才可继承；否则必须清理旧 `requiredOutput.mustIncludeStateChangingNode`、旧 evidenceKindRequirements 和旧 target evidence，避免已关闭 deliverable target 污染新的 Goal / DoD validation 缺口。
- Verification follow-up 的 executionGraph 判定不得只把 `patch/apply` 当作 state-changing evidence。若 graph 中存在不依赖同 graph LLM 节点的中立 state-changing evidence node（例如 profile/capability 声明的 `*/write` kind），runtime 应按 state-changing evidence graph 接受；read-only graph 仍必须拒绝。
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

版本控制快照不能只依赖仓库初始化。若某个 WorkspaceTemplate 期望 workspace 输出变更
在运行过程中形成稳定证据，必须继续通过中立合同表达：

- `contextHints.versionControl.git.snapshotPolicy` 只声明快照策略意图，例如
  `commit_meaningful_changes`，core 不得根据该值推导具体业务修改内容。
- `profileContracts.<profile>.deliverableExpectations[]` 可声明
  `version_control_snapshot` 或等价中立交付物 id，并设置 `required=true`。
- `profileContracts.<profile>.evidenceRequirements[]` 可声明该交付物的可接受证据
  step kind，例如 `git/write`；必要时还可声明 `acceptedOperations`，例如
  `commit`。core 只校验 evidenceRefs 是否可追踪、step kind / operation 是否匹配，
  不判断提交内容是否满足具体业务。
- `acceptedOperations` 只约束最终用于满足该 evidence requirement 的证据节点，不得
  被解释为整条 graph 只能包含该 operation。若当前 Soul / capability contract 允许，
  LLM 可以在同一 graph 中使用必要的中立支撑节点准备或验证该证据；这些支撑节点只有在
  自身 kind / operation 同时匹配 requirement 时，才可作为满足该 requirement 的 evidenceRef。
- 匹配 kind / operation 的证据节点仍必须是有效非零工作证据。若输出标记
  `zeroWork=true`、`evidenceQuality=invalid/inconclusive` 或等价弱证据信号，
  runtime 不得把它用于关闭 required deliverable / coverage target；应继续 repair、
  validation 或返回 failed assessment。
- 对应 Soul 必须说明何时读取状态、何时暂存路径、何时提交，以及无可提交变更时如何
  输出 zeroWork / skipped 原因。若 Soul 未声明该流程，即使模板初始化了 `.git`，也不能
  假设 LLM 会自动提交。

该设计保持核心中立：runtime 只执行通用 `git/read` / `git/write` 能力和通用
deliverable/evidence 合同校验；“软件开发变更应形成本地提交”属于软件开发
WorkspaceTemplate / Soul 的配置约束，不允许下沉为 orchestrator 的硬编码规则。

当 `execution/strict_convergence` 阶段的非只读步骤失败，且 run 级 contract 要求 `must_produce_state_change` 时，后续 runtime repair 必须继续携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。这包括 shell/test/build、patch/apply、file/edit、MCP tool 等执行或验证 state change 的步骤失败；普通 read observation 失败只有在缺失目标文件、DoD 缺口同时具备 concrete implementation target evidence，或其他明确 contract failure 指向实现目标时才提升为 state-changing repair。只有 coverageTargetSet / missing DoD / missing Goal 时，应进入 bounded validation evidence 或 assessment correction。

若非 LLM 执行步骤失败原因是中立的瞬时执行超时（例如 runtime deadline / timeout，且尚未形成确定性失败证据），runtime repair 不应强制要求 state-changing 输出；应允许一次受预算约束的重试、缩小验证范围或返回 failed assessment / blocker。该规则只基于“执行超时”这一中立失败形态，不得写入特定命令、语言、测试框架或业务领域判断。

当 provider/config 决策解除后需要重试原失败 LLM step 时，decision resolution 只能补充“已授权重试、已提供信息、恢复动作”等恢复上下文，不得覆盖原 step 已存在的 `runtimeStage`、`runtimeStageMode`、Soul 绑定阶段、`stateChangingOutputRequired`、`repairObjective` 等阶段合同。换言之，`decision` 是恢复入口，不是被重试任务的执行阶段；provider retry 必须继续继承失败 step 的原阶段语义。

当 execution graph 以并行 batch 方式调度时，batch 完成后的续跑必须以本批次所属 graph root / lineage 为锚点计算 dependency-satisfied 节点，不得用 run 内全局“最新 completed graph step”推断当前 graph。全局最新节点可能属于其他 repair、replan 或历史 graph scope；runtime 必须按当前批次 root 继续推进，直到该 graph scope 内没有可执行节点，再进入 follow-up / verification / acceptance。

当 state-changing 输出依赖具体 runtime capability 时，prompt / tool schema / repair guidance 必须明确该 capability 的中立能力边界。若某个 state-changing capability 只支持有限操作，LLM 不得用未声明语义伪造操作。例如某能力只支持“写入完整内容”和“应用 diff”时，不得把“空内容写入”解释为删除。Runtime 必须拒绝不满足 capability contract 的输出，并把拒绝原因作为下一轮 repair evidence；prompt 层也必须把该约束暴露给 LLM，减少重复输出必然失败的 artifact。

`unified_diff` 是通用 patch capability，不属于软件开发专用能力。其 hunk 必须包含显式行号范围，例如 `@@ -12,3 +12,4 @@`；裸 `@@` hunk 属于 capability contract violation。若 LLM 无法确定行号，必须选择安全的替代状态变更表达：基于完整安全快照的 `file_write + baseHash`、精确 `oldText/newText` 编辑、先读取目标，或返回 failed assessment / blocker。Runtime prompt、repair guidance 和 dispatch 校验必须共同表达该规则。

当 state-changing repair 输入已经携带当前 target snapshot，例如 `failureTargetRepairContext`、`patchRepairContext` 或 `repairActionPackage.targetSnapshots` 时，`unified_diff` 还必须包含实质性非空变更。只替换为完全相同内容、只新增/删除空白行、没有 changed lines、或其他无实质状态变化的 diff 必须被 runtime 拒绝，并作为 repair contract evidence 回流。该规则只校验通用 diff 形态与 state-changing contract，不判断具体业务内容是否正确。

能力边界属于 runtime capability contract，不属于业务领域规则。Core 可以声明“某 capability 不支持 delete、content 不能为空、diff 必须完整”等中立能力事实；但不能据此推断某个业务文件是否应该删除、保留或重命名。

### 本地 HTTP 能力边界

Runtime 可以限制 LLM 通过 shell 调用 WorkOrch 自身管理面 HTTP API，但该限制必须按“管理面路径”识别，不得按 `localhost`、`127.0.0.1` 或固定端口一刀切。`/api/v1/workorch/*`、`/runs`、`/observability`、`/metrics` 等 WorkOrch 管理面路径必须经过 `runtimeCapabilities.serviceAPIs` 显式声明；未声明的管理面路径应被拒绝并进入 repair。

Workspace 交付物自身启动的本地服务端点属于当前 workspace 的运行时验证对象。只要命令仍满足 shell 安全边界、非破坏性、有界超时和当前 Goal / DoD，core 不得把 `/health`、协议兼容端点、临时端口上的 mock/stub/fixture 等普通本地服务访问误判为 WorkOrch 管理面调用。若端口、外部依赖或凭证不可用，LLM 应优先选择中立受控替代，例如临时 fixture、stub/mock、隔离工作目录、临时配置或可用替代端口；不得默认杀进程、清理系统端口或执行其他破坏性环境修改。

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
- `dod_evidence_quality_gap` 至少应携带 `scope`、`missingDoDItems` 或对应 coverage 缺口、原 `evidenceRefs`、`reason`、`source`。当失败来自 claim alignment 时，必须携带 `claimAlignedEvidenceNeeded=true`，并在 repair 输入的 `verificationEvidenceGap` 中继续保留该标记，防止后续 concrete validation evidence repair 丢失关键约束。

Core 不得根据 `id` 名称推断业务含义。例如 `persistence_or_side_effect`、`concurrency` 只是 profile 声明的字符串；core 不知道它们对应软件开发、数据写入、测试或任何具体领域。

Run activation 必须把当前有效的 profile/template contract 组装进 `executionContract`。若 workspace 持久化了 `TemplateID`，activation 必须优先按该 ID 读取当前 template 的 `ContextHints` 作为 contract 基线，再保留 workspace 中 template 未声明的扩展字段；template 已声明的同名 contract 字段以当前 template 为准，避免旧 workspace 长期携带过期的 `profileContracts`、`deliverableExpectations` 或 `evidenceRequirements`。仅当旧 workspace 缺少 `TemplateID` 时，activation 才允许根据 default soul / stage soul bindings 兜底匹配 template。该刷新只作用于中立 execution contract，不得在 activation 阶段重新写入或覆盖 workspace 的 soul bindings、skill、mcp 或其他业务绑定。

`executionContextPackage` 交给 LLM 的 `executionContract` 必须保留完整结构，尤其是 profile 下的 `acceptanceDimensions`、`deliverableExpectations`、`evidenceRequirements` 及其 `required`、`acceptedStepKinds`、`rejectedStepKinds` 等字段。Goal / DoD / 历史上下文可以按预算压缩，但 contract 不能被压缩成数组数量或摘要，否则 LLM 无法按 profile 约束做决策，runtime 也会在后续 verification / repair 阶段反复纠偏。

### Profile 映射边界

具体 profile 示例、术语和交付习惯不在 core 中展开。

profile 映射统一转入：

- `../40-profiles-capabilities/overview.md`
- `../40-profiles-capabilities/software-dev/goal-coverage-and-dod.md`
- `../40-profiles-capabilities/software-dev/code-agent.md`

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

## 抽象矩阵收口说明

针对“核心保持业务中立，template 避免绑定具体 Goal / DoD 内容”的历史收口表
已迁入：

- `../90-archive/completed/core-runtime-closure-history.md`

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
4. Stage Contract：当前 `runtimeStage`、`runtimeStageMode`、`stateChangingOutputRequired`、`repairObjective`、`outputMode`、`outputModeSelector`、`artifactOutputContract`、`artifactContinuationContext`、`requiredOutput`、当前阶段可接受输出与禁止输出。
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

### STG-10 Prompt / Memory 收口说明

`STG-10` 历史收口表已迁入：

- `../90-archive/completed/core-runtime-closure-history.md`

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

所有来自 stderr/stdout、coverage、shell 输出、LLM assessment、failure signature 或历史 result 的 targetRef / failureTargets 在进入强修复范围前，必须先与当前 `workspaceFileManifest` 做中立归一化。若诊断路径带有额外项目名、二进制名、工作目录名或工具前缀，例如 `<prefix>/path/to/file`，而 manifest 中存在唯一后缀匹配的真实 workspace 文件，则 runtime 必须把强目标归一为 manifest 路径，并保留原始值为 diagnostic/source ref；若无法唯一落到 manifest 中的文件，该诊断路径只能作为弱诊断消息保留，不得单独锁死 `repairTargetSet`、`failureTargetRepairContext`、`repairActionPackage.targetSnapshots` 或 patch 相关性门禁。该规则只处理路径形态和 manifest 事实，不解释语言、项目结构、业务文件类型或 Goal / DoD 语义。

Scoped repair 允许在 state-changing artifact 后追加验证，但验证必须能被 runtime 识别为 bounded。满足任一条件即可视为 bounded verification：`shell/command` 输入显式声明 `targetRef`、`relatedFailureTargets`、`verificationScope` 或 `boundedReason`；或该验证节点依赖一个直接命中当前 failure target / repair target 的 `patch/apply` 节点。若验证命令宽泛且既无显式 scope，也不依赖命中失败目标的 patch，runtime 必须拒绝并要求补 scope 或返回 failed assessment / blocker。该规则只约束“验证与当前失败目标的可追踪关系”，不得硬编码命令、语言、测试框架或业务类型。

该约束属于 core runtime 的中立合同，只比较结构化目标引用、target snapshot 和 capability input，不解释文件类型、测试框架、编程语言或业务正确性。Core 可以判断“patch target 与失败目标不相关”，但不能判断“某个业务文件应如何修改”。当没有任何显式目标引用时，runtime 不得臆造目标；此时只能依赖 capability contract、LLM 输出和后续 evidence 判断。

若失败证据本身明确指向依赖、manifest、metadata、checksum、lock 等配套资源缺失，state-changing repair 可以写入与失败目标同目录的中立配套元数据目标。该放宽只基于失败文本中的通用前置资源信号、目标路径关系和目标文件名的通用 metadata/manifest/lock/checksum 形态，不得硬编码语言、框架、包管理器或具体业务文件。若配套目标不在同一目录、失败证据没有前置资源信号，或目标只是无关 marker/report/no-op 文件，仍必须按无关 patch 拒绝。

`strategyReassessment.required=true` 时，runtime 不得只使用上一轮 reassessment 记录的 `failedSurface` 作为唯一匹配口径。若上一轮失败是合同错误、schema 错误或 repair-of-repair 错误，而当前 `failureTargetScope`、`repairActionPackage.failureTargetScope`、`currentExecutionProgress.failureTargetScope` 或 `currentFailure.failedSurface` 已经保留原始失败目标，runtime 必须把这些结构化失败表面共同作为可接受匹配集合。LLM 输出只要声明并命中其中任一当前有效失败表面，且满足目标相关性、根因目标和验证合同，就不应被旧的合同错误文本阻塞。

当 LLM 在 `continue_same_target_repair` 中选择修复与失败表面不同的根因目标时，runtime 只能在该目标已经被中立证据落地时接受，例如目标存在于 `failureTargetRepairContext`、`patchRepairContext`、`repairActionPackage.targetSnapshots`、显式 `targetRef`、通用 prerequisite/manifest/metadata/lock/checksum 候选或等价 root-cause candidate。若输出声明了 `rootCauseTarget`，实际 `patch/apply.targetRef` 必须与该声明一致，并且不得把原失败表面再次作为 patch 目标后仅通过文字声明伪装成根因转移。该规则只判断“失败表面、根因配套资源和 patch target 是否由同一组结构化证据连接”，不解释具体语言、包管理器、文件类型或业务语义。

Shell / tool 调用不得被用来隐藏实际目标变更，再通过一个无关 `patch/apply` marker 来满足 state-changing 合同。若某个 capability 本身是声明过的 state-changing artifact，必须由该 capability 的输入、输出和 evidence 直接表达目标关系；否则修复应使用目标相关的 `patch/apply`、返回 failed assessment，或进入 blocker。

当 run 级 `stateChangePolicy=must_produce_state_change` 且失败输出包含 `missingDoDItems` 时，runtime repair 必须先区分中立缺口类型：

- `implementation_gap`：已有 evidence 表明实现缺失、实现错误、artifact 无法应用或非只读执行失败，repair 必须显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。后续 provider retry 也必须继承该约束，不能把实现缺口修复降级为只读 exploration graph。
- `evidence_gap`：verification 阶段只证明“缺少 concrete evidence / 未执行验证 / evidence 不足”，或只恢复出 `coverageTargetSet` / missing DoD / missing Goal / missing deliverable，但尚未形成实现错误诊断、target refs、line refs、当前 target snapshot、patch repair context 或等价 implementation target evidence 时，repair 应携带 `repairObjective=produce_concrete_validation_evidence`，允许受限 tool / shell / mcp 验证图生成证据，且必须清除顶层 `stateChangingOutputRequired` 和旧的 patch-only required output；若该验证图后续产生确定性失败，再由新的失败证据触发 state-changing repair。
- `hybrid_gap`：validation / completion 阶段已经具备明确 coverage gap，并且当前代码/graph evidence 已提示至少一部分缺口需要 state-changing implementation 或 state-changing test updates 才能关闭。此时 runtime 不应把 failed `dodAssessment` 仅视为纯 assessment 结果；应显式把 repair 目标提升为“优先输出 bounded state-changing repair graph，必要时附带对应 verification nodes”，避免先输出 failed assessment 再由 runtime 被动二次归约。

同一 coverage gap 已经过至少一轮 validation-only repair 仍未关闭，并且当前输入仍缺少 concrete implementation target evidence 时，不得继续把下一轮建模为纯 `evidence_gap`。Runtime 必须把该循环标记为 repeated validation-only repair，并升格到 implementation repair path：若已有 target evidence，则要求 state-changing artifact；若仍无 target evidence，则先进入 bounded implementation-target evidence acquisition，而不是继续执行宽泛测试/验证。该规则只比较 repair attempt、coverage target、target evidence 与 state-changing evidence 的结构化状态，不按文件类型、语言、测试框架或业务关键词做判断。

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

该转换只依据中立 assessment 结构、coverage gap、evidence refs/candidates、manifest、target refs、nextActions 和当前 evidence，不得按软件开发、具体文件名、供应商、语言或测试框架做 core 特判。连续多次相同 coverage target 的 failed assessment 必须消耗同一 coverage repair budget；step ID 或 follow-up 包装变化不得重置预算。若 failed assessment 已经包含可行动 coverage gap，runtime 不应再派发额外的 `no-actionable` LLM continuation 来解释同一个失败，而应直接进入 runtime repair / evidence acquisition 闭环；`no-actionable` continuation 只适用于没有 assessment 或没有结构化 coverage gap 的 completion failure。

LLM schema invalid、runtime contract invalid、provider retry 或其他框架级失败中的 JSON field path / schema path / contract path 只能作为诊断标识，不得被提升为 workspace targetRef、failureTargetRepairContext 或 patch target。例如 `dodCoverage.dodAssessment.acceptanceDimensions.items`、`executionGraph.nodes`、`repairActionPackage.requiredOutput`、`convergenceContract.state` 这类路径表达的是 runtime 决策结构，不是交付物文件。runtime 可以保留这些名称作为 `identifiers` / `messages`，但必须从 `targetRefs`、`repairTargetSet`、当前目标快照和 patch 准入目标中过滤，避免 LLM 尝试修复不存在的 schema 文件。

`currentExecutionProgress` 必须反映“最新仍未关闭的缺口”。当后续 completed action 通过 `repairOfStepId`、相同 graph node 或等价中立证据关系明确覆盖某个早先 failed action 时，该早先 failure 不得继续作为 `lastObservedFailure` 或 `currentGap` 传给 LLM；它只能作为 superseded / historical evidence。若仍需保留历史失败，必须显式标记已被后续成功证据覆盖，避免 prompt 同时呈现“已通过”和“仍失败”的冲突语义。

所有 state-changing artifact 的合同校验必须发生在副作用执行之前。`patchRequests`、`patch/apply` graph node、`nextStep=patch/apply` 和 runtime patch output 在写入 workspace、创建 applied patch evidence、记录 completed step evidence 之前，都必须先通过适用的 coverage target、target relevance、state-changing graph、bounded verification 等中立合同校验。只有 coverage-bound 输入缺少或不匹配 `coverageTargets` 时，runtime 才应 fail-fast 并进入 repair/provider retry/schema recovery；普通诊断修复不得被提升为 coverage target violation。`coverageTargets` 属于 patch 的中立治理元数据，必须从 LLM decision schema、top-level `patchRequests`、`executionGraph.nodes[].input`、patch evidence、runtime validator 与后续 `repairActionPackage` 全链路保留；任何 schema normalization、decision mapping、evidence compaction 或 provider retry 都不得静默丢弃该字段。

Runtime 必须维护中立 coverage ledger 或等价的 coverage 状态视图。每个 coverage target 至少包含 `kind`、`targetText`、可选 `goalRequirementId` / `coverageTargetToken`、`state`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs` 与 `reason`。Core 只记录结构关系和证据质量，不判断业务正确性。证据质量至少分为 `direct`、`indirect`、`smoke`、`unrelated`、`invalid`：只有 `direct` / 合理 `indirect` evidence 可以进入相应 coverage target 的 `evidenceCandidates` 或关闭 evidence 链；`smoke` evidence 只能证明基础可执行或环境可用，不得进入功能 Goal / DoD / deliverable coverage candidates；`unrelated` / `invalid` 必须记录 rejected reason 并从后续候选中过滤。

### Coverage Work Items

`coverageWorkItems` 是 coverage ledger 的可执行投影，用于把当前有效 Goal / DoD / deliverable coverage 中需要被证明或关闭的叶子要求拆成可跟踪的中立收敛项。它不是新的业务需求定义，不是新的 orchestrator 状态机，也不得写回或覆盖 workspace 的正式 Goal / DoD。

严格定义：

- 一个 `coverageWorkItem` 只能来源于当前 effective definition baseline 中的一个可追踪 coverage source，coverage source 包括：Goal 叶子需求、DoD 条目、profile 声明的 required acceptance dimension、profile 声明的 required deliverable expectation，或这些 source 在当前 assessment / rejected result 中形成的 failed coverage item。
- 一个 `coverageWorkItem` 只能表达“该 coverage source 当前还缺哪些中立 evidence / governance / artifact 事实才能关闭”，不能表达新的功能、额外质量要求、实现方案、测试方案、技术栈选择或业务解释。
- `coverageWorkItem` 的 `targetText` 必须等于 source 原文、source 的规范化叶子文本，或 profile 已声明的 coverage target 文本；runtime 不得用 LLM 自由总结、历史 memory、失败 reason、nextActions 或自然语言推断生成超出 source 文本范围的新 target。
- LLM 可以建议拆分、合并或澄清 work item，但 core 只能在这些建议能机械映射回当前 effective Goal / DoD / profile coverage source 时采纳；无法映射的建议只能作为 planning note / blocker candidate，不能成为 active work item。
- 当 source 本身是父级标题、章节名、分组项、纯说明段或以冒号引出子项的容器项时，它只能作为 parent context，不得成为必须单独 satisfied 的 work item；只有可判定的叶子项进入 active work item。

Core 可以从当前 definition revision 的 Goal / DoD / deliverable coverage 中生成或更新 `coverageWorkItems`，但只能保留中立结构：

```json
{
  "id": "cwi-001",
  "source": "goal",
  "sourceScope": "goal_leaf",
  "sourceGoalId": "goal-xxx",
  "sourceDoDId": "dod-xxx",
  "definitionRevision": "rev-xxx",
  "sourceRequirementId": "goal-req-001",
  "sourcePath": "goal.leaves[0]",
  "targetText": "leaf requirement text",
  "normalizedSourceKey": "goal:goal-req-001",
  "requiredEvidenceKinds": ["state_change", "validation"],
  "status": "open",
  "acceptedEvidenceRefs": [],
  "rejectedEvidenceRefs": [],
  "coverageBoundary": "must stay within source targetText and profile-declared evidence requirements",
  "currentGap": "missing accepted evidence binding for required evidence kind: state_change",
  "reason": "derived from failed goal coverage item"
}
```

字段规则：

- `source` / `sourceScope` 必须使用中立枚举，例如 `goal` / `goal_leaf`、`dod` / `dod_item`、`acceptance_dimension`、`deliverable`、`result_gap`。它们只表达 coverage 来源，不表达业务类型。
- `sourceGoalId`、`sourceDoDId`、`definitionRevision`、`sourceRequirementId`、`sourcePath`、`normalizedSourceKey` 必须指向当前有效定义或其可追踪叶子项；不得基于历史 superseded Goal / DoD 继续生成新的 active work item。
- `targetText` 只保存叶子要求文本或 profile 已声明的 coverage target 文本，不解释业务含义。
- `requiredEvidenceKinds` 只能使用中立 evidence kind，例如 `state_change`、`validation`、`artifact`、`decision`、`approval`、`external_dependency`。Core 不得把这些 kind 解释为代码、测试、文档、数据集、天气查询或任何具体业务对象。
- `status` 只能表达中立进展：`open`、`in_progress`、`evidence_ready`、`satisfied`、`failed`、`blocked`。它不得替代 run / step 的 `planning`、`execution`、`repair`、`verification`、`acceptance` 等主状态。
- `acceptedEvidenceRefs` / `rejectedEvidenceRefs` 必须引用可追踪 step / artifact / result / decision evidence；不可追踪的 run id、memory-only ref、自然语言总结或旧失败 wrapper 不得关闭 work item。
- `coverageBoundary` 必须说明该 work item 的范围限制：只能关闭对应 source 与 profile 声明的 evidence requirement，不能扩张到相邻 Goal / DoD、历史失败、LLM 建议项或模板通用建议。
- `currentGap` 只能描述缺失的中立 evidence kind 绑定、未满足的 coverage target、当前失败证据或缺少 target evidence；不得写入 profile 领域修复方案。对于 `missing accepted evidence binding ...` 类缺口，语义是 coverage ledger 尚未绑定 accepted evidence ref，不等价于历史 evidence、memory 或当前上下文中不存在候选证据；LLM 应优先把可追踪候选证据回链到 assessment / result，而不是把它解释成 governance 冲突。

Core 可以基于 profile contract 为 work item 计算 `requiredEvidenceKinds`。例如 profile 声明某 scope 需要 direct evidence 或 state-changing evidence 时，runtime 可以把该需求表达为 `state_change` / `validation` 的中立组合；但“哪些文件、命令、测试、接口或业务流程能产生这些 evidence”只能由 workspaceTemplate / profile / Soul / skill / LLM 在当前上下文中决定。

生成规则必须同时满足“不越界”和“全覆盖”：

- Baseline 构造：每次 activation、Goal/DoD 变更、replan successor、rejected result continuation 或 acceptance repair 前，runtime 必须先构造当前 effective coverage baseline。baseline 至少包含当前有效 Goal 叶子需求、DoD 条目、profile required acceptance dimensions、profile required deliverable expectations，以及当前 rejected / failed assessment 中仍未关闭的 coverage source。
- 叶子化：Goal / DoD 文本的拆分只能做结构性叶子化，例如 Markdown list item、编号条目、明确子项、profile 已声明 coverage target；不得按业务关键词继续扩展出额外隐含目标。父级标题和分组项保留为 `parentContext`，不得重复生成 active item。
- 一源一项：同一个 `normalizedSourceKey` 在同一 `definitionRevision` 内最多只能有一个 active work item；若一个 source 需要多种 evidence，用 `requiredEvidenceKinds` 表达，不得拆成多个语义重复的 work item。
- 多源不合并：不同 Goal leaf / DoD item / deliverable expectation 即使文本相似，也不得默认合并；只有它们具有相同 canonical source key 或 profile 明确声明等价映射时，才允许共享 evidence refs，但仍要保留各自的 work item 状态。
- 不从 reason 扩张：failed assessment 的 `reason`、`nextActions`、`knownLimitations`、`residualRisks` 可以帮助描述 `currentGap`，但不能单独生成新的 work item；只有其中存在可机械回链到 baseline source 的 `goalRequirementId`、DoD item、deliverable id 或 coverage target 时，才能更新对应 work item。
- 不从实现方案扩张：计划、patch summary、测试名、文件名、工具输出、memory、LLM 自述或历史 ticket 不能成为新的 active coverage source；它们只能成为 evidence candidate、diagnostic 或 target evidence。
- 完整性门禁：进入 acceptance 前，runtime 必须能证明 baseline 中每个 required source 都有 active / satisfied / blocked / skipped-with-reason work item，或者有明确 `excludedReason`。若缺失 work item，必须先补齐账本或 fail-fast，不得让 LLM 基于不完整 coverage 直接 accepted。
- 排除原因：允许排除的 source 仅限 superseded definition、父级容器项、profile 声明 optional 且未被 Goal/DoD 激活、用户明确移除或治理决策明确放弃。排除必须保留 `excludedReason` 与 source ref，不能静默丢弃。
- 顺序稳定：work item 顺序必须按 baseline source 顺序稳定生成；后续状态变化不得重排 source 顺序，避免 LLM request 与 Web/TUI 显示在多轮执行中来回漂移。

阶段拆分与更新规则：

| 阶段 / 入口 | 允许动作 | 禁止动作 | 输出要求 |
| --- | --- | --- | --- |
| activation / definition changed | 构造新的 effective coverage baseline；基于当前 Goal / DoD / profile required coverage 生成或重建 work item 基线；标记旧 revision item 为 superseded / historical | 复用旧 revision 的 active item 作为当前目标；把新增 work item 写回正式 Goal / DoD | `coverageWorkItems` 带新的 `definitionRevision`、稳定顺序、`normalizedSourceKey`、`excludedReason` |
| `planning` | 将 baseline source 投影为计划单元；补齐缺失的 work item；给 work item 绑定计划级 expected evidence kind | 根据计划、方案或 LLM 推测扩张出超出 baseline 的 active item；关闭 work item | plan / executionGraph 必须能引用 work item 或 coverage target，未覆盖 source 必须显式说明 |
| `execution` | 根据实际 state-changing step、tool action、artifact 生成 candidate evidence；把相关 work item 更新为 `in_progress` 或记录 `acceptedEvidenceRefs` / `rejectedEvidenceRefs` | 新建超出 baseline 的 work item；仅凭执行摘要标记 satisfied；绕过 required evidence kind | state-changing output 必须通过 `coverageTargets` / target evidence 与 work item 对齐 |
| `verification` | 将验证 step 与 work item 对齐；满足验证要求时补 `validation` evidence；不足时更新 `currentGap` / `rejectedEvidenceRefs` | 因总体验证通过而批量关闭无直接对齐的 work item；修改 work item 的 source / targetText | verification evidence 必须保留 step ref、quality、claim alignment 结果 |
| `acceptance` | 执行完整性门禁；把 evidence kinds 齐备且质量通过的 item 标记为 `evidence_ready` / `satisfied`；为未关闭项生成 failed coverage / rejected result scope | 在 baseline 不完整时 accepted；用自然语言 summary / memory-only ref 关闭 item；静默跳过 required source | accepted result 必须引用所有 required work item 的关闭证据或排除原因 |
| `repair` | 针对 failed / open work item 的 `currentGap` 生成 `need_evidence` / `need_state_change` / `need_validation`；继承 work item、coverage target 和 latest evidence | 重新拆分正式 Goal/DoD；把 repair reason 扩张成新 work item；丢弃原 source lineage | repair input 必须带 unresolved work items、required evidence kinds、current gap、latest accepted/rejected evidence |
| `follow-up` / provider retry / schema repair | 继承上轮 work item 账本，修复输出结构或补上下文；只允许更新诊断、rejected evidence 和当前 gap | 因 wrapper failure 清空 work items；把 schema/path 字段当成 coverage source | 下一轮 LLM input 必须继续看到原 unresolved work item scope |
| `replan` / successor run | 从 predecessor 的 unresolved work items 与当前 effective definition baseline 重建 successor scope；保留 evidence lineage | 直接复制旧 run 的 active item 而不校验 definition revision；扩大到历史已 superseded source | successor 初始 input 必须包含 unresolved work item summary 与 source refs |
| `blocked_on_decision` / approval | 将无法自动推进的 work item 标记为 `blocked` 或等待 governance evidence；记录 blocker scope | 把 blocker 文案追加为新 Goal/DoD 或新 active item | ticket / approval 必须引用受影响 work item 与缺失的中立信息 |

更新权限规则：

- `source`、`sourceScope`、`sourceGoalId`、`sourceDoDId`、`definitionRevision`、`sourceRequirementId`、`sourcePath`、`targetText`、`normalizedSourceKey` 只允许在 baseline 构造 / definition changed / successor scope rebuild 时写入；普通 execution / verification / repair 只能读取，不得修改。
- `requiredEvidenceKinds` 只允许由 profile contract、Goal/DoD definition baseline 或治理决策的结构化结果更新；不得由 LLM 自由文本直接改写。
- `status`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs`、`currentGap`、`reason` 可以在 execution / verification / acceptance / repair 中更新，但必须保留 `updatedByStepId` 或等价 source ref。
- 若某阶段发现 work item source 错误或遗漏，必须回到 baseline rebuild / planning correction，而不是在 repair 中临时改写 source 字段。
- 任意阶段更新 work item 后，都必须保持同一 `normalizedSourceKey` 的单 active item 约束，并保留 superseded / historical lineage。

LLM-facing 投影规则：

`coverageWorkItems` 必须以独立、稳定、可行动的 section 进入 LLM-facing context，不能只隐藏在 runtime 内部 ledger、自然语言 summary 或 failed assessment reason 中。投影目标是让 LLM 在每一轮 strict convergence 中直观看到：

1. 当前仍需关闭哪些 Goal / DoD / deliverable source。
2. 每个 source 已有哪些 accepted / rejected evidence。
3. 每个 source 仍缺哪些中立 evidence kind。
4. 本轮应该优先处理哪个 work item，以及允许输出什么。

推荐 LLM-facing 结构：

```json
{
  "coverageWorkItemContext": {
    "definitionRevision": "rev-xxx",
    "coverageBaselineHash": "sha256:...",
    "summary": {
      "total": 7,
      "open": 2,
      "inProgress": 1,
      "satisfied": 4,
      "blocked": 0
    },
    "currentFocus": [
      {
        "id": "cwi-002",
        "sourceScope": "goal_leaf",
        "sourceRequirementId": "goal-req-002",
        "targetText": "leaf requirement text",
        "requiredEvidenceKinds": ["state_change", "validation"],
        "missingEvidenceKinds": ["state_change"],
        "status": "open",
        "currentGap": "missing accepted evidence binding for required evidence kind: state_change",
        "acceptedEvidenceRefs": [],
        "rejectedEvidenceRefs": [
          "run-028/graph-001/003-verify"
        ],
        "nextAllowedConvergenceStates": ["need_evidence", "need_state_change"],
        "forbiddenClosure": [
          "do not mark satisfied with summary-only evidence",
          "do not create new Goal/DoD text"
        ]
      }
    ],
    "ledgerSummary": [
      {
        "id": "cwi-001",
        "sourceScope": "goal_leaf",
        "status": "satisfied",
        "acceptedEvidenceRefs": ["run-028/graph-001/001-change"]
      }
    ]
  }
}
```

投影分层：

- `coverageWorkItemContext.summary` 必须始终进入 P2 Coverage Scope，提供总体数量和状态，避免 LLM 误以为只剩最近失败项。
- `currentFocus` 必须包含本轮可行动的 top-k unresolved work items。严格收敛阶段至少包含 1 个 unresolved item；若没有 unresolved item，应说明进入 acceptance / result 的原因。
- `currentFocus[].targetText`、`requiredEvidenceKinds`、`missingEvidenceKinds`、`currentGap`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs` 是指导 LLM 正向收口的最小必要字段，不能被 summary 替代。
- `ledgerSummary` 只保留非 focus work item 的 id、sourceScope、status、关键 evidence refs 和 omitted reason；不得把全量历史 reason、raw output 或长 evidence body 放入 LLM request。
- full ledger 可以落盘并通过 raw payload ref 引用；LLM request 默认只带 summary + currentFocus + 必要 ledgerSummary，避免单 JSON 膨胀。

按阶段投影：

- `planning`：投影完整 baseline summary、所有 required source 的 work item id / targetText / expected evidence kinds；要求计划覆盖每个 required work item 或给出结构化排除原因。
- `execution`：投影当前 focus work items、允许的 state-changing / tool / artifact evidence kind、coverageTargets 要求和已拒绝 evidence；要求输出的 state-changing artifact 绑定 work item 或 coverage target。
- `verification`：投影已产生 state-changing evidence 但缺 validation 的 work items；要求验证输出显式引用 work item id / coverage target，并说明 evidence quality。
- `acceptance`：投影全部 required work item 的状态矩阵；要求 accepted / rejected result 逐项引用 work item closure evidence 或 blocker / excludedReason。
- `repair`：投影 failed / open work items、currentGap、latest accepted/rejected evidence、nextAllowedConvergenceStates 和 forbiddenClosure；要求 repair 输出只针对这些 unresolved work item，不能扩张新目标。
- `follow-up` / provider retry / schema repair：投影上一轮 unresolved work item scope 与 wrapper failure 摘要；要求 LLM 修复输出结构时不丢失原 work item 目标。
- `replan`：投影 predecessor unresolved work item summary 与当前 definition baseline diff；要求 successor plan 明确保留、关闭或排除每个 unresolved item。

Current action 合同必须由 work item 派生，而不是由自由文本失败摘要单独派生：

- 若 `currentFocus` 缺少 `state_change` 且没有 concrete target evidence，current action 应是 `need_evidence`，要求有界取证 / target discovery。
- 若 `currentFocus` 缺少 `state_change` 且已有 concrete target evidence，current action 应是 `need_state_change`，要求 state-changing artifact 或真实 blocker。
- 若 `currentFocus` 已有 state-changing evidence 但缺少 validation，current action 应是 `need_validation`，要求 bounded validation evidence。
- 若 `currentFocus` evidence kinds 齐备但 assessment 不完整，current action 应是 `need_assessment`，要求结构化 DoD / Goal / deliverable coverage assessment。
- 若 `currentFocus` 依赖用户信息、外部权限、审批或 runtime 依赖，current action 才能是 `need_governance` / `blocked`。

LLM 输出回写规则：

- LLM 输出中的 `coverageTargets`、`evidenceRefs`、`dodAssessment.goalCoverage`、`deliverableCoverage`、`acceptanceDimensions` 或 result coverage 必须能映射到 `coverageWorkItemContext.currentFocus` 或 baseline 中的 work item；无法映射的 claim 只能作为 note / residual risk，不能关闭 work item。
- LLM 可以在 plan / assessment 中提出 `coverageWorkItemSuggestions`，但 runtime 必须先执行“不越界”和“全覆盖”校验，确认建议映射到 baseline source 后才能成为 active item。
- 若 LLM 输出忽略 `currentFocus`，转而处理无关目标、只重复测试/验证而不补缺失 evidence kind，或声称 satisfied 但未引用 work item evidence，runtime 必须把该输出归为 contract violation / failed assessment，并在下一轮 repair 中明确指出未关闭 work item。

LLM 阶段业务语义装配：

`coverageWorkItem` 本身只表达中立 coverage source 与 evidence 缺口。实际业务语义装配发生在 LLM 当前阶段决策中，由以下输入共同完成：

- `coverageWorkItemContext.currentFocus`：说明当前要关闭的 Goal / DoD / deliverable 叶子项、缺失 evidence kind 和当前 gap。
- `executionContract.profileContracts`：说明当前 workspaceTemplate/profile 对 evidence、acceptance dimension、deliverable expectation 的领域映射要求。
- 当前 stage Soul / skill / capability catalog：说明该领域允许如何理解目标、选择工具、设计实现和验证方式。
- `processEvidenceTimeline` / `currentProgressDigest`：说明已经做过什么、哪些证据被接受或拒绝、当前失败是否只是 wrapper / contract failure。
- `repairActionPackage` / `targetSnapshots` / `workspaceFileManifest`：说明当前是否具备可行动目标、当前资源形状和 state-changing artifact 的准入边界。

装配流程必须保持分层：

1. Core 只生成中立 work item 与 current action，例如 `need_evidence`、`need_state_change`、`need_validation`、`need_assessment`。
2. Runtime request builder 把 work item、profile contract、Soul、capability、当前证据和 target snapshot 放在同一 stage bundle 中，并保持字段分层，不把它们合并成自然语言大摘要。
3. LLM 根据 profile / Soul 做业务语义装配：判断该 work item 在当前领域下应通过哪类 state-changing artifact、验证动作、交付证据或治理信息关闭。
4. LLM 输出必须把业务动作重新绑定回中立字段：`coverageTargets` / `workItemIds`、`targetRef` / `relatedFailureTargets`、`evidenceRefs`、`dodAssessment.goalCoverage`、`deliverableCoverage` 或 `acceptanceDimensions`。
5. Runtime 只校验这些输出是否回链到 work item、是否满足 required evidence kind、是否有可追踪 evidence 和 capability contract；不判断业务实现是否语义正确，业务正确性由后续 profile validation evidence 和 acceptance assessment 证明。

示例：在 software-dev profile 中，某个 `coverageWorkItem` 的 `requiredEvidenceKinds=["state_change","validation"]` 并不让 core 理解“功能代码”或“测试代码”。Core 只要求存在 state-changing evidence 与 validation evidence。software-dev Soul 可以在 LLM 阶段进一步装配为“先核对现有源码和测试，若实现缺失则补生产实现并补验证，若实现已存在但证据不足则补针对性测试断言”。该策略属于 software-dev profile / Soul，不得写入 core runtime。

业务语义装配输出必须满足以下回链要求：

- 每个 state-changing graph node 或 patch/tool action 若用于关闭 work item，必须声明 `coverageTargets` 或等价 `workItemIds`，并引用 currentFocus 中的 item。
- 每个 validation graph node 若用于关闭 work item，必须声明 `verificationScope`、`targetRef`、`relatedFailureTargets`、`coverageTargets` 或依赖一个已绑定 work item 的 state-changing node。
- 每个 satisfied assessment item 必须引用能够回链到 work item 的 evidence refs；不能只引用总体 run、泛化测试通过、memory 或自然语言摘要。
- 若 LLM 判断当前 work item 无法自动推进，必须输出真实 blocker / governance scope，并说明缺失的是信息、权限、外部依赖还是 profile 能力；不得把 blocker 文案追加为新 Goal / DoD。

禁止项：

- Core 不得预装配领域语义，例如不得在 core 中把 `state_change` 翻译成“修改功能代码”、把 `validation` 翻译成“补测试”，或根据文件名判断 action 是否符合业务目标。
- LLM 不得把 profile 通用建议、测试名、文件名、错误 reason、历史 memory 或工具输出扩张为新的 work item。
- LLM 不得在没有回链到 `currentFocus` 的情况下输出看似合理的领域修复；这类输出只能作为 rejected evidence 或 planning note。
- Request builder 不得把 work item、profile contract、Soul 和 current evidence 混成一段无结构文本；否则 runtime 无法验证 LLM 是否沿着 Goal / DoD 正向收口。

持久化与读模型：

- `coverageWorkItems` 的权威数据属于 run-scoped runtime state，必须落在当前 run 的 coverage ledger / runtime state 中；workspace 的正式 Goal / DoD 定义仍由 goal / DoD 管理模块持久化。不得把 work item 作为新的 Goal / DoD 写入 `goals.json`、`dods.json` 或等价正式定义文件。
- workspace 级 read model 可以投影“当前 active run 的 coverage work item 摘要”，用于 Web / TUI / API 展示，但该投影不是权威定义。投影缺失时只能影响展示，不得影响 run 内收敛判断；run 内判断必须回到 run-scoped ledger。
- full ledger 必须保留完整 source refs、definitionRevision、status history、accepted / rejected evidence refs、superseded lineage、excludedReason 和更新 step refs；LLM-facing request 默认只读取 summary / currentFocus / compact ledgerSummary。
- `coverageTargetSet` 是当前收敛范围的目标集合，`coverageWorkItems` 是该集合在当前 definition baseline 下的逐项账本。`coverageTargetSet` 可以作为 patch / validation / assessment 的 scope，不能替代 work item 的 evidence 状态；work item satisfied 也不能绕过 `coverageTargetSet` 的准入边界。
- `missingGoalCoverage`、`missingDoDItems`、`missingDeliverableCoverage` 是 assessment / rejected result 的缺口视图。它们必须被 rehydrate 到对应 work item 的 `currentGap`、`missingEvidenceKinds`、`rejectedEvidenceRefs` 或 `status=failed`，不得作为另一套长期状态并行推进。
- `repairActionPackage` 只携带当前 action 所需的 work item 投影，例如 unresolved ids、required evidence kinds、current gap、target snapshots、latest evidence refs 和 required output。它不得成为新的 work item 存储，也不得在 repair-of-repair 中覆盖 source identity。
- result artifacts / run summary / memory 只能引用 work item closure evidence 或 rejected fact，不能复制 full ledger 作为长期 memory 主体；memory recall 中出现的旧 work item 只作为 historical reference，不能驱动当前 revision 的 active scope。
- Web / TUI 展示必须区分 formal definition、runtime coverage ledger 和 current LLM-facing projection：Formal Goal / DoD 表示用户目标，coverage work items 表示当前 run 的收敛账本，currentFocus 表示下一轮 LLM 最小行动面。三者不得在界面文案或 API 字段中混为同一概念。

上下文预算裁剪规则：

- `coverageWorkItemContext` 归属 P2 Coverage Scope，但其内部字段也必须分级保护。裁剪顺序为：先保留 `currentFocus` 最小行动字段，其次保留 `summary`，再保留 compact `ledgerSummary`；full ledger 永远通过 ref / payloadRef 访问。
- `currentFocus` 的最小不可裁剪字段包括：`id`、`definitionRevision`、`normalizedSourceKey` 或等价 source ref、`sourceScope`、`targetText`、`requiredEvidenceKinds`、`missingEvidenceKinds`、`status`、`currentGap`、`acceptedEvidenceRefs`、`rejectedEvidenceRefs`、`nextAllowedConvergenceStates`。strict convergence 中缺少任一字段都必须记录 `contextBudget.integrityWarnings`。
- top-k 选择必须稳定且可解释。优先级为：当前 failed / blocked blocker 所指向 item、缺少 `state_change` 且已有 target evidence 的 item、缺少 `state_change` 但需要取证的 item、缺少 `validation` 的 item、缺少 assessment 的 item、其他 open item。相同优先级按 baseline source 顺序排序。
- 当 unresolved work item 数量超过 top-k 时，`ledgerSummary` 必须包含 omitted counts、omitted status distribution、omitted source scopes 和 `omittedReason=context_budget_top_k`；不得让 LLM 误以为未展示的 item 已关闭。
- 若预算裁剪导致存在 unresolved item 但没有任何 actionable `currentFocus`，runtime 必须 fail-fast 或进入 context required / evidence acquisition；不得提交只包含泛化 summary 的 strict convergence request。
- 若当前 focus item 的 `targetText` 超出预算，应保留 source ref、短摘要、contentHash / normalizedSourceKey、omitted reason，并优先引用 formal Goal / DoD payloadRef；但 `requiredEvidenceKinds`、`missingEvidenceKinds`、`currentGap` 和 evidence refs 仍不可裁剪。
- `acceptedEvidenceRefs` / `rejectedEvidenceRefs` 在 LLM-facing context 中可以 top-k 摘要，但必须保留最新 accepted、最新 rejected、当前 failure 对应 rejected，以及所有用于关闭 required evidence kind 的 refs。被省略的 evidence 必须记录 count 与 payloadRef。
- provider request audit 必须能说明 `coverageWorkItemContext` 的 included / omitted / redacted sections、top-k 策略、protected focus ids 与 full ledger ref，便于离线核对 cache 退化或上下文缺失。

与 `ConvergenceContract` 的精确映射：

| Work item 状态 / 缺口 | 必要条件 | `ConvergenceContract` 状态 | 下一步要求 |
| --- | --- | --- | --- |
| `open` / `failed` 且缺 `state_change`，无 concrete target evidence | `missingEvidenceKinds` 包含 `state_change`，但 target snapshot / targetRef / lineRef / manifest evidence 不足 | `need_evidence` | 有界读取、取证、列举 manifest、运行允许的诊断能力，不能直接要求 state-changing artifact |
| `open` / `failed` 且缺 `state_change`，已有 concrete target evidence | 具备 target snapshot、targetRef、lineRef、repairTargetSet 或等价当前目标证据 | `need_state_change` | 生成 state-changing artifact、真实 blocker 或 failed assessment；输出必须绑定 work item / coverage target |
| 已有可接受 `state_change`，缺 `validation` | `acceptedEvidenceRefs` 中存在 state-changing evidence，`missingEvidenceKinds` 包含 `validation` | `need_validation` | 生成 bounded validation evidence；不得只返回泛化 summary 或只读 inspection |
| required evidence kinds 齐备，但 coverage assessment / claim alignment 不完整 | evidence refs 存在但 result / DoD / Goal / deliverable coverage 字段缺失或无法对齐 | `need_assessment` | 输出结构化 assessment，逐项引用 work item evidence refs |
| 缺少用户信息、审批、权限、外部依赖或 runtime 能力配置 | blocker scope 指向 work item，且无法由 runtime 能力机械获取 | `need_governance` | 创建 decision / approval / dependency ticket，并绑定受影响 work item |
| 预算耗尽、重复失败达到治理阈值或不可恢复合同冲突 | 已无法自动推进，且不是 provider transient / context 可补齐问题 | `blocked` | 终止当前自动闭环，保留 rejected result / blocker evidence |
| 所有 required evidence kinds 齐备且 claim-to-evidence 通过 | item 状态可进入 `evidence_ready` / `satisfied` | 不再作为 primary contract focus | 进入 acceptance 或处理下一个 unresolved item |

primary focus 选择规则：

- 同一 run 同一时刻只能有一个 primary convergence state，但可以有多个 current focus item。若多个 item 对应不同缺口，按 `need_state_change`、`need_evidence`、`need_validation`、`need_assessment`、`need_governance` 的顺序选择 primary state；这是为了优先处理能改变状态或获取目标证据的缺口，而不是先做总结性 assessment。
- 若多个 unresolved item 可被同一个 state-changing 或 validation action 覆盖，可以同时进入 `currentFocus`，但 LLM 输出必须逐项声明 `coverageTargets` / `workItemIds`。runtime 不得因为一个 action 提到了多个 item 就自动关闭未满足 evidence kind 的 item。
- 若当前 latest failure 明确指向某个 work item，该 item 优先成为 primary focus；否则按 top-k 稳定顺序选择。历史失败只在仍未被 accepted evidence 覆盖时参与排序。
- `ConvergenceContract` 的 `repairActionPackage` 必须记录 primary focus id、secondary focus ids、primary state、required evidence kinds、current gap 和 allowed output。任一恢复入口若无法恢复这些字段，必须重新从 run ledger 构造，而不是从历史 LLM summary 猜测。

更新冲突与一致性规则：

- 一个 step 可以更新多个 work item，但必须为每个 item 单独记录 alignment 结果、evidence kind、quality、accepted / rejected 状态和 reason；不得用 step-level completed 批量关闭所有关联 item。
- 一个 evidenceRef 可以被多个 work item 共享，前提是该 evidence 的 declared scope、coverageTargets、workItemIds、targetRef 或 assessment item 能分别回链到每个 source。共享 evidence 不改变“一源一项”，也不允许合并 work item 状态。
- candidate evidence 在产生时默认处于 pending / candidate 状态。只有经过 capability contract、target relevance、evidence quality、claim-to-evidence alignment 和 requiredEvidenceKinds 校验后，才能进入 `acceptedEvidenceRefs`；失败或不相关证据进入 `rejectedEvidenceRefs` 并带 reason。
- 若同一 step 对某 work item 同时产生 accepted 与 rejected evidence，必须按 evidence kind 分开记录。例如 state-changing evidence accepted 但 validation rejected 时，item 只能推进到“缺 validation”，不能整体 satisfied。
- schema repair、provider retry、runtime contract recovery 产生的 wrapper evidence 默认不能关闭 work item；它们只能解释为什么上一轮输出无效。只有 wrapper recovery 后的实际 action / validation / assessment evidence 才能更新 work item closure。
- definitionRevision 变化时，successor run 必须先执行 baseline diff：相同 `normalizedSourceKey` 且 source 文本未变的 item 可以继承 accepted / rejected evidence lineage；source 文本或 profile evidence requirement 变化的 item 必须重新评估，不能直接继承 satisfied 状态。
- 旧 revision unresolved item 不得与新 revision active item 混排进同一个 `currentFocus`。若旧 item 仍有参考价值，只能放入 historical / superseded summary，并标记不再驱动 current contract。
- 并发或重复恢复路径同时更新同一 work item 时，必须按 run sequence / event sequence 应用；旧事件不得覆盖新状态。若出现状态回退，例如 `satisfied -> open`，必须有新的 rejected evidence 或 definitionRevision change 作为 source ref。
- 当 runtime 发现 ledger 与 read model、LLM-facing projection 或 result artifact 不一致时，以 run-scoped ledger 为准，并记录 consistency repair event；不得让 Web/TUI 投影反向覆盖 ledger。

回归测试矩阵要求：

| 场景 | 必须断言 |
| --- | --- |
| baseline full coverage | 当前 effective Goal leaf、DoD item、required acceptance dimension、deliverable expectation 全部生成 active / satisfied / blocked / excluded item |
| one-source-one-item | 同一 `normalizedSourceKey` 在同一 `definitionRevision` 内不会生成重复 active item；DoD 多次补充不会重复生成相同 item |
| no cross-source merge | 文本相似但 source 不同的 Goal / DoD 不会被静默合并；共享 evidence 不合并状态 |
| definition change rebuild | Goal / DoD 变更后生成新 `definitionRevision`，旧 item superseded，新 item 不继承不适用 satisfied 状态 |
| currentFocus context gate | strict convergence 存在 unresolved item 时，缺少 `currentFocus` 最小字段会 fail-fast，不提交 provider |
| context budget trimming | full ledger 超预算时仍保留 primary focus、summary、omitted counts、full ledger ref 和 protected evidence refs |
| convergence mapping | 缺 target evidence -> `need_evidence`，有 target evidence -> `need_state_change`，缺 validation -> `need_validation`，缺 assessment -> `need_assessment`，真实外部依赖 -> `need_governance` |
| output relink failure | LLM 输出没有 `coverageTargets` / `workItemIds` / evidence refs 回链时，不关闭 work item，只生成 rejected evidence |
| wrapper failure inheritance | schema invalid / provider retry / repair-of-repair 不清空 unresolved work items 和 required evidence kinds |
| evidence sharing | 一个 validation evidence 可关闭多个 item 的 validation kind，但每个 item 都必须有独立 alignment 记录 |
| revision isolation | successor run 不把旧 revision unresolved item 和新 revision active item 混入同一个 currentFocus |
| neutrality guard | 同一结构化输入下，目标文本中的语言、框架、业务关键词变化不改变 core work item 生成、状态映射或 contract state；领域语义只能影响 profile / Soul 输出 |

`coverageWorkItems` 的状态推进必须遵守以下门禁：

- work item 缺少 `state_change` evidence 时，后续 strict convergence 输入必须显式说明该 work item 仍缺少 state-changing evidence；若没有 concrete target evidence，则进入 `need_evidence`，不得直接强制 patch。
- work item 已有可接受 `state_change` evidence 但缺少 `validation` evidence 时，后续输入应进入 `need_validation` 或 bounded validation evidence repair。
- work item 已具备所有 required evidence kinds，且 claim-to-evidence alignment / evidence quality / result policy 通过时，才允许进入 `evidence_ready` 或 `satisfied`。
- work item 失败或阻塞只能作为当前 coverage scope 的治理事实；不得自动把失败文本追加到正式 DoD，也不得生成重复 DoD。
- Goal / DoD 运行期变更后，runtime 必须用新的 `definitionRevision` 重建 work item 基线：仍适用的 work item 继承 evidence lineage，已被新定义移除或替代的 work item 标记为 superseded / historical，不再驱动 repair。

`coverageWorkItems` 对 orchestrator 的影响只限于决策条件，不改变业务流转模型：

- `planning` 仍负责把当前 Goal / DoD 拆成可执行计划；work item 只是计划与 coverage ledger 的结构化桥接。
- `execution` / `repair` 仍通过现有 step / graph / tool / patch capability 产生 state-changing evidence；work item 只说明哪个 coverage target 仍缺哪类 evidence。
- `verification` 仍负责产生验证 evidence；work item 只要求验证 evidence 与目标叶子项可追踪对齐。
- `acceptance` 仍对 Goal / DoD 做最终判定；work item 全部 satisfied 是 accepted 的必要输入之一，但不能绕过 result policy、deliverable coverage 或人工治理要求。
- `replan` 仍创建 successor run 并保持 lineage；work item 可作为 successor 的 unresolved coverage scope 继承，但不得形成独立 run。

该机制保持 core 业务中立的边界：

- Core 管结构、状态、证据、顺序、一致性和准入。
- Template / profile / Soul 管领域解释、实现策略、验证方式和 evidence 产生路径。
- Core 不得根据文件名、命令名、测试框架、语言、协议、供应商、业务关键词或自然语言 summary 判断 work item 应该由“功能实现”还是“测试验证”关闭。
- 当缺口被 profile 解释为软件开发能力时，`software-dev` template 可以要求先核对现有实现、再决定补功能代码或补测试断言；这个策略必须停留在 software-dev Soul / profile contract 中，不得下沉到 core runtime。

Follow-up、repair、provider retry、replan successor 和 validation 输入必须从 coverage ledger / 等价状态视图重建上下文，至少包含未关闭 coverage targets、最近 accepted evidence、最近 rejected evidence 及原因、当前 failure surface、`repairObjective`、`stateChangingOutputRequired`、`requiredOutput`、`currentExecutionProgress` 与 `processEvidenceTimeline`。若 follow-up 输入只包含最近 step 摘要而缺少 Goal / DoD coverage contract，runtime 必须先补齐上下文或 fail-fast，不得让 LLM 基于低价值 smoke evidence 继续生成 state-changing 输出或 failed assessment。

当上述 coverage repair 因 repair 输出合同违规而再次进入 repair-of-repair，例如 patch/apply 缺少 `coverageTargets`、返回只读 graph、返回未声明 state-changing intent 或缺少 bounded verification 的 shell/tool graph，或 LLM decision JSON 无效，runtime 必须继承上一轮 `missingDoDItems`、`missingGoalCoverage`、`missingDeliverableCoverage`、候选 evidence、已完成 action evidence、当前失败信息和最近 repair 尝试摘要。`repairActionPackage` 必须携带中立 `coverageRepairRequired=true`、`coverageTargetSet`、`currentExecutionProgress` 与 `requiredOutput.mustDeclareCoverageTargets=true`。这里的 `currentExecutionProgress` 只保留 step id、kind、state、命令/patch 摘要、stdout/stderr 预览、覆盖缺口计数和失败摘要，不得复制大段 patch content、历史 raw LLM 输出或被拒绝的失败正文。该规则保证 LLM 能看到“已有内容、当前进展、遇到的问题和仍未关闭的覆盖目标”，同时避免单 JSON 膨胀和失败产物复用。

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

当输入合同包含 `repairObjective=produce_concrete_validation_evidence`、`boundedValidationRequired=true` 或 `repairActionPackage.requiredOutput.mustProduceConcreteValidationEvidence=true` 时，LLM 输出必须满足该 concrete validation evidence contract：返回受限 `executionGraph`，且 graph 至少包含一个 `shell/command`、`mcp/tool_call` 或 profile/capability 显式允许的等价验证动作；不得只返回 failed `dodAssessment`、泛化 summary、read-only graph、context rehydration、自检或新的嵌套 LLM graph。若无法产出验证证据，只能返回真实治理 blocker / approval / decision，并说明 runtime 无法自行取得的外部信息、权限、配置或依赖。普通 failed `dodAssessment` 不能替代 `requiredOutput` 要求的验证动作；只有 satisfied assessment 已提供可被 completion gate 校验的 concrete evidenceRefs 时，才可作为验收输出继续进入 acceptance 校验。否则 runtime 必须作为 stage contract violation 拒绝并进入 repair/retry 收口。

No-actionable continuation、completion follow-up、provider retry、repair-of-repair 和 replan successor 不得清空 concrete validation evidence contract。只要上一轮 step 输入仍携带 `repairObjective`、`boundedValidationRequired`、`convergenceContract`、`repairActionPackage.requiredOutput` 或 `coverageTargetSet`，下一轮 continuation 必须从上一轮输入或等价 coverage ledger 中恢复这些字段，并继续要求受限验证动作。只有 coverage target 已由可追踪证据关闭、预算耗尽进入 rejected result，或真实治理 blocker 被确认时，才允许退出该合同。

若 `dodAssessment.satisfied=true` 但 runtime 校验发现 DoD / Goal / acceptance dimension / deliverable coverage 字段缺失或不完整，该失败首先属于 assessment contract correction，而不是新的 state-changing implementation 缺口。runtime 必须先尝试以 `assessmentOnly` / `verification` 模式要求 LLM 补齐 `dodAssessment.items`、`dodAssessment.goalCoverage`、`dodAssessment.acceptanceDimensions`、`dodAssessment.deliverableCoverage` 与 evidenceRefs / artifactRefs；不得在没有新的实现缺口证据时强制重新生成 patch、shell 或其他 state-changing 工作。若补齐后的 coverage 缺口仍没有任何中立候选证据，也不能从已有 satisfied evidence 机械映射，runtime 应切换为 concrete validation evidence repair，要求 LLM 产生受限 verification evidence 或明确 failed assessment / blocker。

如果 assessment-only coverage correction 仍返回 `dodAssessment.satisfied=true` 但继续缺失 DoD / Goal / acceptance dimension / deliverable coverage，runtime 不得立即把 run 终结为 rejected result；应在 runtime repair budget 内继续派发 assessment-only correction，并把最新 missingDoDItems / missingGoalCoverage / missingDeliverableCoverage 作为强约束输入。只有 coverage correction 预算耗尽、LLM 返回明确 unsatisfied assessment、或出现不可恢复 contract violation 时，才进入 rejected / decision / fail 路径。

如果 assessment-only coverage correction / verification assessment 的失败是 `llm_schema_invalid` / `llm_decision_invalid`，该失败仍属于 LLM decision JSON 输出合同问题，而不是新的实现缺口或不可恢复终态。runtime 必须在 repair budget 内继续派发 assessment-only schema recovery：保留 `runtimeStage=verification`、`assessmentOnly=true`、`repairObjective=complete_dod_goal_coverage_assessment`、`convergenceContract.state=need_assessment` 以及最新 missingDoDItems / missingGoalCoverage / missingDeliverableCoverage / requiredDeliverableCoverage；同时设置 `schemaInvalidRepairMode=single_valid_json_decision` 或等价中立约束，要求 LLM 只返回一个合法 JSON object，内容仅能是 completed/failed `dodAssessment` 或真实 governance blocker。该恢复不得引入 `executionGraph`、shell、patch、MCP、memory write 或其他可执行工作，也不得复用 rejected rawContentPreview / repairRawContentPreview。只有 schema recovery 预算耗尽或后续 assessment 明确无法满足，才进入 rejected / decision / fail 路径。

如果 runtime repair follow-up 返回的 `dodAssessment.satisfied=true` 仍因 evidence quality 或 claim alignment 被拒绝，runtime 同样不得直接终结为 rejected result。该情况说明“实现或通用验证可能已完成，但 accepted claim 尚缺少可追踪、对齐的证据表达”，必须在预算内继续派发 concrete validation evidence repair，要求 LLM 产出带 `coverageTargets`、明确验证 scope、可追踪 evidence text 或 profile 允许的等价证据。该规则只约束证据链质量，不要求 core 理解具体业务语义。

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

State-changing repair 必须有 concrete implementation target evidence 才能要求 LLM 输出 `patch/apply`。Runtime 在设置 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output`、`repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true` 或 `acceptedKinds=["patch/apply"]` 之前，必须确认至少存在一个可追踪实现目标证据：`failureTargetRepairContext`、`patchRepairContext`、`repairActionPackage.targetSnapshots`、可解析到 workspace manifest 的 `repairTargetSet` / `structuredFailureSignals.targetRefs` / `lineRefs`，或带当前快照的 `rootCauseCandidateContext`。`coverageTargetSet` 只能说明“哪些 Goal / DoD / deliverable 仍未关闭”，不能单独证明应该修改哪个 workspace 资源；只有当它与上述实现目标证据同时存在时，才可作为 `patch/apply.coverageTargets` 的匹配范围。如果这些实现目标证据都不存在，runtime 不得把“必须修复状态”合同发送给 LLM；否则会诱导无关 marker、报告、README 或其他形式性 patch。

当 replan、repair-of-repair、schema invalid recovery 或 runtime contract recovery 缺少 concrete repair target evidence 时，runtime 必须进入中立 `acquire_concrete_failure_evidence` 模式。该模式允许有界 `file/list`、`file/read`、`shell/command`、`mcp/tool_call` 或等价能力来获取当前 workspace 的真实失败面、target refs、line refs、stdout/stderr、exit code 和当前快照；不得要求 `patch/apply`，不得接受无边界探索、自检、run snapshot、context rehydration 或新的 LLM 嵌套。若 evidence acquisition 产出确定性失败，再由该失败证据触发 target-scoped state-changing repair。

`acquire_concrete_failure_evidence` 是 state-changing repair 的前置取证阶段，不是新的 verification 终态。取证 graph 完成后，若 graph evidence 中已经包含可追踪 target refs、line refs、manifest/file-list 资源引用、当前资源快照、结构化诊断或等价 concrete evidence，runtime 必须恢复原 repair lineage 与 state-changing repair 合同：`runtimeStage=repair`、`runtimeStageMode=strict_convergence`、`repairObjective=produce_state_changing_output`、`stateChangingOutputRequired=true`，并把原 `repairActionPackage` 中的 failure signature、target scope、structuredFailureSignals、previous rejected summary 与新获得的 target snapshots / target refs / line refs 合并为下一轮 primary actionable contract。不得因为取证 graph 本身成功完成，就把 follow-up 降级为普通 verification，也不得把 `requiredOutput` 置空。

取证 graph 的 follow-up 生成必须以 graph root 的 evidence-acquisition repair 输入作为合同源，而不是只读取最后一个 graph node 的输入。普通 `file/list`、`file/read`、`shell/command` 节点通常不携带原始 `repairActionPackage`；如果 follow-up 只继承最后节点，会丢失原始 repair lineage 并退化为 `verification`。runtime 应从 graph root 恢复原始取证合同，同时把各已完成 graph node 的输出作为 concrete evidence 汇总。可升级为 repair target 的证据仅限通过中立准入的 workspace-relative 路径、line-ref、manifest/file-list entry、当前快照或 capability output；不得根据语言、框架、测试文件名、业务字段名或领域语义做判断。

若 evidence acquisition 完成后仍然没有 concrete target evidence，runtime 不得伪造 state-changing contract；应继续要求有界取证、返回 failed assessment/blocker，或进入真实治理决策。该判断只基于路径/line-ref、manifest、当前快照、failure signature、输出合同和 capability evidence，不得基于语言、框架、测试文件名或具体业务语义。

当 LLM 在 state-changing repair 阶段返回 `blocked_on_decision` / `llm_decision_required`，runtime 必须先判断该阻塞是否可由已有中立证据机械推进。若当前输入已经同时具备：state-changing repair 合同、`repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true`、允许 `patch/apply`、concrete target evidence（failureTargetRepairContext / patchRepairContext / targetSnapshots / repairTargetSet / structured target refs）以及 bounded verification 约束，且 decision context 未声明外部依赖、runtime 配置、上下文门禁、审批或 rejected-result 治理策略，则不得直接创建人工 ticket；runtime 应把它作为 continuable repair contract violation，记录 converted decision 摘要并进入下一轮 repair/replan。只有缺少目标证据、缺少必要外部信息、涉及真实外部依赖/权限/审批、或输出合同不允许 runtime 自行继续时，才允许创建 decision ticket。

Replan continuation 不得只因为 predecessor run 的 execution contract 要求 state change 就直接派发 state-changing repair。它必须先从 predecessor lineage 恢复最近未解决的 concrete failure target、target snapshot、contentHash、coverage target 或 patch repair context；若只能恢复到 LLM schema/runtime contract wrapper failure，例如 `llm_schema_invalid`、`no json object found`、`executionGraph without patch/apply` 或 `coverageTargets missing`，则这些 wrapper failure 只能作为 rejected output / schema recovery evidence，不能替代原始未解决的 workspace 失败面。找不到原始目标证据时，replan successor 必须进入 `acquire_concrete_failure_evidence`。

本约束的实现收口必须覆盖以下检查点：

- Repair action package 统一由 required output contract 决定是否允许 state-changing output；当 `mustIncludeStateChangingNode=false`，任何后续证据门禁不得再把它提升回 patch，例如 `currentTargetAlreadySatisfiesFailedPatch` 必须保持 concrete validation evidence contract。
- 缺少 concrete repair target evidence 时，runtime 必须移除顶层 `stateChangingOutputRequired`，设置 `repairObjective=acquire_concrete_failure_evidence` 与 `concreteFailureEvidenceRequired=true`，并把 `repairActionPackage.requiredOutput.acceptedKinds` 限定为 evidence acquisition 能力。
- Replan continuation 必须把 predecessor failure context 中可机械解析的 `failureTargetScope`、`targetRef`、target snapshot 写入 `repairActionPackage`，并把 coverage target 仅作为验收 scope 写入；只有该 package 确认可追踪实现目标证据存在时，才能保持 state-changing repair。
- Runtime contract wrapper failure、schema invalid、`patch/apply targetRef is required`、`executionGraph without patch/apply` 等只描述输出合同的失败，不能单独作为 workspace repair target；若无法恢复原始 target context，必须先获取证据。
- 无论进入 state-changing repair 还是 evidence acquisition，`repairActionPackage` 都是 primary actionable contract；runtime 应压缩重复的 `failedStepOutput`、`previousStepOutput`、`failedExecutionGraph` 等历史 payload，只保留被 package 摘要化后的中立事实，避免 LLM 上下文再次膨胀和冲突。
- 测试必须同时覆盖“无实现目标证据进入 evidence acquisition / validation evidence”和“有 target refs / target snapshots / patch repair context 时保持 patch contract”两类路径；还必须覆盖“只有 coverageTargetSet 时不得保持 patch-only contract”，避免后续修改重新回到按失败类别一刀切。

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

`shell/command` 的工作目录属于 runtime capability contract，而不是业务语义。若 capability 已声明其工作目录由 runtime 绑定到 workspace 交付目录，则 LLM 不得再硬编码 `cd ../../../../../user/workspace`、`.workorch-workspaces/...` 或等价的绝对 workspace 路径；core 可以把这类输出视为 capability-boundary violation 并拒绝，因为它破坏的是通用执行能力合同，而不是某个具体业务规则。

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
| 具体软件开发实现流程、测试命令、代码 patch 规则 | 属于 software-dev profile，不属于 core runtime | `../40-profiles-capabilities/software-dev/` |
| Web/TUI/API 交互细节 | 属于产品交互和服务接口，不是 runtime stage contract | `../50-product-interface/` |
| 数据库存储路径、file backend layout 细节 | 属于持久化实现，不是阶段合约 | `../30-module-architecture/storage-readmodel.md` |
| 已完成任务流水和日期记录 | 只用于追溯，不应污染权威设计 | `../90-archive/` / `../60-verification/audits/` |
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
| P2 Coverage Scope | missingDoDItems、missingGoalCoverage、missingDeliverableCoverage、coverageTargetSet、coverageWorkItemContext、evidenceCandidates | 不得因 contract violation 被清空；strict convergence 至少保留 summary + currentFocus |
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

LLM request 的缓存友好组织仍属于 core 合同的从属规则，但具体 message/section
组织、hash、cachePlan、provider-neutral cache hint、request-log 校验和实现落点，
后续统一由以下文档承接：

- `runtime-request-cache-note.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`

本文在 core 层只保留以下不可降级约束：

- request 必须按稳定性分区组织，且稳定合同不得被当前失败、当前进展和动态 provider 绑定污染。
- Goal / DoD 必须完整位于 definition block，不能为了缓存复用被裁剪、摘要或移入易变尾部。
- capability contract 必须保持声明式稳定，assessment-only 或 repair mode 不得改写能力目录本身。
- 当前失败、当前进展、latest validation、memory recall、requested action 等高频变化信息必须进入易变尾部。
- provider 私有 cache-control 只能由 adapter 消费，不能进入业务恢复状态。

### Stage Input Bundle 合同

stage input bundle 的详细 section 结构、输入组装顺序和 request-side 表达，
后续由以下文档承接：

- `../20-runtime-flow/context-assembly-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`

本文在 core 层只保留以下边界：

- Goal / DoD 不可被压缩、改写或用摘要替代。
- memory recall 只应进入高价值决策信息，不得退化为完整运行流水。
- skill/tool/mcp/soul/prompt 进入 bundle 时只能提供能力摘要和结构化约束。
- repair / replan bundle 必须保留 failure scope、evidence、target snapshots / manifest 和必须继承的 contract。
- profile-specific bundle 字段必须保留在 profile namespace，不能提升为 core 字段。

### Codex 启发的中立过程证据合同

Core 在此只保留三条不可降级约束：

- 仅吸收“上下文组织机制”，不吸收任何 Codex 私有 schema、隐藏推理、工具命名或软件开发专用提示词。
- strict convergence、repair、replan continuation 输入必须同时具备 `processEvidenceTimeline` 和 `currentProgressDigest`，且都可追溯到 source refs。
- 过程证据必须按时间/序列稳定排序；历史 superseded/rejected/provider 动态绑定不得覆盖当前 failure surface。

`processEvidenceTimeline` 的结构细节、压缩规则、request 投影与实现映射，统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`
- `../60-verification/core-runtime-verifier.md`

## Runtime Context Enforcement Contract

上下文完整性不能只依赖 prompt 文案。Runtime 在向 LLM provider 提交 request 前，必须先对 stage input bundle 执行中立准入校验，并把校验结果写入 request audit / step output。该合同只校验结构、引用、完整性和阶段约束，不判断任何具体业务语义。

### Context Bundle And Gate Boundary

Core 只要求：

- 每次 provider 调用前，输入都必须被归一化为稳定的 context bundle section
- 所有保护字段都必须保留 source refs / omitted reason，不能留空后让 LLM 猜测
- Runtime 必须先执行 context gate，再决定 provider submit、read、blocker、fail-fast 或 replan
- `contextBudget.integrityWarnings` 若涉及 P0-P4 必要字段，必须升级为 gate action，不能只留作观测日志
- `llm_context_integrity_failed` 属于 provider 前准入失败，不得进入 provider retry、普通 runtime repair 或 state-changing repair
- context gate 恢复必须是结构化恢复链，不能靠普通 repair prompt 间接推断

`Context Bundle` 的 section 投影、`Context Gate` 动作表、canonical failure hydration、
coverage target auto-rebuild、字段级保护与 request audit 细节，统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../20-runtime-flow/repair-replan-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`
- `../60-verification/core-runtime-verifier.md`

### Stage-Specific Required Context

Core 只保留一条边界：不同 stage 必须具备不同的最小必要上下文，缺失时只能走
`read_required`、`degrade_to_assessment`、`blocker_required`、`fail_fast`
或 `replan_required` 的中立处置，而不能让 LLM 伪装成继续严格收敛。

按 stage 的最小上下文基线和缺失处置表统一转入：

- `../20-runtime-flow/context-assembly-flow.md`

### Implementation Mapping

Core 只要求“每条合同约束都能映射到唯一实现责任点”。
实现映射统一转入：

- `../30-module-architecture/overview.md`
- `../30-module-architecture/kernel-orchestrator-governance.md`
- `../30-module-architecture/llm-provider-assembly.md`

### Non-Core Landing Boundary

为避免 core 文档继续吸纳流程细节和实现枚举，以下内容必须优先下沉，不再在本文扩展为更细实现说明：

| 内容族 | core 只保留 | 应下沉到 |
| --- | --- | --- |
| context bundle / selected context / compression | 保护等级、阶段必需字段、中立 gate | `../20-runtime-flow/context-assembly-flow.md` |
| provider request rendering / cache-aware request / request-log ref | provider-neutral contract 与 protected audit fields | `../30-module-architecture/llm-provider-assembly.md` / `../00-core/runtime-request-cache-note.md` |
| coverage ledger / work item / claim-evidence 投影 | 中立约束与不变量 | `../20-runtime-flow/verification-acceptance-flow.md` / `../30-module-architecture/storage-readmodel.md` |
| repair / replan / governance 恢复动作表 | 中立恢复原则与 blocker gate | `../20-runtime-flow/repair-replan-flow.md` |
| result / artifact / raw payload / manifest 打开链路 | 接受/拒绝/证据闭环合同 | `../20-runtime-flow/evidence-result-flow.md` / `../50-product-interface/service-api.md` |
| request-scoped session / bootstrap / repository / route wiring | 唯一责任点要求 | `../30-module-architecture/application-workorchd.md` / `../30-module-architecture/repository-structure.md` |

当前若某条补充更像“端点、字段、流程步骤、文件布局、脚本入口、枚举清单”，
默认应先迁到 flow / module / product / verification 文档，而不是继续加到 core contract。

### Core / Profile Boundary

Core 只处理中立结构字段（stage/contract/evidence/ref/budget/lineage/capability kind），不处理任何业务语义推断。领域映射、术语、验证习惯和交付规范必须留在 profile/template/soul/skill 层。

边界落点详见：

- `../10-domain-model/context-capability-memory.md`
- `../40-profiles-capabilities/overview.md`
- `../40-profiles-capabilities/software-dev/code-agent.md`

## Further Entry Points

除 core 合同外，以下入口分别承接：

- verifier / acceptance
  - `../60-verification/core-runtime-verifier.md`
  - `../60-verification/profile-software-dev-verifier.md`
  - `../60-verification/service-acceptance.md`
- roadmap / gap tracking
  - `../70-roadmap/active/overview.md`
  - `../01-overview/incomplete-and-connection-gap-tracker.md`
- dated closure history
  - `../90-archive/completed/core-runtime-closure-history.md`

## 非兼容性说明

当前阶段以彻底收口 WorkOrch 能力为目标，不优先保证旧行为兼容。
允许为满足当前合同而做破坏性收口，但必须满足两条边界：

- 破坏性调整只能服务当前中立合同，不得把历史兼容逻辑继续回写进 core
- 即使不考虑兼容，也不得破坏核心中立原则

典型历史不兼容收口已涉及：

- `soulStage` -> `runtimeStage` 的显式化
- memory / run-summary 历史兼容标记清理
- 自然语言关键词驱动的 decision / repair 兼容推断移除
- runtime file-backend 命名与 raw payload ref 形态替换

更细 dated closure 说明统一转入：

- `../90-archive/completed/core-runtime-closure-history.md`

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

Core 只保留以下边界：

- Decision / Approval 是 runtime 治理入口，但不承载具体业务领域语义
- queue / resume / replan continuation 必须基于可追踪的时间戳、checkpoint、
  latest ticket/failure context 和最小过程证据恢复
- successor / continuation 只能继承诊断摘要与结构化恢复事实，不能把旧的 raw
  execution payload 直接回灌到下一轮 prompt

更细的治理队列排序、resume 语义、replan continuation 继承与 context 恢复规则统一转入：

- `../20-runtime-flow/repair-replan-flow.md`
- `../30-module-architecture/kernel-orchestrator-governance.md`
- `../30-module-architecture/storage-readmodel.md`

## LLM Context Admission Contract

LLM 输入必须是当前阶段可行动、可验证、无冲突的任务合同。runtime 可以传递 Goal / DoD、当前进展、失败证据、修复目标、历史摘要和 memory，但必须在发送前完成中立准入，不得把运行态噪点、历史失败产物或本地绝对路径包装成业务修复目标。

### Repair Target Admission

Core 只保留边界：repair target 必须是 workspace deliverable 下、已通过准入且可行动的
workspace-relative 资源；若没有 concrete target snapshot，state-changing repair 不能继续。

更细准入规则、诊断路径与 evidence acquisition 分流统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../20-runtime-flow/repair-replan-flow.md`

### Context Semantic Layers

Core 只保留边界：LLM-facing context 必须区分 Goal/DoD、当前进展、过程证据、
当前失败、可行动 repair contract 与历史参考信息，且历史层不得覆盖当前事实。

更细层级投影与 section 组织统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../30-module-architecture/llm-provider-assembly.md`

### Memory Recall Admission

Core 只保留边界：memory 是参考知识，不是运行流水；selected/excluded 一致性必须成立；
瞬态 wrapper / retry / loop 失败不得提升为可复用 memory。

更细 selected/excluded 规则与 recall 注入策略统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../10-domain-model/context-capability-memory.md`

### Semantic Context Gate

Core 只保留边界：发送 LLM 前必须执行语义门禁。若 Goal/DoD 不匹配、repair target
未准入、memory selected/excluded 不一致、当前过程证据不可行动，或“必须 patch”
与“目标不可行动”同时成立，则只能 fail-fast、read-required 或转入 evidence acquisition。

更细触发条件与恢复动作统一转入：

- `../20-runtime-flow/context-assembly-flow.md`
- `../20-runtime-flow/repair-replan-flow.md`

## 核对清单

后续每次收口前至少按以下审计族核对：

1. neutrality / profile boundary
2. stage / context / repair contract
3. closure / result / evidence gate
4. coverage ledger / convergence mapping
5. projection / read-model / payload boundary

对应正式核对入口：

- `../60-verification/core-runtime-verifier.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../20-runtime-flow/repair-replan-flow.md`
- `../20-runtime-flow/verification-acceptance-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`
- `../30-module-architecture/storage-readmodel.md`

## CWI Active Entry

`coverageWorkItems` 的当前稳定规则定义已经迁入：

- `../20-runtime-flow/verification-acceptance-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`
- `../30-module-architecture/storage-readmodel.md`
- `../60-verification/core-runtime-verifier.md`

按日期整理的阶段性实现与回归收口历史已迁出至：

- `../90-archive/records/coverage-work-item-closure-history.md`
