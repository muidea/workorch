# WorkOrch Software-Dev Profile Verifier

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件定义 `software-dev` profile 的验证入口。

## Scope

profile verifier 可以验证：

- goal coverage and acceptance dimensions
- evidence quality policy
- runtime entrypoint integrity
- external contract / error path / persistence / concurrency 等 profile-specific 规则

## Verification Entry

当前 profile 入口：

```bash
./scripts/verify-workorch-runtime-profile-software-dev.sh
make verify-runtime-profile-software-dev
```

在更完整的链路上，相关 e2e / acceptance 还会调用：

- `./scripts/verify-software-dev-goal-dod-e2e.sh`
- `./scripts/verify-software-dev-code-agent.sh`
- `./scripts/verify-software-requirement-development-orchestration.sh`

## Main Assertion Areas

software-dev profile verifier 主要检查：

- goal coverage / DoD coverage / acceptance dimensions 的闭环
- evidence quality policy，例如 weak / zero-work / inconclusive evidence 的处理
- runtime entrypoint integrity
- external contract / error path / persistence / streaming / concurrency 等 profile-specific 维度
- deliverable coverage、artifact expectation 和 profile-specific verifier 规则
- Soul / template / capability 约束是否仍停留在 profile 层，而不是回流到 core

## Mapping To Test Layers

### Profile Script Layer

- `scripts/verify-workorch-runtime-profile-software-dev.sh`

用于校验 snapshot / result / evidence / coverage 的 profile-specific 结构与约束。

当前 fixture / 断言方向：

- 支持两种输入：
  - `--snapshot <run-snapshot.json> [run-results.json]`
  - `<workspace-id> <run-id>` + file backend data root
- 若当前 run 不是 `software-dev`，会显式 skip，而不是误报失败
- 对 accepted result 的关键断言包括：
  - `executionContract.profileContracts.software-dev` 必须存在
  - `acceptanceDimensions` / `deliverableExpectations` / `evidenceRequirements` 不能为空
  - `dodAssessment.goalCoverage.satisfied=true`
  - satisfied goalCoverage item 必须带 `evidenceRefs`
  - required `acceptanceDimensions` 必须 satisfied 且带 `evidenceRefs`
  - required `deliverableCoverage` 必须带 `artifactRefs`
  - 要求 direct evidence 的 acceptance dimension，其 `evidenceRefs` 必须可追溯到允许的 runtime step kind，而不能全是只读证据

### E2E Layer

- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-dev-code-agent.sh`
- `verify-software-requirement-development-orchestration.sh`

这些脚本验证 software-dev 模板在真实执行链路中的：

- 新增 Goal / DoD 覆盖
- patch / test / artifact / result 闭环
- no-actionable continuation / assessment-only / repair/replan 收口
- 自动治理 / blocker / evidence quality 等复杂场景

当前 `verify-software-dev-goal-dod-e2e.sh` 已覆盖的稳定场景包括：

1. 自动 repair 并完成交付闭环
2. repair exhausted 后 replan continuation
3. 空 workspace 初始化 + 累积 Goal/DoD + secret redaction + 完整交付
4. weak verification evidence 不得满足 DoD，必须进入 repair/escalation
5. no-actionable continuation 必须进入 validation follow-up 并完成 DoD assessment
6. graph follow-up 超限后必须进入 assessment-only 收口

各场景当前直接产物包括：

- `scenario1-snapshot.json`
- `scenario1-observability.json`
- `scenario2-snapshot.json`
- `scenario2-workspace-snapshot.json`
- `scenario3-snapshot.json`
- `scenario3-observability.json`
- `scenario3-context-bundle.json`
- `scenario3-events.json`
- `scenario4-snapshot.json`
- `scenario5-snapshot.json`
- `scenario6-snapshot.json`
- `scenario6-observability.json`

### Fixture Matrix

| 入口 | 主要输入/fixture | 主要断言 |
| --- | --- | --- |
| `verify-workorch-runtime-profile-software-dev.sh` | run snapshot / results page 或 file backend run records | profile contract 存在、goal coverage 完整、required acceptance dimension / deliverable coverage 完整、direct evidence 可追溯 |
| `verify-software-dev-goal-dod-e2e.sh` | deployed service + mock LLM + 多场景 snapshot/observability/events/request logs | patch/validation/artifact/result 闭环、repair escalation、secret redaction、弱证据拒绝、follow-up / assessment-only 收口 |
| `verify-software-dev-code-agent.sh` | orchestrator/unit tests + CLI/TUI verifier + 可选 deployed LLM e2e | code-agent 架构边界、approval contract、TUI/CLI 集成、治理恢复链 |
| `verify-software-requirement-development-orchestration.sh` | appservice/orchestrator go tests + scenario snapshots from software-dev e2e | `taskPlans` / `resultQuality` / `doDEvidence` / `repairLoop` / `memoryRecallTraces` 等 read model 投影 |

### First-Cut Assertion Matrix

| 规则 | 当前主要证据 |
| --- | --- |
| goal coverage / DoD coverage 闭环 | `verify-workorch-runtime-profile-software-dev.sh` + `scenario1/3/5/6` snapshots |
| patch / diff / artifact / accepted result 闭环 | `scenario1-snapshot.json` |
| repair loop escalation / decision replan continuation | `scenario2-snapshot.json` + `scenario2-workspace-snapshot.json` |
| secret redaction / external dependency scope | `scenario3-snapshot.json` + `scenario3-observability.json` + `scenario3-context-bundle.json` + `scenario3-events.json` + request logs |
| weak evidence rejection | `scenario4-snapshot.json` |
| no-actionable continuation -> validation follow-up | `scenario5-snapshot.json` |
| graph follow-up limit -> assessment-only closure | `scenario6-snapshot.json` + `scenario6-observability.json` |
| approval action/context contract | `verify-software-dev-code-agent.sh` 中 approval/block tests |
| `taskPlans` / `resultQuality` / `doDEvidence` / `repairLoop` / `memoryRecallTraces` read model 投影 | `verify-software-requirement-development-orchestration.sh` + `scenario1` artifacts |

### Script-To-Rule Coverage

| Script | 主要覆盖规则 |
| --- | --- |
| `verify-workorch-runtime-profile-software-dev.sh` | required `acceptanceDimensions` / `deliverableCoverage` / `evidenceRequirements` |
| `verify-workorch-runtime-profile-software-dev.sh` | direct evidence 必须落在允许的 runtime step kind |
| `verify-software-dev-goal-dod-e2e.sh` | patch / validation / artifact / accepted result 闭环 |
| `verify-software-dev-goal-dod-e2e.sh` | repair exhausted -> replan continuation |
| `verify-software-dev-goal-dod-e2e.sh` | secret redaction / external dependency scope |
| `verify-software-dev-goal-dod-e2e.sh` | weak evidence rejection |
| `verify-software-dev-goal-dod-e2e.sh` | no-actionable continuation -> validation follow-up |
| `verify-software-dev-goal-dod-e2e.sh` | graph follow-up limit -> assessment-only closure |
| `verify-software-dev-code-agent.sh` | code-agent 架构边界 / approval action-context / TUI-CLI integration |
| `verify-software-requirement-development-orchestration.sh` | `taskPlans` / `resultQuality` / `doDEvidence` / `repairLoop` / `memoryRecallTraces` 投影 |

### Fixture / Deployed Mapping

| 规则层 | 主要本地/fixture 证据 | 主要 deployed 对应 |
| --- | --- | --- |
| profile contract completeness | `verify-workorch-runtime-profile-software-dev.sh --snapshot` / file backend run records | `verify-software-dev-goal-dod-e2e.sh` 产出的 scenario snapshots |
| code change / validation / artifact closure | scenario1 snapshot fixture | deployed software-dev e2e scenario1 |
| repair exhaustion / replan continuation | scenario2 snapshot + workspace snapshot | deployed software-dev e2e scenario2 |
| secret redaction / external dependency scope | scenario3 snapshot + observability + context bundle + events + request logs | deployed software-dev e2e scenario3 |
| weak evidence rejection | scenario4 snapshot | deployed software-dev e2e scenario4 |
| no-actionable continuation / assessment-only closure | scenario5 / scenario6 snapshots | deployed software-dev e2e scenario5 / scenario6 |
| code-agent architecture / approval contract | orchestrator, approval, TUI/CLI local tests | optional `WORKORCH_SOFTWARE_DEV_VERIFY_LLM_E2E=1` path in `verify-software-dev-code-agent.sh` |
| read model projection (`taskPlans` / `resultQuality` / `doDEvidence` / `repairLoop`) | orchestration script + scenario artifacts | deployed orchestration verification path |

补细说明：

- scenario1
  - 主要对应 patch diff、accepted result、artifact summary、repairLoop.attemptCount
- scenario2
  - 主要对应 repair exhaustion、resolved replan decision、continuation run 聚合
- scenario3
  - 主要对应 empty workspace initialization、cumulative Goal/DoD、secret redaction、request-log prompt scope
- scenario4
  - 主要对应 weak evidence rejection 与 escalated decision
- scenario5
  - 主要对应 no-actionable continuation -> validation follow-up
- scenario6
  - 主要对应 graph follow-up limit -> assessment-only closure trace
- `verify-software-dev-code-agent.sh`
  - deployed 可选路径只补治理/LLM 主链，不替代 scenario1~6 的 software-dev e2e

### Unit-Test-To-Rule Inventory

| 测试或测试组 | 主要覆盖规则 |
| --- | --- |
| `runtime_verify_script_test.go` 中 software-dev fixtures | profile script 对 accepted result / direct evidence / deliverable coverage 的门禁 |
| `TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff` | software-dev repair -> diff -> accepted result 闭环 |
| `TestRuntimeOrchestrator_ResultArtifactsIncludePatchDiff` / `ResultManifestListsOnlyPatchTargets` | patch diff / result artifact 闭环 |
| `TestRuntimeOrchestrator_RecordsValidationCommandMemoriesFromDoDAssessment` | validation command memory / DoD evidence 关联 |
| `TestReadRunTaskDecompositionsWithBackendReturnsExecutionGraphProjection` | `taskDecompositions` read model 投影 |
| `TestReadRunSnapshotWithBackendReturnsExecutionEvidence` | snapshot evidence 聚合 |
| `TestMapResultQualityFlagsWeakResult` | weak evidence quality 投影 |
| `TestMapTaskProgressProjectsRepairLoopState` / `Escalation` | repairLoop read model 投影 |
| approval action/context tests in `verify-software-dev-code-agent.sh` | approval contract 不丢失 action/context |

### Representative Rule To Test Mapping

| 规则 | 代表性测试或场景 |
| --- | --- |
| direct evidence 不能落在只读 step kind | `runtime_verify_script_test.go` 中 `software dev read only direct evidence fails` |
| required deliverableCoverage 不能为空 | `runtime_verify_script_test.go` 中 `software dev missing deliverable coverage fails` |
| 非 software-dev run 应显式 skip | `runtime_verify_script_test.go` 中 `generic profile skips` |
| snapshot fixture 形式也应通过 profile verifier | `TestVerifyWorkorchRuntimeSoftwareDevProfileScriptSnapshotFixture` |
| raw-cased DoD assessment 也应被接受 | `TestVerifyWorkorchRuntimeSoftwareDevProfileScriptSnapshotFixtureAcceptsRawResultCasing` |
| software-dev repair -> diff -> accepted result | `TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff` |
| result artifact 只列 patch 目标 | `TestRuntimeOrchestrator_ResultManifestListsOnlyPatchTargets` |
| accepted result artifact 包含 patch diff | `TestRuntimeOrchestrator_ResultArtifactsIncludePatchDiff` |
| validation command memories 进入 DoD evidence 链 | `TestRuntimeOrchestrator_RecordsValidationCommandMemoriesFromDoDAssessment` |
| weak result quality 正确投影 | `TestMapResultQualityFlagsWeakResult` |
| repairLoop 正确投影 | `TestMapTaskProgressProjectsRepairLoopState` / `TestMapTaskProgressProjectsRepairLoopEscalation` |
| profile 脚本在标准 snapshot fixture 上通过 | `runtime_verify_script_test.go` 中 `software dev accepted coverage passes` |
| `taskDecompositions` 正确投影 | `TestReadRunTaskDecompositionsWithBackendReturnsExecutionGraphProjection` |
| `snapshot` 正确聚合 execution evidence | `TestReadRunSnapshotWithBackendReturnsExecutionEvidence` |
| secret redaction / external dependency scope 正确收口 | deployed `scenario3` in `verify-software-dev-goal-dod-e2e.sh` |
| weak evidence escalation 写出 rejected/decision 收口 | deployed `scenario4` in `verify-software-dev-goal-dod-e2e.sh` |
| graph follow-up limit -> assessment-only closure trace | deployed `scenario6` in `verify-software-dev-goal-dod-e2e.sh` |
| repair exhaustion -> continuation run 聚合 | deployed `scenario2` in `verify-software-dev-goal-dod-e2e.sh` |
| no-actionable continuation 最终进入 accepted closure | deployed `scenario5` in `verify-software-dev-goal-dod-e2e.sh` |

### Rule Inventory Notes

当前 software-dev verifier 规则族可归纳为：

- coverage gate
  - goal coverage / DoD coverage / acceptanceDimensions / deliverableCoverage
- evidence quality
  - weak / zero-work / inconclusive / direct evidence step-kind gate
- delivery closure
  - patch / validation / artifact / accepted result
- recovery closure
  - repair escalation / replan continuation / no-actionable continuation / assessment-only
- security and scope
  - secret redaction / external dependency scope
- read model projection
  - `taskPlans` / `resultQuality` / `doDEvidence` / `repairLoop` / `memoryRecallTraces`

当前文档已为这些规则族建立了：

- fixture matrix
- script-to-rule coverage
- fixture/deployed mapping
- unit-test-to-rule inventory

当前收口判断：

- representative rule/scenario/test mapping 已足以支撑 active 设计闭环
- 若继续补写，也只应是“穷举断言索引”层面的便利增强
- 不再把“是否列出所有断言点”视为信息模型未闭环问题

### Rule Family To Execution Path

| 规则族 | 本地主要入口 | deployed 主要入口 |
| --- | --- | --- |
| coverage gate / direct evidence gate | `verify-workorch-runtime-profile-software-dev.sh` | `verify-software-dev-goal-dod-e2e.sh` |
| delivery closure / patch+artifact closure | orchestrator tests + profile script | `verify-software-dev-goal-dod-e2e.sh` |
| recovery closure | orchestration tests + profile script | `verify-software-dev-goal-dod-e2e.sh` scenario2/4/5/6 |
| security and scope | request-log / snapshot fixture checks | `verify-software-dev-goal-dod-e2e.sh` scenario3 |
| code-agent architecture / TUI-CLI integration | `verify-software-dev-code-agent.sh` | optional `WORKORCH_SOFTWARE_DEV_VERIFY_LLM_E2E=1` |
| read model projection | `verify-software-requirement-development-orchestration.sh` | same script with deployed scenario artifacts |

### Boundary Layer

profile verifier 可以使用 `verify-boundary-contract.sh` 的结果作为辅助守卫，
但不能把它当作替代 profile-specific 验收的证明。

## Mapping To Product / Read Model

software-dev verifier 当前主要对以下读模型施加约束：

- `run snapshot`
  - `results`
  - `artifacts`
  - `patches`
  - `taskPlans`
  - `resultQuality`
  - `doDEvidence`
  - `taskProgress.repairLoop`
  - `memoryRecallTraces`
- `run observability`
  - `closureTraces`
  - `taskPlans`
  - `resultQuality`
  - `doDEvidence`
- `workspace snapshot`
  - continuation run / accepted result 聚合
- `context bundle`
  - secret redaction / cumulative goal-dod context

因此 profile 规则若无法投影到这些产品读模型之一，也不应被视为稳定已收口规则。

## Boundary

- profile verifier 可以验证领域特化规则
- profile verifier 不应反向改写 core runtime contract
- profile verifier 不应把模板具体实现方案写成通用框架规则

## Cross Reference

- core 中立断言见 `core-runtime-verifier.md`
- software-dev profile 规则来源见 `../40-profiles-capabilities/software-dev/goal-coverage-and-dod.md`
- code-agent 特化见 `../40-profiles-capabilities/software-dev/code-agent.md`

## Source Documents

- `../40-profiles-capabilities/software-dev/goal-coverage-and-dod.md`
- `../90-archive/completed/software-requirement-development-orchestration-plan.md`（历史过程稿）
