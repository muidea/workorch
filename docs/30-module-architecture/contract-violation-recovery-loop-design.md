# Contract Violation Recovery Loop Design

Status: implemented

Type: module-architecture

Last Updated: 2026-06-20

Last Verified: 2026-06-20

Related:

- [Runtime Execution Event Queue State Loop Design](runtime-execution-event-queue-state-loop-design.md)
- [LLM Cache Improvement Design](llm-cache-improvement-design.md)
- [MVVP Workspace ViewModel Architecture](mvvp-workspace-viewmodel-architecture.md)
- [Kernel Orchestrator Governance](kernel-orchestrator-governance.md)

## Purpose

本文固化 WorkOrch 对 LLM 输出合同违例的长期处理方向：合同门禁继续保持严格，但对可恢复的 LLM 违例输出不再直接升级为人工 decision。Runtime 应将违例输出作为被拒绝的诊断 evidence 持久化，向 LLM 返回精确违例原因，并在同一 runtime stage / repair objective 下重试，直到成功、达到可恢复预算上限，或确认需要人工决策。

这里的“允许返回违例”不是允许执行违例输出，而是允许系统接收、记录、解释并反馈违例输出。任何未通过合同门禁的 `executionGraph`、`dodAssessment`、artifact plan 或 decision JSON 都不得触发 state-changing runtime block，不得写入 accepted result，也不得覆盖 coverage ledger。

当前阶段不考虑兼容性。本文作为后续代码收口的设计依据、调整范围清单和验收标准。

## Design Decisions

1. 合同门禁不放松。LLM 可以返回违例内容，但违例内容只能被 runtime 接收、记录、解释和反馈，不能被执行、接受或用于完成 DoD。
2. 可恢复违例优先自动重试。字段缺失、output shape 错误、缺 evidenceRefs、coverage map 不完整、误返回 `blocked_on_decision` 等可修正漂移，应进入 contract violation recovery loop，而不是直接要求用户 replan。
3. 重试必须继承原合同。retry 只能在 C4 增加 `contractViolationRecovery`，不得弱化 `requiredOutput`、`stateChangingOutputRequired`、`repairActionPackage`、Goal/DoD definition 或 evidence contract。
4. 重试通过 EventHub 主路径推进。contract violation retry 必须发布 dispatch requested event，由 runtime event loop 调度；不得在 finalize / projection / TUI 中同步递归执行 LLM。
5. 预算独立且可观测。contract violation retry 与 provider retry、runtime repair、coverage follow-up 分账，避免某类失败吞掉其他恢复预算。
6. 重复违例必须升级。相同 signature 超预算、输出形态无变化、不可解析输出持续出现时，进入 decision / failed，并携带 violation history，避免无限低质量循环。
7. UI 只展示 projection。TUI/Web 不判断 recoverability，不自行补偿 retry；所有状态来自 workorchd ViewModel/projection。
8. Core 保持业务中立。任何语言、模板、命令、文件名、provider 品牌或具体项目目标相关策略只能落在 workspaceTemplate、Soul、policy、skill 或 e2e fixture。

## Strictness and Tolerance Model

本设计把“严格合同”和“容错恢复”拆成两个连续但职责不同的阶段：

```text
LLM output
  -> strict contract admission
  -> if valid: allow side effects / evidence acceptance / next step
  -> if invalid but recoverable: record rejected diagnostic + retry same contract
  -> if invalid and not recoverable: escalate decision / approval / failed
```

因此，当前问题不应通过降低合同门禁解决。真正需要调整的是违例后的恢复闭环：

- 允许 runtime 接收违例输出，是为了提取违例原因、形成 rejected diagnostic evidence 并反馈给 LLM。
- 不允许 runtime 执行违例输出，也不允许把违例输出折算为 accepted evidence、result、artifact 或长期 memory。
- 可恢复违例应自动回到同一 stage / repair objective / requiredOutput 下重新生成，而不是立即把 workspace 卡到人工 decision。
- 不可恢复违例、外部依赖失败、权限/审批缺失和重复同签名违例仍然必须升级治理。

这可以同时解决两个目标：

1. 保持 WorkOrch 作为业务中立调度框架的安全边界，避免低质量输出进入执行链路。
2. 降低 LLM 输出偶发 schema 漂移、字段缺失、错误 output kind 导致的人工 replan 次数，提高自动收敛概率。

### Why Not Relax the Prompt Contract

如果把 prompt 改成“尽量返回即可”，会产生以下副作用：

- state-changing gate 失效，可能执行缺少 evidence / validation / approval 语义的 graph。
- DoD coverage ledger 被不完整 assessment 污染，后续 completion gate 难以判断真实完成度。
- rejected output 可能进入 memory 或 process evidence timeline，形成后续上下文噪声。
- TUI/Web 看到的 projection 与真实执行准入不一致，用户会误判 workspace 已经推进。

因此 prompt 层只允许更精确地描述违例原因和修正目标，不允许弱化原 `requiredOutput`、Goal/DoD、evidence contract 或 runtime stage contract。

### Recovery Contract

每次 contract violation retry 必须满足：

- 原失败 step 已经 terminal，且违例 output 没有任何 side effect。
- retry 继承原 `runtimeStage`、`runtimeStageMode`、`repairObjective`、`requiredOutput`、`stateChangingOutputRequired`、Goal/DoD definition 和 evidence timeline。
- retry 只新增 C4 `contractViolationRecovery`，不得修改 C0/C1/C2 stable prefix。
- retry 通过 EventHub dispatch 主路径进入队列，不在 finalize、projection、TUI 或 REST handler 中同步递归执行。
- retry 有独立预算和 idempotency key，重复提交不会创建多个等价 retry step。

### Failure Boundary

以下不是 contract violation recovery 的责任：

- provider timeout、stream incomplete、auth、rate limit、network failure。
- capability 权限不足、MCP/server 不可用、repository 访问失败。
- patch / artifact / state-changing operation 需要人工 approval。
- 用户目标冲突、需要用户选择方向、外部凭证或配置缺失。

这些情况应继续走 provider retry、runtime repair、approval、decision 或 failed 路径，不能被包装成“让 LLM 再试一次”的合同违例。

## Background

近期部署验证中多次出现以下现象：

- LLM 已经返回结构化内容，但因为字段缺失、输出类型不符合当前 `requiredOutput`、`evidenceRefs` 不完整、graph kind 与状态变更合同不一致等原因被判定为 `llm_decision_invalid` 或 runtime contract failure。
- 这些违例本质上经常是“可被重新说明后修正”的 LLM 输出漂移，不等同于 provider 不可用、权限缺失、approval required 或用户必须选择方向。
- 现有链路中，部分可恢复违例会很快进入 decision ticket，导致 run 停在 `blocked_on_decision` 且 projection 显示 decision prompt，需要用户反复 replan，而不是由 runtime 自己把违例原因反馈给 LLM 并重试。
- 为了促进 goal 收敛，不能简单放松合同。放松合同会让无效或不安全输出进入执行链路，进一步污染 evidence、result、memory 和 DoD coverage。

因此需要新增一个中立的 contract violation recovery loop：严格校验仍然是唯一准入门禁；门禁失败后先判断是否可自动恢复；可恢复时形成结构化诊断并重试；不可恢复或重复违例才升级 governance。

## Boundary Principles

本设计必须遵循 WorkOrch core / workspaceTemplate 边界：

- Core 只实现通用合同校验、违例诊断、重试预算、状态机和投影。
- Core 不根据模板 ID、Soul ID、语言、文件名、测试命令、MCP server 名称或业务文本做特判。
- 具体业务策略仍由 workspaceTemplate、Soul、DoD、policy、capability contract 和 skill 提供。
- Core 可以消费结构化 `requiredOutput`、`acceptedKinds`、`acceptedOperations`、`evidencePolicy`、`runtimeStage`、`repairObjective` 等字段。
- Core 不执行未通过合同的 output；违例 output 只能作为 rejected diagnostic evidence。
- TUI/Web 只展示 workorchd projection 和提交用户命令，不复制违例恢复状态机。
- LLM context/cache 分区保持不变：稳定合同在 C0/C1/C2，违例诊断和 retry instruction 进入 C4 current-action，不污染 stable prefix。

## Goals

1. 保持合同严格性，不降低 graph、DoD、approval、evidence 和 state-changing gate。
2. 将可恢复的 LLM 输出违例结构化记录为 `ContractViolation`。
3. 把精确违例原因反馈给 LLM，要求在同一或收窄后的合同下重新输出。
4. 对 contract violation retry 使用独立预算，避免与 provider retry、runtime repair、coverage no-delta 混在一起。
5. 对重复相同违例、不可恢复违例、外部依赖问题和人工治理问题明确升级 decision / approval / failed。
6. 保证多 workspace 并发执行时，每个 workspace/run/step 的违例恢复都具备完整 scope 和幂等语义。
7. 在 projection/TUI/Web 中展示“正在修正合同违例”，避免用户误判为无进展或 decision 未提交。

## Non-Goals

1. 不允许执行未通过合同的 graph node。
2. 不把自然语言违例文本作为业务规则来源。
3. 不在 orchestrator 中写入软件开发、验证命令、特定文件或 provider 品牌特判。
4. 不把所有失败都自动重试；外部依赖、权限、approval、不可解析 provider 输出仍应按现有治理路径处理。
5. 不用无限 replan/resume 替代明确的 failure / decision。
6. 不让端侧 UI 自己判断是否可恢复。

## Core Concepts

### ContractViolation

新增中立运行期诊断模型，用于描述一次合同违例。它可以落在 runtime checkpoint / execution context / read model 事件中，长期如需要可独立 owner repository。

```go
type ContractViolation struct {
    ID string
    WorkspaceID string
    RunID string
    StepID string
    SourceStepID string

    Stage string
    RuntimeStage string
    RuntimeStageMode string
    RepairObjective string

    ViolatedRule string
    ExpectedContract map[string]any
    ActualOutputSummary map[string]any
    RejectedOutputRef string

    FailureClass string
    FailureSignature string
    Recoverability string
    RetryHint string
    ForbiddenRepeat []string
    SourceRefs []string

    Attempt int
    SignatureRepeatCount int
    CreatedAt string
}
```

字段语义：

- `ViolatedRule`: 中立规则 ID，例如 `required_output_missing`、`state_changing_graph_missing`、`dod_evidence_refs_missing`。
- `ExpectedContract`: 当前 step 已经声明的合同摘要，例如 `requiredOutput`、`acceptedKinds`、`stateChangingOutputRequired`、`assessmentOnly`。
- `ActualOutputSummary`: 被拒绝 output 的安全摘要，不保存大对象正文。
- `RejectedOutputRef`: 指向持久化 raw rejected output / checkpoint / step output snapshot。
- `Recoverability`: `retry_same_contract`、`narrow_contract`、`normalize_then_retry`、`decision_required`、`fail_fast`。
- `ForbiddenRepeat`: 下一次输出必须避免重复的违例签名或 output shape。

### Rejected Diagnostic Evidence

可恢复违例必须作为 rejected diagnostic evidence 进入审计链路：

- 可以作为 LLM 下一步 C4 输入。
- 可以出现在 read model 的 progress / diagnostics 中。
- 不能作为 accepted evidence 满足 DoD。
- 不能触发 artifact/result acceptance。
- 不能写成高价值 memory recall，除非它被明确标记为 diagnostic-only 且只用于当前 recovery。

### Contract Violation Gate

contract violation gate 是 provider output 之后、任何 state-changing side effect 之前的准入层。它的职责是把“LLM 输出没有满足当前合同”从普通执行失败中拆出来：

```text
provider output
  -> parse / schema gate
  -> contract violation gate
  -> side-effect admission gate
  -> runtime block dispatch
```

要求：

- gate 输入只能来自当前 step input、step output、runtime policy、Goal/DoD definition 和已接受 evidence ledger。
- gate 输出必须是 valid output、recoverable violation、non-recoverable violation 三选一。
- gate 不发布 event、不创建 ticket、不调度 retry；这些动作由 orchestrator state loop 根据 gate result 完成。
- gate 不能读取 TUI/Web 状态，不能访问具体 template 名称或 workspace title。

### Recovery Retry Step

当违例可恢复时，orchestrator 创建一个新的 LLM step 或复用现有 repair/follow-up step 语义进行 retry。该 retry 必须继承原始 step 的收敛合同：

- `runtimeStage`
- `runtimeStageMode`
- `repairObjective`
- `repairActionPackage`
- `requiredOutput`
- `stateChangingOutputRequired`
- `assessmentOnly`
- 当前 Goal/DoD definition revision
- accepted / rejected evidence refs
- process evidence timeline

retry 只补充 `contractViolationRecovery` C4 信息，不得把原合同改写为更弱的合同。

retry step 的幂等约束：

- idempotency key: `workspaceId/runId/sourceStepId/violationSignature/attempt`。
- 同一个 idempotency key 重放时只允许返回已有 retry step，不允许重复创建多个 LLM step。
- retry step 的 source lineage 必须指向原失败 step，不能以最新 UI 选中 workspace 或 active prompt 作为隐式 scope。
- 服务重启后不依赖 watchdog 补偿来“猜测”是否 retry；应由已持久化 step/checkpoint/event 恢复出明确 pending retry 或 idle 状态。

## Recoverability Classification

### Recoverable

以下场景默认可自动恢复，前提是 provider 输出可解析、没有外部依赖阻塞、没有 approval 风险：

| Rule | Condition | Recovery |
| --- | --- | --- |
| `required_output_missing` | 当前 step 明确要求 graph / assessment / artifact chunk，但 output 未包含 | `retry_same_contract`，重申 required output |
| `wrong_output_shape` | 返回了 assessment，但当前需要 state-changing graph；或返回 graph，但当前是 assessment-only | `retry_same_contract`，列出当前允许 output kind |
| `accepted_kind_mismatch` | graph node kind 不在 `acceptedKinds` | `narrow_contract`，只列出允许 kind |
| `state_changing_required_missing` | `stateChangingOutputRequired=true`，但返回 read-only graph 或无 state-changing evidence | `retry_same_contract`，明确 state-changing evidence contract |
| `concrete_validation_contract_missing` | 需要 concrete validation evidence，但 graph 缺少 `validationEvidence` 声明 | `normalize_then_retry`，要求补齐声明和 bounded execution |
| `dod_assessment_missing_evidence_refs` | satisfied assessment 缺少 evidence refs 或 coverage map | `retry_same_contract`，要求基于现有 evidence refs 重写 assessment |
| `coverage_map_incomplete` | DoD/Goal coverage 缺 item 或 state | `retry_same_contract`，提供缺失项列表 |
| `traceability_ref_mismatch` | artifactRefs / evidenceRefs 可追溯但字段位置错误 | `normalize_then_retry` 或由中立 normalizer 修正后重评估 |
| `decision_blocked_when_actionable` | LLM 返回 blocked_on_decision，但 runtime 仍有明确可执行 output contract | `retry_same_contract`，说明不能请求 decision 的原因 |

### Non-Recoverable

以下场景不应自动重试，或只能在外部条件恢复后由用户/系统命令重新激活：

| Rule | Condition | Handling |
| --- | --- | --- |
| `provider_unavailable` | provider timeout、rate limit、auth/config、stream incomplete 超出 provider retry | decision / failed，等待外部恢复 |
| `approval_required` | state-changing patch / operation 需要人工审批 | approval ticket |
| `human_choice_required` | 目标冲突、用户必须选择方案 | decision ticket |
| `permission_denied` | capability/MCP/repository 权限被拒绝 | decision 或 failed |
| `unparseable_output` | provider 返回无法解析 JSON / schema completely broken | provider schema drift / retry budget 后 decision |
| `repeated_same_violation` | 相同 failure signature 超过预算 | decision ticket，携带 violation history |
| `unsafe_output` | 涉及 destructive command、越权访问、未授权 mutation | fail_fast 或 approval/decision |

## State Machine

### Normal Path

```text
runtime completed event
  -> orchestrator finalize
  -> output contract validators
  -> valid output
  -> apply side effects / update evidence / coverage
  -> dispatch next step or complete
```

### Recoverable Violation Path

```text
runtime completed event
  -> orchestrator finalize
  -> output contract validators
  -> ContractViolation(recoverable)
  -> persist rejected output + violation checkpoint
  -> mark original step failed with structured diagnostics
  -> create contract violation retry step
  -> publish dispatch requested event
  -> LLM receives same contract + C4 violation diagnostic
```

要求：

- 原 step 必须 terminal，不能保持 `dispatched` / `running`。
- retry 通过现有 EventHub dispatch requested 主路径推进，不能同步递归调用 runtime block。
- retry 需要稳定幂等 key：`workspace/run/sourceStep/violationSignature/attempt`。
- 多 workspace 下所有 event payload 必须携带 workspaceId/runId/stepId。

### Escalation Path

```text
runtime completed event
  -> output contract validators
  -> ContractViolation(non-recoverable or budget exhausted)
  -> persist violation history
  -> create decision / approval / failed state
  -> projection shows exact blocker and retry history
```

### User Command Interaction

用户通过 TUI/Web 提交 `/decision replan`、`/run resume` 或 REST command 时，只产生 command event：

```text
TUI/Web command
  -> port/client command DTO
  -> workorchd command handler
  -> EventHub command event
  -> owner module updates run/ticket state
  -> orchestrator dispatches next step when state permits
  -> projection emits updated ViewModel
```

要求：

- 如果 run 处于 `blocked_on_decision` 且用户选择 replan，应关闭或 resolve 对应 ticket，并发布可调度事件。Decision blocker 不改变 workspace owner state。
- 如果 replan 后仍有可恢复 contract violation，runtime 应自动进入 retry loop，而不是继续停留在 stale decision prompt。
- TUI prompt 只能依据 projection 中的 active ticket / active approval / active run state 生成。
- 多 workspace 下 command payload 必须携带 workspaceId，不能依赖当前端侧选中状态作为服务端唯一依据。

## Budget Policy

Contract violation retry 使用独立预算，不直接消耗 provider retry，也不立即进入 coverage no-delta budget。

建议新增或固化以下中立配置：

```go
type ContractViolationRecoveryPolicy struct {
    Enabled bool
    MaxRetriesPerStep int
    MaxRetriesPerRun int
    MaxRepeatedSignature int
    MaxConsecutiveNoShapeChange int
    EscalateOnUnsafeOutput bool
}
```

默认策略建议：

- `MaxRetriesPerStep`: 2
- `MaxRetriesPerRun`: 8
- `MaxRepeatedSignature`: 1
- `MaxConsecutiveNoShapeChange`: 1
- `EscalateOnUnsafeOutput`: true

预算归属：

- provider timeout / stream failure 仍归 provider retry。
- runtime tool failure 仍归 runtime repair。
- LLM 输出合同违例归 contract violation retry。
- 合同违例重试成功后，正常进入 coverage / completion gate。
- 合同违例重试失败且签名重复，升级 decision，不继续无效循环。

## Prompt and Context Design

### C0 / C1 / C2

不新增动态违例内容：

- C0 保持稳定 runtime schema 与中立合同。
- C1 保持 Goal/DoD definition。
- C2 保持 capability contract。

### C3

保持当前 stage contract：

- runtime stage
- required output
- accepted output kinds
- state-changing requirement
- assessment-only requirement

### C4

新增 `contractViolationRecovery`：

```json
{
  "contractViolationRecovery": {
    "attempt": 1,
    "violatedRule": "state_changing_required_missing",
    "failureSignature": "rule=state_changing_required_missing;required=patch/apply;actual=read_only_graph",
    "expectedContract": {
      "stateChangingOutputRequired": true,
      "acceptedKinds": ["patch/apply", "git/write", "mcp/tool_call"]
    },
    "actualOutputSummary": {
      "outputKinds": ["executionGraph"],
      "graphNodeKinds": ["file/read", "shell/command"]
    },
    "retryInstruction": "Return a valid output that satisfies the same requiredOutput. Do not repeat the rejected output shape.",
    "forbiddenRepeat": [
      "read-only graph without state-changing evidence"
    ],
    "rejectedOutputRef": "run-001/step-012/contract-violation-001"
  }
}
```

要求：

- 违例诊断必须短、准确、结构化。
- 不把完整 rejected output 大对象直接塞入 prompt；大对象通过 ref 访问。
- 不让 violation diagnostic 覆盖原始 `repairActionPackage`。
- 不把 “为了通过某个模板” 的业务建议写入 C4；C4 只表达合同违例。

### Prompt Strictness Policy

本设计不采用“放松合同换取推进”的方式。prompt 可以更明确地告诉 LLM：

- 上一次输出被拒绝的具体中立原因。
- 当前仍然必须满足的 `requiredOutput`。
- 不允许重复的 output shape。
- 如果确实缺少外部信息、权限或审批，应返回结构化 blocker。

prompt 不应要求 LLM 猜测业务策略，也不应把 rejected output 当作可继承的有效进展。contract violation retry 的目标是“让 LLM 重新生成一个满足合同的输出”，不是“接受有缺陷输出后继续执行”。

## Normalization Policy

部分违例可以先做中立 normalization，再决定是否重试：

- 可追溯的 `artifactRefs` 转 `evidenceRefs`。
- 字段大小写或数组/单值形式的中立归一。
- graph kind alias 归一到 canonical kind。
- `deliverableCoverage.items` / `goalCoverage.items` 中可确定的 refs 映射。

normalization 边界：

- 只能修复结构位置或 canonical form，不能替 LLM 推断业务结论。
- normalization 后仍必须重新跑同一合同校验。
- normalization 必须留下 `normalizedFromViolation` 诊断，便于审计。
- 无法无歧义归一时进入 retry，不得静默接受。

## EventHub and Projection Design

新增中立 runtime/orchestrator event，或在现有 step checkpoint event 中承载等价结构化 payload。长期建议使用独立 topic，便于 projection 和 e2e 观测。

建议 topic：

| Topic | Owner | Payload |
| --- | --- | --- |
| `/orchestrator/contract_violation/recorded` | orchestrator | `ContractViolation` |
| `/orchestrator/contract_violation/retry_requested` | orchestrator | workspace/run/sourceStep/retryStep/violation |
| `/orchestrator/contract_violation/escalated` | orchestrator | violation history + blocker id |

lane 策略：

- recorded / escalated 属于 workspace/run state lane。
- retry dispatch 使用现有 `TopicOrchestratorDispatchStepRequested`。
- 不在 observer lane 内直接执行 LLM retry。

projection 字段：

```json
{
  "contractViolation": {
    "active": true,
    "rule": "dod_assessment_missing_evidence_refs",
    "attempt": 1,
    "maxAttempts": 2,
    "sourceStepId": "012-...",
    "retryStepId": "013-contract-retry-...",
    "summary": "LLM returned satisfied dodAssessment without evidenceRefs",
    "recoverability": "retry_same_contract"
  }
}
```

TUI/Web 只展示：

- `Retrying contract violation`
- 当前 rule / attempt / source step
- 若升级 decision，则展示 blocker 和 violation history

TUI/Web 不根据 `rule` 自行决定 replan、resume 或 retry。

## Consistency and Idempotency

### Workspace Scope

所有 contract violation 相关对象必须完整携带：

- `workspaceId`
- `runId`
- `sourceStepId`
- `retryStepId` 或 `ticketId`
- `violationSignature`
- `attempt`

禁止跨 workspace 只按 run id、ticket id、decision id 或 step id 查询并执行恢复动作。无法唯一定位时必须返回 conflict。

### Event Ordering

同一 workspace/run 的状态变更必须遵循 owner lane 串行化：

1. step terminal checkpoint
2. violation recorded
3. retry requested 或 escalation ticket created
4. dispatch accepted
5. projection updated

无状态观察、UI SSE、日志聚合不应阻塞 owner lane。runtime dispatch event 必须使用异步 post 或事件队列机制，避免在同 lane 内同步等待自身消费。

### Persistence

最小持久化集合：

- source step terminal state
- rejected output safe summary 或 ref
- `ContractViolation`
- retry step idempotency key
- retry requested checkpoint 或 escalation ticket

服务重启后：

- 已 terminal 且无 retry/ticket 的 recoverable violation 应按持久化 checkpoint 恢复到 pending retry 或明确 failed。
- 运行中但无实际执行上下文的 workspace 不应保持 stale active projection；应刷新为 idle / activation required，并等待用户或系统命令重新激活。

## Scope List

本节是后续代码收口的范围清单。收口时应先完成 core 主路径，再补齐 projection/端侧展示，最后补 e2e 和 skill 固化。

### Scope Summary

| Scope | Must Change | Purpose |
| --- | --- | --- |
| Contract model | Yes | 建立中立 `ContractViolation` / recoverability / retry policy。 |
| Contract validators | Yes | 把散落 error 文本升级为结构化 violation result。 |
| Orchestrator state loop | Yes | 在严格门禁失败后自动创建 retry 或 escalation。 |
| EventHub dispatch | Yes | retry 走事件驱动调度，避免同步 Send 卡同 lane。 |
| Prompt/context C4 | Yes | 将违例诊断反馈给 LLM，保持 C0/C1/C2 稳定。 |
| Runtime policy/config | Yes | 提供独立预算和可配置上限。 |
| Read model/projection | Yes | 展示 active violation、retry attempt、escalation blocker。 |
| TUI/Web/port/client | Yes | 只消费 projection，保持 prompt/status 一致。 |
| Memory/result/evidence | Yes | rejected diagnostics 不进入 accepted result、artifact 或高价值 memory。 |
| Template/Soul | Optional | 只补充业务提示或 template-bound `requiredOutput`，不写 core 特判。 |
| E2E/tests | Yes | 证明第一次违例、第二次合法输出可以自动推进。 |
| Skill/docs | Yes | 固化 core/template 边界和违例处理规则。 |

### Synchronized Adjustment Matrix

以下范围需要同步核对。只改其中一层会导致恢复闭环不完整，例如 orchestrator 能重试但 TUI 仍显示 stale decision，或 prompt 能收到违例诊断但 rejected output 被错误写入 memory。

| Layer | Owner Responsibility | Required Adjustment | Must Not Do |
| --- | --- | --- | --- |
| Runtime contract DTO | 定义跨模块共享的中立违例模型 | 固化 `ContractViolation`、recoverability、retry policy、safe summary、signature | 不放入模板名、语言、文件名、provider 品牌 |
| Validators | 判断输出是否满足当前 step 合同 | 将纯 error 转为 structured validation result；区分 valid / recoverable / non-recoverable | 不创建 ticket、不发布 event、不执行 retry |
| Orchestrator state loop | 拥有 run/step 状态机和恢复动作 | terminal source step、记录 rejected diagnostic、创建 retry/escalation、保证幂等 | 不同步调用 LLM，不绕过 EventHub dispatch |
| EventHub events | 承载模块间状态变化和调度请求 | 增加 recorded / retry_requested / escalated 事件或等价 checkpoint；retry 走 dispatch requested | 不让 observer/projection lane 执行业务恢复 |
| Runtime policy/config | 控制预算和熔断 | 独立配置 per step/per run/repeated signature/no shape change 上限 | 不与 provider retry、runtime repair 共用模糊计数 |
| Prompt/context | 把违例原因反馈给 LLM | C4 注入 `contractViolationRecovery`；保持 C0/C1/C2 稳定 | 不降低 `requiredOutput`，不把动态失败写进 stable prefix |
| Evidence/result/artifact | 保证准入前后数据不污染 | rejected output 只作为 diagnostic refs；accepted evidence 只来自合法输出 | 不把违例输出标记为完成证据或产物 |
| Memory | 控制长期可召回上下文 | 拒绝 diagnostic-only violation memory；仅当前 retry 通过 C4 注入 | 不把违例 signature、临时 retry history 写入高价值 memory |
| Workorchd projection | 拥有 workspace ViewModel | 从 checkpoint/event 聚合 active violation、attempt、blocker、retry state | 不根据文本内容二次推断 recoverability |
| Port/client | 保持 Web/TUI/CLI DTO 一致 | REST/client/port 透传同一 `contractViolation` 摘要字段 | 不让 remote/local backend 字段不一致 |
| TUI/Web | 只负责交互和显示 | 展示 retrying/escalated 状态，prompt 基于 projection 刷新 | 不自动补偿 retry，不保留另一套状态机 |
| Template/Soul/policy | 承载业务策略 | 必要时补充 `requiredOutput`、acceptedKinds、evidencePolicies 和业务提示 | 不要求 core 根据模板或业务文本判定违例 |
| Tests/e2e | 证明主路径闭环 | 覆盖 invalid first output -> automatic retry -> valid output -> resume | 不只测 helper，不验证 side effect 隔离 |

### Minimal Atomic Delivery Units

后续代码收口应按以下原子单元推进，每个单元都要能独立验证：

1. Model and policy: 新增/固化 DTO、recoverability、预算默认值和配置读写。
2. Validator result: 把至少一类历史高频违例从 `llm_decision_invalid` 拆成 structured `ContractViolation`。
3. Recovery dispatch: recoverable violation 能创建 retry step，并通过 EventHub dispatch requested 主路径执行。
4. Prompt recovery context: retry LLM input 包含 C4 `contractViolationRecovery`，stable prefix 不变。
5. Side-effect isolation: 违例 graph 不触发 patch/file/shell/mcp mutation，不进入 accepted evidence/result/artifact。
6. Projection chain: workorchd、port/client、TUI/Web 能看到同一 active violation / retry / escalation 状态。
7. Escalation budget: 重复违例达到预算后只升级一次治理 ticket，且带完整 violation history。
8. E2E proof: mock provider 第一次返回违例、第二次返回合法 output，workspace 不需要人工 replan 即可继续推进。

### Cross-Cutting Audit Points

每次收口必须同步搜索和核对：

- 是否仍有 `llm_decision_invalid` 把可恢复 output shape 漂移直接转成人工 decision。
- 是否仍有 `blocked_on_decision` 在 runtime 具备明确可执行合同时被直接接受。
- 是否存在 rejected output 进入 `acceptedEvidenceRefs`、result、artifact 或 memory recall。
- 是否存在 retry step 丢失 Goal/DoD definition、requiredOutput、repairActionPackage 或 process evidence timeline。
- 是否存在 TUI/Web 根据本地 stale state 生成 prompt，而不是基于 workorchd projection。
- 是否存在同 lane 同步 `Send` 导致 dispatch 卡住或递归等待。
- 是否存在 provider/external failure 被误归为 contract violation retry。
- 是否存在 core 目录新增模板、语言、文件名、命令、provider 品牌特判。

### Kernel Orchestrator

| Path | Adjustment |
| --- | --- |
| `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go` | 将现有 `error` 型合同校验逐步升级为可返回 `ContractViolation` 的结构化结果；保持原错误兼容只作为内部过渡，不作为长期模型。 |
| `internal/modules/kernel/orchestrator/biz/runtime_graph_contract.go` | 对 graph kind、state-changing、acceptedKinds、validationEvidence 等违例生成中立 rule ID 和 expected/actual 摘要。 |
| `internal/modules/kernel/orchestrator/biz/runtime_dod.go` | DoD assessment 缺 coverage、缺 evidence refs、缺 required dimensions 时生成可恢复违例。 |
| `internal/modules/kernel/orchestrator/biz/runtime_dod_evidence_refs.go` | refs normalization 与 traceability 诊断统一输出，避免分散字段修补。 |
| `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality.go` | 保证 rejected violation evidence 不进入 accepted evidence quality。 |
| `internal/modules/kernel/orchestrator/biz/failure_classification.go` | 增加 `contract_violation` / recoverability 分类，避免所有合同漂移都落入不可区分的 `llm_decision_invalid`。 |
| `internal/modules/kernel/orchestrator/biz/runtime_result_finalize_event.go` | finalize 时遇到 recoverable violation 创建 retry step；遇到不可恢复或预算耗尽创建 blocker。 |
| `internal/modules/kernel/orchestrator/biz/runtime_repair.go` | 将 contract violation retry 纳入 event-driven dispatch，禁止递归执行。 |
| `internal/modules/kernel/orchestrator/biz/runtime_repair_policy.go` | 区分 provider retry、runtime repair、coverage repair、contract violation retry。 |
| `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go` | 将 `contractViolationRecovery` 注入 C4 current-action，保持原 stage contract。 |
| `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go` | 新增独立 retry budget 和 repeated signature budget。 |
| `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go` | 确认 violation 只进入 C4，不改变 stable prefix。 |
| `internal/modules/kernel/orchestrator/biz/runtime_checkpoints.go` | 持久化 violation recorded / retry requested / escalated checkpoint。 |
| `internal/modules/kernel/orchestrator/pkg/contracts` | 如跨 module 需要 command/event DTO，新增中立 contract 类型。 |

必须同步核对的现有逻辑：

- `llm_decision_invalid` 是否被过早转成人工 decision。
- `blocked_on_decision` 是否在 runtime 仍有 actionable contract 时被直接接受。
- `runtime_failed` 是否混入了可恢复 output shape 漂移。
- `provider_unavailable` 是否被误归为 contract violation。
- state-changing repair 是否因为缺少上下文而反复要求同一错误 output。

### Runtime Contract / Shared DTO

| Path | Adjustment |
| --- | --- |
| `internal/pkg/workorchruntimecontract` | 放置可跨 block/orchestrator/read model 复用的 `ContractViolation`、`ContractViolationRecoveryPolicy`、recoverability enum。 |
| `internal/pkg/events` | 仅聚合 owner event contract 和 lane policy，不在此处定义业务判断。 |

DTO 规则：

- shared DTO 只能表达中立 runtime contract，不包含 template/soul/provider 特判。
- REST/client/port DTO 字段名保持一致，避免 TUI/Web 读取不同形态。
- 对外 projection 使用摘要字段，不暴露完整 rejected output 大对象。

### Run / Evidence / Memory

| Path | Adjustment |
| --- | --- |
| `internal/modules/kernel/run/**` | 如 step model 需要记录 violation attempt / sourceStep lineage，补充中立字段或 executionContext patch。 |
| `internal/modules/kernel/memory/**` | 防止 rejected diagnostic-only violation 被高权重召回；只允许当前 recovery 显式注入。 |
| `internal/modules/kernel/result/**` | 确保 rejected output 不生成 accepted result/artifact。 |

Memory 收口规则：

- `contractViolationRecovery`、`contractViolationType`、`runtime_contract_violation`、违例 rule ID 和 diagnostic-only tag 都属于当前恢复诊断，不能写成长期 `project_context`、`user_preference`、`execution_lesson`、`dod_feedback`、`skill_feedback` 或 `tool_feedback` memory。
- Contract violation retry step 不应自动生成 runtime repair lesson；其诊断只能通过 C4 `contractViolationRecovery` 注入当前 retry。
- 如果用户显式提供了与业务策略相关的长期约束，应通过普通 user/project memory 写入；不能混入 rejected output、retry attempt、违例 signature 或临时恢复预算。

### Workorchd Appservice / Projection

| Path | Adjustment |
| --- | --- |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**` | 聚合 violation recorded/retry/escalated 事件到 workspace/run projection。 |
| `internal/modules/application/workorchd/biz/appservice/readmodelports/**` | 暴露 `contractViolation` projection DTO。 |
| `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**` | 以 workspace 为单位管理 active violation / blocker / runtime status，避免端侧二次推断。 |
| `internal/modules/application/workorchd/service/server/**` | REST snapshot/status/SSE 返回 projection 字段；server 不参与 recovery 判断。 |

投影必须保证：

- workspace list、workspace detail、run status、event stream 看到同一份 active violation 状态。
- projection detail 未 ready 时不返回会导致 TUI 卡死的半状态；应返回明确 `projection_refreshing` 或保留最后一致快照。
- SSE 中断不影响正式状态，只影响端侧显示刷新；重连后应能从 projection 补齐当前状态。

### TUI / Web / Client Port

| Path | Adjustment |
| --- | --- |
| `internal/pkg/workorchport` | 增加端侧展示所需的中立 violation DTO。 |
| `internal/pkg/workorchclient` | REST client 透传 violation projection，不做业务解释。 |
| `internal/services/workorch/service/tui/**` | prompt、主视图、workspace list 显示 `retrying contract violation` / blocker；不自行发起自动重试。 |
| Web frontend | 服务状态、workspace detail、event stream 展示 violation progress；避免白屏或 incomplete chunk 时丢失状态。 |

端侧同步调整点：

- prompt 生成以 ViewModel 为唯一来源，删除本地 stale decision / approval / running 推断。
- workspace 切换后主动重置 transient input state，但不清理服务端 projection。
- `/decision replan` 提交成功后等待 projection event 更新 prompt，不在端侧假设已进入 running。
- remote/local backend 都走同一 port DTO，避免 remote 模式和 local 模式状态字段不一致。

### Template / Soul / Policy

| Path | Adjustment |
| --- | --- |
| `internal/pkg/workorchdefaults/defaults/templates.json` | 如需更细的 `requiredOutput` / `acceptedKinds`，由 template 数据声明，不写入 core。 |
| `internal/pkg/workorchdefaults/defaults/souls.json` | 可补充业务提示词：遵循 core 合同、收到违例诊断后只修正输出形态，不改变目标。 |
| `/home/rangh/aispace/.agents/skills/workorch-core-template-boundary` | 后续代码落地后更新 skill，固化“违例可记录但不可执行”的规则。 |

模板侧只能做以下调整：

- 补充更明确的 `requiredOutput`、`acceptedKinds`、`evidencePolicies`。
- 补充 Soul 提示：收到 `contractViolationRecovery` 时只修正输出形态和缺失 evidence refs，不改变 goal。
- 软件开发等场景的文件粒度、验证方式、工具使用策略应留在 template/Soul/skill，不进入 orchestrator。

### Tests and E2E

| Area | Required Tests |
| --- | --- |
| Contract validator unit tests | graph kind mismatch、state-changing missing、DoD evidence refs missing、wrong output shape 均产生 structured violation。 |
| Recovery loop tests | recoverable violation 创建 retry step，继承 runtime stage / repair objective / requiredOutput。 |
| No side effects tests | invalid graph 不执行 patch/apply、git/write、mcp/tool_call、shell mutation。 |
| Budget tests | repeated same signature 达到预算后升级 decision。 |
| Context/cache tests | contract violation retry 不改变 C0 stable prefix；violation diagnostic 只出现在 C4。 |
| Projection tests | read model 展示 active violation / retry attempt / escalated blocker。 |
| TUI tests | workspace 切换后 prompt 与 projection 一致，不保留 stale decision 或 stale violation。 |
| E2E tests | mock provider 第一次返回违例、第二次返回合法 output，workspace 能自动继续推进。 |
| Boundary tests | core 通用目录无模板、语言、命令、文件名特判。 |

## Implementation Plan

### Phase 1: Design and Taxonomy

- 固化本文档。
- 梳理现有合同校验函数的错误文本，映射为中立 rule ID。
- 明确 recoverability matrix 和默认预算。

### Phase 2: Data Model and Helpers

- 新增 `ContractViolation` / `ContractViolationRecoveryPolicy`。
- 新增 violation signature builder。
- 新增 rejected output safe summary builder。
- 新增 neutral normalizer hook。

### Phase 3: Validator Structured Result

- 将 graph / execution / DoD 合同校验从纯 `error` 扩展为 `contractValidationResult`。
- 保持 validators 只判断合同，不创建 step、不发布 event。
- 为每类可恢复违例补单测。

### Phase 4: Orchestrator Recovery Loop

- finalize 中识别 recoverable violation。
- 持久化 rejected output + violation checkpoint。
- 创建 contract retry LLM step。
- 通过 `TopicOrchestratorDispatchStepRequested` 发布 dispatch request。
- 确保 retry step 幂等和多 workspace scope 完整。

### Phase 5: Prompt / Context Integration

- 在 C4 注入 `contractViolationRecovery`。
- 保持 C0/C1/C2 stable prefix 不变。
- 为 retry prompt 添加 request log / stable hash 回归。

### Phase 6: Projection / TUI / Web

- workorchd 投影 active violation。
- TUI/Web 展示 retry progress 和 escalated blocker。
- 端侧不实现 recovery 判断。

### Phase 7: E2E and Boundary Validation

- 增加 mock provider contract violation recovery e2e。
- 增加 no side effects e2e。
- 执行 boundary search 和全量测试。

### Phase 8: Skill and Operating Rule Update

- 更新 `/home/rangh/aispace/.agents/skills/workorch-core-template-boundary/SKILL.md`。
- 固化“违例可记录、可反馈、可重试，但不可执行”的审核规则。
- 补充修改审核 checklist：core 不得通过业务字符串判断 recoverability；template 负责业务策略；TUI/Web 不复制状态机。

## Rollout Order

建议按以下顺序收口，避免同时改动过大导致排查困难：

1. `internal/pkg/workorchruntimecontract` 与 runtime policy/config。
2. orchestrator contract violation helper 与预算单测。
3. validator structured result 改造，覆盖 graph/DoD/assessment/output shape。
4. recovery loop 接入 EventHub dispatch 主路径。
5. C4 prompt/context 注入与 cache stable prefix 回归。
6. projection/readmodel/port/client/TUI/Web 展示链路。
7. e2e mock provider 场景和 boundary contract 验证。
8. skill 文档固化。

每一步都必须能独立证明“旧问题命中新路径”。如果某次修改无法让历史失败样本进入 structured violation/retry/escalation，不应归类为本设计收口。

## Acceptance Criteria

1. Recoverable LLM contract violation 不直接进入 decision，而是生成 rejected diagnostic evidence 并自动重试。
2. Retry step 保留原 Goal/DoD、runtime stage、repair objective、required output 和 evidence timeline。
3. 未通过合同的 output 不产生任何 state-changing side effect。
4. 相同违例重复超过预算后升级 decision，并展示完整 violation history。
5. Provider / external dependency failure 不被误归类为 contract violation retry。
6. TUI/Web 只展示服务端 projection，workspace 切换后 prompt、主视图、状态栏一致。
7. LLM request stable prefix 不因 contract violation retry 漂移。
8. Core 通用层无模板、语言、命令、文件名、provider 品牌特判。

## Implementation Verification

2026-06-20 已按本文完成主路径代码收口，并通过以下验证：

- `go test ./internal/pkg/events ./internal/modules/kernel/orchestrator/pkg/events -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1`
- `go test ./internal/modules/blocks/llm/biz ./internal/services/workorch/service/tui ./internal/pkg/workorchruntimecontract ./internal/pkg/workorchruntimepolicy ./internal/pkg/workorchconfig -count=1`
- `WORKORCH_CONTRACT_VIOLATION_VERIFY_BASE_URL=http://127.0.0.1:18080 ./scripts/verify-contract-violation-recovery-e2e.sh`
- `./scripts/verify-boundary-contract.sh`
- `go test ./... -count=1`

本轮最终修正的主路径缺口是 `SubscribeWorkorch` 未解码 `/orchestrator/contract_violation/*` 事件，导致 eventstream 无法持久化 violation recorded / retry_requested / escalated。已新增中立 `OrchestratorContractViolationEvent` wrapper，只做版本校验和 payload 透传，不引入 template、语言、文件、命令或 provider 业务判断。

2026-06-22 追加核对发现 embedded EventHub backend 虽然订阅了
`WorkorchTopics()`，但 `convertEnvelope` 未把
`/orchestrator/contract_violation/*` 转换成端侧事件，导致本地 TUI 在
violation recorded / retry requested / escalated 后无法自动刷新服务端
projection。已按“workorchd 是 projection owner，TUI 只消费 projection”的
边界收口：三类 contract violation 事件统一转换为
`workspace/projection/changed`，端侧只触发重新读取服务端快照，不复制
contract violation 状态机，也不自动发起 retry/replan/restart。

## Review Checklist

代码收口前后必须逐项核对：

- [x] 本次修改是否只实现中立合同/状态机能力。
- [x] 是否没有执行任何 rejected output。
- [x] 是否没有把具体业务策略写入 kernel/orchestrator。
- [x] 是否所有 command/event 都携带 workspace/run/step scope。
- [x] 是否 retry 通过 EventHub dispatch requested 主路径推进。
- [x] 是否 projection 是唯一 UI 状态来源。
- [x] 是否新增 tests 能证明旧失败样本进入 recovery loop。
- [x] 是否 boundary search 无新增场景特判。

2026-06-22 追加验证命令：

```bash
go test ./internal/services/workorch/biz/backends ./internal/pkg/events ./internal/modules/kernel/orchestrator/pkg/events -count=1 -timeout=180s
go test ./internal/services/workorch/service/tui -count=1 -timeout=180s
git diff --check -- internal/services/workorch/biz/backends/eventhub.go internal/services/workorch/biz/backends/eventhub_test.go docs/30-module-architecture/contract-violation-recovery-loop-design.md
git diff -U0 -- internal/services/workorch/biz/backends/eventhub.go internal/services/workorch/biz/backends/eventhub_test.go docs/30-module-architecture/contract-violation-recovery-loop-design.md | rg '^\+' | rg -v '^\+\+\+' | rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|mcp-lsp|lsp|server" || true
```

建议验证命令：

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/services/workorch/service/tui/... -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./... -count=1
./scripts/verify-boundary-contract.sh
```
