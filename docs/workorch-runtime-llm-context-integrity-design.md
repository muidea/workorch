# WorkOrch Runtime LLM Context Integrity Design

## 背景

近期 `ws-5823c555519e601c` 等长流程运行暴露了一个核心问题：WorkOrch 能持续调度 step、graph 和 repair，但传递给 LLM 的上下文并不总能稳定表达“实际已经完成什么、当前处理到哪里、遇到什么错误、仍缺哪些 Goal / DoD / deliverable 覆盖”。当 repair-of-repair 的当前失败只是输出合同错误，例如缺少 `coverageTargets`、返回只读 graph、schema invalid JSON，后续输入容易只保留外壳错误，丢失原始 coverage gap 和已有执行证据，导致 LLM 继续扩图或泛化探索，无法收敛。

本设计定义 WorkOrch runtime 向 LLM 传递上下文的完整性合约。它不是承诺“所有未来场景都绝对不会缺信息”，而是把必须稳定传递、必须排序、必须避免误脱敏、必须可追溯的内容形式化，作为后续实现和验收的核心依据。

本设计与以下文档共同构成上下文治理体系：

- `docs/workorch-runtime-stage-contract-neutrality-design.md`
- `docs/workorch-runtime-core-flow-contract.md`
- `docs/codex-like-llm-prompt-context-design.md`
- `docs/workorch-codex-request-context-comparison.md`
- `docs/model-aware-llm-context-budget-design.md`
- `docs/workorch-llm-auto-context-profile-design.md`
- `docs/workorch-runtime-neutral-repair-grounding-design.md`

## 设计目标

- 保证 LLM 至少获得当前决策所需的中立必要上下文，而不是只获得最近一个外壳错误。
- 保证 Goal / DoD / deliverable coverage 目标、当前失败、已完成 action evidence、当前进展和最近 repair 尝试在 repair 链路中不丢失。
- 保证上下文顺序可解释、可复现、可审计，避免历史失败覆盖最新成功证据。
- 保证 redaction / compaction 不把可用于修复的当前目标快照替换成不可用内容，也不把 rejected raw content 重新喂给 LLM。
- 保证上下文预算内优先保留 P0 / P1 必要信息；若必要信息超预算，应进入治理或缩容策略，而不是静默裁剪。
- 保持 WorkOrch core runtime 功能中立，只表达 stage、failure、coverage、evidence、resource snapshot、capability contract 等抽象事实，不写入软件开发、语言、框架、供应商或具体业务语义。

## 非目标

- 不要求每次 LLM 调用都携带完整历史明文。
- 不把 Codex 私有请求结构复制为 WorkOrch 内部模型。
- 不把 tool / shell / patch 输出原文无限注入 prompt。
- 不在 core runtime 中判断某个具体业务能力应该如何实现。
- 不绕过 context budget、secret redaction 或 rejected output 隔离。

## 核心判断

当前修复已经覆盖一个明确缺陷：coverage repair 的 repair-of-repair 不再只传递“输出合同错误”，而是携带 coverage targets、current execution progress、候选证据和最近 repair 摘要。

但仅凭该修复仍不能宣称上下文体系完全完备。仍需补齐：

- 明确的 step / event 顺序合同。
- 上下文完整性审计与 request trace。
- redaction / compaction 豁免的一致性验证。
- 单 JSON 膨胀时的分层引用和按需取证机制。
- 部署态真实 request.json 核对和回归验证。

## 上下文完整性原则

### 1. 当前任务优先

LLM 输入必须首先回答四个问题：

- 当前目标是什么：workspace、run、stage、step、Goal、DoD、deliverable expectation。
- 已经完成什么：最新 completed action evidence、patch evidence、validation evidence、artifact refs。
- 当前失败是什么：failedStepId、failureClass、failureSignature、stage、error、reason、diagnostics。
- 下一步必须做什么：repairObjective、requiredOutput、forbiddenActions、coverageTargetSet、exitCriteria。

历史信息只有在能解释当前任务、避免重复失败或补足决策约束时才进入高优先级上下文。

### 2. 外壳失败不清空原始语义

repair-of-repair 的当前失败可能只是 LLM 输出合同错误，例如：

- patch/apply 缺少 `coverageTargets`。
- 返回 read-only / shell-only / self-inspection graph。
- 返回 malformed JSON。
- patch hunk 格式错误。

这些失败不能覆盖或清空上一轮原始缺口。后续 repair input 必须继承：

- `missingDoDItems`
- `missingGoalCoverage`
- `missingDeliverableCoverage`
- `evidenceCandidates`
- `failureTargetRepairContext`
- `patchRepairContext`
- `workspaceFileManifest`
- `currentExecutionProgress`
- `recentRepairAttempts`
- state-changing / evidence repair / assessment-only 等阶段约束

### 3. 最新有效证据优先

对于同一中立 scope，如果历史失败之后已经出现最新成功 action evidence，则历史失败只能作为 lineage 或避免重复策略的背景，不得继续主导当前判断。

中立 scope 可以来自：

- step id / graph node id / graph root step id
- targetRef / failureTargetScope
- coverage target token
- failure class / signature
- deliverable expectation id

### 4. 摘要必须可追溯

任何被摘要、裁剪、脱敏、引用化的内容都必须保留 source refs：

- workspaceId
- runId
- stepId
- graphNodeId / graphRootStepId
- artifactRef / payloadRef
- targetRef
- contentHash
- omitted reason
- redaction reason
- compaction rule

LLM 可以不看到完整原文，但 runtime 和调试者必须能通过 refs 找回原始证据。

### 5. Redaction 不能伪造可用快照

如果一个当前目标快照被标记为可用于完整 patch：

- `contentComplete=true`
- `safeForFileWrite=true`
- `redacted=false`
- `truncated=false`
- `isBinary=false` 或未声明
- 存在字面 `contentHash`

则 provider prompt normalization / redaction / context budget 不得把其 `content` 替换为 `[REDACTED]`、省略片段或截断内容后仍让 LLM 误以为这是完整快照。

如果快照无法完整提供，必须显式标记不可用于完整 `file_write`，要求 LLM 使用当前可见 diff、重新读取目标，或返回 blocker / failed assessment。

### 6. Rejected 内容不可复用

以下内容不得作为可复制上下文进入后续 LLM 输入：

- malformed JSON raw output
- rejected raw LLM decision
- failed patch body / stale diff / stale oldText
- stream partial output
- 包含 redaction marker 的文本
- 被截断后可能误导 patch 的内容

只允许保留中立摘要：stage、failureClass、error/reason、streamDiagnostics、rejected graph 摘要、patch target metadata、omitted reason。

## 上下文分层

| 层级 | 内容 | 压缩规则 |
| --- | --- | --- |
| P0 Contract | runtime contract、输出 schema、workspace/run/stage/step identity、当前 Goal / DoD、当前动作要求 | 不得静默删除；超预算进入治理或更大模型策略 |
| P1 Current Failure | failedStepId、failureClass、failureSignature、stage、error、runtimeDiagnostics、repairObjective、requiredOutput | 不得丢失关键字段；只压缩长文本 |
| P2 Coverage Scope | missingDoDItems、missingGoalCoverage、missingDeliverableCoverage、coverageTargetSet、evidenceCandidates | 不得被外壳失败清空；可摘要候选证据输出 |
| P3 Current Evidence | latest action evidence、patch evidence、validation evidence、artifact refs、currentExecutionProgress | 保留 step/ref/摘要；大 stdout/stderr 使用 head/tail/preview + ref |
| P4 Current Snapshots | failureTargetRepairContext、patchRepairContext、targetSnapshots、workspaceFileManifest | 小目标完整；大目标窗口化或 ref；安全完整快照不得误脱敏 |
| P5 Recent Attempts | 最近 repair attempts、invalid strategy、duplicate failure、forbiddenActions | top-k 摘要；保留失败原因和避免重复策略 |
| P6 Historical Context | superseded runs、早期 memory、旧 governance、旧 graph evidence | 默认摘要或引用；不得覆盖 P0-P5 |
| P7 Raw Archives | response.sse、full tool output、完整 payload、历史 raw artifacts | 默认 ref only |

## 当前执行进展包

`currentExecutionProgress` 是传给 LLM 的中立进展摘要，用于避免 LLM 只看到局部失败。

建议字段：

```json
{
  "runId": "run-...",
  "failedStepId": "run-.../repair-...",
  "failedStepKind": "llm/workspace_execution",
  "failedStepState": "failed",
  "failureClass": "runtime_failed",
  "failureStage": "llm_execution_graph_plan",
  "failureReason": "coverageTargets missing",
  "coverageGapCount": 2,
  "unresolvedCoverageTargets": [
    {
      "kind": "dod",
      "targetText": "original DoD item text",
      "coverageTargetToken": "original DoD item text",
      "state": "failed",
      "evidenceRefs": ["run-.../graph-002/002-run-tests"]
    }
  ],
  "latestActionEvidence": [
    {
      "stepId": "run-.../graph-002/002-run-tests",
      "graphNodeId": "run-tests",
      "kind": "shell/command",
      "state": "completed",
      "outputSummary": {
        "command": "test",
        "exitCode": 0,
        "stdoutPreview": "ok ..."
      }
    }
  ],
  "recentRepairAttempts": [
    {
      "stepId": "run-.../repair-001",
      "state": "failed",
      "attempt": 1,
      "failureClass": "runtime_failed",
      "failureSummary": "patch/apply must declare coverageTargets"
    }
  ]
}
```

约束：

- 不包含大段 patch content。
- 不包含 rejected raw LLM output。
- 不包含完整 stream。
- 不包含 secret 原文。
- 必须保留 step refs 和 target refs。

## Coverage Repair 上下文合同

当 repair input 包含 `missingDoDItems`、`missingGoalCoverage` 或 `missingDeliverableCoverage`，并且当前 repair 需要 state-changing output 时：

- `repairActionPackage.coverageRepairRequired=true`
- `repairActionPackage.coverageTargetSet` 必须列出所有未关闭目标。
- `repairActionPackage.requiredOutput.mustDeclareCoverageTargets=true`
- patch/apply 节点必须声明 `coverageTargets`。
- `coverageTargets` 必须匹配 DoD item text、Goal `goalRequirementId`、Goal raw requirement、deliverable item / expectation id。
- 如果 patch 无法绑定具体 coverage target，LLM 应返回 failed `dodAssessment` 或 blocker，而不是生成泛化骨架。

repair-of-repair 不能因为当前失败是 contract violation 而删除上述字段。

## 顺序合同

LLM 输入中的 step / event / evidence 顺序必须稳定：

1. 同一 run 内按发生顺序排序，优先使用 monotonic sequence / event sequence。
2. 若没有 sequence，使用 occurredAt / createdAt。
3. 若时间缺失，使用 step id 中的 graph / node / attempt 编号。
4. repair attempt 按 `runtimeRepairAttempt` 升序。
5. 同一 scope 中，最新 successful action evidence 优先于旧 failed evidence。
6. superseded run 只能进入历史层，不得排在 active run 当前证据之前。

待实现时应新增明确字段：

- `sequence`
- `eventSequence`
- `stepSequence`
- `graphSequence`
- `repairAttempt`
- `evidenceOrdinal`

## Redaction 与 Compaction 合同

### 必须保留语义的字段

- Goal / DoD 原文。
- 当前失败的 class / signature / stage / error / reason。
- `coverageTargetSet`。
- `requiredOutput`。
- 当前 target snapshot 的安全元数据和必要内容。
- 当前 step refs、artifact refs、content hash。

### 可以摘要的字段

- stdout / stderr。
- 历史 governance。
- 历史 memory。
- superseded run 证据。
- rejected executionGraph。
- 大 workspace manifest。

### 必须 ref-only 的字段

- response.sse。
- raw stream delta。
- rejected raw LLM output。
- full tool output。
- 大 patch content。
- secret 原文。

## LLM 输入审计

每次 LLM 调用应能形成上下文审计记录：

```json
{
  "workspaceId": "ws-...",
  "runId": "run-...",
  "stepId": "run-.../repair-...",
  "stage": "repair",
  "contextProfile": "...",
  "contextBudget": {
    "maxTokens": 28672,
    "actualTokens": 18000,
    "truncated": false,
    "shrinkLevel": 1
  },
  "includedSections": ["p0", "p1", "p2", "p3"],
  "omittedSections": ["p6.rawHistory", "p7.rawStream"],
  "protectedFields": ["goal", "dod", "repairActionPackage.coverageTargetSet"],
  "redactedFields": ["secretRef"],
  "sourceRefs": ["input.executionContext", "step.graph-002.002", "artifact://..."],
  "integrityWarnings": []
}
```

如果出现以下情况，必须写入 `integrityWarnings` 或阻断：

- P0 / P1 必要字段缺失。
- active run 当前 evidence 被 superseded run 覆盖。
- `coverageRepairRequired=true` 但缺少 `coverageTargetSet`。
- `mustDeclareCoverageTargets=true` 但缺少目标 token。
- target snapshot 标记完整但 content 被 redacted / truncated。
- rejected raw content 被重新注入。
- contextBudget.truncated=true 且 P0-P2 有字段被裁剪。

## 验收标准

### 单元测试

- repair-of-repair 继承 coverage gaps、candidate evidence、current progress。
- `repairActionPackage` 包含 `coverageTargetSet`、`currentExecutionProgress` 和 `mustDeclareCoverageTargets`。
- current progress 不泄漏大 patch content / rejected raw output。
- Goal coverage 只有 raw / requirement 时，coverageTargets 校验可匹配。
- target snapshot 完整豁免不被 redaction 替换。
- context ordering 按 sequence / time / step id 稳定排序。

### 集成测试

- 构造一个 failed coverage assessment -> state-changing repair -> contract violation -> repair-of-repair 场景，核对 request.json。
- request.json 必须包含当前未关闭 coverage targets、最新 completed action evidence、当前 failure summary 和最近 repair attempts。
- request.json 不应包含 rejected raw malformed JSON、大段 patch content 或 stream partial。
- 若 contextBudget.truncated=true，P0-P2 必须仍完整。

### 部署态核对

- 对真实 workspace 运行后抽查 `llm-calls/<run>/<step>/request.json`。
- 核对 active run 的最新 evidence 是否排在 superseded 历史之前。
- 核对 redaction 是否只影响 secret / raw rejected / raw stream，不影响当前安全 target snapshot。
- 核对 LLM 输出是否减少泛化扩图，能够围绕 coverageTargetSet 收敛。

## 待办跟踪

| ID | 优先级 | 状态 | 内容 | 验收 |
| --- | --- | --- | --- | --- |
| LCI-01 | P0 | Done | Coverage repair-of-repair 继承 `missingDoDItems` / `missingGoalCoverage` / `missingDeliverableCoverage` | 单测覆盖继承行为 |
| LCI-02 | P0 | Done | `repairActionPackage` 增加 `coverageRepairRequired`、`coverageTargetSet`、`currentExecutionProgress`、`mustDeclareCoverageTargets` | 单测覆盖 action package |
| LCI-03 | P0 | Done | current progress 摘要避免复制大 patch content / rejected raw output | 单测断言不泄漏 patch body |
| LCI-04 | P0 | Done | Goal coverage raw / requirement 参与 coverageTargets 校验 | 单测覆盖 raw target |
| LCI-05 | P0 | Done | 明确 step / event / evidence sequence 字段与排序实现 | 已新增 runtime context step ordering；优先使用 `sequence` / `eventSequence` / `stepSequence` / `graphSequence` / `evidenceOrdinal`，再按 readable step id 顺序兜底；单测覆盖乱序输入稳定输出 |
| LCI-06 | P0 | Done | LLM request context integrity audit trace | `contextBudget` 已输出 `protectedFields` / `integrityWarnings`，并继续保留 included / omitted / redacted / source refs；单测覆盖 protected fields 与 warning |
| LCI-07 | P0 | Done | Redaction / compaction 保护当前安全 target snapshot | 安全 repair snapshot 继续在 context budget shrink 后恢复；缺失 protected snapshot / coverage target 会进入 integrity warning；单测覆盖安全快照不被误替换 |
| LCI-08 | P1 | Done | P0-P2 超预算阻断或治理策略 | `contextBudget.overBudget=true` 时 runtime 在 provider 调用前 fail-fast；`repairActionPackage.requiredOutput` / `coverageTargetSet` / `currentExecutionProgress` 在无 target snapshot 时仍受保护；单测覆盖 |
| LCI-09 | P1 | Done | 分层引用和按需取证机制 | `runEvidenceSummary` / `graphEvidence` 增加 `outputRef`，大 stdout/stderr 使用 head/tail targeted excerpt 并保留 bytes/truncated；单测覆盖 |
| LCI-10 | P1 | Done | 部署态 e2e request.json 核对脚本 | 新增 `scripts/verify-workorch-llm-context-integrity.sh`，自动检查 coverageTargetSet/currentExecutionProgress、contextBudget 审计字段、Goal/DoD 压缩风险、rejected raw/stream/redaction marker 泄漏 |

## 设计结论

WorkOrch 不应追求把所有历史原文都塞给 LLM，也不能只给 LLM 一个最近失败摘要。正确边界是：对当前决策必需的 Goal / DoD / coverage / failure / progress / target snapshot 必须完整、稳定、有序、可追溯；历史和大文本必须摘要化、引用化、可按需恢复；被拒绝和敏感内容必须隔离。

该设计保持 core runtime 中立，只治理上下文形态和证据传递，不决定具体业务如何实现。后续所有涉及 LLM 输入构造、context budget、repair input、redaction、memory recall、run evidence summary 的修改，都必须按本设计核对。
