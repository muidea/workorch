# WorkOrch Core Runtime Verifier

Status: active

Type: verification

Last Updated: 2026-07-27

## Purpose

本文件定义 core runtime 合同的验证入口。

## Scope

core verifier 应只验证：

- Runtime V2 四状态机合法/禁止迁移与协调协议
- stage contract
- context gate
- evidence/result boundary
- repair/replan/governance neutrality
- request audit / protected fields / source refs
- incremental artifact mode 的中立 contract

## Verification Entry

当前核心入口：

```bash
./scripts/verify-workorch-runtime-core-flow.sh
make verify-boundary-contract
```

其中：

- `verify-workorch-runtime-core-flow.sh` 用于核对 runtime flow / contract / closure 相关的中立断言
- `verify-boundary-contract.sh` 用于阻止 generic runtime / prompt / Web 代码重新引入领域化偏置

## Main Assertion Areas

core verifier 需要覆盖：

- SM-1 Run lifecycle：Run owner-local pending/active/blocked/terminal、terminal immutable、lineage；Workspace release 由 Workspace owner-local delivery 完成
- SM-2 authority/focus：prerequisite、feasibility、focus/evidence/assessment/repair/governance/completion
- SM-3 candidate/dispatch：attempt 0/1、stable transaction/prefix、seal-before-execute、stale/duplicate evidence
- SM-4 drive/recovery/projection：StateRevision、transition guard、singleflight、restart idempotency、projection one-way delivery
- `runtimeStage` / `runtimeStageMode` / stage contract 的一致性
- context gate fail-fast / read_required / blocker_required / replan_required 的中立分流
- `stateChangingOutputRequired`、assessment-only、follow-up graph、repair/replan continuation 的合同边界
- Goal / DoD / coverage / evidenceRefs / accepted vs rejected result 的核心闭环
- request audit、protected fields、redaction summary、source refs、cache-aware 关键字段
- repair / provider retry / duplicate failure / runtime context recovery 等恢复链的中立行为
- `incrementalArtifact` 的 selector / continuation / apply / validation / projection 边界
- 领域中立守卫：core 不解析业务词、语言词、测试框架词来决定治理动作
- canonical failure hydration、coverage target auto-rebuild、context-gate recovery routing

### Four-State-Machine Mandatory Matrix

| 状态机 | 必须验证的合法迁移 | 必须拒绝的迁移 |
| --- | --- | --- |
| SM-1 | Run owner internal pending→active、active↔blocked、active→completed/failed、新 Run lineage | terminal→active、无 resolution blocked→active、非 Run owner 直接终态 |
| SM-2 | prerequisite→reduce、missing evidence→state/validation、partial assessment→remaining、repairable gap→same-focus repair、all gates→sealed terminal | prerequisite→candidate、false assessment 无条件 terminal、跨 focus 弱证据关闭、capability gap 消耗 candidate attempt、直接写 Run/Workspace/Result |
| SM-3 | attempt0→attempt1、accepted→seal→execute→evidence→reduce | attempt2、transaction/prefix 漂移、未 seal 执行、executor 选择 next action、stale/duplicate 推进 authority |
| SM-4 | Run state fact→singleflight→load/initialize→bounded drive、restart load、delivery acknowledgement retry | 重复 Initialize、重复副作用、无 revision 空转、projection/watchdog/application 反向驱动 reducer、直接 owner state write |

任何涉及四状态机职责的代码变更，都必须在测试说明中标明受影响 SM 编号，并提供：

- transition table unit test；
- invariant/forbidden transition test；
- restart/idempotency test；
- stale/duplicate/CAS test；
- budget/transition guard test；
- 至少一个跨状态机 contract test。

缺少上述映射的 runtime 变更不得通过 core verifier 收口。

## Mapping To Test Layers

### Shell/Script Layer

- `scripts/verify-workorch-runtime-core-flow.sh`
- `scripts/verify-boundary-contract.sh`

用于快速核查 contract regression、boundary regression 和关键 fixture。

当前 script 层断言方向：

- `verify-workorch-runtime-core-flow.sh`
  - 读取 file backend 中的 `workspace.json` / `run.json` / `step.json` /
    `result.json` / `ticket.json`
  - 校验：
    - graph step id / provider retry lineage / recovery fields
    - `stateChangingOutputRequired` 与 `patch/apply` graph 的一致性
    - planning 阶段 state-changing graph 的边界
    - accepted / rejected result 与 DoD / resultPolicy 的闭环
    - blocked / superseded / failed 运行的 rejected result 收口
    - workspace `activeRunId` 与 terminal run 的一致性
  - 输出：
    - `acceptedResultCount`
    - `rejectedResultCount`
    - warnings for handled violations

- `verify-boundary-contract.sh`
  - 扫描 reusable runtime / prompt / Web 代码
  - 阻止 software-dev、LSP、测试框架、具体业务 deliverable 等 profile 语义回流到 core

### First-Cut Core Fixture Matrix

| 入口 | 主要输入/fixture | 主要断言 |
| --- | --- | --- |
| `verify-workorch-runtime-core-flow.sh` | file backend workspace/run/step/result/ticket records | stage contract、recovery lineage、state-changing gate、accepted/rejected result 闭环、workspace activeRun consistency |
| `verify-incremental-artifact-e2e.sh` | deployed incremental-artifact fixture / snapshot / observability / raw content open | selector、continuation lifecycle、apply/validation routing、metadata-only projection、raw content leakage guard |
| `verify-boundary-contract.sh` | runtime/core/prompt/Web code tree | reusable runtime 不得包含 software-dev / LSP / deliverable / test-command 偏置 |

### Unit / Go Test Layer

核心覆盖主要来自：

- `internal/modules/kernel/orchestrator/...`
- `internal/modules/blocks/llm/...`
- `internal/pkg/events`
- `internal/pkg/repository/...`

这些测试验证 contract 细节、恢复链、result gate、event contract 和 repository 行为。

当前典型 fixture 方向包括：

- orchestrator runtime verify script tests
  - `runtime_verify_script_test.go`
  - 校验 core/profile 脚本在代表性 fixture 上的通过/失败条件
- appservice / read model tests
  - 校验 result gate、repair loop、event / snapshot / observability 投影
- blocks/llm tests
  - 校验 decision parsing、protected fields、approval action/context、schema normalization
- repository / events tests
  - 校验 append-only、event decoding、approval / decision repository contract

### First-Cut Assertion Matrix

| 规则 | 当前主要证据 |
| --- | --- |
| `runtimeStage` / `runtimeStageMode` 一致性 | `verify-workorch-runtime-core-flow.sh` + orchestrator tests |
| context gate fail-fast / read_required / blocker_required / replan_required | core flow script + runtime contract conformance audit |
| `llm_context_integrity_failed` 不得进入 provider retry / state-changing repair | core flow script + orchestrator tests |
| canonical failure hydration 不得把可行动失败误判成“当前失败缺失” | orchestrator tests + core flow fixture |
| coverage gap 存在时 `coverageTargetSet` 可自动补齐 | orchestrator tests + core flow fixture |
| `stateChangingOutputRequired` -> `patch/apply` gate | core flow script |
| assessment-only / follow-up / repair / replan continuation 边界 | core flow script + full-chain assessment audit |
| accepted vs rejected result 闭环 | core flow script |
| provider retry / runtime repair / duplicate failure lineage | core flow script + orchestrator tests |
| incremental artifact mode 只承载 `need_state_change`，不得绕过 apply/validation | orchestrator tests + incremental artifact e2e |
| continuation metadata 可投影但正文不泄漏到默认 snapshot/observability | incremental artifact e2e + appservice/read-model tests |
| reusable runtime 不得含 software-dev / LSP 偏置 | `verify-boundary-contract.sh` |

### Script-To-Rule Coverage

| Script | 主要覆盖规则 |
| --- | --- |
| `verify-workorch-runtime-core-flow.sh` | `runtimeStage` / `runtimeStageMode` consistency |
| `verify-workorch-runtime-core-flow.sh` | context gate 与 recovery lineage fields 完整性 |
| `verify-workorch-runtime-core-flow.sh` | `llm_context_integrity_failed` recovery routing 不得进入普通 repair |
| `verify-workorch-runtime-core-flow.sh` | protected fields / target snapshot / process evidence gate |
| `verify-workorch-runtime-core-flow.sh` | `stateChangingOutputRequired` -> `patch/apply` gate |
| `verify-workorch-runtime-core-flow.sh` | planning 阶段 state-changing graph 边界 |
| `verify-workorch-runtime-core-flow.sh` | accepted / rejected result 与 `resultPolicy` 闭环 |
| `verify-workorch-runtime-core-flow.sh` | blocked / superseded / failed run 的 rejected-result 收口 |
| `verify-workorch-runtime-core-flow.sh` | workspace `activeRunId` 不得指向 terminal run |
| orchestrator / repository / read-model tests | incremental artifact selector、continuation lifecycle、restart recovery 与 metadata-only projection |
| `verify-incremental-artifact-e2e.sh` | active continuation 默认只暴露 metadata，不暴露正文 |
| `verify-incremental-artifact-e2e.sh` | continuation 完成后必须收口到 apply/validation evidence 与 accepted/rejected result |
| `verify-boundary-contract.sh` | reusable runtime / prompt / Web 不得含 software-dev 语义 |
| `verify-boundary-contract.sh` | LSP / test command / deliverable id 不得下沉到 core |

### Fixture / Deployed Mapping

| 规则层 | 主要本地 fixture | 主要 deployed 对应 |
| --- | --- | --- |
| core flow contract | file backend `workspace.json` / `run.json` / `step.json` / `result.json` / `ticket.json` | `verify-llm-workspace-execution.sh` 的 run/workspace snapshot + observability |
| boundary contract | source tree scan | deployed Web/console/runtime code path 不应出现 profile 偏置 |
| incremental artifact neutrality | orchestrator / script fixture | `verify-incremental-artifact-e2e.sh` |
| MCP stdio neutral execution | block/orchestrator fixture | `verify-mcp-stdio-workspace-execution.sh` |
| startup + minimal runtime projection | appservice/repository fixture | `verify-workorch-container.sh` / `service-acceptance.md` |

补细说明：

- `verify-llm-workspace-execution.sh`
  - deployed 侧主要观察 `runs/:id/snapshot`、`runs/:id/observability`、`workspaces/:id/snapshot`
- `verify-incremental-artifact-e2e.sh`
  - deployed 侧主要观察 `artifactContinuations`、accepted result、event traces、raw content 不泄漏
- `verify-mcp-stdio-workspace-execution.sh`
  - deployed 侧主要观察 `selectedCapabilities`、`usedCapabilities`、tool/validation/checkpoint evidence
- `verify-workorch-container.sh`
  - deployed/container 侧主要观察 startup、snapshot、status、decision/approval/resume 最小链

### Unit-Test-To-Rule Inventory

| 测试或测试组 | 主要覆盖规则 |
| --- | --- |
| `runtime_verify_script_test.go` | core/profile verify scripts 在代表性 fixture 上的通过/失败条件 |
| incremental artifact orchestrator / repository / appservice tests | output mode selector、continuation lifecycle、restart recovery、projection redaction |
| `TestRuntimeOrchestrator_DoesNotRepairLLMProviderError` / retry timeout tests | provider retry 与 config block 边界 |
| `TestRuntimeOrchestrator_RuntimeRepairAttemptCountIsScopedByFailureSignature` | runtime repair scope / failure signature 预算隔离 |
| `TestRuntimeOrchestrator_DispatchesValidationFollowUpForContinueWithoutActionableContinuation` | no-actionable continuation -> validation follow-up |
| `TestRuntimeOrchestrator_GraphFollowUpExpansionLimitRoutesToAssessmentOnly` | graph follow-up expansion limit -> assessment-only |
| `TestRuntimeOrchestrator_AssessmentOnlyContractViolationDispatchesOneCorrection` / repeated graph tests | assessment-only contract correction 边界 |
| `TestRuntimeOrchestrator_RecordsRejectedResultForUnsatisfiedDoDAssessment` / weak satisfied tests | rejected result / weak evidence 闭环 |
| `TestNormalizeDecisionOutputPreservesRequiredApprovalActionContext` | approval action/context 保真 |
| approval / decision repository and event decode tests | governance repository / event contract |

### Representative Rule To Test Mapping

| 规则 | 代表性测试 |
| --- | --- |
| provider error 不进入 runtime repair | `TestRuntimeOrchestrator_DoesNotRepairLLMProviderError` |
| provider timeout 先重试再失败/阻塞 | `TestRuntimeOrchestrator_RetriesLLMProviderTimeoutBeforeFailingRun` / `TestRuntimeOrchestrator_BlocksOnDecisionAfterProviderTimeoutRetriesExhausted` |
| repair 预算按 failure signature 隔离 | `TestRuntimeOrchestrator_RuntimeRepairAttemptCountIsScopedByFailureSignature` |
| continue without actionable continuation 进入 validation follow-up | `TestRuntimeOrchestrator_DispatchesValidationFollowUpForContinueWithoutActionableContinuation` |
| graph follow-up 超限进入 assessment-only | `TestRuntimeOrchestrator_GraphFollowUpExpansionLimitRoutesToAssessmentOnly` |
| assessment-only 违规只派发一次修正 | `TestRuntimeOrchestrator_AssessmentOnlyContractViolationDispatchesOneCorrection` |
| unsatisfied/weak satisfied DoD 生成 rejected result | `TestRuntimeOrchestrator_RecordsRejectedResultForUnsatisfiedDoDAssessment` / `TestRuntimeOrchestrator_RecordsRejectedResultForWeakSatisfiedDoDAssessment` |
| invalid patch graph 被脚本拦截 | `runtime_verify_script_test.go` 中 `unhandled invalid patch graph fails` |
| invalid patch graph 可被 runtime repair 吸收并只保留 warning | `runtime_verify_script_test.go` 中 `handled invalid patch graph passes with warning` |
| graph node step id 必须 graph-scoped | `runtime_verify_script_test.go` 中 `legacy block graph node id fails` |
| graph node 缺少 `graphSeq` 必须失败 | `runtime_verify_script_test.go` 中 `missing graph sequence fails` |
| workspace activeRun 不得指向 terminal run | `runtime_verify_script_test.go` 中 `workspace active run points to terminal run fails` |
| provider retry step 必须保留 recovery context | `runtime_verify_script_test.go` 中 `provider retry graph context is recovery step` |
| repair exhaustion + rejected policy 无 result 失败 | `runtime_verify_script_test.go` 中 `superseded repair exhausted rejected policy without result fails` |
| assessment-only 存在正式 assessment 时应忽略 graph | `TestRuntimeOrchestrator_AssessmentOnlyIgnoresGraphWhenAssessmentIsPresent` |
| execution graph 完成后必须进入 follow-up | `TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterExecutionGraphCompletes` / `TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterLLMPatchSideEffect` |
| incremental artifact chunk/apply 后必须进入 validation / acceptance | incremental artifact orchestrator tests + `verify-incremental-artifact-e2e.sh` |
| active continuation 默认只暴露 metadata，不暴露正文 | appservice/read-model tests + `verify-incremental-artifact-e2e.sh` |
| graph follow-up 在获得具体证据后恢复 repair contract | `TestRuntimeOrchestrator_GraphFollowUpRestoresRepairContractAfterConcreteEvidenceAcquisition` |
| provider timeout 触发后可升级为决策阻塞 | `TestRuntimeOrchestrator_BlocksOnDecisionAfterProviderTimeoutRetriesExhausted` |
| follow-up evidence 必须复用已完成并行证据 | `TestRuntimeOrchestrator_GraphFollowUpIncludesAllCompletedParallelEvidence` |
| 决策解决后必须从下一步继续 | `TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeDecisionResolved` |
| 审批通过后必须从下一步继续 | `TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeApprovalApproved` |

### Rule Inventory Notes

当前 core verifier 规则族可以继续压缩为以下 inventory：

- stage contract
  - `runtimeStage` / `runtimeStageMode` / stage boundary
- context gate
  - `read_required` / `blocker_required` / `replan_required` / fail-fast
  - canonical failure hydration / protected field preservation / semantic admission
- state change gate
  - `stateChangingOutputRequired` / patch/apply / planning restrictions
  - incremental artifact selector / continuation / metadata-only projection
- closure gate
  - accepted vs rejected result / `resultPolicy` / DoD evidence
- recovery lineage
  - provider retry / runtime repair / duplicate failure / recovery scope
- boundary neutrality
  - reusable runtime 不得含 profile-specific 语义

当前文档已为这些规则族建立了：

- script-to-rule coverage
- fixture/deployed mapping
- unit-test-to-rule inventory

当前收口判断：

- representative rule-to-test mapping 已足以支撑 active 设计闭环
- 若继续补写，也只应是“穷举索引”层面的便利增强
- 不再把“是否列出所有测试名”视为信息模型未闭环问题

### Rule Family To Execution Path

| 规则族 | 本地主要入口 | deployed 主要入口 |
| --- | --- | --- |
| stage contract / context gate / closure gate | `verify-workorch-runtime-core-flow.sh` | `verify-llm-workspace-execution.sh` |
| state change gate | orchestrator tests + core flow script | `verify-llm-workspace-execution.sh` |
| incremental artifact neutrality | orchestrator / repository / read-model tests | `verify-incremental-artifact-e2e.sh` |
| MCP stdio neutral execution | blocks/orchestrator tests | `verify-mcp-stdio-workspace-execution.sh` |
| boundary neutrality | `verify-boundary-contract.sh` | console/runtime built assets + deployed code path review |

### Deployed Verification Layer

当脚本或 e2e 需要对已部署实例复核时，core verifier 只检查：

- runtime flow 是否符合中立 contract
- provider-facing request / result / blocker 路径是否仍满足核心边界

它不负责验证 software-dev 的具体测试矩阵或业务目标覆盖。

在 deployed 场景中，core verifier 当前主要依赖：

- `service-acceptance.md` 中的通用 runtime 读模型端点
- `verify-llm-workspace-execution.sh`
- `verify-incremental-artifact-e2e.sh`
- `verify-mcp-stdio-workspace-execution.sh`

这些入口只用于证明：

- deployed runtime 仍遵守中立 contract
- runtime snapshot / observability / event / artifact 不因具体 profile 而破坏 core 规则

## Mapping To Product / Read Model

core verifier 当前主要约束以下读模型面：

- `run snapshot`
  - result / evidence / closure / selected capabilities
- `run observability`
  - timeline / closure traces / llm usage / evidence
- `workspace snapshot`
  - activeRun / accepted closure / run aggregation
- `events`
  - 事件 contract 与 replay 输入面

若某条“核心规则”无法映射到这些运行态投影之一，也应怀疑它是否已经滑回 profile-specific 规则。

## Boundary

core verifier 不应验证：

- software-dev 特定测试命令
- 具体项目结构
- 特定 workspaceTemplate 业务语义

## Cross Reference

- software-dev 特化断言见 `profile-software-dev-verifier.md`
- 服务启动与 HTTP/API 最小验收见 `service-acceptance.md`

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `audits/runtime-contract-conformance-audit.md`
